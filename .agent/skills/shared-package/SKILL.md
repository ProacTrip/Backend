# shared-package

SKILL: Load go
SKILL: Load go-testing

## Trigger

Create or update packages in `internal/shared/` following Proactrip conventions. Activate when user mentions:
- Creating a new shared utility package (cache, validation, encoding, pagination, errors, etc.)
- Adding a function to an existing shared package
- "Crear paquete compartido", "nuevo shared", "agregar a shared/"
- Any new `internal/shared/*/` directory creation request
- Extending `shared/errors`, `shared/http`, `shared/cache`, `shared/pagination`, `shared/context`, `shared/eventbus`, `shared/middleware`, `shared/ratelimit`, `shared/database`

## Questions to Ask (ALWAYS ask first — never generate code without answers)

1. **¿Qué problema resuelve este paquete en `internal/shared/`?**
   - Is it: cache (Dragonfly), encoding (base64, hex), HTTP middleware, error mapping RFC 7807, pagination (cursor/offset), validation, security headers, rate limiting, context utilities, event types, database connection pool, crypto utilities?
   - This determines which existing patterns to follow and which skills to load conditionally.

2. **¿Es un paquete NUEVO o estás agregando a uno EXISTENTE?**
   - NEW → create directory + `{name}.go` + `{name}_test.go`
   - EXISTING → read existing file, append function following existing conventions, add test case to `{name}_test.go`
   - Which existing package? (e.g., `internal/shared/http/cookies.go`)

3. **¿Qué funciones/structs/tipos públicos va a exponer?**
   - List exported symbols: functions, types, interfaces, constants, sentinel errors
   - What are the parameters and return types?
   - Does it need a `Config` struct? Factory function `New*()`?

4. **¿El paquete necesita dependencias externas?**
   - DragonflyDB/Redis? → needs `github.com/redis/go-redis/v9` and `miniredis` for testing
   - Echo v5? → needs `github.com/labstack/echo/v5` and `httptest` for testing
   - Crypto? → needs `crypto/rand`, `crypto/hkdf`, `encoding/hex`, etc.
   - Standard library only? → zero external deps, pure `testing` package
   - PostgreSQL pool? → needs `github.com/jackc/pgx/v5`

5. **¿Qué otros módulos van a consumir este paquete?**
   - List modules that will import from this package (e.g., `auth`, `booking`, `notification`)
   - This validates S1: the shared package imports FROM nothing in `modules/`
   - Shared packages are consumed BY modules — never the reverse

## Rules (Non-Negotiable — fail if violated)

### Specific Rules (S1-S4)

| # | Rule | Severity |
|---|------|----------|
| S1 | MUST NOT import any `modules/` package — no `internal/modules/` anywhere in imports | CRITICAL |
| S2 | Public API MUST have godoc comments on ALL exported symbols (types, functions, methods, constants, variables) | CRITICAL |
| S3 | MUST follow existing shared package conventions: RFC 7807 for errors, `pool PgxPool` for DB, `echo.MiddlewareFunc` for middleware, `context.Context` first param | MUST |
| S4 | Test files MUST use `package {name}_test` (black-box testing) — never `package {name}` for test files | MUST |

### Global Architecture Rules (R1-R9)

| # | Rule | Severity |
|---|------|----------|
| R1 | Modules communicate only via injected interfaces or published events | MUST |
| R2 | NEVER import another module's `features/` or `adapters/` | MUST NOT |
| R3 | `shared/` packages MUST NOT import from `modules/` | MUST NOT |
| R4 | Domain errors → `RegisterDomainErrorMapper()` → RFC 7807 Problem JSON | MUST |
| R5 | Manual constructor injection, zero globals, zero singletons | MUST |
| R6 | Always generate `_test.go` alongside code | MUST |
| R7 | Go 1.26 patterns: `omitzero`, `new(expr)`, `errors.AsType`, `uuid.Must(uuid.NewV7())` | MUST |
| R8 | Echo v5: `*echo.Context` pointer, `echo.StartConfig`, `echo.PathParam[T]()` | MUST |
| R9 | Adapter files named after technology (`echo.go`, `paseto.go`, `resend.go`, `blake3.go`) | MUST |

### Critical Anti-Patterns

| # | Do NOT | Do INSTEAD |
|---|--------|------------|
| A1 | Import `internal/modules/` from any shared package | Import from other `internal/shared/` packages or standard library only |
| A2 | Skip godoc on exported symbols ("it's obvious") | Always write `// FunctionName does X.` — it's enforced by `go vet` |
| A3 | Use `package {name}` for test files (white-box) | Use `package {name}_test` and import the package explicitly |
| A4 | Hardcode config values | Use `Config` struct with `DefaultConfig()` factory |
| A5 | Use `errors.New("plain string")` for domain errors | Use `"ERROR_CODE: descripción en español"` format |
| A6 | Return bare `err` without context | Use `fmt.Errorf("operation: %w", err)` wrapping |
| A7 | Use `SERIAL` or `BIGSERIAL` for DB PKs | Use `uuid PRIMARY KEY DEFAULT uuidv7()` (for migration skills, not shared packages directly) |
| A8 | Comment in English | Write package documentation and section comments in Spanish |

## Patterns

Real patterns extracted from `internal/shared/` in the Proactrip codebase.

### Pattern 1: Package Header Documentation Block

Every `.go` file in shared/ starts with a package-level comment describing what the package does. This is the `go doc` package comment.

From `internal/shared/context/context.go`:
```go
// =============================================================================
// Context Keys - Typed keys para evitar colisiones (WARN: string key collisions)
// =============================================================================

package contextutil
```

From `internal/shared/cache/dragonfly.go`:
```go
package cache

// =============================================================================
// Cliente de Dragonfly/Redis para cache y event bus
// Operaciones: strings, hashes, sets, counters con hashtags
// =============================================================================
```

From `internal/shared/pagination/cursor.go`:
```go
// Utilidades para codificar/decodificar cursores de paginación.
// Convierte offset numérico a string opaco base64 para API.
package pagination
```

**Rule**: The package doc block goes BEFORE or immediately AFTER the `package` declaration. Section separators use `// =====...=====` with 77 `=` characters per line. Comments are in Spanish.

### Pattern 2: Config Struct with Default Factory

Shared packages that need configuration follow this pattern.

From `internal/shared/cache/dragonfly.go`:
```go
// Config contiene la configuración para la conexión a Dragonfly
type Config struct {
	URL          string
	Password     string
	PoolSize     int
	MinIdleConns int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DefaultConfig crea una configuración con valores por defecto optimizados para Dragonfly
func DefaultConfig(addr, password string) *Config {
	return &Config{
		URL:          addr,
		Password:     password,
		PoolSize:     200,
		MinIdleConns: 20,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}
}
```

**Key rules**:
- `Config` struct uses exported fields with sensible zero values
- `DefaultConfig()` factory function provides optimized defaults
- Godoc comments on the struct AND the factory function
- Use `*Config` return — configs are always passed by pointer

### Pattern 3: Factory Constructor with Error Wrapping

```go
// NewDragonfly crea un nuevo cliente de Dragonfly
func NewDragonfly(cfg *Config) (*Dragonfly, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.URL,
		Password:     cfg.Password,
		DB:           0,
		Protocol:     2,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("DragonflyDB connection failed: %w", err)
	}

	return &Dragonfly{client: client}, nil
}
```

**Key rules**:
- Constructor returns `(*Type, error)` — it can fail
- Error wrapping with `fmt.Errorf("context: %w", err)` — NEVER bare `return nil, err`
- Connection verification on construction (ping, health check)
- Use `context.WithTimeout(context.Background(), duration)` for non-request-scoped operations

### Pattern 4: Public Function with Godoc

From `internal/shared/pagination/cursor.go`:
```go
// EncodeCursor encodes a numeric offset into a base64-encoded JSON cursor string.
// Returns a cursor suitable for use as a next/prev cursor in paginated responses.
func EncodeCursor(offset int) string {
	payload, _ := json.Marshal(cursorPayload{Offset: offset})
	return base64.StdEncoding.EncodeToString(payload)
}

// DecodeCursor decodes a base64-encoded JSON cursor string back into a numeric offset.
// Returns 0 and nil error on empty or malformed input (graceful first-page fallback).
func DecodeCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	raw, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return 0, nil // graceful: malformed -> first page
	}
	var payload cursorPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, nil
	}
	return payload.Offset, nil
}
```

**Key rules**:
- Godoc starts with the function name: `// FunctionName does X.`
- Second line (if needed) provides elaboration
- Return zero value + nil on graceful degradation (not error)
- Unexported types for internal payloads (`cursorPayload`)

### Pattern 5: Echo v5 Middleware Pattern

From `internal/shared/middleware/security_headers.go`:
```go
package middleware

// Middleware que agrega headers de seguridad HTTP:
// CSP, X-Frame-Options, HSTS, etc.
import "github.com/labstack/echo/v5"

func SecurityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Set("Content-Security-Policy", "default-src 'self'")
			c.Response().Header().Set("X-Content-Type-Options", "nosniff")
			c.Response().Header().Set("X-Frame-Options", "DENY")
			c.Response().Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			c.Response().Header().Set("Strict-Transport-Security", "max-age=31536000")
			return next(c)
		}
	}
}
```

**Key rules**:
- Middleware returns `echo.MiddlewareFunc`
- Receive `next echo.HandlerFunc`, return `echo.HandlerFunc`
- Inner function receives `*echo.Context` (pointer, Echo v5)
- Set headers on `c.Response().Header()`
- Always call `next(c)` (unless blocking, e.g., auth)

### Pattern 6: Type-Safe Context Key Pattern

From `internal/shared/context/context.go`:
```go
type contextKey string

const (
	TraceIDKey   contextKey = "trace_id"
	RequestIDKey contextKey = "request_id"
)

func GetTraceID(ctx context.Context) string {
	if v := ctx.Value(TraceIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
```

**Key rules**:
- Define a package-private key type (`contextKey string`)
- Constants for known keys
- Helper getter/setter functions with type assertion
- Safe zero-value return on missing key

### Pattern 7: Section Separators

All shared packages use section separator comments:

```go
// =============================================================================
// Section Name — Breve descripción
// =============================================================================
```

77 `=` characters, matches the codebase convention. Use this between logical groups of functions (CRUD operations, health checks, helpers).

### Naming Conventions

| Element | Convention | Example |
|---------|-----------|---------|
| Package name | lowercase, single word, avoid collisions with stdlib | `cache`, `pagination`, `ratelimit`, `eventbus` |
| Package name (stdlib collision) | append `util` suffix | `contextutil` (not `context`) |
| File name | `snake_case.go` describing contents | `dragonfly.go`, `error_mapper.go`, `cursor.go` |
| Config struct | `Config` (exported, always in same package) | `cache.Config`, `ratelimit.Config` |
| Factory function | `Default{Name}()` or `New{Name}()` | `DefaultConfig()`, `NewRateLimiter()` |
| Test file | `{name}_test.go` with `package {name}_test` | `cursor_test.go`, `limiter_test.go` |

## Output

| File | Template | Where |
|------|----------|-------|
| `{name}.go` | New Package | `internal/shared/{package}/` |
| `{name}_test.go` | Black-Box Test | `internal/shared/{package}/` (same dir, `package {name}_test`) |
| `{name}.go` (append) | Add Function | Existing file in `internal/shared/{package}/` |
| `{name}_test.go` (append) | Add Test Case | Existing test file, appending `func Test*()` |

## Templates

### Template 1: New Package (complete)

Use this for creating a brand new `internal/shared/{name}/` package. Replace all `{{.Placeholder}}` values before writing.

```go
// {{.PackageDescription}}
package {{.PackageName}}

// =============================================================================
// {{.SectionName}}
// =============================================================================

{{if .HasImports}}
import (
	{{range .Imports}}
	"{{.}}"
	{{end}}
)
{{end}}

{{if .HasConfig}}
// Config contiene la configuración para {{.PackageDescriptionLower}}.
type Config struct {
	{{range .ConfigFields}}
	{{.Name}} {{.Type}} // {{.Comment}}
	{{end}}
}

// DefaultConfig crea una configuración con valores por defecto.
func DefaultConfig() *Config {
	return &Config{
		{{range .ConfigDefaults}}
		{{.Name}}: {{.Value}},
		{{end}}
	}
}
{{end}}

{{if .HasStruct}}
// {{.StructName}} {{.StructDescription}}
type {{.StructName}} struct {
	{{range .StructFields}}
	{{.Name}} {{.Type}} // {{.Comment}}
	{{end}}
}
{{end}}

{{if .HasConstructor}}
// New{{.StructName}} crea un nuevo {{.StructName}}.
func New{{.StructName}}({{.ConstructorParams}}) (*{{.StructName}}, error) {
	{{.ConstructorBody}}
}
{{end}}

{{range .Functions}}
// {{.Godoc}}
func {{.Signature}} {
	{{.Body}}
}
{{end}}
```

**Placeholders**:
- `{{.PackageDescription}}` — One-line Spanish description, e.g., `Utilidades de validación para inputs de usuario.`
- `{{.PackageName}}` — lowercase single word, e.g., `validator`, `encoding`
- `{{.SectionName}}` — First logical section name, e.g., `Funciones de Validación`
- `{{.HasImports}}` — `true` if the package needs imports, `false` otherwise
- `{{.Imports}}` — Slice of import strings, e.g., `"fmt"`, `"context"`, `"github.com/redis/go-redis/v9"`
- `{{.HasConfig}}` — `true` if the package has a `Config` struct
- `{{.ConfigFields}}` — Slice of `{Name, Type, Comment}`
- `{{.ConfigDefaults}}` — Slice of `{Name, Value}`
- `{{.HasStruct}}` — `true` if the package has a main struct type
- `{{.StructName}}` — PascalCase, e.g., `Validator`, `Client`
- `{{.Functions}}` — Slice of `{Godoc, Signature, Body}` for all public functions

### Template 2: Add Function to Existing Package

Use this when adding a new public function to an existing shared package file. Read the existing file first to match patterns.

```go
// {{.FunctionName}} {{.ShortDescription}}
{{if .LongDescription}}// {{.LongDescription}}{{end}}
func {{.FunctionName}}({{.Params}}) {{.ReturnType}} {
	{{.Body}}
}
```

**Placeholders**:
- `{{.FunctionName}}` — PascalCase for exported functions
- `{{.ShortDescription}}` — One-line godoc summary
- `{{.LongDescription}}` — Optional second godoc line for elaboration
- `{{.Params}}` — Function parameters with types, e.g., `ctx context.Context, key string`
- `{{.ReturnType}}` — Return type or `(result, error)` tuple
- `{{.Body}}` — Function body respecting existing patterns in the file

**Important**: When appending to an existing file:
1. Read the file first — match indentation, error wrapping style, and section structure
2. If a relevant section separator exists, add the function inside that section
3. If not, add a new section separator before the function: `// =============================================================================`
4. Match the existing godoc comment style (full sentences ending in `.`)
5. Match existing parameter naming conventions (`ctx`, `c *echo.Context`, `cfg *Config`)

### Template 3: Black-Box Test File

Use this for creating `{name}_test.go` with `package {name}_test`. NEVER use `package {name}` for test files.

#### 3a: Standard (no external deps — pure `testing` package)

```go
package {{.PackageName}}_test

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/shared/{{.PackageName}}"
)

func Test{{.FunctionName}}(t *testing.T) {
	tests := []struct {
		name    string
		{{range .TestFields}}
		{{.Name}} {{.Type}} // {{.Comment}}
		{{end}}
		wantErr bool
	}{
		{
			name: "{{.FirstTestCaseName}}",
			{{range .FirstTestCaseFields}}
			{{.Name}}: {{.Value}},
			{{end}}
		},
		{{range .AdditionalTestCases}}
		{
			name: "{{.Name}}",
			{{range .Fields}}
			{{.Name}}: {{.Value}},
			{{end}}
		},
		{{end}}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			{{.TestBody}}
		})
	}
}
```

**Placeholders**:
- `{{.PackageName}}` — The package being tested (imported), e.g., `pagination`, `validator`
- `{{.TestFields}}` — Slice of table fields: `{Name, Type, Comment}`
- `{{.TestBody}}` — The actual assertions inside `t.Run`

#### 3b: Echo v5 Middleware Test (httptest)

```go
package {{.PackageName}}_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ProacTrip/Backend/internal/shared/{{.PackageName}}"
	"github.com/labstack/echo/v5"
)

func Test{{.FunctionName}}(t *testing.T) {
	e := echo.New()

	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, {{.PackageName}}.{{.MiddlewareName}}({{.MiddlewareArgs}}))

	req := httptest.NewRequest(http.Method{{.HTTPMethod}}, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
```

#### 3c: Dragonfly/Redis Test (miniredis)

```go
package {{.PackageName}}_test

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/shared/{{.PackageName}}"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func Test{{.FunctionName}}(t *testing.T) {
	mr := miniredis.RunT(t)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { rdb.Close() })

	{{.TestBody}}
}
```

### Template 4: Error Types (RFC 7807 Pattern)

Use this when creating a shared errors/errors-like package. Follows the exact pattern from `internal/shared/errors/errors.go`.

```go
package errors

// =============================================================================
// RFC 7807 Problem Details — Formato estándar de errores
// https://www.rfc-editor.org/rfc/rfc7807
// =============================================================================

import (
	"net/http"

	"github.com/google/uuid"
)

// ProblemType identifica categorías de errores.
type ProblemType string

const (
	{{range .ProblemTypes}}
	// {{.Description}}
	{{.Name}} ProblemType = "{{.Value}}"
	{{end}}
)

// Problem es el formato RFC 7807 Problem Details.
type Problem struct {
	Type     ProblemType `json:"type"`
	Title    string      `json:"title"`
	Status   int         `json:"status"`
	Detail   string      `json:"detail"`
	Instance string      `json:"instance,omitzero"`
	TraceID  string      `json:"trace_id,omitzero"`
	Err      error       `json:"-"`
}

// Error implementa error interface.
func (p *Problem) Error() string {
	return p.Detail
}

// Unwrap retorna el error subyacente para errors.Is/As.
func (p *Problem) Unwrap() error {
	return p.Err
}

// New crea un nuevo Problem con los campos requeridos.
func New(typ ProblemType, title, detail string, httpStatus int, err error) *Problem {
	return &Problem{
		Type:    typ,
		Title:   title,
		Status:  httpStatus,
		Detail:  detail,
		Err:     err,
		TraceID: uuid.Must(uuid.NewV7()).String(),
	}
}

// WithInstance agrega el path del endpoint.
func (p *Problem) WithInstance(path string) *Problem {
	p.Instance = path
	return p
}
```

## Uses Skills

| Skill | When |
|-------|------|
| `go` | Always — enforce Go 1.26 patterns (`new(expr)`, `errors.AsType`, `omitzero` tags) |
| `go-testing` | Always — enforce table-driven tests, standard `testing` package (no testify), `t.Cleanup()` |
| `echo` | When the shared package uses Echo v5: `*echo.Context`, `echo.MiddlewareFunc`, `echo.HandlerFunc` |
| `dragonfly` | When the shared package uses Dragonfly/Redis: `redis.Client`, `redis.Nil`, hexpire, streams |

**Conditional skill loading**:
- If the user answers "middleware" or "HTTP" to question 1 → load `echo`
- If the user answers "cache" or "rate limiting" to question 1 → load `dragonfly`
- If the user answers "crypto" or "encoding" → standard library only, no extra skills

## Verification

After generating shared package code, verify ALL of the following:

1. **Zero module imports (S1)**: Generated code MUST NOT import anything from `internal/modules/`
   ```bash
   rg 'internal/modules/' internal/shared/{{.PackageName}}/*.go && echo "FAIL: S1 violation — imports from modules/" || echo "PASS"
   ```

2. **Godoc on all exported symbols (S2)**: Every exported function, type, constant, variable must have a godoc comment
   ```bash
   # Check for exported functions without godoc
   go doc internal/shared/{{.PackageName}}
   # OR use go vet which warns on missing godoc with certain analyzers
   ```

3. **Black-box test package (S4)**: Test file MUST use `package {{.PackageName}}_test`
   ```bash
   head -1 internal/shared/{{.PackageName}}/*_test.go | rg "package {{.PackageName}}_test" || echo "FAIL: S4 violation — not black-box testing"
   ```

4. **Compilation check**: Generated code must compile
   ```bash
   go build ./internal/shared/{{.PackageName}}/...
   ```

5. **Vet check**: No warnings from `go vet`
   ```bash
   go vet ./internal/shared/{{.PackageName}}/...
   ```

6. **Convention: Section separators**: All section blocks use `// =====...=====` with 77 `=`
   ```bash
   rg '^// ={77}$' internal/shared/{{.PackageName}}/*.go | wc -l
   # Should match the number of logical sections
   ```

7. **Convention: Spanish comments**: Package doc and section headers are in Spanish
   ```bash
   rg '^// [A-Z]' internal/shared/{{.PackageName}}/*.go
   # Review output — section headers should be in Spanish
   ```

8. **Convention: Error wrapping**: No bare `return err` — must be wrapped with context
   ```bash
   rg 'return err$' internal/shared/{{.PackageName}}/*.go | rg -v '//' && echo "WARN: possible bare error return" || echo "PASS"
   ```

9. **Placeholder verification**: No unexpanded `{{.Placeholder}}` in generated files
   ```bash
   rg '{{\.' internal/shared/{{.PackageName}}/*.go && echo "FAIL: unexpanded template placeholder" || echo "PASS"
   ```

10. **Go 1.26 patterns (R7)**: Use `new(expr)` for inline pointers, `omitzero` JSON tags, `errors.AsType` (where applicable)
    ```bash
    # Check for old patterns
    rg ':= .*; &' internal/shared/{{.PackageName}}/*.go | rg -v '_test.go' && echo "WARN: use new(expr) instead of temp var" || echo "PASS"
    rg '"json:.*omitempty"' internal/shared/{{.PackageName}}/*.go && echo "WARN: consider omitzero for time.Duration/time.Time fields" || echo "PASS"
    ```
