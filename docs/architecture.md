# Architecture Notes

Reviewed by `/plan-eng-review` on 2026-06-13. This document captures the shipped hexagonal refactor and the constraints that remain in force.

## Shared `pkg/`

| Package | Purpose |
|---------|---------|
| `config` | Environment variable loading via `EnvOr`, extracted from duplicated `main.go` helpers |
| `logger` | Structured `slog` initialization with JSON output and service name tagging |
| `database` | PostgreSQL connection pool setup |
| `grpcutil` | gRPC metadata helpers plus shared server lifecycle wiring |

## Internal Architecture Pattern

### gRPC services

Applied to `url-service`, `key-gen-service`, `redirect-service`, and `user-service`.

```text
cmd/main.go                 -> Wiring, startup, graceful shutdown
internal/
  domain/                   -> Entities, value objects, domain errors (zero deps)
  port/                     -> Interfaces only (repository, cache)
  service/                  -> Business logic (accepts/returns domain types only)
  server/                   -> gRPC transport layer (maps proto <-> domain)
  adapter/
    postgres/               -> Repository implementations
    redis/                  -> Cache implementations (redirect-service only)
```

### HTTP service

`api-gateway` remains intentionally flat:

```text
cmd/main.go                 -> Wiring, startup, graceful shutdown
internal/
  handler/                  -> HTTP transport, middleware, gRPC error mapping
```

It is an edge proxy, not a domain service, so it does not carry the hexagonal layering used by the gRPC services.

## Dependency Direction

Dependencies flow inward. Adapters implement ports. Services depend on ports. Domain depends on nothing.

```text
server/ -> service/ -> port/ <- adapter/
                       ^
                       |
                    domain/
```

## Layer Rules

| Layer | Contains | Rule |
|-------|----------|------|
| `domain/` | Entities, value objects, typed errors | No imports from proto, SQL, HTTP, or JSON transport concerns |
| `port/` | Interface contracts | Abstract only, no implementation details |
| `service/` | Business logic | Accepts and returns domain types only |
| `server/` | gRPC handlers | Only layer that knows about proto types |
| `adapter/` | Postgres and Redis implementations | Only layer that knows about DB/cache types |

If a type references a proto message, HTTP request/response type, or JSON tag, it does not belong in `domain/`.

## Domain Error Ownership

Each service owns its own domain errors inside `internal/domain/`.

| Service | Domain errors |
|---------|---------------|
| `url-service` | `ErrNotFound`, `ErrNotOwner` |
| `key-gen-service` | `ErrPoolExhausted` |
| `redirect-service` | `ErrNotFound` |
| `user-service` | `ErrEmailExists`, `ErrUserNotFound`, `ErrTokenInvalid` |

## Protobuf and Code Generation

Proto definitions remain at `proto/{keygen,url,redirect,user}/`. Versioned proto paths and buf-based generation were intentionally deferred and are tracked in [TODOS.md](/Users/daryl/Desktop/code/gotiny/TODOS.md).

## Service Responsibilities

| Service | Responsibility |
|---------|----------------|
| `url-service` | Create and store shortened URLs and own URL domain logic |
| `key-gen-service` | Generate and vend unique short keys and maintain the key pool |
| `redirect-service` | Resolve short codes through Postgres with Redis cache-aside behavior |
| `user-service` | Handle registration, authentication, and JWT token lifecycle |
| `api-gateway` | Accept HTTP traffic, apply JWT middleware, and route to downstream gRPC services |

## Shared Extractions

### `pkg/config/env.go`

`EnvOr(key, fallback string) string` replaces duplicated environment lookup helpers from the service binaries.

### `pkg/logger/logger.go`

`Init(serviceName string)` centralizes JSON logger setup and service tagging.

### `pkg/grpcutil/server.go`

`RunServer` centralizes listener setup, health registration, graceful shutdown, and force-stop handling for the gRPC services.

## Redirect Service Follow-On

The redirect service now has `internal/service/redirect.go` as a service-layer boundary. That keeps the shipped refactor consistent across services and makes the planned L1 cache plus singleflight work additive instead of structural.

## Deliberate Non-Changes

- `cmd/main.go` stays the binary entrypoint pattern for every service
- Proto file locations remain unchanged for now
- `api-gateway` stays flat under `internal/handler/`
- `url-service` still uses the generated gRPC key-gen client directly rather than introducing another adapter layer
