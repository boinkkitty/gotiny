# gotiny

A URL shortener built in Go with gRPC microservices architecture, demonstrating distributed key generation, Redis caching, and production-grade service patterns.

> **Note:** This uses a microservices architecture deliberately to demonstrate systems design patterns (gRPC inter-service communication, distributed coordination, cache-aside). A URL shortener at this scale would be better served by a monolith. The architecture decisions here are intentional for the engineering challenge, not the domain.

## Architecture

```
                    ┌─────────────────────────┐
                    │   REST API Gateway      │
                    │   (Go, net/http + chi)   │
                    │     (port 8080)          │
                    └────┬──────────┬──────────┘
                         │ gRPC     │ gRPC
                         │          │
              ┌──────────▼──┐  ┌────▼──────────┐
              │ URL Service │  │Redirect Service│
              │ (port 50051)│  │  (port 50052)  │
              └──┬───┬──────┘  └──┬────┬────────┘
                 │   │ SQL        │    │ SQL
          gRPC   │   │            │    │
              ┌──▼───┘         ┌──▼    │
              │                │       │
              │  ┌──────────┐  │  ┌────▼───┐
              │  │Key Gen   │  │  │ Redis  │
              │  │Service   │  │  │ Cache  │
              │  │(port     │  │  └────────┘
              │  │ 50053)   │  │
              │  └────┬─────┘  │
              │       │ SQL    │
              ▼       ▼        ▼
           ┌────────────────────────┐
           │      PostgreSQL        │
           └────────────────────────┘
```

### Services

| Service | Port | Protocol | Responsibility |
|---------|------|----------|----------------|
| API Gateway | 8080 | REST | HTTP → gRPC translation, URL validation |
| URL Service | 50051 | gRPC | Creates short URLs, stores mappings |
| Redirect Service | 50052 | gRPC | Resolves short codes, Redis cache-aside |
| Key Gen Service | 50053 | gRPC | Pre-generated key pool, distributed coordination |

### Key Design Decisions

**Distributed Key Generation.** The key-gen service pre-generates base62 short codes and stores them in PostgreSQL. Instances claim batches atomically using `SELECT ... FOR UPDATE SKIP LOCKED`, preventing two instances from ever returning the same key. Each instance buffers 100 keys in memory and refills asynchronously.

**Cache-Aside Pattern.** The redirect service checks Redis first, falls back to PostgreSQL on cache miss, and populates the cache on read. If Redis is unavailable, the service degrades gracefully to PostgreSQL-only.

**Multi-Module Monorepo.** Each service has its own `go.mod` with `replace` directives pointing to shared modules (`proto/`, `pkg/`). This enforces real module boundaries while keeping everything in one repo.

## Quick Start

```bash
# Start all services
make up

# Seed the key pool
make seed

# Create a short URL
curl -X POST http://localhost:8080/shorten \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://example.com"}'

# Follow the redirect
curl -L http://localhost:8080/<short_code>

# Shut down
make down
```

## Project Structure

```
gotiny/
├── api-gateway/         # REST → gRPC translation layer
├── url-service/         # Short URL creation
├── redirect-service/    # Short code resolution + Redis cache
├── key-gen-service/     # Distributed key pool management
├── proto/               # Shared Protocol Buffer definitions
├── pkg/                 # Shared Go packages (database)
├── migrations/          # PostgreSQL schema migrations
├── docker-compose.yml   # Full stack orchestration
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
