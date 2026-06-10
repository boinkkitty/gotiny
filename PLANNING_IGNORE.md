# GoTiny - URL Shortener Planning

## Architecture Overview

```
                        ┌─────────────────────────────────────────────────────┐
                        │                 REST API Gateway                    │
                        │         (auth, rate limiting, routing)              │
                        └────┬──────────┬──────────┬──────────┬──────────────┘
                             │          │          │          │
                        gRPC │     gRPC │     gRPC │     gRPC │
                             │          │          │          │
                     ┌───────▼──┐ ┌─────▼────┐ ┌──▼───────┐ ┌▼─────────────┐
                     │  User    │ │   URL    │ │  KeyGen  │ │  Analytics   │
                     │  Service │ │  Service │ │  Service │ │  Service     │
                     └───────┬──┘ └────┬─────┘ └──┬───────┘ └──────┬───────┘
                             │         │          │                │
                             ▼         ▼          ▼                ▼
                        ┌──────────────────────────────┐    ┌────────────┐
                        │     PostgreSQL (source of     │    │   Redis    │
                        │     truth for all services)   │    │  Streams   │
                        └──────────────────────────────┘    │ stream:    │
                                                            │  clicks    │
                                                            └─────┬──────┘
                                                                  │ consume
            ┌──────────────────────┐                              │
  Users ──► │   Redirect Service   │──publish click──►────────────┘
            │   (standalone, not   │
            │   behind API GW)     │
            └──────────┬───────────┘
                       │
              ┌────────▼────────┐
              │  Redis Cache    │
              │  (L2, fallback  │
              │   to Postgres)  │
              └─────────────────┘
```

---

## Service Definitions

### 1. REST API Gateway
The single external entry point for all authenticated API traffic (URL creation, user management, analytics queries). Does NOT handle redirects — those go directly to the Redirect Service for performance isolation.

**Responsibilities:**
- Route requests to downstream gRPC services
- JWT validation and authentication enforcement
- Rate limiting (per-IP, per-user)
- Request/response transformation (REST <-> gRPC via grpc-gateway or manual)
- TLS termination
- Request logging and correlation ID propagation

### 2. User Service (gRPC)
Manages user identity, authentication, and authorization. Owns the user lifecycle.

**Responsibilities:**
- User registration (email/password with bcrypt hashing)
- Login — issue JWT access token (short-lived, 15min) + refresh token (long-lived, 7 days)
- Token refresh endpoint
- Logout — revoke refresh token (stored in PostgreSQL, checked on refresh)
- Ownership validation — exposes an internal RPC for other services to verify "does user X own resource Y"

**Data store:** PostgreSQL — `users`, `refresh_tokens` tables.

**Why gRPC:** Internal service only. No browser clients call it directly — the API Gateway translates. gRPC gives us strong typing via protobuf, efficient binary serialization, and codegen for Go clients.

### 3. URL Service (gRPC)
Core business logic for short link lifecycle. Translates between long URLs and short codes.

**Responsibilities:**
- Create short URL — request a key from KeyGen Service, persist mapping `(short_code, long_url, user_id, created_at, expires_at)`
- Read/Get — look up a short code's metadata (for the owner)
- Update — change the destination URL of an existing short code (ownership validated)
- Delete — soft-delete a short code (ownership validated)
- List — paginated list of a user's links with click count summaries
- Validate long URL format and reachability (optional HEAD request)

**Data store:** PostgreSQL — `links` table. Indexed on `short_code` (unique) and `user_id`.

### 4. KeyGen Service (gRPC)
Pre-generates short code keys so that URL creation never blocks on key generation. Acts as a buffered pool of ready-to-use, collision-free keys.

**Responsibilities:**
- Claim a batch of integer ranges from a PostgreSQL sequence (e.g., 1000 at a time)
- Encode integers to Base62 strings (7 characters = 3.5 trillion possible keys)
- Maintain an in-memory buffer of pre-generated keys
- Serve keys to URL Service on demand via `GetKey` / `GetKeys(n)` RPC
- Auto-refill buffer when it drops below low-water threshold

**Data store:** PostgreSQL sequence as the coordination point. In-memory buffer for serving.

**Key design decisions:**
- Each instance claims a non-overlapping range from a PostgreSQL sequence — two instances can never produce the same key
- If an instance crashes, unused keys in its claimed range are simply lost (negligible waste out of 3.5 trillion)
- On restart, the instance claims a fresh range — no need to recover previous state
- No bloom filter needed here — the sequence guarantees uniqueness by construction

### 5. Redirect Service (standalone, NOT behind API Gateway)
The hot path. Receives a short code and returns an HTTP redirect to the original URL. Separated from the API Gateway to scale independently and avoid redirect latency being affected by API traffic.

**Responsibilities:**
- Receive `GET /:shortCode` requests
- Look up destination: L1 in-process cache -> L2 Redis cache -> L3 PostgreSQL (fallback)
- Return `302 Found` redirect (302, not 301 — we need every click to hit our server for analytics)
- Publish click event to Redis Streams (`stream:clicks`) with: short_code, timestamp, IP, User-Agent, Referer
- Return 404 for non-existent or expired codes

**Data store:** Redis (primary read path), PostgreSQL (fallback).

**Why standalone:** Read:write ratio for URL shorteners is typically 100:1 or higher. The redirect service handles the overwhelming majority of traffic. Isolating it means:
- Independent horizontal scaling (add redirect instances without touching API infra)
- No auth overhead on the hot path (no JWT validation needed for redirects)
- Dedicated resource allocation — a traffic spike on redirects doesn't starve URL creation or analytics

### 6. Analytics Service (gRPC)
Consumes click events, aggregates them, and serves analytics queries to users.

**Responsibilities:**
- Consume from Redis Streams `stream:clicks` via consumer group with auto-claim for reliability
- Record click events to PostgreSQL: `link_visits` table (short_code, visitor_ip, user_agent, referer, country, timestamp)
- Deduplicate unique visitors by IP per short_code per day
- Serve stats queries via gRPC: total clicks, unique visitors, daily/weekly counts, top referrers
- Daily aggregation job (2am UTC): roll up raw events from `link_visits` into `link_visit_daily_summary`
- Retention purge: delete raw `link_visits` older than 30 days in batches (summary data retained indefinitely)

**Data store:** PostgreSQL — `link_visits`, `link_visit_daily_summary` tables.

---

## Requirements & Non-Functional Requirements

### Functional Requirements
| ID | Requirement | Service |
|----|-------------|---------|
| FR-1 | Users can register, login, logout | User Service |
| FR-2 | JWT-based auth with access + refresh tokens | User Service |
| FR-3 | Users can create short URLs from long URLs | URL Service + KeyGen |
| FR-4 | Users can read, update, delete their own links | URL Service |
| FR-5 | Ownership is enforced on all link mutations | URL Service + User Service |
| FR-6 | Short codes redirect to original URLs | Redirect Service |
| FR-7 | Users can view click analytics (daily/weekly summaries, totals) | Analytics Service |
| FR-8 | Click events are captured on every redirect | Redirect Service -> Analytics |
| FR-9 | Expired links return 404 | Redirect Service |

### Non-Functional Requirements

#### Throughput Targets (Initial — single-node reasonable targets)
| Metric | Target | Rationale |
|--------|--------|-----------|
| Redirects/sec (sustained) | 5,000 | Go + Redis can comfortably serve this on a single node |
| Redirects/sec (burst) | 20,000 | Viral link scenario; L1 in-process cache absorbs burst |
| URL creations/sec | 100 | Write path is much lighter; 100/sec = 8.6M/day |
| Redirect p95 latency | < 50ms | Redis lookup + 302 response; most traffic is cache hits |
| Redirect p99 latency | < 200ms | PostgreSQL fallback on cache miss |
| URL creation p95 latency | < 200ms | Key fetch + DB insert |

#### Availability
| Metric | Target |
|--------|--------|
| Redirect Service uptime | 99.9% (8.7 hours downtime/year) |
| API Gateway uptime | 99.9% |
| Data durability | 99.99% (PostgreSQL is source of truth with backups) |

#### KeyGen Buffer
| Parameter | Value | Rationale |
|-----------|-------|-----------|
| Buffer size per instance | 1,000 keys | At 100 URL creations/sec, this is ~10 seconds of buffer |
| Low-water refill threshold | 200 keys (20%) | Trigger async refill before exhaustion |
| Batch claim size from sequence | 1,000 | One DB round-trip per refill |
| Key length | 7 characters (Base62) | 3.5 trillion possible keys — effectively inexhaustible |

#### Caching
| Layer | TTL | Size | Purpose |
|-------|-----|------|---------|
| L1: In-process (sync.Map or LRU) | 30 seconds | 50,000 entries | Hot keys, zero network cost |
| L2: Redis | 6 hours + jitter | Millions of entries | Distributed cache, sub-ms lookups |
| Cache hit ratio target | > 95% | — | Only ~5% of requests should reach PostgreSQL |

#### Rate Limiting
| Dimension | Limit |
|-----------|-------|
| Unauthenticated IP | 10 requests/minute (URL creation) |
| Authenticated user | 100 URL creations/minute |
| Redirect per IP | 1,000/minute (prevent abuse) |

#### Data Retention
| Data | Retention |
|------|-----------|
| Raw click events (`link_visits`) | 30 days |
| Daily summaries (`link_visit_daily_summary`) | Indefinite |
| Short code mappings | Until user deletes or link expires |
| User accounts | Until user deletes |

---

## User Stories

### Authentication & Account
| ID | Story | Acceptance Criteria |
|----|-------|-------------------|
| US-1 | As a user, I can register with email and password | Account created, JWT issued, password stored as bcrypt hash |
| US-2 | As a user, I can log in and receive a JWT | Access token (15min) + refresh token (7d) returned |
| US-3 | As a user, I can refresh my access token | New access token issued if refresh token is valid and not revoked |
| US-4 | As a user, I can log out | Refresh token revoked, subsequent refresh attempts fail |

### Link Management
| ID | Story | Acceptance Criteria |
|----|-------|-------------------|
| US-5 | As a user, I can create a short URL from a long URL | Short code returned, mapping persisted, key consumed from KeyGen pool |
| US-6 | As a user, I can view all my created links | Paginated list with short_code, long_url, created_at, click count |
| US-7 | As a user, I can update the destination of my short link | Destination changed, Redis cache invalidated, old destination no longer served |
| US-8 | As a user, I can delete my short link | Soft-deleted, redirects return 404, cache entry evicted |
| US-9 | As a user, I cannot modify or delete links I don't own | 403 Forbidden returned |

### Redirect
| ID | Story | Acceptance Criteria |
|----|-------|-------------------|
| US-10 | As a visitor, I am redirected when I visit a short URL | 302 redirect to destination URL |
| US-11 | As a visitor, I see a 404 for non-existent or expired short URLs | 404 Not Found returned |
| US-12 | As a visitor, my click is recorded for analytics | Click event published to Redis Streams with IP, User-Agent, Referer, timestamp |

### Analytics
| ID | Story | Acceptance Criteria |
|----|-------|-------------------|
| US-13 | As a user, I can view total click count for my link | Total clicks returned |
| US-14 | As a user, I can view unique visitor count for my link | Deduplicated by IP per day |
| US-15 | As a user, I can view daily click summaries | Daily breakdown for the last 30 days |
| US-16 | As a user, I can view weekly click summaries | Aggregated from daily summaries |

---

## Open Questions & Decisions

### Resolved
| Question | Decision | Rationale |
|----------|----------|-----------|
| In-memory queue or PostgreSQL for KeyGen? | PostgreSQL sequence + in-memory buffer | Sequence guarantees uniqueness across instances. Buffer provides speed. On crash, lost keys are negligible (out of 3.5T). On restart, claim fresh range. |
| Two instances claiming same keys? | Impossible with PostgreSQL sequence | Each `nextval()` call is atomic and returns a unique value. Claiming ranges of 1000 means each instance gets a non-overlapping block. |
| Bloom filter for KeyGen? | No | Sequence-based generation guarantees zero collisions by construction. No need for probabilistic checks. |
| Same long URL — reuse short code or generate new? | Generate new | Reuse requires a reverse index lookup on every creation (expensive). Different users should own their own links. Dedup adds complexity for marginal storage savings. |
| Redis Streams vs RabbitMQ? | Redis Streams | Already have Redis for caching. Consumer groups with auto-claim provide reliable delivery. Simpler ops. Sufficient throughput for initial scale. |
| User Service — gRPC or REST? | gRPC | Consistency with other internal services. Only the API Gateway faces the public. |
| Redirect: 301 or 302? | 302 Found | 301 is cached by browsers — we'd lose analytics. 302 ensures every click hits our server. |
| Dedup unique visits by IP? | Yes, per short_code per day | Use a composite unique constraint or upsert pattern. Keeps unique visitor counts accurate without over-counting refreshes. |

### Open
| Question | Options | Considerations |
|----------|---------|----------------|
| Link expiration default? | No expiry / 1 year / 5 years | No expiry is simplest. Expiry helps with storage over time but adds complexity. |
| Soft delete or hard delete links? | Soft (set `deleted_at`) / Hard | Soft is safer and allows undo. Hard is simpler and saves storage. Soft delete means redirect service must check `deleted_at`. |
| Should redirect service connect directly to PostgreSQL? | Direct connection / via URL Service gRPC | Direct is faster (one fewer hop). Via gRPC is cleaner separation but adds latency on cache miss. |

---

## Eventual Targets & Possible Upgrades

### Tier 1 — High Value, Moderate Effort
| Feature | Description | Why |
|---------|-------------|-----|
| Custom aliases | Users provide their own short code (e.g., `gotiny.co/my-brand`) | Table-stakes feature for any serious URL shortener. Namespace isolation: auto-generated = exactly 7 chars, custom = 8+ chars or contains non-Base62 chars (e.g., hyphens). Prevents collisions by construction. |
| Link expiration / TTL | Per-link configurable expiry | Controls storage growth. Lazy check on redirect (expired = 404) + background purge job. |
| Bot detection in analytics | Filter bots from click counts | Without this, analytics are inflated. Use User-Agent signatures, known datacenter IP ranges, and behavioral heuristics. Show "total clicks" vs "human clicks" separately. |
| Geographic data (IP geolocation) | Country/city per click using MaxMind GeoLite2 | High-value analytics feature. Embed lookup in redirect service, include in click event payload. Free GeoLite2 database, updated monthly. |

### Tier 2 — Scale & Performance
| Feature | Description | Why |
|---------|-------------|-----|
| DynamoDB for URL lookups | Migrate link mappings from PostgreSQL to DynamoDB | Single-digit-ms reads at any scale. Key-value access pattern is a perfect fit. Keep PostgreSQL for user data and analytics. |
| ClickHouse for analytics | Migrate analytics from PostgreSQL to ClickHouse | Columnar storage, millions of inserts/sec, sub-second aggregation queries. Partition by day, order by `(short_code, timestamp)`. |
| Kafka for click events | Replace Redis Streams with Kafka | When you need: replay capability, multiple independent consumers (real-time dashboard, batch aggregation, fraud detection), and >100K events/sec. |
| CDN / edge caching | Cloudflare or CloudFront in front of redirect service | Serve redirects from edge PoPs globally. Sub-10ms for cached redirects. Reduces origin load dramatically. Note: requires careful cache invalidation on link update/delete. |
| L1 in-process cache with singleflight | `golang.org/x/sync/singleflight` for cache miss coalescing | When a popular key expires from cache, only one goroutine fetches from Redis/DB — others wait. Prevents thundering herd. |

### Tier 3 — Product Features
| Feature | Description |
|---------|-------------|
| QR code generation | Generate QR code on URL creation, serve as PNG/SVG. On-demand generation with caching. |
| Link previews / OG metadata | Crawl destination URL on creation, extract Open Graph tags. Show preview cards in dashboard. |
| Branded / custom domains | Users bring their own domain (e.g., `links.acme.com`). DNS CNAME + TLS cert provisioning. |
| A/B testing | Split traffic between two destinations at configurable percentages. Track clicks per variant. |
| Malicious URL detection | Check destination URLs against Google Safe Browsing API on creation. Periodic rescan of existing links. |
| Top referrers analytics | Track and display where clicks are coming from (Twitter, email, direct, etc.) |

### Architecture Techniques Worth Knowing
| Technique | When to Apply | Detail |
|-----------|---------------|--------|
| Bloom filter on redirect path | When invalid/bot traffic is high | Fast "definitely not in DB" check before any cache or DB lookup. Rejects enumeration attacks cheaply. Use RedisBloom module. |
| Cache jitter | Always | Add random +-60s to TTLs to prevent cache stampede when many keys expire simultaneously. |
| Read replicas | When read volume exceeds single Postgres | Route redirect service fallback reads to replicas. Writes stay on primary. |
| CQRS | At scale | Fully separate read model (redirect) and write model (URL creation). Different data stores, different scaling characteristics. |
| Circuit breaker on DB fallback | Always (good practice) | If PostgreSQL is down, fail fast with 503 rather than queuing thousands of connections. Use `sony/gobreaker`. |
| Graceful degradation | At scale | If analytics pipeline is backed up, redirect service should still redirect — just drop click events or buffer locally. Never let analytics failures impact redirects. |
