# Auditoría: Módulo User

Actuá como un **senior backend engineer + QA lead** que audita el módulo `user` de Proactrip antes de subir a producción. Tu objetivo: encontrar bugs, inconsistencias, código muerto, violaciones de arquitectura, y cualquier cosa que falle en producción.

---

## 🎯 Alcance

Todo bajo `internal/modules/user/` y sus conexiones con `internal/shared/`.

---

## 🔍 Qué investigar (checklist exhaustivo)

### 1. Arquitectura y límites de módulo

- [ ] ¿El módulo user importa directamente de `modules/auth/features/` o `modules/auth/adapters/`?
- [ ] ¿El módulo user importa directamente de `modules/search/features/` o `modules/search/adapters/`?
- [ ] ¿Hay imports circulares o dependencias ocultas entre módulos?
- [ ] ¿`shared/` importa algo de `modules/user/`? (violaría la ley de hierro)
- [ ] ¿Las interfaces (puertos) en `domain/` son puras? (solo stdlib + domain types + uuid)
- [ ] ¿Los adapters en `adapters/` están correctamente aislados? (pgx, MinIO/R2, Redis)
- [ ] ¿El `module.go` expone solo lo necesario? ¿Hay campos exportados que deberían ser privados?

### 2. Feature slices — integridad y consistencia

- [ ] ¿Cada feature slice tiene los 5 archivos requeridos? `command.go`, `handler.go`, `usecase.go`, `response.go`, `usecase_test.go`
- [ ] ¿Todos los `command.go` tienen `Validate() error`?
- [ ] ¿Todos los `handler.go` usan `Handle(c *echo.Context) error` (pointer)?
- [ ] ¿Todos los `usecase.go` tienen `Execute(ctx context.Context, cmd Command) (*Response, error)`?
- [ ] ¿Hay handlers con lógica de negocio? (violación: solo bind → usecase → mapError → JSON)
- [ ] ¿Hay usecases que usan `context.Background()` en vez de `ctx`?
- [ ] ¿Se usa `uc.wg.Go()` para async o hay `go func()` suelto?

### 3. Código muerto y basura

- [ ] ¿Hay funciones, tipos o variables definidos pero nunca usados?
- [ ] ¿Hay imports no utilizados?
- [ ] ¿Hay comentarios `// TODO` o `// FIXME` sin resolver?
- [ ] ¿Hay handlers o endpoints registrados en `module.go` que no tienen implementación real?
- [ ] ¿Hay feature slices incompletos? (ej: handler existe pero usecase vacío)
- [ ] ¿Hay código comentado (bloques enteros de `//`)?
- [ ] ¿Hay métodos en repositorios que no se usan en producción? (solo en tests)

### 4. Errores y validación

- [ ] ¿Todos los errores de dominio están mapeados a HTTP? (buscar `RegisterDomainErrorMapper`)
- [ ] ¿Hay errores definidos en `domain/errors.go` que nunca se producen? (buscar sentinel references)
- [ ] ¿Hay errores producidos pero no mapeados a HTTP? (caerían en 500 genérico)
- [ ] ¿Los handlers retornan errores sin wrappear? (deben usar `fmt.Errorf("context: %w", err)`)
- [ ] ¿Los mensajes de error al usuario son en español?
- [ ] ¿Se usa `errors.AsType[T]()` (Go 1.26) o el viejo `errors.As()`?
- [ ] ¿El `UpdateStatus` recién agregado (migración 007) funciona correctamente? ¿Está en la interfaz?

### 5. Conexiones con otros módulos

- [ ] ¿User consume eventos de auth? (UserRegistered → crear perfil) — ¿el consumer está implementado?
- [ ] ¿User publica eventos que otros módulos consumen? — ¿los streams están correctos?
- [ ] ¿Los contratos en `shared/user/prefs.go` están completos? ¿Falta algún campo que el frontend necesita?
- [ ] ¿Hay campos de perfil que auth debería conocer pero user no expone?
- [ ] ¿El `EnvironmentResolver` (IP → país → moneda) se usa para defaults de perfil? ¿O hay lógica duplicada?

### 6. Base de datos y migraciones

- [ ] ¿Todas las migraciones tienen `-- +migrate Up` y `-- +migrate Down`?
- [ ] ¿Las migraciones son idempotentes? (`IF NOT EXISTS`, `DO $$` blocks)
- [ ] ¿Las tablas usan `uuid PRIMARY KEY DEFAULT uuidv7()`?
- [ ] ¿Los CHECK constraints cubren todos los valores válidos?
- [ ] ¿Hay índices para las queries más frecuentes?
- [ ] ¿La migración 007 (`status` en user_profiles) está correctamente aplicada?
- [ ] ¿Hay columnas sin usar creadas por migraciones viejas?

### 7. DragonflyDB / Cache

- [ ] ¿Todas las keys usan hashtag `{user}:` para co-ubicación en shard?
- [ ] ¿Hay TTLs hardcodeados? (deberían ser configurables o constantes documentadas)
- [ ] ¿Las operaciones de cache son best-effort? (no deberían hacer fallar requests)
- [ ] ¿Se usa `GetOrSet` donde corresponde?
- [ ] ¿El prefetch/prewarm de perfil al hacer login funciona?

### 8. Documentos, avatares y archivos

- [ ] ¿Los uploads a MinIO/R2 tienen validación de tamaño y tipo?
- [ ] ¿Los nombres de archivo son UUIDs (no user-provided)?
- [ ] ¿Hay rate limiting en endpoints de upload?
- [ ] ¿Las URLs de descarga tienen TTL o son permanentes?
- [ ] ¿Se limpian archivos huérfanos? (upload fallido, perfil eliminado)

### 9. Tests

- [ ] ¿Todos los tests pasan? `go test -race ./internal/modules/user/...`
- [ ] ¿Hay tests que usan `testify` o `gomock`? (prohibido — solo `testing`)
- [ ] ¿Hay tests con `time.Sleep`? (deben usar `synctest` o `t.Context()`)
- [ ] ¿Los tests de handler usan `httptest`?
- [ ] ¿Los tests de cache usan `miniredis`?
- [ ] ¿Los tests de repo PostgreSQL usan test DB o mock?
- [ ] ¿Hay assertions sobre implementación en vez de comportamiento?

### 10. Documentación vs implementación

- [ ] `docs/USER_API.md`: ¿todos los endpoints están documentados?
- [ ] ¿Los campos de respuesta en docs coinciden con los struct tags JSON?
- [ ] ¿Los códigos de error en docs coinciden con los mapeados?
- [ ] ¿Hay endpoints implementados pero no documentados?
- [ ] ¿Hay endpoints documentados pero no implementados?
- [ ] ¿El perfil incluye todos los campos que el frontend espera? (email, preferencias, avatar, documentos)

### 11. Seguridad y configuración

- [ ] ¿Hay secretos hardcodeados? (API keys, passwords, tokens)
- [ ] ¿Los endpoints de perfil requieren autenticación?
- [ ] ¿Un usuario puede modificar el perfil de otro usuario? (chequear `userID` en claims vs param)
- [ ] ¿Los documentos/avatares son accesibles solo por el dueño?
- [ ] ¿Hay rate limiting en endpoints sensibles? (cambio de email, upload)
- [ ] ¿Los logs incluyen datos sensibles? (emails completos, tokens, documentos)

### 12. Convenciones Go 1.26

- [ ] ¿Se usa `new(expr)` o el patrón viejo `x := val; &x`?
- [ ] ¿Se usa `omitzero` para `time.Time`, punteros, slices? (no `omitempty`)
- [ ] ¿Se usa `slices.Contains`, `maps.Keys`, `cmp.Or`?
- [ ] ¿Se usa `t.Context()` en tests?
- [ ] ¿Se usa `wg.Go()` en vez de `go func()`?
- [ ] ¿Los comentarios de código están en español?

---

## 📤 Output esperado

Clasificá cada hallazgo como:
- **🔴 CRÍTICO**: rompe compilación, seguridad, o comportamiento incorrecto
- **🟠 WARNING**: bug potencial, inconsistencia, o mala práctica
- **🟡 SUGGESTION**: mejora de calidad, documentación, o refactor

Para cada hallazgo: archivo, línea, problema, y sugerencia de fix.

Terminá con un veredicto: `PASS` / `PASS WITH WARNINGS` / `FAIL`.
