# domain-core

## Trigger
When the user needs to define domain entities, repository interfaces (ports), domain errors, or value objects in any Proactrip module. Keywords: "crear entidad", "domain entity", "interfaz repositorio", "puerto", "domain error", "errores de dominio", "value object", "definir dominio"

## Questions to Ask (ALWAYS ask first — never generate code without answers)

1. ¿Cuál es el módulo y el nombre de la entidad? (ej: módulo `booking`, entidad `Reservation`)
2. ¿Qué campos tiene la entidad? (nombre, tipo Go, si es obligatorio, si es nullable)
3. ¿Qué métodos de comportamiento tiene? (ej: `Confirm()`, `Cancel()`, `MarkAsPaid()`)
4. ¿Qué puertos/interfaces necesita? (repository con Create/GetByID/Update, service provider)
5. ¿Qué errores de dominio puede generar? (códigos como BOOKING_NOT_FOUND, mensajes en español)

## Rules (Non-Negotiable — fail if violated)

### Domain-Specific Rules (D1-D5 from spec)
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| D1 | Entity Purity | CRITICAL | Entity structs use domain fields only — NO ORM tags (`db:`, `sql:`, `gorm:`). JSON tags are OK |
| D2 | UUIDv7 | CRITICAL | Factory functions generate IDs with `uuid.Must(uuid.NewV7())` |
| D3 | Error Format | CRITICAL | Sentinel errors use `errors.New("CODE: descripción en español")` grouped by category |
| D4 | Port Purity | CRITICAL | Port interfaces use ONLY `context.Context` and domain types. No Echo, Dragonfly, pgx types |
| D5 | Behaviors | MUST | Behavioral methods mutate state AND update `UpdatedAt` time.Time |

### Global Architecture Rules (R1-R9)
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| R1 | Module Isolation | CRITICAL | Modules communicate via injected interfaces or published events |
| R2 | No Cross-Module Imports | CRITICAL | NEVER import another module's `features/` or `adapters/` |
| R3 | Shared Boundaries | CRITICAL | `shared/` packages MUST NOT import from `modules/` |
| R4 | Error Flow | MUST | Domain errors → `RegisterDomainErrorMapper()` → RFC 7807 Problem JSON |
| R5 | DI | MUST | Manual constructor injection, zero globals, zero singletons |
| R6 | Testing | MUST | Generate `_test.go` when entity has behavior methods |
| R7 | Go 1.26 | MUST | Use `omitzero`, `new(expr)`, `errors.AsType`, `uuid.Must(uuid.NewV7())` |
| R8 | Echo v5 | MUST | *echo.Context pointer, echo.StartConfig, echo.PathParam[T]() |
| R9 | Naming | MUST | Adapter files named after technology (paseto.go, resend.go, blake3.go) |

### Additional Conventions
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| C1 | Imports | MUST | Domain packages only import `stdlib` (errors, time, context, uuid) |
| C2 | Comments | SHOULD | Comments in Spanish for entity descriptions |
| C3 | Groups | MUST | Sentinel errors grouped by comment sections: `// Errores de Usuario`, `// Errores de Validación` |

## Patterns

### Pattern 1: Entity with UUIDv7 and behaviors (from auth/domain/user.go)
```go
type User struct {
    ID           uuid.UUID
    Email        string
    Status       UserStatus
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

func NewUser(email string) *User {
    return &User{
        ID:        uuid.Must(uuid.NewV7()),
        Email:     email,
        Status:    StatusPending,
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }
}

func (u *User) VerifyEmail() {
    u.Status = StatusActive
    u.UpdatedAt = time.Now()
}
```

### Pattern 2: Port Interface (from auth/domain/repository.go)
```go
type UserRepository interface {
    Create(ctx context.Context, user *User) error
    GetByID(ctx context.Context, id uuid.UUID) (*User, error)
    GetByEmail(ctx context.Context, email string) (*User, error)
    Update(ctx context.Context, user *User) error
}
```

### Pattern 3: Domain Errors (from auth/domain/errors.go)
```go
// Errores de Usuario
var (
    ErrUserNotFound      = errors.New("USER_NOT_FOUND: el usuario no existe")
    ErrEmailAlreadyExists = errors.New("EMAIL_ALREADY_EXISTS: el email ya está registrado")
)

// Errores de Validación
var (
    ErrInvalidEmail = errors.New("INVALID_EMAIL: formato de email inválido")
    ErrWeakPassword = errors.New("WEAK_PASSWORD: la contraseña debe tener al menos 8 caracteres")
)
```

## Templates

### Template: domain/entity.go
```go
package domain

import (
    "time"
    "github.com/google/uuid"
)

// {{.EntityName}} representa {{.EntityDescription}}.
type {{.EntityName}} struct {
    ID        uuid.UUID
    // TODO: add fields
    CreatedAt time.Time
    UpdatedAt time.Time
}

// New{{.EntityName}} crea una nueva {{.EntityName}} con valores por defecto.
func New{{.EntityName}}() *{{.EntityName}} {
    return &{{.EntityName}}{
        ID:        uuid.Must(uuid.NewV7()),
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }
}
```

### Template: domain/repository.go
```go
package domain

import (
    "context"
    "github.com/google/uuid"
)

// {{.EntityName}}Repository define las operaciones de persistencia para {{.EntityName}}.
type {{.EntityName}}Repository interface {
    Create(ctx context.Context, {{.EntityVar}} *{{.EntityName}}) error
    GetByID(ctx context.Context, id uuid.UUID) (*{{.EntityName}}, error)
    Update(ctx context.Context, {{.EntityVar}} *{{.EntityName}}) error
}
```

### Template: domain/errors.go
```go
package domain

import "errors"

// Errores de {{.EntityName}}
var (
    Err{{.EntityName}}NotFound = errors.New("{{.EntityNameUpper}}_NOT_FOUND: {{.EntityVar}} no encontrado")
)

// Errores de Validación
var (
    ErrInvalidInput = errors.New("INVALID_INPUT: datos de entrada inválidos")
)
```

### Template: domain/value_object.go
```go
package domain

import (
    "errors"
    "fmt"
)

// {{.ValueObjectName}} representa {{.ValueObjectDescription}}.
type {{.ValueObjectName}} struct {
    // TODO: add fields
}

// New{{.ValueObjectName}} crea y valida un {{.ValueObjectName}}.
func New{{.ValueObjectName}}() (*{{.ValueObjectName}}, error) {
    // TODO: validate
    return &{{.ValueObjectName}}{}, nil
}
```

## Uses Skills

| Skill | When |
|-------|------|
| `go` | Always loaded — enforces Go 1.26 patterns |
| `migration-sql` | When the entity needs a database table (ask user) |
| `adapter-infra` | After defining port interfaces, to implement them |

## Verification

```bash
# 1. Check zero infrastructure imports
grep -E 'echo|pgx|redis|dragonfly' internal/modules/{{.ModuleName}}/domain/*.go && echo "FAIL: domain imports infrastructure" || echo "PASS"

# 2. Check UUIDv7 in factory
grep 'uuid.Must(uuid.NewV7())' internal/modules/{{.ModuleName}}/domain/*.go || echo "WARN: no UUIDv7 found"

# 3. Check behavioral methods update UpdatedAt
grep 'UpdatedAt.*=.*time.Now()' internal/modules/{{.ModuleName}}/domain/*.go || echo "WARN: behaviors may not update UpdatedAt"

# 4. Check error format
grep -E 'var Err.*= errors.New\("[A-Z_]+:' internal/modules/{{.ModuleName}}/domain/errors.go || echo "WARN: errors don't follow CODE: message format"

# 5. Compile check
cd internal && go build ./modules/{{.ModuleName}}/domain/... && echo "PASS: compiles"

# 6. Only stdlib imports
grep -c 'github.com/ProacTrip' internal/modules/{{.ModuleName}}/domain/*.go | grep -v ':0$' && echo "FAIL: non-stdlib imports" || echo "PASS: only stdlib"
```
