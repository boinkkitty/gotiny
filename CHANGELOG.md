# Changelog

All notable changes to this project will be documented in this file.

## [0.2.1.0] - 2026-06-13

### Changed
- Restructured all four gRPC services to hexagonal architecture with `domain/`, `port/`, `adapter/`, and `server/` packages
- Extracted shared `pkg/config.EnvOr` helper, replacing duplicated `envOr` functions across five services
- Extracted shared `pkg/logger.Init` helper for consistent structured logging setup
- Extracted shared `pkg/grpcutil.RunServer` for gRPC server lifecycle with health checking and graceful shutdown with force-stop timeout
- Split redirect-service repository into separate postgres adapter and redis cache adapter behind port interfaces
- Moved domain errors and entities from repository/service packages into dedicated `domain/` packages
- Moved gRPC transport handlers from `handler/` to `server/` for clear transport-type naming
- Removed unused `MarkUsed` method from key-gen repository
- Removed `Revoked` field from `domain.RefreshToken`, enforcing revocation checks at the adapter layer

### Fixed
- Cache miss in redirect service no longer logs spurious WARN on every cold request

## [0.2.0.0] - 2026-06-13

### Added
- User authentication service with registration, login, JWT token issuance, refresh token rotation, and logout
- JWT middleware on API gateway protecting link creation, listing, and deletion endpoints
- Link ownership enforcement: users can only view and delete their own shortened URLs
- New REST endpoints: POST /register, POST /login, POST /refresh, POST /logout, GET /urls, DELETE /urls/:code
- gRPC metadata-based user_id propagation from API gateway to backend services via shared `pkg/grpcutil` helper
- Cache invalidation RPC on redirect service, called on link deletion to bust stale Redis entries
- Schema migration 002: users table, refresh_tokens table with partial index, user_id foreign key on urls
- User service Docker container with JWT_SECRET configuration

### Changed
- POST /shorten now requires Bearer token authentication (breaking change)
- Redis cache TTL changed from fixed 24h to 6h base with random 0-60s jitter to prevent cache stampede
- API gateway error handling expanded with 401, 403, and 409 responses for auth flows

## [0.1.0.0] - 2026-06-10

### Added
- URL shortener with 4-service gRPC microservices architecture
- API gateway with REST endpoints for creating short URLs and following redirects
- Key generation service with pre-generated base62 key pool and distributed coordination via `SELECT ... FOR UPDATE SKIP LOCKED`
- URL service for storing short code to URL mappings
- Redirect service with Redis cache-aside pattern and graceful fallback to PostgreSQL
- Input validation: URL scheme/host checks, request body size limits, short code format validation
- Docker Compose orchestration for full local stack (PostgreSQL 17, Redis 7, all 4 services)
- gRPC health checking on all backend services
- Graceful shutdown with signal handling across all services
- Unit tests for HTTP handlers, gRPC handlers, service layer, and repository layer
- PostgreSQL schema migrations with keys and urls tables
- Makefile for build, test, lint, and Docker operations
