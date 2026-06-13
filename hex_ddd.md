# Hexagonal Architecture Plan

Reviewed by /plan-eng-review on 2026-06-13. Decisions below reflect the agreed scope.

## Shared /pkg/

| Package    | Purpose                                                           |
| ---------- | ----------------------------------------------------------------- |
| `config`   | Environment variable loading (`envOr` extracted from 5 main.go)   |
| `logger`   | Structured logging init (`slog` setup extracted from 5 main.go)   |
| `database` | Postgres connection pool (already exists)                         |
| `grpcutil` | gRPC metadata helpers (exists) + server lifecycle helper (new)    |

## Internal Architecture Pattern

### gRPC services (url-service, key-gen-service, redirect-service, user-service)

```
cmd/main.go                 → Wiring, startup, graceful shutdown
internal/
  domain/                   → Entities, value objects, domain errors (zero deps)
  port/                     → Interfaces only (repository, cache)
  service/                  → Business logic (accepts/returns domain types only)
  server/                   → gRPC transport layer (maps proto ↔ domain)
  adapter/
    postgres/               → Repository implementations
    redis/                  → Cache implementations (redirect-service only)
```

### HTTP service (api-gateway)

```
cmd/main.go                 → Wiring, startup, graceful shutdown
internal/
  handler/                  → HTTP transport (routes, middleware, gRPC error mapping)
```

api-gateway is a reverse proxy with no domain logic. It does not need hex layers.

Dependencies flow inward: adapters implement ports, services depend on ports, domain depends on nothing.

## What lives where

| Type               | Layer              | Notes                                                       |
| ------------------ | ------------------ | ----------------------------------------------------------- |
| Entities           | `domain/`          | Structs with identity (e.g. `URL`, `User`, `RefreshToken`)  |
| Value objects      | `domain/`          | Immutable, identity-less types (e.g. `Slug`, `Email`)       |
| Domain errors      | `domain/`          | Per-service typed errors (e.g. `ErrSlugNotFound`)           |
| Port interfaces    | `port/`            | Abstract contracts, no proto or DB types                    |
| Proto mapping      | `server/`          | The only layer that knows about proto types                 |
| DB models          | `adapter/postgres/`| The only layer that knows about SQL/pgx types               |

Rule: if a type references a proto, HTTP struct, or JSON tag, it does not belong in `domain/`.

## Domain Errors (per-service, not shared)

Each service defines its own errors in `internal/domain/`. Semantically different even when named similarly.

| Service          | Domain errors                                           |
| ---------------- | ------------------------------------------------------- |
| url-service      | `ErrNotFound`, `ErrNotOwner`                            |
| key-gen-service  | `ErrPoolExhausted`                                      |
| redirect-service | `ErrNotFound`                                           |
| user-service     | `ErrEmailExists`, `ErrUserNotFound`, `ErrTokenInvalid`  |

## Protobuf & Code Generation

Proto definitions stay at `proto/{keygen,url,redirect,user}/` (current structure). Versioned paths + buf migration deferred to a separate PR.

## Service Responsibilities

| Service            | Responsibility                                                                                  |
| ------------------ | ----------------------------------------------------------------------------------------------- |
| `url-service`      | Create and store shortened URLs, owns URL domain logic                                          |
| `key-gen-service`  | Generate and vend unique short keys, maintain key pool                                          |
| `redirect-service` | Resolve short codes to original URLs via Postgres with Redis caching (CQRS-lite read path)      |
| `user-service`     | User registration, authentication, JWT token management                                        |
| `api-gateway`      | Edge entry point, routes HTTP requests to downstream gRPC services, JWT middleware              |

## DRY Extractions (zero marginal cost during refactor)

### pkg/config/env.go
`envOr(key, fallback string) string` extracted from 5 identical copies in cmd/main.go files.

### pkg/logger/logger.go
`Init(serviceName string)` sets up slog with JSON handler, stdout, and service name attribute. Extracted from 5 identical copies. When OTel trace correlation lands later, only this file changes.

### pkg/grpcutil/server.go
gRPC server lifecycle helper consolidating: server creation, health check registration, listener setup, graceful shutdown on context cancellation. Extracted from 4 identical patterns in gRPC service main.go files. When OTel interceptors land, they get added here once.

## redirect-service Service Layer

redirect-service gets a new `internal/service/redirect.go` as a thin passthrough during this refactor. This aligns its structure with the other 3 gRPC services and makes the L1 cache + singleflight TODO (P1) purely additive later.

## Dependency Graph

```
server/ → service/ → port/ ← adapter/
                       ↕
                    domain/
```

## What Does NOT Change

- `cmd/main.go` path (single binary per service, no extra nesting)
- Proto file locations (deferred to separate PR)
- api-gateway internal structure (it's a proxy, not a domain service)
- Proto-generated gRPC client injection into url-service (no keyclient/ adapter wrapper needed)
