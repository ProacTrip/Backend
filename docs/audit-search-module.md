# Auditoría: Módulo Search

Actuá como un **senior backend engineer + QA lead** que audita el módulo `search` de Proactrip antes de subir a producción. Tu objetivo: encontrar bugs, inconsistencias, código muerto, violaciones de arquitectura, y cualquier cosa que falle en producción.

---

## 🎯 Alcance

Todo bajo `internal/modules/search/` y sus conexiones con `internal/shared/`.

---

## 🔍 Qué investigar (checklist exhaustivo)

### 1. Arquitectura y límites de módulo

- [ ] ¿El módulo search importa directamente de `modules/user/features/` o `modules/user/adapters/`?
- [ ] ¿El módulo search importa directamente de `modules/auth/features/` o `modules/auth/adapters/`?
- [ ] ¿Hay imports circulares o dependencias ocultas entre módulos?
- [ ] ¿`shared/` importa algo de `modules/search/`? (violaría la ley de hierro)
- [ ] ¿Las interfaces (puertos) en `domain/` son puras? (solo stdlib + domain types)
- [ ] ¿Los adapters en `adapters/` están correctamente aislados? (pgx, redis, SerpAPI, AI providers)
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

### 4. Errores y validación

- [ ] ¿Todos los errores de dominio están mapeados a HTTP? (buscar `RegisterDomainErrorMapper`)
- [ ] ¿Hay errores definidos en `domain/errors.go` que nunca se producen? (buscar sentinel references)
- [ ] ¿Hay errores producidos pero no mapeados a HTTP? (caerían en 500 genérico)
- [ ] ¿Los handlers retornan errores sin wrappear? (deben usar `fmt.Errorf("context: %w", err)`)
- [ ] ¿Los mensajes de error al usuario son en español?
- [ ] ¿Se usa `errors.AsType[T]()` (Go 1.26) o el viejo `errors.As()`?

### 5. Conexiones con otros módulos

- [ ] ¿Search usa datos de user? Revisar `shared/user/prefs.go` — ¿está completo o hay campos sin usar?
- [ ] ¿Search usa datos de environment? Revisar `shared/environment/dto.go` — ¿el contrato está completo?
- [ ] ¿Search publica eventos? Revisar `shared/eventbus/` — ¿los streams están correctamente nombrados?
- [ ] ¿Hay campos en shared/ que search necesita pero no existen?
- [ ] ¿Hay datos que search debería consumir de otro módulo pero los está duplicando?

### 6. DragonflyDB / Cache

- [ ] ¿Todas las keys usan hashtag `{search}:` para co-ubicación en shard?
- [ ] ¿Hay TTLs hardcodeados? (deberían ser configurables o constantes documentadas)
- [ ] ¿Las operaciones de cache son best-effort? (no deberían hacer fallar requests)
- [ ] ¿Hay `GetOrSet` donde corresponde para evitar thundering herd?
- [ ] ¿Hay Lua scripts? Verificar que usen hashtag `{search}` para evitar Global Lock

### 7. AI / LLM

- [ ] ¿Los prompts del LLM tienen guardrails? ("No inventes destinos", "Solo datos reales")
- [ ] ¿Los tokens de AI provider están en `.env`, no hardcodeados?
- [ ] ¿Hay timeout en las llamadas al LLM?
- [ ] ¿Hay fallback si el LLM no responde? (degraded mode)
- [ ] ¿El language del LLM response respeta `environment.language`?

### 8. Tests

- [ ] ¿Todos los tests pasan? `go test -race ./internal/modules/search/...`
- [ ] ¿Hay tests que usan `testify` o `gomock`? (prohibido — solo `testing`)
- [ ] ¿Hay tests con `time.Sleep`? (deben usar `synctest` o `t.Context()`)
- [ ] ¿Los tests de handler usan `httptest`?
- [ ] ¿Los tests de cache usan `miniredis`?
- [ ] ¿Hay assertions sobre implementación en vez de comportamiento?
- [ ] ¿Hay tests que solo hacen smoke test? (render + exist, sin verificar comportamiento)

### 9. Documentación vs implementación

- [ ] `docs/search_ai_api.md`: ¿todos los endpoints están documentados?
- [ ] `docs/search_flights_api.md`: ¿coincide con la implementación real?
- [ ] `docs/search_hotels_api.md`: ¿coincide con la implementación real?
- [ ] ¿Los campos de respuesta en docs coinciden con los struct tags JSON?
- [ ] ¿Los códigos de error en docs coinciden con los mapeados?
- [ ] ¿Hay endpoints implementados pero no documentados?
- [ ] ¿Hay endpoints documentados pero no implementados?

### 10. Seguridad y configuración

- [ ] ¿Hay secretos hardcodeados? (API keys, passwords, tokens)
- [ ] ¿Hay valores por defecto inseguros? (timeouts muy largos, rate limits muy altos)
- [ ] ¿Las variables de entorno tienen defaults sensatos?
- [ ] ¿Hay CORS configurado correctamente?
- [ ] ¿Los rate limits están aplicados consistentemente?
- [ ] ¿Los logs incluyen datos sensibles? (emails completos, tokens, passwords)

---

## 📤 Output esperado

Clasificá cada hallazgo como:
- **🔴 CRÍTICO**: rompe compilación, seguridad, o comportamiento incorrecto
- **🟠 WARNING**: bug potencial, inconsistencia, o mala práctica
- **🟡 SUGGESTION**: mejora de calidad, documentación, o refactor

Para cada hallazgo: archivo, línea, problema, y sugerencia de fix.

Terminá con un veredicto: `PASS` / `PASS WITH WARNINGS` / `FAIL`.
