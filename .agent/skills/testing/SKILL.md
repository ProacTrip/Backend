# testing

## Trigger
When the user needs to generate tests for any Proactrip code: usecases, handlers, repositories, cache adapters. Keywords: "test", "testing", "pruebas", "unit test", "table driven", "mock", "httptest", "miniredis"

## Questions to Ask (ALWAYS ask first)

1. ¿Qué componente vas a testear? (usecase, handler HTTP, repository PostgreSQL, cache Dragonfly)
2. ¿Cuáles son los casos de prueba? (happy path, error de validación, recurso no encontrado, timeout, rate limit)
3. ¿Qué dependencias necesitan mock? (repository, cache, proveedor externo, event publisher)
4. ¿El componente usa código concurrente? (goroutines, WaitGroup — usar synctest)
5. ¿Qué edge cases querés cubrir? (inputs vacíos, nil pointers, valores límite)

## Rules

### Testing Rules (T1-T5)
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| T1 | Standard Library | MUST | Use ONLY `testing` package — no testify, no gomock, no external test frameworks |
| T2 | Table-Driven | MUST | Table-driven tests with `name`, `mockSetup`, `input`, `wantErr`, `wantResp` fields |
| T3 | httptest | MUST | Use `net/http/httptest` for handler tests — NewRequest, NewRecorder |
| T4 | miniredis | MUST | Use `github.com/alicebob/miniredis/v2` for Dragonfly/Redis tests |
| T5 | synctest | SHOULD | Use `synctest.Test(t, func(t testing.TB) {...})` for concurrent code |

### Global Rules
| Rule | Severity |
|------|----------|
| R6 Testing | MUST — generate _test.go alongside code |
| R7 Go 1.26 | MUST — use `new(expr)`, `errors.AsType` |

## Patterns

### Pattern 1: Table-Driven Usecase Test (from ratelimit/limiter_test.go)
```go
func TestAllow(t *testing.T) {
    mr := redis.NewMiniRedis(t)
    rl := NewRateLimiter(mr.Client(), DefaultConfig())
    tests := []struct {
        name    string
        key     string
        max     int
        wantErr bool
    }{
        {"primera petición", "test:1", 10, false},
        {"límite excedido", "test:2", 1, true},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            result, err := rl.Allow(context.Background(), tc.key, tc.max, time.Minute)
            if tc.wantErr && err == nil {
                t.Error("esperaba error, obtuve nil")
            }
        })
    }
}
```

### Pattern 2: Handler Test with httptest (from middleware_test.go)
```go
func TestMiddleware(t *testing.T) {
    e := echo.New()
    req := httptest.NewRequest(http.MethodPost, "/v1/test", strings.NewReader(`{"field":"value"}`))
    req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)
    // call handler
    if rec.Code != http.StatusOK {
        t.Errorf("esperaba 200, obtuve %d", rec.Code)
    }
}
```

## Templates

### Template: usecase_test.go
```go
package {{.Package}}

import (
    "context"
    "errors"
    "testing"
    "github.com/alicebob/miniredis/v2"
)

func Test{{.UseCaseName}}Execute(t *testing.T) {
    tests := []struct {
        name     string
        cmd      Command
        setup    func(*mocks)
        wantErr  bool
        wantResp *Response
    }{
        {
            name: "happy path",
            cmd:  Command{Field: "valid"},
            setup: func(m *mocks) {
                m.repo.OnGetByID = func(ctx context.Context, id string) (*Entity, error) {
                    return &Entity{ID: id}, nil
                }
            },
            wantErr: false,
        },
        {
            name: "validation error",
            cmd:  Command{Field: ""},
            wantErr: true,
        },
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            m := newMocks()
            if tc.setup != nil { tc.setup(m) }
            uc := NewUseCase(UseCaseDeps{Repo: m.repo, Cache: m.cache})
            resp, err := uc.Execute(context.Background(), tc.cmd)
            if tc.wantErr && err == nil {
                t.Error("esperaba error, obtuve nil")
            }
            if !tc.wantErr && err != nil {
                t.Errorf("error inesperado: %v", err)
            }
            _ = resp
        })
    }
}
```

### Template: handler_test.go
```go
package {{.Package}}

import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "github.com/labstack/echo/v5"
)

func Test{{.HandlerName}}Handle(t *testing.T) {
    e := echo.New()
    uc := &mockUseCase{}
    h := NewHandler(uc)

    body := `{"field":"value"}`
    req := httptest.NewRequest(http.MethodPost, "/v1/{{.Path}}", strings.NewReader(body))
    req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)

    err := h.Handle(c)
    if err != nil {
        t.Fatalf("Handle error: %v", err)
    }
    if rec.Code != http.StatusOK {
        t.Errorf("status = %d, want 200", rec.Code)
    }
}
```

### Template: adapter_test.go (miniredis)
```go
package {{.Package}}

import (
    "context"
    "testing"
    "time"
    "github.com/alicebob/miniredis/v2"
)

func TestCacheSetAndGet(t *testing.T) {
    mr := miniredis.NewMiniRedis(t)
    cache := NewCache(mr.Client(), time.Minute)

    ctx := context.Background()
    err := cache.Set(ctx, "key", "value", time.Minute)
    if err != nil {
        t.Fatalf("Set error: %v", err)
    }

    val, err := cache.Get(ctx, "key")
    if err != nil {
        t.Fatalf("Get error: %v", err)
    }
    if val != "\"value\"" { // JSON-marshaled
        t.Errorf("Get = %s, want \"value\"", val)
    }
}
```

## Uses Skills
| Skill | When |
|-------|------|
| `go-testing` | Always — testing conventions, patterns, synctest |
| `go` | Always — Go 1.26 patterns |

## Verification
```bash
# 1. No testify import
grep 'github.com/stretchr/testify' {{.TestFile}} && echo "FAIL: testify import" || echo "PASS"

# 2. Table-driven pattern
grep 't.Run(tc.name' {{.TestFile}} || echo "WARN: not table-driven"

# 3. Tests run
go test ./{{.PackagePath}}/... -v -count=1 && echo "PASS: tests pass"
```
