# gotiny

A URL shortener built in Go with gRPC microservices architecture, demonstrating JWT authentication, distributed key generation, Redis caching, and production-grade service patterns.

> **Note:** This uses a microservices architecture deliberately to demonstrate systems design patterns (gRPC inter-service communication, distributed coordination, cache-aside). A URL shortener at this scale would be better served by a monolith. The architecture decisions here are intentional for the engineering challenge, not the domain.

## Architecture

```
                    ┌──────────────────────────┐
                    │    REST API Gateway       │
                    │  (Go, net/http + ServeMux)│
                    │  JWT Middleware (8080)     │
                    └──┬────────┬────────┬──────┘
                       │ gRPC   │ gRPC   │ gRPC
                       │        │        │
            ┌──────────▼──┐ ┌───▼──────────┐ ┌──▼──────────┐
            │ URL Service │ │Redirect Svc  │ │ User Service │
            │ (50051)     │ │  (50052)     │ │   (50054)    │
            └──┬───┬──────┘ └──┬────┬──────┘ └──────┬──────┘
               │   │ SQL       │    │ SQL            │ SQL
        gRPC   │   │           │    │                │
            ┌──▼───┘        ┌──▼    │                │
            │               │       │                │
            │  ┌──────────┐ │  ┌────▼───┐            │
            │  │Key Gen   │ │  │ Redis  │            │
            │  │Service   │ │  │ Cache  │            │
            │  │ (50053)  │ │  └────────┘            │
            │  └────┬─────┘ │                        │
            │       │ SQL   │                        │
            ▼       ▼       ▼                        ▼
           ┌──────────────────────────────────────────┐
           │               PostgreSQL                  │
           └──────────────────────────────────────────┘
```

### Services

| Service | Port | Protocol | Responsibility |
|---------|------|----------|----------------|
| API Gateway | 8080 | REST | HTTP → gRPC translation, JWT auth, URL validation |
| URL Service | 50051 | gRPC | Creates short URLs, stores mappings, ownership enforcement |
| Redirect Service | 50052 | gRPC | Resolves short codes, Redis cache-aside with TTL jitter |
| Key Gen Service | 50053 | gRPC | Pre-generated key pool, distributed coordination |
| User Service | 50054 | gRPC | Registration, login, JWT token lifecycle, refresh rotation |

### Key Design Decisions

**Distributed Key Generation.** The key-gen service pre-generates base62 short codes and stores them in PostgreSQL. Instances claim batches atomically using `SELECT ... FOR UPDATE SKIP LOCKED`, preventing two instances from ever returning the same key. Each instance buffers 100 keys in memory and refills asynchronously.

**JWT Authentication Across Services.** Access tokens (15min TTL) are validated locally by the API gateway using a shared HMAC key, so protected endpoints don't call the user service on every request. Refresh tokens (7d TTL) are stored in PostgreSQL with rotation on refresh and database-backed revocation.

**Cache-Aside Pattern.** The redirect service checks Redis first, falls back to PostgreSQL on cache miss, and populates the cache on read. TTL uses 6h base + random 0-60s jitter to prevent cache stampede. If Redis is unavailable, the service degrades gracefully to PostgreSQL-only.

**Multi-Module Monorepo.** Each service has its own `go.mod` with `replace` directives pointing to shared modules (`proto/`, `pkg/`). This enforces real module boundaries while keeping everything in one repo.

## Quick Start

```bash
# Start all services
make up

# Seed the key pool
make seed

# Register a user
curl -X POST http://localhost:8080/register \
  -H 'Content-Type: application/json' \
  -d '{"email": "user@example.com", "password": "password123"}'

# Login (save the access_token from the response)
curl -X POST http://localhost:8080/login \
  -H 'Content-Type: application/json' \
  -d '{"email": "user@example.com", "password": "password123"}'

# Create a short URL (requires auth)
curl -X POST http://localhost:8080/shorten \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <access_token>' \
  -d '{"url": "https://example.com"}'

# Follow the redirect (no auth required)
curl -L http://localhost:8080/<short_code>

# List your URLs
curl http://localhost:8080/urls \
  -H 'Authorization: Bearer <access_token>'

# Delete a URL
curl -X DELETE http://localhost:8080/urls/<short_code> \
  -H 'Authorization: Bearer <access_token>'

# Shut down
make down
```

## Project Structure

```
gotiny/
├── api-gateway/         # REST → gRPC translation, JWT middleware
├── url-service/         # Short URL creation + ownership enforcement
├── redirect-service/    # Short code resolution + Redis cache
├── key-gen-service/     # Distributed key pool management
├── user-service/        # User auth (register, login, JWT lifecycle)
├── proto/               # Shared Protocol Buffer definitions
├── pkg/                 # Shared Go packages (database, grpcutil)
├── docs/                # Architecture and flow diagrams
├── migrations/          # PostgreSQL schema migrations
├── docker-compose.yml   # Full stack orchestration (6 services)
└── Makefile             # Build, test, run commands
```

## Development

```bash
make proto    # Regenerate gRPC code from .proto files
make build    # Build all services
make test     # Run all tests
make lint     # Run go vet on all services
```

### Running Benchmarks

```bash
cd redirect-service
go test -bench=. -benchmem ./internal/repository/
```

## Tech Stack

- **Go 1.26** with `log/slog` structured logging
- **gRPC** with Protocol Buffers for inter-service communication
- **PostgreSQL 17** as the single source of truth
- **Redis 7** for redirect caching (cache-aside pattern)
- **Docker Compose** for local orchestration
