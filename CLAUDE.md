# gotiny

A URL shortener built as a gRPC microservices system in Go. The microservices split is
deliberate — it exists to demonstrate systems-design patterns (gRPC inter-service calls,
distributed key coordination, cache-aside, JWT auth across a trust boundary), not because
the domain needs it.

See `AGENTS.md` for the layering rules and change guidance — that file is authoritative for
how to edit code. This file is the orientation map: what exists, how it fits together, and
where things live.

## Tech stack & versions

- **Go 1.26.2** (`log/slog` for structured JSON logging)
- **gRPC** `google.golang.org/grpc v1.81.1` + Protocol Buffers (`protobuf v1.36.11`)
- **PostgreSQL 17** (`postgres:17-alpine`) — single source of truth, accessed via `pgx/v5 v5.10.0`
- **Redis 7** (`redis:7-alpine`) — redirect cache only
- **Docker Compose** for local orchestration
- Code generation via raw `protoc` (not buf); JWT + `bcrypt` for auth

## Repository layout

This is a **multi-module monorepo**: each service and each shared dir has its own `go.mod`.
There is **no `go.work`**. Service modules use `replace` directives pointing at `../pkg` and
`../proto`.

```
gotiny/
├── api-gateway/         # Only HTTP entrypoint: REST → gRPC, JWT middleware (flat handler/)
├── url-service/         # Short URL creation + ownership enforcement (hexagonal)
├── redirect-service/    # Short code resolution + Redis cache-aside (hexagonal)
├── key-gen-service/     # Distributed short-key pool (hexagonal)
├── user-service/        # Auth: register, login, JWT + refresh-token lifecycle (hexagonal)
├── proto/               # Shared .proto + generated Go (keygen, url, redirect, user)
├── pkg/                 # Shared Go: config, database, grpcutil, logger
├── migrations/          # Postgres schema (001_init, 002_auth)
├── docs/                # architecture.md, auth.md
├── docker-compose.yml   # Full stack: postgres, redis, 5 services
└── Makefile             # proto / build / test / lint / up / down / seed / migrate
```

## Services

| Service | Port | Protocol | Responsibility | Talks to |
|---------|------|----------|----------------|----------|
| api-gateway | 8080 | REST | HTTP→gRPC translation, JWT middleware, input validation | url, redirect, user (gRPC) |
| url-service | 50051 | gRPC | Create/store/list/delete short URLs, enforce ownership | key-gen (gRPC), Postgres |
| redirect-service | 50052 | gRPC | Resolve short code → original URL, cache-aside | Redis, Postgres |
| key-gen-service | 50053 | gRPC | Vend unique base62 keys from a pre-generated pool | Postgres |
| user-service | 50054 | gRPC | Registration, login, JWT issue/validate, refresh rotation | Postgres |

> Note: `docker-compose.yml` maps the gateway to host port **8081** (`8081:8080`).
> Container-internal and local-run port is 8080.

### Request flow
- **Write/list/delete** (auth required): client → gateway (validates JWT locally, injects
  `x-user-id` gRPC metadata) → url-service → key-gen-service for a code → Postgres.
- **Redirect** (no auth): client → gateway → redirect-service → Redis (hit) or Postgres
  (miss, then populate cache) → 302.
- **Auth**: client → gateway → user-service → Postgres. See `docs/auth.md` for the full
  login/refresh/logout sequence diagrams.

### HTTP routes (gateway)
Public: `POST /register`, `POST /login`, `POST /refresh`, `POST /logout`, `GET /health`,
`GET /{code}` (redirect). JWT-protected: `POST /shorten`, `GET /urls`, `DELETE /urls/{code}`.

## Internal architecture (hexagonal)

The four gRPC services follow strict inward-pointing hexagonal layering. The api-gateway is
an edge proxy and stays intentionally flat (`internal/handler/` only).

```
cmd/main.go        Wiring, startup, graceful shutdown
internal/
  domain/          Entities, value objects, typed errors — ZERO external deps
  port/            Interfaces only (repository, cache)
  service/         Business logic — accepts/returns domain types only
  server/          gRPC transport — the ONLY layer that touches proto types
  adapter/
    postgres/      Repository implementations
    redis/         Cache implementation (redirect-service only)
```

Dependency direction: `server/ → service/ → port/ ← adapter/`, with `domain/` depended on by
all and depending on nothing.

**Hard rules (from AGENTS.md / docs/architecture.md):**
- Nothing in `domain/` may import proto, SQL, HTTP, or JSON-transport packages.
- Proto ↔ domain mapping lives only in `server/`. DB/cache types live only in `adapter/`.
- Each service owns its own domain errors in `internal/domain/`.
- Do not add hexagonal layers to api-gateway unless the architecture itself is changing.

## Shared packages (`pkg/`)

| Package | Provides |
|---------|----------|
| `config` | `EnvOr(key, fallback)` env loading |
| `logger` | `Init(serviceName)` — JSON `slog` handler tagged with service name |
| `database` | Postgres connection pool setup |
| `grpcutil` | `RunServer` (listener, health check, graceful + force shutdown); gRPC metadata helpers |

Reuse these rather than reintroducing per-service helpers.

## Key design decisions

- **Distributed key generation.** key-gen pre-generates base62 codes into Postgres. Instances
  claim batches with `SELECT … FOR UPDATE SKIP LOCKED` (no two instances ever vend the same
  code), buffer 100 in memory, and refill asynchronously.
- **Local JWT validation.** Access tokens (15min, HMAC) are verified in the gateway — protected
  endpoints make no per-request call to user-service. Refresh tokens (7d) live in Postgres,
  are single-use, and rotate on refresh (RT1 revoked before RT2 issued).
- **Cache-aside redirects.** Redis first, Postgres on miss, populate on read. TTL = 6h + random
  0–60s jitter (anti-stampede). Redis down → degrade gracefully to Postgres-only.

## Data model (Postgres)

- `keys` — pre-generated short codes with `status`/`claimed_by`/`used_at` for pool coordination.
- `urls` — `short_code` → `original_url`, plus `user_id` (ownership, added in 002).
- `users` — email + bcrypt `password_hash`.
- `refresh_tokens` — per-user tokens with `revoked` flag and `expires_at`.

## Common commands

```bash
make up      # docker compose up --build -d (full stack)
make seed    # seed 1000 keys into the pool
make down    # stop stack
make test    # go test ./... across every service module + pkg
make lint    # go vet ./... across every service module
make build   # go build ./cmd for each service
make proto   # regenerate gRPC code from proto/*.proto via protoc
```

**Verification:** code changes → `make test` then `make lint`. These iterate per-module
(each service has its own `go.mod`), so run them from the repo root.

Benchmarks: `cd redirect-service && go test -bench=. -benchmem ./internal/adapter/postgres/`

## When you change things

If you alter architecture, service boundaries, ports, startup wiring, or shared-package
responsibilities, update `README.md`, `docs/architecture.md`, `CHANGELOG.md`, **and** keep
`AGENTS.md` + this file in sync. Pending/deferred work (versioned proto paths, buf migration,
L1 cache + singleflight) is tracked in `TODOS.md`.
