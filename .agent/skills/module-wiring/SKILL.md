# module-wiring

## Trigger
When the user needs to create or update a `module.go` file — the DI composition root for any Proactrip module. Keywords: "crear module.go", "module wiring", "inyección dependencias", "DI container", "NewModule", "composición módulo", "Config struct"

## Questions to Ask (ALWAYS ask first — never generate code without answers)

1. ¿Cuál es el nombre del módulo? (camelCase: booking, payment, analytics)
2. ¿Qué features/handlers va a exponer el módulo? (SearchHandler, CreateHandler, etc.)
3. ¿Qué dependencias externas necesita? (pgx pool, Dragonfly client, rate limiter, event bus, API keys)
4. ¿El módulo publica eventos? ¿Consume eventos? (necesita EventPublisher en Config, consumer en Module)
5. ¿Necesita middleware especial o configuración adicional? (auth middleware propio, rate limiting específico)

## Rules (Non-Negotiable — fail if violated)

### Module Wiring Rules (W1-W5 from spec)
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| W1 | Config Purity | CRITICAL | Config struct uses interfaces for swappable dependencies |
| W2 | Wiring Order | CRITICAL | NewModule wires: adapters → services → usecases → handlers in order |
| W3 | Error Mapping | MUST | Calls `register{Module}Errors()` to map domain errors → HTTP |
| W4 | Handler Exposure | MUST | Handlers exposed as public Module struct fields |
| W5 | Initialization Log | MUST | `slog.Info("module initialized", "features", [...])` on success |

### Global Rules (R1-R9)
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| R1 | Module Isolation | CRITICAL | Modules communicate via injected interfaces or events |
| R2 | No Cross-Module Imports | CRITICAL | NEVER import another module's `features/` or `adapters/` |
| R3 | Shared Boundaries | CRITICAL | `shared/` packages MUST NOT import from `modules/` |
| R4 | Error Flow | MUST | Domain errors → RegisterDomainErrorMapper() → RFC 9457 Problem JSON |
| R5 | DI | MUST | Manual constructor injection, zero globals, zero singletons |
| R6 | Testing | MUST | Generate `_test.go` when entity has behavior methods |
| R7 | Go 1.26 | MUST | `omitzero`, `new(expr)`, `errors.AsType`, `uuid.Must(uuid.NewV7())` |
| R8 | Echo v5 | MUST | *echo.Context pointer, echo.StartConfig, echo.PathParam[T]() |
| R9 | Naming | MUST | Adapter files named after technology |

### Additional Conventions
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| C1 | Import Alias | MUST | `serrors "github.com/ProacTrip/Backend/internal/shared/errors"` for shared errors |
| C2 | Import Alias | MUST | `httperr "github.com/ProacTrip/Backend/internal/shared/http"` for HTTP error mapper |
| C3 | Error Mapper | MUST | Error mapper function is private (lowercase `register{Module}Errors`) |
| C4 | slog | MUST | Use `slog.Warn` for missing optional deps, `slog.Info` on init success |
| C5 | Handler Factory | SHOULD | Use `func() *Handler` factory for idempotent handlers |
| C6 | Init | MUST NOT | Zero `init()` functions — all wiring in NewModule |

### Error Mapper Reference
| serrors Factory | HTTP Status | When to Use |
|----------------|-------------|-------------|
| `serrors.ErrBadRequest(msg, err)` | 400 | Invalid input, validation errors |
| `serrors.ErrUnauthorized(msg, err)` | 401 | Invalid/expired token |
| `serrors.ErrForbidden(msg, err)` | 403 | Valid token but insufficient permissions |
| `serrors.ErrNotFound(msg, err)` | 404 | Entity not found |
| `serrors.ErrConflict(msg, err)` | 409 | Duplicate, state conflict |
| `serrors.ErrTooManyRequests(msg, err)` | 429 | Rate limit exceeded |
| `serrors.ErrInternalError(msg, err)` | 500 | Unexpected errors |
| `serrors.ErrServiceUnavailable(msg, err)` | 503 | External service down |

## Patterns

### Pattern 1: Module struct with handlers (from auth/module.go)
```go
type Module struct {
    Repository        domain.UserRepository
    EventPublisher    eventbus.EventPublisher
    Register          *register.UseCase
    LoginHandler      *login.Handler
    RegisterHandler   func() *register.Handler  // factory para idempotencia
}
```

### Pattern 2: Config struct (from auth/module.go)
```go
type Config struct {
    PostgresPool    PgxPool
    DragonflyClient *redis.Client
    PasetoKey       []byte
    EventPublisher  eventbus.EventPublisher
    IsProduction    bool
}
```

### Pattern 3: NewModule Wiring Order (from auth/module.go)
```go
func NewModule(cfg Config) (*Module, error) {
    m := &Module{}
    // 1. Adapters (infrastructure)
    m.Repository = postgres.NewUserRepository(cfg.PostgresPool)
    // 2. Services (business logic dependencies)
    m.PasswordHasher = password.NewHasher()
    // 3. UseCases (application logic)
    m.Register = register.NewUseCase(register.UseCaseDeps{...})
    // 4. Handlers (HTTP layer)
    m.RegisterHandler = func() *register.Handler { return register.NewHandler(m.Register) }
    // 5. Error Mappers
    registerAuthErrors()
    slog.Info("module initialized", "features", [...]string{"register", "login", "logout"})
    return m, nil
}
```

## Templates

### Template: Module struct
```go
// Module agrupa todos los servicios y handlers del módulo {{.ModuleName}}.
type Module struct {
    // Handlers
    {{range .Features}}
    {{.HandlerName}} *{{.Package}}.{{.HandlerType}}
    {{end}}
}
```

### Template: Config struct
```go
// Config contiene todas las dependencias externas del módulo {{.ModuleName}}.
type Config struct {
    PostgresPool    PgxPool
    {{if .HasDragonfly}}
    DragonflyClient *redis.Client
    {{end}}
    {{if .HasEventBus}}
    EventPublisher  eventbus.EventPublisher
    {{end}}
    {{if .HasRateLimiter}}
    RateLimiter     *ratelimit.RateLimiter
    {{end}}
    IsProduction    bool
}
```

### Template: NewModule Factory
```go
// NewModule inicializa el módulo {{.ModuleName}} con todas sus dependencias.
func NewModule(cfg Config) (*Module, error) {
    m := &Module{}

    // 1. Repositories (adapters)
    m.{{.EntityName}}Repo = postgres.New{{.EntityName}}Repository(cfg.PostgresPool)

    {{if .HasCache}}
    // 2. Cache
    if cfg.DragonflyClient != nil {
        slog.Info("{{.ModuleName}}: cache enabled")
    } else {
        slog.Warn("{{.ModuleName}}: dragonfly client is nil, cache disabled")
    }
    {{end}}

    // 3. Use Cases
    {{range .Features}}
    m.{{.UseCaseName}} = {{.Package}}.NewUseCase({{.Package}}.UseCaseDeps{
        Repo:         m.{{$.EntityName}}Repo,
        {{if $.HasCache}}Cache: cfg.DragonflyClient,{{end}}
        {{if $.HasEventBus}}EventBus:      cfg.EventPublisher,{{end}}
        {{if $.HasRateLimiter}}RateLimiter:   cfg.RateLimiter,{{end}}
    })
    {{end}}

    // 4. Handlers
    {{range .Features}}
    m.{{.HandlerName}} = {{.Package}}.NewHandler(m.{{.UseCaseName}})
    {{end}}

    // 5. Error Mappers
    register{{.ModuleName | title}}Errors()

    slog.Info("{{.ModuleName}} module initialized",
        "features", []string{ {{range .Features}}"{{.Name}}",{{end}} },
    )

    return m, nil
}
```

### Template: Error Mapper Registration
```go
// register{{.ModuleName | title}}Errors registra los mapeos de errores de dominio a HTTP.
func register{{.ModuleName | title}}Errors() {
    serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
        switch {
        case errors.Is(err, domain.Err{{.EntityName}}NotFound):
            return serrors.ErrNotFound("{{.EntityVar}} no encontrado", err)
        case errors.Is(err, domain.ErrInvalidInput):
            return serrors.ErrBadRequest("datos de entrada inválidos", err)
        {{range .CustomErrors}}
        case errors.Is(err, domain.{{.}}):
            return serrors.{{.Factory}}("{{.Message}}", err)
        {{end}}
        }
        return nil // otro mapper se encarga
    })
}
```

### Template: MustNewModule (optional)
```go
// MustNewModule es como NewModule pero hace panic en caso de error.
func MustNewModule(cfg Config) *Module {
    m, err := NewModule(cfg)
    if err != nil {
        panic(fmt.Sprintf("{{.ModuleName}}: %v", err))
    }
    return m
}
```

## Uses Skills
| Skill | When |
|-------|------|
| `go` | Always loaded — Go 1.26 patterns |
| `echo` | Always loaded — Echo v5 patterns |
| `domain-core` | When generating error mappers (need domain errors) |

## Verification
```bash
# 1. Check module compiles
go build ./internal/modules/{{.ModuleName}}/... && echo "PASS: compiles" || echo "FAIL: compilation error"

# 2. Check no cross-module imports
grep -r 'modules/' internal/modules/{{.ModuleName}}/module.go | grep -v 'modules/{{.ModuleName}}' && echo "FAIL: cross-module imports" || echo "PASS"

# 3. Check error mappers registered
grep 'RegisterDomainErrorMapper' internal/modules/{{.ModuleName}}/module.go || echo "WARN: no error mappers"

# 4. Check all Config fields used
# Manual check: every Config field should appear in NewModule body

# 5. Check slog.Info on success
grep 'slog.Info.*module initialized' internal/modules/{{.ModuleName}}/module.go || echo "WARN: missing init log"

# 6. Check no init() functions
grep '^func init()' internal/modules/{{.ModuleName}}/module.go && echo "FAIL: init() found" || echo "PASS"

# 7. Check handlers exposed
grep -E 'Handler.*\*.*\.Handler' internal/modules/{{.ModuleName}}/module.go || echo "WARN: no public handlers"
```
