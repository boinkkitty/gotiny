# TODOS

## Performance

### L1 In-Process Cache + Singleflight

**What:** Add L1 in-process LRU cache and singleflight to the redirect service.

**Why:** The "performance story with real numbers" from the design doc. Without L1, benchmarks only show Redis hit vs Postgres fallback. L1 is where sub-millisecond (~100ns) numbers come from. Singleflight prevents thundering herd when popular keys expire from cache.

**Context:** Use hashicorp/golang-lru/v2 expirable.NewLRU (50K entries, 30s TTL). Introduce a service layer to redirect-service (currently handler->repo direct). New `redirect-service/internal/service/redirect.go` wrapping the repository. L1 cache lives in the service layer. Singleflight wraps the full L2 (Redis) + L3 (Postgres) fallback chain. New service-level benchmarks: BenchmarkRedirectL1Hit, BenchmarkRedirectL2Hit, BenchmarkRedirectPostgresFallback, BenchmarkSingleflightCoalescing. Update README with benchmark ns/op output and annotated architecture diagram.

**Effort:** M
**Priority:** P1
**Depends on:** v0.2.0.0 (auth) merged

### Bcrypt Password Length Cap

**What:** Cap password input at 72 characters in the user-service handler.

**Why:** bcrypt silently truncates passwords at 72 bytes. Passwords longer than 72 bytes are effectively identical to their 72-byte prefix. Adding an explicit cap prevents user confusion and wasted hashing work on oversized inputs.

**Context:** Add validation in `user-service/internal/handler/grpc.go` alongside the existing `minPasswordLength` check. Return InvalidArgument for passwords exceeding 72 characters. Surfaced by adversarial review during v0.2.0.0 ship.

**Effort:** S
**Priority:** P3
**Depends on:** None

## Completed
