# adapter-infra

## Trigger
When the user needs to implement a domain interface (port) with infrastructure code: PostgreSQL repositories, Dragonfly/Redis cache adapters, HTTP clients for external APIs, or any technology-specific adapter. Keywords: "adapter", "repositorio postgres", "implementar interfaz", "pgx", "dragonfly cache", "cliente HTTP", "API externa", "adapter infra"

## Questions to Ask (ALWAYS ask first — never generate code without answers)

1. ¿Qué interfaz del dominio vas a implementar? Dame el nombre del paquete y la interfaz (ej: `domain.BookingRepository`)
2. ¿Qué tipo de adapter necesitás? (PostgreSQL/pgx, Dragonfly/Redis, HTTP client externo, API externa)
3. ¿Qué operaciones tiene la interfaz? (Create, GetByID, Update, Delete, Search, List)
4. ¿Necesita rate limiting el adapter? (para APIs externas con cuotas limitadas)
5. ¿Hay campos que requieran serialización especial? (JSONB, encriptación AES, compresión)

## Rules (Non-Negotiable — fail if violated)

### Adapter Rules (A1-A5)
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| A1 | Local Interface | CRITICAL | PostgreSQL repos define local `PgxPool` interface with only needed methods |
| A2 | Compile-time Check | CRITICAL | `var _ domain.XxxRepo = (*Adapter)(nil)` at file top |
| A3 | SQL Placeholders | MUST | Use `$1, $2` pgx placeholders, never `?` or string formatting |
| A4 | Error Wrapping | MUST | `fmt.Errorf("operation desc: %w", err)` — wrapped with context, never bare |
| A5 | No Business Logic | CRITICAL | Adapters translate/map ONLY — zero business decisions |

### Global Rules (R1-R9)
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| R1 | Module Isolation | CRITICAL | Modules communicate via interfaces or events |
| R2 | No Cross-Module | CRITICAL | NEVER import another module's features/ or adapters/ |
| R3 | Shared Boundaries | CRITICAL | shared/ MUST NOT import from modules/ |
| R5 | DI | MUST | Constructor injection, no globals |
| R7 | Go 1.26 | MUST | omitzero, new(expr) |
| R9 | Naming | MUST | File named after technology (echo.go, paseto.go, resend.go) |

## Patterns

### Pattern 1: PostgreSQL Repository with PgxPool (from auth/adapters/postgres/user_repository.go)
```go
type PgxPool interface {
    Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

type UserRepository struct {
    pool PgxPool
}

var _ domain.UserRepository = (*UserRepository)(nil)

func NewUserRepository(pool PgxPool) *UserRepository {
    return &UserRepository{pool: pool}
}
```

### Pattern 2: Dragonfly Cache Adapter (from cache/dragonfly.go)
```go
type CacheAdapter struct {
    client *redis.Client
}

func (c *CacheAdapter) Get(ctx context.Context, key string) (string, error) {
    val, err := c.client.Get(ctx, key).Result()
    if errors.Is(err, redis.Nil) {
        return "", nil // cache miss, not an error
    }
    return val, err
}
```

### Pattern 3: HTTP Client with Rate Limiting (from serpapi/adapter.go)
```go
type Adapter struct {
    client      *http.Client
    apiKey      string
    baseURL     string
    rateLimiter *ratelimit.RateLimiter
}

func (a *Adapter) SetRateLimiter(rl *ratelimit.RateLimiter) {
    a.rateLimiter = rl
}
```

## Templates

### Template: pgx Repository
```go
package postgres

import (
    "context"
    "errors"
    "fmt"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"
    "github.com/ProacTrip/Backend/internal/modules/{{.ModuleName}}/domain"
)

// PgxPool define las operaciones de pgxpool necesarias para {{.EntityName}}Repository.
type PgxPool interface {
    Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

// {{.EntityName}}Repository implementa domain.{{.EntityName}}Repository usando PostgreSQL/pgx.
type {{.EntityName}}Repository struct {
    pool PgxPool
}

var _ domain.{{.EntityName}}Repository = (*{{.EntityName}}Repository)(nil)

func New{{.EntityName}}Repository(pool PgxPool) *{{.EntityName}}Repository {
    return &{{.EntityName}}Repository{pool: pool}
}

func (r *{{.EntityName}}Repository) Create(ctx context.Context, {{.EntityVar}} *domain.{{.EntityName}}) error {
    _, err := r.pool.Exec(ctx,
        `INSERT INTO {{.TableName}} (id, created_at, updated_at) VALUES ($1, $2, $3)`,
        {{.EntityVar}}.ID, {{.EntityVar}}.CreatedAt, {{.EntityVar}}.UpdatedAt,
    )
    if err != nil {
        return fmt.Errorf("create {{.EntityVar}}: %w", err)
    }
    return nil
}

func (r *{{.EntityName}}Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.{{.EntityName}}, error) {
    {{.EntityVar}} := &domain.{{.EntityName}}{}
    err := r.pool.QueryRow(ctx,
        `SELECT id, created_at, updated_at FROM {{.TableName}} WHERE id = $1`, id,
    ).Scan(&{{.EntityVar}}.ID, &{{.EntityVar}}.CreatedAt, &{{.EntityVar}}.UpdatedAt)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, fmt.Errorf("get {{.EntityVar}}: %w", domain.Err{{.EntityName}}NotFound)
    }
    if err != nil {
        return nil, fmt.Errorf("get {{.EntityVar}} by id: %w", err)
    }
    return {{.EntityVar}}, nil
}
```

### Template: Dragonfly Cache Adapter
```go
package dragonfly

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "time"
    "github.com/redis/go-redis/v9"
)

// {{.EntityName}}Cache implementa el caché para {{.EntityName}} usando Dragonfly/Redis.
type {{.EntityName}}Cache struct {
    client *redis.Client
    ttl    time.Duration
}

func New{{.EntityName}}Cache(client *redis.Client, ttl time.Duration) *{{.EntityName}}Cache {
    return &{{.EntityName}}Cache{client: client, ttl: ttl}
}

func (c *{{.EntityName}}Cache) Get(ctx context.Context, key string) (string, error) {
    val, err := c.client.Get(ctx, key).Result()
    if errors.Is(err, redis.Nil) {
        return "", nil
    }
    return val, err
}

func (c *{{.EntityName}}Cache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
    data, err := json.Marshal(value)
    if err != nil {
        return fmt.Errorf("marshal cache value: %w", err)
    }
    return c.client.Set(ctx, key, data, ttl).Err()
}
```

### Template: HTTP Client Adapter
```go
package {{.ProviderName}}

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
    "github.com/ProacTrip/Backend/internal/modules/{{.ModuleName}}/domain"
    "github.com/ProacTrip/Backend/internal/shared/ratelimit"
)

// Adapter implementa domain.{{.InterfaceName}} usando {{.ProviderName}} API.
type Adapter struct {
    client      *http.Client
    apiKey      string
    baseURL     string
    rateLimiter *ratelimit.RateLimiter
}

var _ domain.{{.InterfaceName}} = (*Adapter)(nil)

func NewAdapter(apiKey, baseURL string, timeout time.Duration) *Adapter {
    return &Adapter{
        client:  &http.Client{Timeout: timeout},
        apiKey:  apiKey,
        baseURL: baseURL,
    }
}

func (a *Adapter) SetRateLimiter(rl *ratelimit.RateLimiter) {
    a.rateLimiter = rl
}

func (a *Adapter) call(ctx context.Context, path string) (*http.Response, error) {
    if a.rateLimiter != nil {
        if result, err := a.rateLimiter.ProviderAllow(ctx, "{{.ProviderName}}"); err != nil || !result.Allowed {
            return nil, fmt.Errorf("rate limit exceeded: %w", err)
        }
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }
    return a.client.Do(req)
}
```

## Uses Skills
| Skill | When |
|-------|------|
| `domain-core` | Always — need domain interfaces to implement |
| `dragonfly` | When implementing cache adapters with Dragonfly/Redis |
| `go` | Always — Go 1.26 patterns |

## Verification
```bash
# 1. Compile-time interface check present
grep 'var _ domain\.' internal/modules/{{.ModuleName}}/adapters/**/*.go || echo "WARN: no compile-time interface check"

# 2. Local PgxPool interface defined
grep 'type PgxPool interface' internal/modules/{{.ModuleName}}/adapters/postgres/*.go || echo "WARN: no local PgxPool"

# 3. No business logic in adapters
grep -E 'if.*status|if.*discount|if.*price' internal/modules/{{.ModuleName}}/adapters/**/*.go && echo "WARN: possible business logic" || echo "PASS"

# 4. Error wrapping with context
grep 'fmt.Errorf.*: %w' internal/modules/{{.ModuleName}}/adapters/**/*.go || echo "WARN: errors not wrapped with context"

# 5. Compile check
go build ./internal/modules/{{.ModuleName}}/... && echo "PASS: compiles"
```
