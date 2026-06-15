# TODOS

## Performance

### L1 In-Process Cache + Singleflight

**What:** Add L1 in-process LRU cache and singleflight to the redirect service.

**Why:** The "performance story with real numbers" from the design doc. Without L1, benchmarks only show Redis hit vs Postgres fallback. L1 is where sub-millisecond (~100ns) numbers come from. Singleflight prevents thundering herd when popular keys expire from cache.

**Context:** Use hashicorp/golang-lru/v2 expirable.NewLRU (50K entries, 30s TTL). After the hex refactor, redirect-service already has a service layer at `redirect-service/internal/service/redirect.go`. L1 cache lives in the service layer. Singleflight wraps the full L2 (Redis) + L3 (Postgres) fallback chain. New service-level benchmarks: BenchmarkRedirectL1Hit, BenchmarkRedirectL2Hit, BenchmarkRedirectPostgresFallback, BenchmarkSingleflightCoalescing. Update README with benchmark ns/op output and annotated architecture diagram.

**Effort:** M
**Priority:** P1
**Depends on:** None (hex refactor landed in v0.2.1.0)

### Bcrypt Password Length Cap

**What:** Cap password input at 72 characters in the user-service handler.

**Why:** bcrypt silently truncates passwords at 72 bytes. Passwords longer than 72 bytes are effectively identical to their 72-byte prefix. Adding an explicit cap prevents user confusion and wasted hashing work on oversized inputs.

**Context:** Add validation in `user-service/internal/server/grpc.go` (post hex refactor, formerly `handler/grpc.go`) alongside the existing `minPasswordLength` check. Return InvalidArgument for passwords exceeding 72 characters. Surfaced by adversarial review during v0.2.0.0 ship.

**Effort:** S
**Priority:** P3
**Depends on:** None

## Infrastructure

### Proto Versioning + Buf Migration

**What:** Restructure proto files to versioned paths and adopt buf for code generation.

**Why:** Versioned proto paths (`proto/{service}/v1/`) are the production standard for gRPC APIs. buf provides lint, breaking change detection, and consistent code generation. Current setup uses raw protoc with no versioning.

**Context:** Move `proto/keygen/` to `proto/keygen/v1/`, etc. Add `buf.yaml` and `buf.gen.yaml` at the repo root. Update generated code output to `gen/` directory. Update all service import paths for generated code. This was originally part of the hex architecture plan but deferred as a separate concern (different from the structural refactor). Surfaced by /plan-eng-review on 2026-06-13.

**Effort:** M
**Priority:** P2
**Depends on:** None (hex refactor landed in v0.2.1.0)

### Redis Key Queue Observability

**What:** Add Prometheus metrics for the Redis key queue in key-gen-service.

**Why:** The Redis key queue introduces a new operational layer between Postgres and key-gen instances. Without observability, queue exhaustion, refill lock contention, and Redis degradation are invisible until they surface as user-facing latency or errors.

**Context:** Metrics to add: LLEN gauge (pool depth), LPOP rate, refill events, refill lock contention count, Redis error rate, fallback-to-Postgres activations. Track Redis vs Postgres-direct hit rate as a percentage (target >99% Redis under normal load). Compare p50/p99 latency of shorten endpoint with Redis queue vs baseline. Alert if LLEN drops below critical floor (100). Instrument in `key-gen-service/internal/service/keygen.go` and `key-gen-service/internal/adapter/redis/`.

**Effort:** M
**Priority:** P2
**Depends on:** Redis key queue (feat/shorten-cache)

## Completed
