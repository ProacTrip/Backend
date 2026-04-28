# Proactrip Backend

[![Go 1.26.1](https://img.shields.io/badge/Go-1.26.1-00ADD8?logo=go)](https://go.dev/)
[![Echo v5](https://img.shields.io/badge/Echo-v5.1.0-0d9488?logo=go)](https://echo.labstack.com/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-18.3-336791?logo=postgresql)](https://www.postgresql.org/)
[![DragonflyDB](https://img.shields.io/badge/DragonflyDB-v1.38-EE7600)](https://www.dragonflydb.io/)
[![Docker](https://img.shields.io/badge/Docker-✓-2496ED?logo=docker)](https://www.docker.com/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

API server for the **Proactrip** travel platform — flight search, user authentication, and email notifications.

---

## Tech Stack

| Layer          | Technology            | Version      |
| -------------- | --------------------- | ------------ |
| **Language**   | Go                    | 1.26.1       |
| **HTTP**       | Echo                  | v5.1.0       |
| **Database**   | PostgreSQL + pgx      | 18.3 / v5    |
| **Cache & Bus**| DragonflyDB           | v1.38        |
| **Auth**       | PASETO v4 + Argon2id  | —            |
| **Hashing**    | Blake3                | —            |
| **Email**      | Resend                | v3           |
| **Flights**    | SerpAPI (HTTP client) | —            |
| **DevOps**     | Docker + Compose      | —            |

---

## Architecture

Vertical slice architecture — each module owns its domain, features, adapters, and migrations.

```
cmd/api/                  # Entry point
internal/
├── bootstrap/            # App initialization, routes, middleware wiring
├── config/               # Environment configuration
├── modules/
│   ├── auth/             # Authentication (register, login, logout, verify email)
│   │   ├── domain/       # Entities, repository interfaces, domain errors
│   │   ├── features/     # Use cases: register, login, logout, verify_email, current_user
│   │   ├── adapters/     # Postgres repo, PASETO, Argon2id, Blake3, encryption
│   │   └── migrations/   # SQL migrations
│   ├── search/           # Flight search (SerpAPI adapter, cache, pagination)
│   │   ├── domain/       # Flight entities, provider interface, errors
│   │   ├── features/     # search_flights, flight_details
│   │   ├── adapters/     # SerpAPI HTTP client, Postgres repo
│   │   └── migrations/
│   ├── notification/     # Email notifications (Resend, event consumer)
│   │   ├── adapters/     # Postgres repo, Resend email provider
│   │   ├── consumer/     # Event stream consumer (Dragonfly XREADGROUP)
│   │   ├── domain/       # Notification entity, repository interface
│   │   ├── features/     # send_verification_email
│   │   └── migrations/
│   └── user/             # User profile management
│       ├── consumer/     # Event stream consumer
│       ├── domain/       # User entity, repository interface
│       └── migrations/
└── shared/
    ├── cache/            # DragonflyDB cache helpers (TTL, permission cache)
    ├── context/          # Trace ID propagation
    ├── crypto/           # Cryptographic utilities
    ├── database/         # PostgreSQL connection pool (multi-DB pool manager)
    ├── encoding/         # Cursor-based pagination
    ├── errors/           # RFC 7807 Problem JSON types
    ├── eventbus/         # Event-driven architecture (Dragonfly Streams)
    ├── http/             # Cookie helpers, error mapping
    ├── middleware/       # Security headers (CSP, HSTS, X-Frame-Options)
    ├── ratelimit/        # Multi-tier rate limiting (Dragonfly + Lua)
    └── types.go
```

---

## Request Lifecycle

```
POST /v1/auth/login
    │
    ▼
┌──────────────────────────────────────────────────────┐
│  Middleware Pipeline                                 │
│  Security Headers → Global Rate Limit → CORS →       │
│  Request ID → Trace ID → Logger → Recover            │
└──────────────────────────────────────────────────────┘
    │
    ▼
┌──────────────────────────────────────────────────────┐
│  Handler                                             │
│  Bind JSON → Validate Input → Call UseCase           │
└──────────────────────────────────────────────────────┘
    │
    ▼
┌──────────────────────────────────────────────────────┐
│  UseCase (Business Logic)                            │
│  Domain validation → Orchestrate adapters            │
└──────────────────────────────────────────────────────┘
    │
    ├─────────────────────────┐
    ▼                         ▼
┌─────────────────┐   ┌─────────────────┐
│  PostgreSQL      │   │  DragonflyDB    │
│  (pgx pool/DB)   │   │  Cache + Streams│
└─────────────────┘   └─────────────────┘
    │                         │
    ▼                         ▼
┌─────────────────┐   ┌─────────────────┐
│  JSON Response   │   │  Event Bus      │
│  (RFC 7807 errs) │   │  user.registered│
└─────────────────┘   │  → notification  │
                      │    consumer      │
                      └─────────────────┘
```

---

## Event-Driven Architecture

Dragonfly Streams serve as the event bus for decoupled inter-module communication.

| Pattern           | Implementation                   |
| ----------------- | -------------------------------- |
| **Event Bus**     | Dragonfly Streams                |
| **Producer**      | Auth module (`user.registered`)  |
| **Consumers**     | Notification, User modules       |
| **Delivery**      | XREADGROUP with consumer groups (at-least-once) |
| **Orphan Rescue** | XAUTOCLAIM for unacknowledged messages |

```
Auth Module             Notification Module        User Module
───────────            ───────────────────        ───────────
register()                     │                      │
  │                            │                      │
  ├─► DB: INSERT user          │                      │
  │                            │                      │
  └─► Stream: XADD             │                      │
     "user.registered"  ────►  XREADGROUP ────►  XREADGROUP
                               │                    │
                               ├─► RESEND email     ├─► Create profile
                               │   verification     │   record
                               │                    │
                               └─► XACK             └─► XACK
```

**Event types:** `user.registered`, `trip.created`, `trip.updated`, `trip.deleted`

---

## API Documentation

| API          | Docs                                | Base Path   |
| ------------ | ----------------------------------- | ----------- |
| Auth         | [docs/AUTH_API.md](docs/AUTH_API.md)         | `/v1/auth`  |
| Flight Search| [docs/search_flights_api.md](docs/search_flights_api.md) | `/v1/search` |

All errors follow **RFC 7807 Problem JSON** format with `type`, `title`, `status`, `detail`, and `instance` fields.

### Auth Endpoints

| Method | Path                   | Auth Required | Rate Limit       |
| ------ | ---------------------- | :-----------: | ---------------- |
| POST   | `/v1/auth/register`    | No            | Global + Anon    |
| POST   | `/v1/auth/verify-email`| No            | Global + Anon    |
| POST   | `/v1/auth/login`       | No            | Global + Anon    |
| GET    | `/v1/auth/current-user`| PASETO        | Auth (10 req/min)|
| POST   | `/v1/auth/logout`      | PASETO        | Auth (10 req/min)|
| POST   | `/v1/auth/logout/all`  | PASETO        | Auth (10 req/min)|

### Search Endpoints

| Method | Path                        | Rate Limit                             |
| ------ | --------------------------- | -------------------------------------- |
| POST   | `/v1/search/flights`        | Anon (5 req/min) + SerpAPI (50 req/hr) |
| POST   | `/v1/search/flight-details` | Anon (5 req/min) + SerpAPI (50 req/hr) |

### Health

| Method | Path    | Description                |
| ------ | ------- | -------------------------- |
| GET    | `/health` | Liveness probe             |
| GET    | `/ready`  | Readiness probe (DB + Redis) |

---

## Rate Limiting

Multi-tier rate limiting using DragonflyDB with Lua scripts for atomic counters.

| Tier               | Limit                | Key                  |
| ------------------ | -------------------- | -------------------- |
| **Global**         | 100 req/min per IP   | `ratelimit:global:{ip}` |
| **Authenticated**  | 10 req/min per user  | `ratelimit:auth:{userID}` |
| **Anonymous**      | 5 req/min per cookie | `ratelimit:anon:{anonID}` |
| **Resend**         | 100 req/day          | `ratelimit:provider:resend` |
| **SerpAPI**        | 50 req/hour          | `ratelimit:provider:serpapi` |

All tiers expose `RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset`, and `Retry-After` response headers (RFC 6585 `429 Too Many Requests` on violation).

Configurable via environment variables (see [Environment Variables](#environment-variables)).

---

## Security

- **Headers:** CSP (`default-src 'self'`), HSTS (`max-age=31536000`), `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`
- **Cookies:** `HttpOnly`, `Secure` (production), `SameSite=Lax`
- **Passwords:** Argon2id with OWASP-recommended parameters (t=3, m=64MB, p=4)
- **Tokens:** PASETO v4 symmetric tokens with access/refresh token rotation
- **SQL Injection:** Parameterized queries via pgx (no raw SQL string concatenation)
- **Secrets:** 32-byte PASETO key (64 hex chars), stored in `.env`, never committed

---

## Docker Setup

Three services managed by Docker Compose:

```bash
# Development — live code mount, auto-restart
docker compose up -d

# Rebuild after code changes
docker compose exec api go build ./cmd/api

# Production — multi-stage optimized image (~15MB runner)
docker build -f Dockerfile.prod -t proactrip-backend .
docker run -p 8080:8080 --env-file .env proactrip-backend
```

**Compose services:**

| Service     | Image                           | Port  | Health Check                |
| ----------- | ------------------------------- | ----- | --------------------------- |
| postgres    | `postgres:18.3`                 | 5432  | `pg_isready`                |
| dragonfly   | `ghcr.io/dragonflydb/dragonfly:v1.38.0` | 6379  | `redis-cli ping`            |
| api         | `Dockerfile.dev` (build)        | 8080  | `wget /health`              |

---

## Environment Variables

| Variable                               | Default           | Description                                     |
| -------------------------------------- | ----------------- | ----------------------------------------------- |
| `SERVER_PORT`                          | `8080`            | HTTP server port                                |
| `SERVER_ENV`                           | `dev`             | Environment: `dev`, `staging`, `prod`           |
| `DB_HOST`                              | `localhost`       | PostgreSQL host                                 |
| `DB_PORT`                              | `5432`            | PostgreSQL port                                 |
| `DB_USER`                              | `postgres`        | Database user                                   |
| `DB_PASSWORD`                          | *(required)*      | Database password                               |
| `DB_NAME`                              | `proactrip`       | Database name                                   |
| `DB_SSLMODE`                           | `disable`         | SSL mode (`require` in production)              |
| `DRAGONFLY_HOST`                       | `localhost`       | DragonflyDB host                                |
| `DRAGONFLY_PORT`                       | `6379`            | DragonflyDB port                                |
| `DRAGONFLY_PASSWORD`                   | *(optional)*      | DragonflyDB password                            |
| `PASETO_KEY`                           | *(required)*      | 32-byte key (64 hex chars) — `openssl rand -hex 32` |
| `SERPAPI_KEY`                          | *(required)*      | SerpAPI key for flight search                   |
| `RESEND_API_KEY`                       | *(required)*      | Resend API key for email delivery               |
| `FRONTEND_URL_DEV`                     | `http://localhost:3000` | Frontend URL (development)                 |
| `FRONTEND_URL_PROD`                    | `https://proactrip.com` | Frontend URL (production)                 |
| `RATELIMIT_GLOBAL_PER_MINUTE`          | `100`             | Global rate limit (req/min per IP)              |
| `RATELIMIT_AUTH_PER_MINUTE`            | `10`              | Authenticated rate limit (req/min per user)     |
| `RATELIMIT_ANON_PER_MINUTE`            | `5`               | Anonymous rate limit (req/min per cookie)       |
| `RATELIMIT_PROVIDER_RESEND_MAX`        | `100`             | Resend max requests                             |
| `RATELIMIT_PROVIDER_RESEND_WINDOW_SEC` | `86400`           | Resend window (seconds, default: 24h)           |
| `RATELIMIT_PROVIDER_SERPAPI_MAX`       | `50`              | SerpAPI max requests                            |
| `RATELIMIT_PROVIDER_SERPAPI_WINDOW_SEC`| `3600`            | SerpAPI window (seconds, default: 1h)           |

---

## Quick Start

```bash
# 1. Clone and enter directory
cd Backend

# 2. Create environment file
cp .env.example .env

# 3. Edit .env — add your API keys
#    Required: DB_PASSWORD, PASETO_KEY, SERPAPI_KEY, RESEND_API_KEY
#    Generate PASETO_KEY: openssl rand -hex 32

# 4. Start infrastructure (PostgreSQL + DragonflyDB)
docker compose up -d

# 5. Run the API server
go run cmd/api/main.go
```

The API will be available at `http://localhost:8080`.

Verify with: `curl http://localhost:8080/health`
