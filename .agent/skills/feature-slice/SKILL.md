# feature-slice

SKILL: Load go
SKILL: Load echo
SKILL: Load go-testing

## Trigger

Generate a complete vertical feature slice in a Proactrip module. Activate when user mentions:
- Adding a new feature, endpoint, or use case to an existing module
- Creating a command, handler, usecase, or response for an API endpoint
- "Crear un feature", "nuevo endpoint", "agregar handler", "command + usecase"
- Any request to write Go code under `internal/modules/{module}/features/{feature}/`
- Phrases like "vertical slice", "feature slice", "usecase pattern", "handler echo"

## Questions to Ask (ALWAYS ask first — never generate code without answers)

1. **¿En qué módulo estás trabajando y cuál es el nombre del feature?**
   - Module name (lowercase, e.g., `booking`) and feature name (snake_case, e.g., `search_flights`, `book`, `cancel`)
   - Determines package path: `internal/modules/{modulo}/features/{feature}/`

2. **¿Qué endpoint HTTP?**
   - Método HTTP: `POST`, `GET`, `PUT`, `DELETE`, `PATCH`
   - Path: e.g., `/v1/bookings/search`, `/v1/bookings/:id/cancel`
   - ¿Qué hace? (descripción funcional en español, una oración)
   - ¿Necesita `Cache-Control` header? (valores típicos: `no-store, private` para auth/login; `public, max-age=900, s-maxage=900, stale-while-revalidate=300` para search público)

3. **¿Qué campos recibe el request?** (Command struct)
   - Para cada campo: nombre (camelCase JSON), tipo Go (`string`, `int`, `*float64`, `[]string`, `*time.Time`), si es requerido, validaciones, valor por defecto
   - ¿Hay campos que el handler popula (no vienen del JSON body)? Ej: `IPAddress`, `UserAgent`
   - ¿Hay campos que requieren mapeo a un tipo de dominio diferente?

4. **¿Qué dependencias necesita el usecase?**
   - Repositorios (interfaces del dominio): e.g., `domain.BookingRepository`
   - Caché: ¿usa Dragonfly? (si es así genera el puerto local `Cache interface`)
   - Proveedores externos: e.g., `domain.FlightProvider`
   - Servicios del módulo: hasher, token service, event publisher
   - ¿Usa `sync.WaitGroup` para operaciones asíncronas (caché, historial)?

5. **¿El feature publica eventos? ¿Necesita caché? ¿Idempotencia?**
   - Eventos: ¿qué stream de Dragonfly? ¿qué tipo de evento publica después de Execute()?
   - Caché: ¿qué TTL? ¿clave de caché? ¿cache-hit modifica la respuesta?
   - Idempotencia: ¿usa `Idempotency-Key` header? ¿checkea operaciones duplicadas?

## Rules (Non-Negotiable — fail if violated)

### Feature-Slice Specific Rules (F1-F5)

| # | Rule | Severity |
|---|------|----------|
| F1 | `command.go` MUST have `Validate() error` method using domain sentinel errors (`fmt.Errorf("%w: detail", domain.ErrXxx)`) | CRITICAL |
| F2 | `handler.go` MUST use `Handle(c *echo.Context) error` with bind → usecase → `httperr.MapError(c, err)` → `c.JSON(200, resp)` pattern | CRITICAL |
| F3 | `usecase.go` MUST define local `Cache interface` port (when caching) and `UseCaseDeps` struct | CRITICAL |
| F4 | `Execute(ctx context.Context, cmd Command) (*Response, error)` MUST be the canonical usecase method signature | CRITICAL |
| F5 | `response.go` MUST use type alias `type Response = domain.XxxResponse` when the domain type already has JSON tags | SHOULD |

### Handler Rules (from real patterns)

| # | Rule | Severity |
|---|------|----------|
| H1 | Handler sets `Cache-Control` header BEFORE calling usecase | MUST |
| H2 | Handler calls `c.Bind(&cmd)` → checks error → `httperr.MapError(c, err)` | MUST |
| H3 | Handler calls `h.usecase.Execute(c.Request().Context(), cmd)` — never `context.Background()` | MUST |
| H4 | Handler logs errors via `slog.ErrorContext(c.Request().Context(), "feature failed", ...)` before `MapError` | SHOULD |
| H5 | Handler NEVER contains business logic — it is a thin translation layer | MUST |

### UseCase Rules (from real patterns)

| # | Rule | Severity |
|---|------|----------|
| U1 | `Execute()` MUST call `cmd.Validate()` as its first operation | MUST |
| U2 | Async cache writes MUST use `wg.Go(func(){...})` with `context.WithoutCancel(ctx)` | MUST |
| U3 | `UseCase` struct MUST expose `Wait()` method for graceful shutdown (`uc.wg.Wait()`) | MUST |
| U4 | Local port interfaces (Cache, Repository) MUST be defined in usecase.go — never import another module's concrete adapter | CRITICAL |
| U5 | `UseCaseDeps` struct MUST use domain interfaces and local ports — never concrete types | MUST |
| U6 | Error wrapping: `fmt.Errorf("context: %w", err)` — always wrap, never return bare errors | MUST |

### Command Rules (from real patterns)

| # | Rule | Severity |
|---|------|----------|
| C1 | `Validate()` applies defaults for zero-values BEFORE validation | MUST |
| C2 | `Validate()` returns `domain.ErrXxx` wrapped with `fmt.Errorf` — never bare sentinel | MUST |
| C3 | If the command maps to a domain type, define `ToDomain() domain.XxxRequest` method | SHOULD |
| C4 | Constants for valid enum values MUST be defined as `const` block with `validXxx` map | SHOULD |
| C5 | Fields populated by handler (not from JSON) MUST use `json:"-"` tag | MUST |

### Global Architecture Rules (R1-R9)

| # | Rule | Severity |
|---|------|----------|
| R1 | Modules communicate only via injected interfaces or published events | CRITICAL |
| R2 | NEVER import another module's `features/` or `adapters/` | CRITICAL |
| R3 | `shared/` packages MUST NOT import from `modules/` | CRITICAL |
| R4 | Domain errors → `RegisterDomainErrorMapper()` → RFC 9457 Problem JSON | MUST |
| R5 | Manual constructor injection, zero globals, zero singletons | MUST |
| R6 | Always generate `_test.go` alongside code | MUST |
| R7 | Go 1.26 patterns: `omitzero`, `new(expr)`, `errors.AsType`, `uuid.Must(uuid.NewV7())` | MUST |
| R8 | Echo v5: `*echo.Context` pointer, `echo.StartConfig`, `echo.PathParam[T]()` | MUST |
| R9 | Adapter files named after technology (`echo.go`, `paseto.go`, `resend.go`, `blake3.go`) | MUST |

### Critical Anti-Patterns

| # | Do NOT | Do INSTEAD |
|---|--------|------------|
| A1 | Put business logic in the handler | Handler only binds, calls usecase, maps errors, returns JSON |
| A2 | Use `context.Background()` in handlers or usecases | Use `c.Request().Context()` (handler) or `ctx context.Context` (usecase) |
| A3 | Import concrete adapter types in usecase | Define local port interfaces in usecase.go |
| A4 | Return bare `err` from usecase | `fmt.Errorf("operation: %w", err)` — always wrap |
| A5 | Hardcode HTTP status in usecase | Usecase returns domain errors; handler maps them via `httperr.MapError` |
| A6 | Use `echo.NewHTTPError` directly in handlers | Use `domain.ErrXxx` sentinel + `httperr.MapError(c, err)` — all errors flow through the error mapper |
| A7 | Use `go func()` for fire-and-forget | Use `uc.wg.Go(func(){...})` with `context.WithoutCancel(ctx)` |
| A8 | Skip `Validate()` call in `Execute()` | Always validate command as first step |
| A9 | Define response struct manually when domain has JSON tags | Use type alias `type Response = domain.XxxResponse` |
| A10 | Use `time.Sleep` in async cache tests | Use `synctest` for deterministic concurrent testing |
| A11 | Use `omitempty` for `time.Time`/`time.Duration`/pointer fields | Use `omitzero` (Go 1.24+) |
| A12 | Return user data without `id` field | Always include `"id"` in user response — frontend needs it for profile/bookings |

## Patterns

Real patterns extracted from the Proactrip codebase.

### Pattern 1: Thin Handler (login — auth module)

From `internal/modules/auth/features/login/handler.go`:

```go
package {{.FeatureName}}

import (
	"net/http"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

type Handler struct {
	usecase      *UseCase
	{{if .HasProductionFlag}}isProduction bool{{end}}
}

func NewHandler(usecase *UseCase{{if .HasProductionFlag}}, isProduction bool{{end}}) *Handler {
	return &Handler{
		usecase:      usecase,
		{{if .HasProductionFlag}}isProduction: isProduction,{{end}}
	}
}

func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "{{.CacheControl}}")

	var cmd Command
	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	{{if .HasPostExecute}}// Post-execute: set cookies, publish events, etc.{{end}}

	return c.JSON(http.StatusOK, resp)
}
```

Key observations from the real code:
- Handler struct only has `usecase` pointer (plus optional `isProduction` flag)
- `Cache-Control` header set BEFORE binding — always
- `c.Bind(&cmd)` returns `error` — check immediately
- `httperr.MapError(c, err)` handles both binding errors and domain errors uniformly
- `c.Request().Context()` — NEVER `context.Background()` in a handler

### Pattern 2: Handler with Defaults + Logging (search_flights)

From `internal/modules/search/features/search_flights/handler.go`:

```go
func (h *Handler) Handle(c *echo.Context) error {
	var cmd Command

	// Set defaults before binding so they act as fallbacks
	cmd.Adults = 1
	cmd.TravelClass = "economy"
	cmd.Currency = "USD"
	cmd.SortBy = "top"
	cmd.Stops = "any"

	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "search_flights failed",
			slog.String("error", err.Error()),
			slog.String("trip_type", cmd.TripType),
		)
		return httperr.MapError(c, err)
	}

	c.Response().Header().Set("Cache-Control", "public, max-age=900, s-maxage=900, stale-while-revalidate=300")
	return c.JSON(http.StatusOK, resp)
}
```

Key observations:
- Defaults set as struct field assignments BEFORE `c.Bind()` — bind overwrites them if the field is present in JSON
- `slog.ErrorContext` logs structured fields before mapping error — aids debugging
- `Cache-Control` header set AFTER execute but before JSON — doesn't matter for correctness, but convention in this codebase

### Pattern 3: Command with Validate + ToDomain (search_flights)

From `internal/modules/search/features/search_flights/command.go`:

```go
package {{.FeatureName}}

import (
	"fmt"

	"{{.ModulePath}}/domain"
)

const (
	{{.EnumConstants}}
)

var valid{{.EnumMapName}} = map[string]bool{
	{{.EnumMapEntries}}
}

type Command struct {
	{{range .Fields}}{{.Name}} {{.Type}} `json:"{{.JSONTag}}"{{if .Omitzero}},omitzero{{end}}`
	{{end}}

	// Request metadata (populated by the handler, not via JSON binding).
	IPAddress string `json:"-"`
	UserAgent string `json:"-"`
}

func (cmd *Command) Validate() error {
	{{if .HasDefaults}}// Apply defaults
	{{.DefaultsCode}}{{end}}

	{{.ValidationCode}}

	return nil
}

func (cmd *Command) ToDomain() domain.{{.DomainRequest}} {
	return domain.{{.DomainRequest}}{
		{{range .DomainMapping}}{{.DomainField}}: cmd.{{.CmdField}},
		{{end}}
	}
}
```

Key observations:
- Constants for valid enum values defined as a `const` block + `var validXxx map[string]bool`
- `Validate()` applies defaults first, then validates — zero values become defaults
- `Validate()` returns `fmt.Errorf("%w: detail", domain.ErrInvalidXxx)` — wrapped sentinel, never bare
- `ToDomain()` method maps the command to a domain request struct (when domain type exists)
- Fields populated by handler use `json:"-"` tag — they are NOT parsed from request body

### Pattern 4: UseCase with Cache + Async Write (search_flights)

From `internal/modules/search/features/search_flights/usecase.go`:

```go
package {{.FeatureName}}

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"{{.ModulePath}}/domain"
)

// =============================================================================
// Puerto de Cache (local port — never import adapter package)
// =============================================================================

type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

// =============================================================================
// UseCase
// =============================================================================

type UseCase struct {
	{{range .Deps}}{{.Field}} {{.Type}}
	{{end}}
	{{if .HasCache}}cacheTTL time.Duration{{end}}
	wg       sync.WaitGroup
}

type UseCaseDeps struct {
	{{range .Deps}}{{.Field}} {{.Type}}
	{{end}}
	{{if .HasCache}}CacheTTL time.Duration{{end}}
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		{{range .Deps}}{{.Field}}: deps.{{.Field}},
		{{end}}
		{{if .HasCache}}cacheTTL: deps.CacheTTL,{{end}}
	}
}

func (uc *UseCase) Wait() {
	uc.wg.Wait()
}

func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	start := time.Now()

	// 1. Validate
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	{{if .HasToDomain}}// 2. Convert to domain request
	domainReq := cmd.ToDomain(){{end}}

	{{if .HasCache}}// 3. Try cache
	cacheKey := generateCacheKey(domainReq)

	if cached, err := uc.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var resp Response
		if err := json.Unmarshal([]byte(cached), &resp); err == nil {
			resp.FromCache = true
			resp.CachedAt = new(time.Now())

			return &resp, nil
		}
		slog.WarnContext(ctx, "cache unmarshal failed",
			slog.String("key", cacheKey),
			slog.Any("err", err),
		)
	}{{end}}

	{{if .HasProvider}}// 4. Call external provider/service
	resp, err := uc.provider.Search(ctx, domainReq)
	if err != nil {
		return nil, fmt.Errorf("{{.OperationName}}: %w", err)
	}{{end}}

	{{if .HasCache}}// 5. Cache response async — fire-and-forget with wg.Go
	if data, err := json.Marshal(resp); err == nil {
		bgCtx := context.WithoutCancel(ctx)
		uc.wg.Go(func() {
			if err := uc.cache.Set(bgCtx, cacheKey, string(data), uc.cacheTTL); err != nil {
				slog.ErrorContext(bgCtx, "cache set failed",
					slog.String("key", cacheKey),
					slog.Any("err", err),
				)
			}
		})
	}{{end}}

	return resp, nil
}
```

Key observations:
- `Cache interface` is a LOCAL port in the usecase file — never imports a concrete adapter
- `UseCaseDeps` bundles ALL dependencies including config values (TTL)
- `NewUseCase` copies every field individually — no `deps` passthrough (manual injection)
- `Execute()` calls `cmd.Validate()` as first step — always
- Async cache writes use `uc.wg.Go(func(){...})` with `context.WithoutCancel(ctx)`
- `uc.Wait()` exposed for graceful shutdown — allows in-flight cache writes to complete
- Error wrapping: `fmt.Errorf("context: %w", err)` — never bare errors

### Pattern 5: Simple UseCase (login — no cache)

From `internal/modules/auth/features/login/usecase.go`:

```go
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	// Validate inline (no cmd.Validate() in this module)
	if !strings.Contains(cmd.Email, "@") {
		return nil, domain.ErrInvalidEmail
	}

	// Call dependencies sequentially
	user, err := uc.repo.GetByEmail(ctx, cmd.Email)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	if !user.EmailVerified {
		return nil, domain.ErrEmailNotVerified
	}

	// ... business logic with entity methods ...
	user.RecordLogin()
	if updateErr := uc.repo.Update(ctx, user); updateErr != nil {
		slog.ErrorContext(ctx, "failed to record successful login",
			slog.String("email", cmd.Email),
			slog.Any("error", updateErr),
		)
	}

	return &Response{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		User: &UserResponse{
			Email:    user.Email,
			RoleName: user.RoleName,
		},
	}, nil
}
```

Key observations:
- Some features validate inline rather than in a separate `Validate()` method — the spec (F1) requires `Validate()` though
- `slog.ErrorContext` used for non-critical failures (best-effort updates)
- Response built directly with struct literal — clean, explicit

### Pattern 6: Type Alias Response (search_flights)

From `internal/modules/search/features/search_flights/response.go`:

```go
package search_flights

import "github.com/ProacTrip/Backend/internal/modules/search/domain"

// Response is the search flights API response.
// Uses type alias to re-export the domain response directly
// since domain types already have JSON tags.
type Response = domain.FlightSearchResponse
```

Key observations:
- When the domain response type already has JSON tags, use a TYPE ALIAS (not a new struct)
- The import path points to the domain package of the SAME module
- Comment explains WHY the alias is used — "domain types already have JSON tags"

### Pattern 7: Inline Response Struct (login — no domain type available)

From `internal/modules/auth/features/login/command.go`:

```go
type Response struct {
	AccessToken  string        `json:"-"`
	RefreshToken string        `json:"-"`
	User         *UserResponse `json:"user"`
}

type UserResponse struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	RoleName      string `json:"role_name"`
}
```

Key observations:
- Used when no domain response type exists with JSON tags
- Sensitive fields (tokens) use `json:"-"` — never serialized in response body
- Spanish comments throughout

### Pattern 8: Local Port Interfaces

From both login and search_flights usecases:

```go
// PasswordService is a local port — defined in the usecase, NOT imported from adapters.
type PasswordService interface {
	Verify(password, encoded string) (bool, error)
}

// TokenService is a local port.
type TokenService interface {
	GenerateTokenPair(userID uuid.UUID, email string, roleID, sessionID uuid.UUID) (*token.TokenPair, error)
}
```

Key observations:
- Local ports use ONLY types from the standard library and domain packages
- The `token.TokenPair` is from the same module's domain — NOT from another module
- Interface methods are minimal — only the methods this usecase needs (Interface Segregation Principle)

### Spanish Comment Convention

All Proactrip code uses Spanish for comments:

```go
// Lógica de negocio para búsqueda de vuelos.
// Orquesta cache, proveedor externo e historial de búsquedas.
```

```go
// Handler HTTP para login.
// Valida credenciales y setea cookies de sesión.
```

## Output

| File | Template | Where |
|------|----------|-------|
| `command.go` | Command + Validate + ToDomain | `internal/modules/{{.Module}}/features/{{.Feature}}/` |
| `handler.go` | Handler + Handle | Same directory |
| `usecase.go` | UseCase + Execute + local ports + UseCaseDeps | Same directory |
| `response.go` | Type alias or inline struct | Same directory |
| `usecase_test.go` | Mock structs + table-driven tests | Same directory |

All five files go in the SAME package (named after the feature, snake_case). Never split across different packages.

## Templates

### Template 1: command.go

```go
// DTO de entrada para {{.FeatureDescription}}.
// Valida parámetros{{if .HasToDomain}} y mapea a dominio{{end}}.
package {{.FeatureName}}

import (
	"fmt"

	"{{.ModulePath}}/domain"
)

// =============================================================================
// Constantes
// =============================================================================

const (
	{{.EnumConstantsBlock}}
)

var valid{{.EnumMapName}} = map[string]bool{
	{{.EnumMapEntries}}
}

// =============================================================================
// Command
// =============================================================================

// Command es el DTO de entrada para {{.FeatureDescription}}.
type Command struct {
	{{range .Fields}}{{.Name}} {{.Type}} `json:"{{.JSONTag}}"{{if .Omitzero}},omitzero{{end}}`
	{{end}}

	{{if .HasRequestMetadata}}
	// Metadatos del request (poblados por el handler, no via JSON).
	IPAddress string `json:"-"`
	UserAgent string `json:"-"`
	{{end}}
}

const (
	{{.DefaultConstants}}
)

// =============================================================================
// Validación
// =============================================================================

func (cmd *Command) Validate() error {
	{{if .HasDefaults}}
	// Aplicar defaults
	{{.DefaultsCode}}
	{{end}}

	// Validar campos requeridos
	{{.ValidationCode}}

	return nil
}

{{if .HasToDomain}}
// =============================================================================
// Mapeo a Dominio
// =============================================================================

func (cmd *Command) ToDomain() domain.{{.DomainRequest}} {
	return domain.{{.DomainRequest}}{
		{{range .DomainMapping}}{{.DomainField}}: cmd.{{.CmdField}},
		{{end}}
	}
}
{{end}}
```

**Placeholders**:
- `{{.FeatureName}}` — snake_case package name, e.g., `search_flights`, `book`
- `{{.FeatureDescription}}` — Spanish description, e.g., `crear una reserva de vuelo`
- `{{.ModulePath}}` — full Go module import path, e.g., `github.com/ProacTrip/Backend/internal/modules/booking`
- `{{.EnumConstantsBlock}}` — Enum constants, e.g.:
  ```go
  StatusPending   = "pending"
  StatusConfirmed = "confirmed"
  StatusCancelled = "cancelled"
  ```
- `{{.EnumMapName}}` — PascalCase name for valid enum map, e.g., `Statuses`
- `{{.EnumMapEntries}}` — Map entries, e.g.: `StatusPending: true, StatusConfirmed: true,`
- `{{.Fields}}` — Iterate over fields with `.Name` (PascalCase), `.Type` (Go type), `.JSONTag` (snake_case), `.Omitzero` (bool)
- `{{.HasRequestMetadata}}` — bool: true if IPAddress/UserAgent fields needed
- `{{.DefaultConstants}}` — Default values as constants, e.g.: `DefaultLimit = 10`, `MaxLimit = 100`
- `{{.DefaultsCode}}` — Default application logic, e.g.: `if cmd.Limit == 0 { cmd.Limit = DefaultLimit }`
- `{{.ValidationCode}}` — Validation logic using `fmt.Errorf("%w: detail", domain.ErrXxx)`
- `{{.DomainRequest}}` — Domain request type name, e.g., `BookFlightRequest`
- `{{.DomainMapping}}` — Field mapping pairs (`.DomainField` → `.CmdField`)

### Template 2: handler.go

```go
// Handler HTTP para {{.FeatureDescription}}.
// Expuesto en {{.HTTPMethod}} {{.HTTPPath}}.
package {{.FeatureName}}

import (
	"log/slog"
	"net/http"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// =============================================================================
// Handler — endpoint HTTP de {{.FeatureName}}
// =============================================================================

// Handler procesa las peticiones HTTP de {{.FeatureDescription}}.
type Handler struct {
	usecase *UseCase
	{{if .HasExtraFields}}{{range .ExtraFields}}{{.Name}} {{.Type}}
	{{end}}{{end}}
}

// NewHandler crea un nuevo handler para {{.FeatureDescription}}.
func NewHandler(usecase *UseCase{{if .HasExtraFields}}{{range .ExtraFields}}, {{.Name}} {{.Type}}{{end}}{{end}}) *Handler {
	return &Handler{
		usecase: usecase,
		{{if .HasExtraFields}}{{range .ExtraFields}}{{.Name}}: {{.Name}},
		{{end}}{{end}}
	}
}

// Handle procesa la petición.
// Route: {{.HTTPMethod}} {{.HTTPPath}}
func (h *Handler) Handle(c *echo.Context) error {
	{{if .HasDefaultsBeforeBind}}var cmd Command

	// Setear defaults antes del binding para que actúen como fallback
	{{.DefaultsBeforeBind}}
	{{else}}var cmd Command
	{{end}}

	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		{{if .HasErrorLogging}}slog.ErrorContext(c.Request().Context(), "{{.FeatureName}} failed",
			slog.String("error", err.Error()),
		)
		{{end}}return httperr.MapError(c, err)
	}

	{{if .HasPostExecute}}{{.PostExecuteCode}}
	{{end}}
	c.Response().Header().Set("Cache-Control", "{{.CacheControl}}")
	return c.JSON(http.StatusOK, resp)
}
```

**Placeholders**:
- `{{.HTTPMethod}}` — `POST`, `GET`, `PUT`, `DELETE`, `PATCH`
- `{{.HTTPPath}}` — e.g., `/v1/bookings/search`, `/v1/bookings/:id/cancel`
- `{{.CacheControl}}` — e.g., `no-store, private` for auth, `public, max-age=900, s-maxage=900, stale-while-revalidate=300` for search
- `{{.HasDefaultsBeforeBind}}` — bool: handler sets default values before binding
- `{{.DefaultsBeforeBind}}` — Default field assignments, e.g., `cmd.Currency = "USD"`
- `{{.HasErrorLogging}}` — bool: whether to log on error (SHOULD for search, optional for auth)
- `{{.HasPostExecute}}` — bool: e.g., set cookies, publish event
- `{{.PostExecuteCode}}` — Code after successful Execute(), e.g., cookie setting
- `{{.HasExtraFields}}` — bool: handler has extra fields beyond usecase (e.g., isProduction flag)
- `{{.ExtraFields}}` — List of extra fields with `.Name` and `.Type`

### Template 3: usecase.go

```go
// Lógica de negocio para {{.FeatureDescription}}.
// {{.UsecaseFlowDescription}}.
package {{.FeatureName}}

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"{{.ModulePath}}/domain"
)

// =============================================================================
// Puertos Locales
// =============================================================================

{{if .HasCache}}// Cache es el puerto local para operaciones de caché.
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}
{{end}}
{{range .LocalPorts}}// {{.Name}} es el puerto local para {{.Description}}.
type {{.Name}} interface {
	{{range .Methods}}{{.Signature}}
	{{end}}
}
{{end}}

// =============================================================================
// UseCase
// =============================================================================

// UseCase orquesta {{.FeatureDescription}}.
type UseCase struct {
	{{range .Deps}}{{.Field}} {{.Type}}
	{{end}}
	{{if .HasCacheTTL}}cacheTTL time.Duration{{end}}
	wg       sync.WaitGroup
}

// UseCaseDeps agrupa las dependencias del caso de uso.
type UseCaseDeps struct {
	{{range .Deps}}{{.Field}} {{.Type}}
	{{end}}
	{{if .HasCacheTTL}}CacheTTL time.Duration{{end}}
}

// NewUseCase crea un nuevo caso de uso para {{.FeatureDescription}}.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		{{range .Deps}}{{.Field}}: deps.{{.Field}},
		{{end}}
		{{if .HasCacheTTL}}cacheTTL: deps.CacheTTL,{{end}}
	}
}

// Wait bloquea hasta que todas las goroutines fire-and-forget hayan terminado.
// Llamar durante graceful shutdown para evitar perder operaciones en vuelo.
func (uc *UseCase) Wait() {
	uc.wg.Wait()
}

// =============================================================================
// Ejecución Principal
// =============================================================================

// Execute ejecuta {{.FeatureDescription}}.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	start := time.Now()

	// 1. Validar comando
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	{{if .HasToDomain}}// 2. Convertir a dominio
	domainReq := cmd.ToDomain()
	{{end}}

	{{if .HasCache}}// 3. Intentar caché
	cacheKey := generateCacheKey({{.CacheKeyInput}})

	if cached, err := uc.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var resp Response
		if err := json.Unmarshal([]byte(cached), &resp); err == nil {
			resp.FromCache = true
			resp.CachedAt = new(time.Now())

			{{if .HasCacheHitSideEffects}}{{.CacheHitSideEffectsCode}}
			{{end}}
			return &resp, nil
		}
		slog.WarnContext(ctx, "cache unmarshal failed",
			slog.String("key", cacheKey),
			slog.Any("err", err),
		)
	}
	{{end}}

	{{if .HasBusinessLogic}}// 4. Ejecutar lógica de negocio
	{{.BusinessLogicCode}}
	{{end}}

	{{if .HasCache}}// 5. Guardar en caché — fire-and-forget con wg.Go
	if data, err := json.Marshal(resp); err == nil {
		bgCtx := context.WithoutCancel(ctx)
		uc.wg.Go(func() {
			if err := uc.cache.Set(bgCtx, cacheKey, string(data), uc.cacheTTL); err != nil {
				slog.ErrorContext(bgCtx, "cache set failed",
					slog.String("key", cacheKey),
					slog.Any("err", err),
				)
			}
		})
	}
	{{end}}

	{{if .HasAsyncSideEffects}}// 6. Efectos secundarios asíncronos — fire-and-forget
	{{.AsyncSideEffectsCode}}
	{{end}}

	return resp, nil
}

{{if .HasCache}}
// =============================================================================
// Generación de Clave de Caché
// =============================================================================

func generateCacheKey({{.CacheKeyParams}}) string {
	{{.CacheKeyCode}}
}
{{end}}
```

**Placeholders**:
- `{{.UsecaseFlowDescription}}` — Spanish description of orchestration flow, e.g., `Orquesta caché, proveedor externo e historial de búsquedas.`
- `{{.HasCache}}` — bool: usecase has caching
- `{{.HasCacheTTL}}` — bool: usecase needs CacheTTL field
- `{{.LocalPorts}}` — Additional local port interfaces beyond Cache (e.g., PasswordService, TokenService)
- `{{.Deps}}` — List of dependencies with `.Field` (camelCase) and `.Type` (Go type) — these become UseCase struct fields AND UseCaseDeps fields
- `{{.HasToDomain}}` — bool: Command has ToDomain() method
- `{{.CacheKeyInput}}` — Input to generateCacheKey, e.g., `domainReq`
- `{{.HasCacheHitSideEffects}}` — bool: has side effects on cache hit (e.g., save search history)
- `{{.CacheHitSideEffectsCode}}` — Code that runs on cache hit
- `{{.HasBusinessLogic}}` — bool: has business logic beyond cache/provider
- `{{.BusinessLogicCode}}` — The core business logic code
- `{{.HasAsyncSideEffects}}` — bool: has async side effects (e.g., save history, publish event)
- `{{.AsyncSideEffectsCode}}` — Async side effect code using `uc.wg.Go(func(){...})` + `context.WithoutCancel(ctx)`
- `{{.CacheKeyParams}}` — Function parameters
- `{{.CacheKeyCode}}` — Cache key generation logic

### Template 4: response.go

**Option A: Type alias (when domain has JSON tags — PREFERRED)**

```go
// DTO de respuesta para {{.FeatureDescription}}.
// Re-exporta el tipo de dominio.
package {{.FeatureName}}

import "{{.ModulePath}}/domain"

// Response es la respuesta de {{.FeatureDescription}}.
// Usa type alias para re-exportar la respuesta de dominio directamente
// ya que los tipos de dominio ya tienen JSON tags.
type Response = domain.{{.DomainResponse}}
```

**Option B: Inline struct (when domain has no JSON tags or needs transformation)**

```go
// DTO de respuesta para {{.FeatureDescription}}.
package {{.FeatureName}}

import "time"

// Response es la respuesta de {{.FeatureDescription}}.
type Response struct {
	{{range .ResponseFields}}{{.Name}} {{.Type}} `json:"{{.JSONTag}}"{{if .Omitzero}},omitzero{{end}}`
	{{end}}
	{{if .HasFromCache}}FromCache bool       `json:"from_cache"`
	CachedAt  *time.Time `json:"cached_at,omitzero"`{{end}}
}
```

**Placeholders**:
- `{{.DomainResponse}}` — Domain response type, e.g., `FlightSearchResponse`
- `{{.ResponseFields}}` — For inline struct: list of `.Name`, `.Type`, `.JSONTag`, `.Omitzero`
- `{{.HasFromCache}}` — bool: response includes cache metadata

### Template 5: usecase_test.go

```go
// Tests para el caso de uso {{.FeatureDescription}}.
package {{.FeatureName}}

import (
	"context"
	"errors"
	"testing"
	"time"

	"{{.ModulePath}}/domain"
)

// =============================================================================
// Mocks
// =============================================================================

{{range .MockStructs}}
type {{.Name}} struct {
	{{range .Methods}}{{.MockField}} {{.MockSignature}}
	{{end}}
}

{{range .Methods}}
func (m *{{$.Name}}) {{.Signature}} {
	{{if .ReturnsError}}if m.{{.MockField}} != nil {
		return nil, m.{{.MockField}}.Error
	}
	return {{.ReturnValues}}, nil{{else}}return m.{{.MockField}}{{end}}
}
{{end}}
{{end}}

// =============================================================================
// Tests
// =============================================================================

func Test{{.UseCaseName}}_Execute(t *testing.T) {
	tests := []struct {
		name      string
		cmd       Command
		mockSetup func({{range .MockSetupParams}}{{.Name}} *{{.Type}}, {{end}})
		wantErr   bool
		wantResp  *Response
	}{
		{{range .TestCases}}
		{
			name: "{{.Name}}",
			cmd: Command{
				{{range .CmdFields}}{{.Name}}: {{.Value}},
				{{end}}
			},
			mockSetup: func({{range $.MockSetupParams}}{{.Name}} *{{.Type}}, {{end}}) {
				{{.SetupCode}}
			},
			wantErr:  {{.WantErr}},
			{{if .WantResp}}wantResp: &Response{
				{{range .WantRespFields}}{{.Name}}: {{.Value}},
				{{end}}
			},{{end}}
		},
		{{end}}
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()

			// Setup mocks
			{{range .MockSetupParams}}{{.Name}} := &{{.Type}}{}
			{{end}}

			if tc.mockSetup != nil {
				tc.mockSetup({{range .MockSetupParams}}{{.Name}}, {{end}})
			}

			// Create usecase with mocks
			uc := NewUseCase(UseCaseDeps{
				{{range .MockDepsMapping}}{{.DepsField}}: {{.MockVar}},
				{{end}}
			})

			// Execute
			resp, err := uc.Execute(ctx, tc.cmd)

			// Assert
			if (err != nil) != tc.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			{{if .HasResponseAssertions}}
			if !tc.wantErr && tc.wantResp != nil {
				{{.ResponseAssertions}}
			}
			{{end}}
		})
	}
}

{{if .HasConcurrentTests}}
// =============================================================================
// Tests Concurrentes (synctest)
// =============================================================================

func Test{{.UseCaseName}}_Concurrent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Setup mocks with async behavior
		{{.ConcurrentTestCode}}
	})
}
{{end}}
```

**Placeholders**:
- `{{.MockStructs}}` — One per dependency interface. Each has `.Name` (e.g., `mockRepo`), `.Methods` list
  - Each method has `.MockField` (e.g., `GetByIDFunc`), `.MockSignature` (full signature including return types), `.Signature`, `.ReturnsError` (bool), `.ReturnValues`
- `{{.UseCaseName}}` — PascalCase, e.g., `BookFlight`
- `{{.TestCases}}` — Table-driven test entries:
  - Minimum 2: happy path + one error case
  - Each has `.Name`, `.CmdFields`, `.SetupCode` (mock expectations), `.WantErr`, `.WantResp`, `.WantRespFields`
- `{{.MockSetupParams}}` — e.g., `repo *mockRepo, cache *mockCache`
- `{{.MockDepsMapping}}` — e.g., `.DepsField: Repo, .MockVar: repo`
- `{{.HasResponseAssertions}}` — bool: validate response fields
- `{{.ResponseAssertions}}` — Assertions on response fields
- `{{.HasConcurrentTests}}` — bool: usecase has async operations requiring synctest
- `{{.ConcurrentTestCode}}` — synctest-based test for concurrent operations

## Uses Skills

| Skill | When |
|-------|------|
| `domain-core` | When reading domain interfaces to construct UseCaseDeps with the correct types |
| `dragonfly` | When the feature uses Dragonfly cache (detected by Cache interface in UseCaseDeps) |
| `testing` | When generating usecase_test.go with mocks and table-driven tests |

`feature-slice` does NOT load these skills directly — it documents them in `## Uses Skills`. The orchestrator or agent that uses this skill is responsible for loading them when needed.

## Verification

After generating files, verify ALL of the following:

1. **Compilation check**: All generated files must compile within the module
   ```bash
   go build ./internal/modules/{{.Module}}/features/{{.Feature}}/...
   ```

2. **Vet check**: No suspicious constructs
   ```bash
   go vet ./internal/modules/{{.Module}}/features/{{.Feature}}/...
   ```

3. **command.go — Validate exists**: Command struct has `Validate() error` method
   ```bash
   rg "func \(cmd \*Command\) Validate\(\) error" command.go
   ```

4. **handler.go — Handle signature correct**: Uses `*echo.Context` pointer
   ```bash
   rg "func \(h \*Handler\) Handle\(c \*echo\.Context\) error" handler.go
   ```

5. **handler.go — No business logic**: Handler doesn't call domain methods or external services directly
   ```bash
   # Handler should only call Bind, Execute, MapError, JSON
   rg -v "(c\.Bind|usecase\.Execute|httperr\.MapError|c\.JSON|Header|Set)" handler.go | rg "(domain\.|provider\.|repo\.)"
   # Should return NO matches
   ```

6. **usecase.go — local ports**: Cache interface (if present) defined in usecase.go, not imported
   ```bash
   rg "type Cache interface" usecase.go
   ```

7. **usecase.go — Wait() method**: Exists for graceful shutdown
   ```bash
   rg "func \(uc \*UseCase\) Wait\(\)" usecase.go
   ```

8. **usecase.go — wg.Go for async**: No bare `go func()`, always `uc.wg.Go(func(){...})`
   ```bash
   rg "go func" usecase.go && echo "FAIL: bare goroutine" || echo "PASS: uses wg.Go"
   ```

9. **response.go — Type alias or inline**: Response type is either alias or struct with JSON tags
   ```bash
   rg "type Response" response.go
   ```

10. **usecase_test.go — Table-driven tests**: Uses `t.Run` with test cases slice
    ```bash
    rg "t\.Run\(" usecase_test.go
    ```

11. **Module isolation (R2)**: No imports from another module's `features/` or `adapters/`
    ```bash
    rg "modules/[^/]+/features/" *.go && echo "FAIL: cross-module import" || echo "PASS"
    ```

12. **Go 1.26 patterns (R7)**: Uses `omitzero` (not `omitempty` for time/ptr), `new(expr)`, `errors.AsType`
    ```bash
    rg "omitempty" *.go && echo "WARN: omitempty found — should be omitzero for time/ptr" || echo "PASS"
    ```

13. **Spanish comments**: Block headers use Spanish
    ```bash
    rg "// [A-Z]" *.go | rg -v "[áéíóúñÁÉÍÓÚÑ]" && echo "WARN: possible English comment" || echo "PASS"
    ```

14. **Echo v5 pattern (R8)**: `*echo.Context` pointer, not `echo.Context` value
    ```bash
    rg "echo\.Context" *.go | rg -v "\*echo\.Context" && echo "FAIL: value context" || echo "PASS"
    ```
