# Changelog

All notable changes to this project will be documented in this file.

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
