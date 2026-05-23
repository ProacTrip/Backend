# Proactrip Backend

[![Go 1.26.1](https://img.shields.io/badge/Go-1.26.1-00ADD8?logo=go)](https://go.dev/)
[![Echo v5](https://img.shields.io/badge/Echo-v5.1.0-0d9488?logo=go)](https://echo.labstack.com/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-18.3-336791?logo=postgresql)](https://www.postgresql.org/)
[![DragonflyDB](https://img.shields.io/badge/DragonflyDB-v1.38-EE7600)](https://www.dragonflydb.io/)
[![Docker](https://img.shields.io/badge/Docker-✓-2496ED?logo=docker)](https://www.docker.com/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

API server for the **Proactrip** travel platform — AI-powered flight + hotel search, user authentication, profile management, admin dashboard, email notifications, and environment detection.

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
| **Hotels**     | SerpAPI (HTTP client) | —            |
| **GeoIP**      | IPQuery (HTTP client) | —            |
| **Weather**    | OpenWeather (HTTP client) | —        |
| **AI / LLM**   | DeepSeek / Ollama       | —            |
| **Storage**    | MinIO / R2 (S3-compatible) | —         |
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
│   ├── auth/             # Authentication + Admin Dashboard
│   │   ├── domain/       # Entities, repository interfaces, domain errors, permissions
│   │   ├── features/     # register, login, logout, verify_email, oauth
│   │   │   └── dashboard/# account_status, feature_limits, list_users,
│   │   │                 # permission_overrides, user_detail
│   │   ├── adapters/     # Postgres repo, PASETO, Argon2id, Blake3, encryption
│   │   └── migrations/   # SQL migrations (4)
│   ├── environment/      # Geolocation + weather from client IP (IPQuery + OpenWeather)
│   │   ├── domain/       # CountryMetadata, WeatherData, Geolocation types
│   │   ├── features/     # get_environment
│   │   ├── adapters/     # IPQuery HTTP client, OpenWeather HTTP client
│   │   └── migrations/
│   ├── search/           # Flight + hotel search + AI natural language
│   │   ├── domain/       # Flight/Hotel entities, provider interfaces, AI types
│   │   ├── features/     # search_flights, flight_details, search_hotels,
│   │   │                 # hotel_details, ai_search, execute_saved_search
│   │   ├── adapters/     # SerpAPI HTTP client, DeepSeek/Ollama AI clients
│   │   └── migrations/
│   ├── notification/     # Email notifications (Resend, event consumer)
│   │   ├── adapters/     # Postgres repo, Resend email provider
│   │   ├── consumer/     # Event stream consumer (Dragonfly XREADGROUP)
│   │   ├── domain/       # Notification entity, repository interface
│   │   ├── features/     # send_verification_email
│   │   └── migrations/
│   └── user/             # User profile management + documents + avatars
│       ├── consumer/     # Event stream consumer (user.registered, user.locale.updated)
│       ├── domain/       # User entity, profile, preferences, repository interfaces
│       ├── features/     # get_profile, update_profile, upsert_profile, update_locale,
│       │                 # upload_avatar, upload_document, download_document,
│       │                 # add_favorite, delete_favorite, list_favorites,
│       │                 # save_search, delete_saved_search, list_saved_searches,
│       │                 # document_events (SSE), document_types
│       ├── adapters/     # Postgres repo, MinIO/R2 storage, OCR, SearchResolver
│       ├── pipeline/     # Async workers: avatar_validator, ocr_worker,
│       │                 # sanitizer_worker, validator_worker
│       └── migrations/   # SQL migrations (7)
└── shared/
    ├── auth/             # Permission constants + JWT claims helpers
    ├── cache/            # DragonflyDB helpers + MetricsDecorator (hit/miss/set counters)
    ├── context/          # Trace ID propagation
    ├── crypto/           # Cryptographic utilities
    ├── database/         # PostgreSQL connection pool (multi-DB pool manager)
    ├── encoding/         # Cursor-based pagination
    ├── errors/           # RFC 9457 Problem JSON types + error mappers
    ├── eventbus/         # Event-driven architecture (Dragonfly Streams)
    ├── http/             # Cookie helpers, error mapping
    ├── middleware/       # Security headers + permission middleware (RBAC)
    ├── ratelimit/        # Multi-tier rate limiting (Dragonfly + Lua)
    ├── search/           # Cross-module contract (SavedSearchProvider interface)
    ├── session/          # Session cache + schema versioning
    ├── user/             # Profile preferences contract (hashtag {user}:prefs: + TTL)
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
│  PostgreSQL     │   │  DragonflyDB    │
│  (pgx pool/DB)  │   │  Cache + Streams│
└─────────────────┘   └─────────────────┘
    │                         │
    ▼                         ▼
┌─────────────────┐   ┌─────────────────┐
│  JSON Response  │   │  Event Bus      │
│ (RFC 9457 errs) │   │  user.registered│
└─────────────────┘   │  → notification │
                      │    consumer     │
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

**Event types:** `user.registered`, `user.locale.updated`, `conversation.saved`, `doc.uploaded`, `trip.created`, `trip.updated`, `trip.deleted`

---

## API Documentation

| API          | Docs                                | Base Path   |
| ------------ | ----------------------------------- | ----------- |
| Auth         | [docs/AUTH_API.md](docs/AUTH_API.md)           | `/v1/auth`  |
| User         | [docs/USER_API.md](docs/USER_API.md)           | `/v1/user`  |
| Flight Search| [docs/search_flights_api.md](docs/search_flights_api.md) | `/v1/search` |
| Hotel Search | [docs/search_hotels_api.md](docs/search_hotels_api.md) | `/v1/search` |
| AI Search    | [docs/search_ai_api.md](docs/search_ai_api.md)    | `/v1/search` |
| Environment  | [docs/ENVIRONMENT_API.md](docs/ENVIRONMENT_API.md) | `/v1/environment` |
| Dashboard    | [docs/DASHBOARD_API.md](docs/DASHBOARD_API.md)    | `/v1/dashboard` |

All errors follow **RFC 9457 Problem JSON** format with `type`, `title`, `status`, `detail`, `instance`, and `trace_id` fields.

### Auth Endpoints

| Method | Path                   | Auth Required | Rate Limit       |
| ------ | ---------------------- | :-----------: | ---------------- |
| POST   | `/v1/auth/register`    | No            | Global + Anon    |
| POST   | `/v1/auth/verify-email`| No            | Global + Anon    |
| POST   | `/v1/auth/login`       | No            | Global + Anon    |
| POST   | `/v1/auth/logout`      | PASETO        | Auth (10 req/min)|

### Search Endpoints

| Method | Path                        | Rate Limit                             |
| ------ | --------------------------- | -------------------------------------- |
| POST   | `/v1/search/flights`        | Anon (5 req/min) + SerpAPI (50 req/hr) |
| POST   | `/v1/search/flight-details` | Anon (5 req/min) + SerpAPI (50 req/hr) |
| POST   | `/v1/search/hotels`         | Anon (5 req/min) + SerpAPI (50 req/hr) |
| POST   | `/v1/search/hotel-details`  | Anon (5 req/min) + SerpAPI (50 req/hr) |
| POST   | `/v1/search/ai`             | Auth (10 req/min) + AI provider        |

> **AI Search**: Interpretación en lenguaje natural con DeepSeek/Ollama. Soporta búsqueda por voz, fechas flexibles, y refinamiento multi-turno con `conversation_id`. Ver [docs/search_ai_api.md](docs/search_ai_api.md).

### User Endpoints

| Method | Path                              | Auth Required | Description                    |
| ------ | --------------------------------- | :-----------: | ------------------------------ |
| GET    | `/v1/user/profile`                | PASETO        | Obtener perfil del usuario     |
| PUT    | `/v1/user/profile`                | PASETO        | Actualizar perfil              |
| PUT    | `/v1/user/locale`                 | PASETO        | Cambiar idioma/moneda/timezone |
| POST   | `/v1/user/avatar`                 | PASETO        | Subir avatar (presigned URL)   |
| POST   | `/v1/user/documents`              | PASETO        | Subir documento                |
| GET    | `/v1/user/documents/:id`          | PASETO        | Descargar documento            |
| GET    | `/v1/user/documents/types`        | No            | Tipos de documento aceptados   |
| GET    | `/v1/user/documents/:id/events`   | PASETO        | SSE: estado de procesamiento   |
| POST   | `/v1/user/favorites`              | PASETO        | Agregar favorito               |
| DELETE | `/v1/user/favorites/:id`          | PASETO        | Eliminar favorito              |
| GET    | `/v1/user/favorites`              | PASETO        | Listar favoritos               |
| POST   | `/v1/user/saved-searches`         | PASETO        | Guardar búsqueda               |
| DELETE | `/v1/user/saved-searches/:id`     | PASETO        | Eliminar búsqueda guardada     |
| GET    | `/v1/user/saved-searches`         | PASETO        | Listar búsquedas guardadas     |

### Dashboard Endpoints (Admin)

| Method | Path                                    | Permission Required        |
| ------ | --------------------------------------- | -------------------------- |
| GET    | `/v1/dashboard/users`                   | `dashboard:users:list`     |
| GET    | `/v1/dashboard/users/:id`               | `dashboard:users:detail`   |
| GET    | `/v1/dashboard/users/:id/status`        | `dashboard:users:status`   |
| GET    | `/v1/dashboard/feature-limits`          | `dashboard:limits:read`    |
| PUT    | `/v1/dashboard/feature-limits`          | `dashboard:limits:write`   |
| GET    | `/v1/dashboard/permission-overrides`    | `dashboard:permissions:read` |
| PUT    | `/v1/dashboard/permission-overrides`    | `dashboard:permissions:write` |

### Environment Endpoint

| Method | Path               | Rate Limit              |
| ------ | ----------------- | ---------------------- |
| GET    | `/v1/environment` | Global + Anon          |

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
| **OpenWeather**    | 1000 req/day         | `ratelimit:provider:openweather` |

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
| `SERPAPI_KEY`                          | *(required)*      | SerpAPI key for flight + hotel search              |
| `RESEND_API_KEY`                       | *(required)*      | Resend API key for email delivery                  |
| `IPQUERY_API_KEY`                      | *(optional)*      | IPQuery API key for IP geolocation                 |
| `OPENWEATHER_API_KEY`                  | *(optional)*      | OpenWeather API key for weather data               |
| `FRONTEND_URL_DEV`                     | `http://localhost:3000` | Frontend URL (development)                 |
| `FRONTEND_URL_PROD`                    | `https://proactrip.com` | Frontend URL (production)                 |
| `RATELIMIT_GLOBAL_PER_MINUTE`          | `100`             | Global rate limit (req/min per IP)              |
| `RATELIMIT_AUTH_PER_MINUTE`            | `10`              | Authenticated rate limit (req/min per user)     |
| `RATELIMIT_ANON_PER_MINUTE`            | `5`               | Anonymous rate limit (req/min per cookie)       |
| `RATELIMIT_PROVIDER_RESEND_MAX`        | `100`             | Resend max requests                             |
| `RATELIMIT_PROVIDER_RESEND_WINDOW_SEC` | `86400`           | Resend window (seconds, default: 24h)           |
| `RATELIMIT_PROVIDER_SERPAPI_MAX`       | `50`              | SerpAPI max requests                            |
| `RATELIMIT_PROVIDER_SERPAPI_WINDOW_SEC`| `3600`            | SerpAPI window (seconds, default: 1h)           |
| `SEARCH_CACHE_TTL`                      | `5m`              | Cache TTL for flight/hotel search results       |
| `AI_INTERPRETATION_CACHE_TTL`           | `10m`             | Cache TTL for AI natural language interpretation |
| `DEEPSEEK_API_KEY`                      | *(optional)*      | DeepSeek API key for AI search                  |
| `AI_SEARCH_BASE_URL`                    | `https://api.deepseek.com/chat/completions` | DeepSeek API base URL for search (sin `/v1`) |
| `AI_SEARCH_MODEL`                       | `deepseek-v4-flash` | AI model for search interpretation             |
| `AI_OCR_MODEL`                          | `deepseek-v4-flash` | AI model for OCR (document processing)         |
| `OLLAMA_BASE_URL`                       | `http://localhost:11434` | Ollama local server URL                    |
| `DOCUMENT_UPLOAD_RATE_LIMIT`            | `10`              | Max document uploads per user per window        |
| `DOCUMENT_UPLOAD_RATE_WINDOW`           | `1h`              | Document upload rate window                    |
| `R2_ENDPOINT`                           | *(optional)*      | R2/S3-compatible storage endpoint               |
| `R2_ACCESS_KEY_ID`                      | *(optional)*      | R2 access key ID                                |
| `R2_SECRET_ACCESS_KEY`                  | *(optional)*      | R2 secret access key                            |
| `R2_BUCKET_NAME`                        | `proactrip`       | R2 bucket name                                  |
| `R2_USE_SSL`                            | `true`            | Use HTTPS for R2 connections                    |

---

## Quick Start

```bash
# 1. Clone and enter directory
cd Backend

# 2. Create environment file
cp .env.example .env

# 3. Edit .env — add your API keys
#    Required: DB_PASSWORD, PASETO_KEY, SERPAPI_KEY, RESEND_API_KEY
#    Optional: IPQUERY_API_KEY, OPENWEATHER_API_KEY
#    Generate PASETO_KEY: openssl rand -hex 32

# 4. Start infrastructure (PostgreSQL + DragonflyDB)
docker compose up -d

# 5. Run the API server
go run cmd/api/main.go
```

The API will be available at `http://localhost:8080`.

Verify with: `curl http://localhost:8080/health`
