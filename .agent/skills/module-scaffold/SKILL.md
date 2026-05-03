# module-scaffold

## Trigger
When the user needs to create a completely new business domain module in Proactrip. Keywords: "nuevo módulo", "crear módulo", "new module", "scaffold module", "booking module", "payment module"

## Questions to Ask (ALWAYS ask first)

1. ¿Cuál es el nombre del nuevo módulo? (camelCase, ej: booking, payment, analytics)
2. ¿Qué entidades de dominio va a manejar? (Booking, Payment, Invoice — una principal + opcionales)
3. ¿Qué features/endpoints HTTP necesitás? (search, create, cancel, list)
4. ¿El módulo necesita consumir eventos de otros módulos? ¿O publicar eventos?
5. ¿Necesita caché? ¿Rate limiting? ¿APIs externas? ¿Autenticación propia?

## Rules (Non-Negotiable)

### Module Scaffold Rules (C1-C6 from spec)
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| C1 | Directory Structure | MUST | Create complete structure: domain/, adapters/postgres/, features/, migrations/, consumer/ (if needed) |
| C2 | Migration | MUST | Invoke migration-sql skill for initial schema |
| C3 | Domain | MUST | Invoke domain-core skill for entities and ports |
| C4 | Wiring | MUST | Invoke module-wiring skill for DI container |
| C5 | Compile | MUST | module.go compiles from the start |
| C6 | Bootstrap | SHOULD | Register module in bootstrap/app.go |

### Global Rules (R1-R9)
| Rule | Category | Severity |
|------|----------|----------|
| R1 | Module Isolation | CRITICAL — communicate via interfaces or events |
| R2 | No Cross-Module | CRITICAL — no imports from other modules |
| R3 | Shared Boundaries | CRITICAL — shared never imports from modules |
| R5 | DI | MUST — manual constructor injection |
| R6 | Testing | MUST — generate _test.go |
| R7 | Go 1.26 | MUST |
| R8 | Echo v5 | MUST |

## Orchestration Flow

When module-scaffold is invoked for "booking":

```
Step 1: Create directory structure
modules/booking/
├── domain/
├── adapters/postgres/
├── features/
├── consumer/        (si consume eventos)
└── migrations/

Step 2: Load migration-sql skill
  → genera migrations/001_initial_booking.sql
     con tabla bookings: uuidv7 PK, timestamps, triggers, CHECKs

Step 3: Load domain-core skill
  → genera domain/booking.go (entidad + factory + behaviors)
  → genera domain/repository.go (BookingRepository port)
  → genera domain/errors.go (sentinel errors)

Step 4: Load adapter-infra skill (opcional)
  → genera adapters/postgres/booking_repository.go

Step 5: Load module-wiring skill
  → genera module.go (Config + NewModule + error mappers)

Step 6: Verify
  → go build ./internal/modules/booking/...

Step 7: Bootstrap integration (guía)
  → Mostrar snippet para bootstrap/app.go
```

## Templates

### Template: Directory Structure
```
modules/{{.ModuleName}}/
├── module.go                           ← module-wiring skill
├── domain/
│   ├── {{.EntityVar}}.go               ← domain-core skill
│   ├── repository.go                   ← domain-core skill
│   └── errors.go                       ← domain-core skill
├── adapters/
│   └── postgres/
│       └── {{.EntityVar}}_repository.go ← adapter-infra skill
├── features/
│   └── (vacío — usar feature-slice skill)
├── consumer/
│   └── (vacío — usar event-consumer skill)
└── migrations/
    └── 001_initial_{{.ModuleName}}.sql  ← migration-sql skill
```

### Template: Bootstrap Integration Snippet
```go
// In bootstrap/app.go — add to NewApp():

// {{.ModuleName | title}} Module
{{.ModuleVar}}Pool, err := poolMgr.GetPool(database.DB{{.ModuleName | title}})
if err != nil {
    return nil, fmt.Errorf("{{.ModuleName}} DB pool: %w", err)
}

{{.ModuleVar}}Mod, err := {{.ModuleName}}.NewModule({{.ModuleName}}.Config{
    PostgresPool:    {{.ModuleVar}}Pool,
    {{if .HasDragonfly}}DragonflyClient: rdb,{{end}}
    {{if .HasEventBus}}EventPublisher:  eventBus,{{end}}
    {{if .HasRateLimiter}}RateLimiter:    rateLimiter,{{end}}
    IsProduction:    cfg.Server.Env == "production",
})
if err != nil {
    return nil, err
}

// Routes
{{.ModuleVar}}Group := e.Group("/v1/{{.ModuleName}}")
// {{.ModuleVar}}Group.POST("/search", {{.ModuleVar}}Mod.SearchHandler.Handle, authMiddleware.Handle)

// Add to App struct:
app := &App{
    // ...
    {{.ModuleName | title}}Module: {{.ModuleVar}}Mod,
}
```

## Uses Skills

| Skill | When |
|-------|------|
| `migration-sql` | Always — creates initial migration for entity table |
| `domain-core` | Always — creates entities, ports, errors |
| `module-wiring` | Always — creates module.go DI container |
| `adapter-infra` | When entity needs PostgreSQL repository |
| `feature-slice` | After scaffold, to add features |
| `event-consumer` | When module consumes events |

## Verification

```bash
# 1. Directory structure exists
test -d internal/modules/{{.ModuleName}}/domain && test -d internal/modules/{{.ModuleName}}/adapters && test -d internal/modules/{{.ModuleName}}/features && test -d internal/modules/{{.ModuleName}}/migrations && echo "PASS: structure" || echo "FAIL"

# 2. module.go compiles
go build ./internal/modules/{{.ModuleName}}/... && echo "PASS: compiles" || echo "FAIL"

# 3. No cross-module imports
grep -r 'modules/' internal/modules/{{.ModuleName}}/ | grep -v 'modules/{{.ModuleName}}' && echo "FAIL: cross-module import" || echo "PASS"

# 4. Migration exists
ls internal/modules/{{.ModuleName}}/migrations/*.sql && echo "PASS: migration" || echo "WARN: no migration"
```
