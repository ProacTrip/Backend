# Dashboard API Documentation (Cookie-Based Authorization)

> **Arquitectura:** Cookie-based authentication con autorización RBAC vía middleware `RequirePermission`. El frontend nunca manipula tokens ni permisos.

---

## Índice

| Endpoint | Estado |
|----------|--------|
| [Arquitectura](#arquitectura) | ✅ |
| [Seguridad](#seguridad) | ✅ |
| [Base URLs](#base-urls) | ✅ |
| [Errores Estándar](#errores-estándar) | ✅ |
| [List Users](#list-users) | ✅ Implementado |
| [User Detail](#user-detail) | ✅ Implementado |
| [Account Status](#account-status) | ✅ Implementado |
| [Feature Limits — Usuario](#feature-limits-—-usuario) | ✅ Implementado |
| [Feature Limits — Rol](#feature-limits-—-rol) | ✅ Implementado |
| [Permission Overrides](#permission-overrides) | ✅ Implementado |
| [Rate Limiting](#rate-limiting) | ✅ |
| [Cache](#cache) | ✅ |
| [Notas de Seguridad](#notas-de-seguridad) | ✅ |

---

## Arquitectura

### Pipeline de Autorización

```
PASETO Token ──→ AuthMiddleware ──→ Session Cache [{auth}:session:{sid}]
   │               (popula              │ HIT: extrae permissions[], status, tv
   │                user_claims)        │ MISS: PermissionResolver → DB → GetOrSet
   ▼                                    ▼
  Cookie                        Compara token_version
                                  └─ MISMATCH → 401 TOKEN_VERSION_STALE
                                  └─ status != active → 403
                                Inyecta claims con Permissions[]
                                      ▼
                               RequirePermission("users:read")
                                 └─ slices.Contains(claims.Permissions, perm)
                                    └─ MISSING → 403 (o observe: log only)
                                      ▼
                                    Handler → DB query → 200
```

### Permisos Requeridos

| Permiso | Código | Alcance |
|---------|--------|---------|
| Lectura de usuarios | `users:read` | GET /users, GET /users/:id |
| Escritura de usuarios | `users:write` | PUT /users/:id/status |
| Lectura de feature limits | `feature_limits:read` | GET /feature-limits |
| Escritura de feature limits | `feature_limits:write` | POST/DELETE /feature-limits |
| Lectura de permisos | `permissions:read` | GET /permission-overrides |
| Escritura de permisos | `permissions:write` | POST/DELETE /permission-overrides |

### Modo Observe

Cuando `AUTHZ_ENFORCE_MODE=observe` (default), el middleware `RequirePermission` **nunca bloquea requests**. Ejecuta la verificación completa, loguea `"authz would deny"` con campos estructurados (`permission`, `user_id`, `path`), e incrementa métricas, pero siempre llama a `next(c)`. Esto permite medir el impacto antes de activar la enforce real (`AUTHZ_ENFORCE_MODE=enforce`).

---

## Seguridad

### Autenticación

Todas las rutas del dashboard requieren cookie `__Secure-access_token` válida (PASETO v4). El middleware de autenticación:
1. Valida el token criptográficamente
2. Verifica que el JTI no esté en la blacklist de DragonflyDB (`{auth}:blacklist:jti:{JTI}`)
3. Lee el cache de sesión (`{auth}:session:{sessionID}`) o resuelve desde DB
4. Compara `token_version` entre el token y la DB/cache → mismatch = 401
5. Verifica que el estado de la cuenta sea `active` → si no, 403
6. Inyecta `user_claims` en el contexto con `Permissions[]`

### Invalidación de Sesiones

| Evento | Acción |
|--------|--------|
| Deshabilitar cuenta | `token_version++` + DELETE todas las `{auth}:session:*` |
| Cambio de rol | `token_version++` + DELETE sesiones cacheadas |
| CRUD de overrides | DELETE sesiones cacheadas del usuario afectado |
| Logout | Blacklist JTI en `{auth}:blacklist:jti:{JTI}` |
| Override expirado | Lazy refresh en próximo request vía TTL |

### Cache de Sesión

- **Clave**: `{auth}:session:{sessionID}` (hash en DragonflyDB)
- **Campos**: `permissions` (comma-separated), `status`, `token_version`, `schema_version`
- **TTL**: 5 minutos, sliding reset en cada request autenticado
- **Cache miss**: `PermissionResolver` → DB → repoblación vía `GetOrSet`
- **DragonflyDB caído**: fallback a DB (nunca bloquea requests)

---

## Base URLs

| Entorno | Base URL |
|---------|----------|
| **Production** | `https://api.proactrip.com/v1/dashboard` |
| **Development** | `http://localhost:8080/v1/dashboard` |

Todos los ejemplos usan `{base_url}` como placeholder.

---

## Errores Estándar

Formato **RFC 9457 Problem Details**. Todas las respuestas de error usan `Content-Type: application/problem+json`.

```json
{
  "type": "https://api.proactrip.com/errors/forbidden",
  "title": "Forbidden",
  "status": 403,
  "detail": "Permiso denegado",
  "instance": "/v1/dashboard/users",
  "trace_id": "019d5439-cb43-716d-90b5-51dcbe980908"
}
```

**Headers de respuesta en TODOS los endpoints:**

| Header | Descripción |
|--------|-------------|
| `X-Trace-Id` | UUID v7 para trazabilidad |
| `traceparent` | W3C Trace Context |

---

## List Users

Lista usuarios del sistema con paginación por cursor y filtros combinables.

### Request

```
GET /v1/dashboard/users
```

**Query Parameters:**

| Campo | Tipo | Requerido | Default | Descripción |
|-------|------|-----------|---------|-------------|
| `limit` | int | No | 20 | Cantidad de resultados por página (max 100) |
| `cursor` | string | No | — | Cursor opaco para paginación (base64) |
| `role` | string | No | — | Filtrar por nombre de rol (exact match) |
| `status` | string | No | — | Filtrar por estado: `active`, `disabled`, `suspended`, etc. |
| `search` | string | No | — | Búsqueda por email (ILIKE) |
| `created_before` | string | No | — | Fecha ISO 8601 (límite superior) |
| `created_after` | string | No | — | Fecha ISO 8601 (límite inferior) |

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Cookie` | string | Sí | `__Secure-access_token` válido |

**Ejemplo:**

```bash
curl -X GET "{base_url}/users?limit=10&status=active&role=client" \
  -H "Cookie: __Secure-access_token=v4.local.eyJ..."
```

### Responses

#### 200 OK

```json
{
  "users": [
    {
      "id": "019d5439-cb43-716d-90b5-51dcbe980908",
      "email": "usuario@example.com",
      "status": "active",
      "role_id": "019d5439-cb43-716d-90b5-51dcbe980909",
      "role_name": "client",
      "email_verified": true,
      "created_at": "2026-01-15T10:30:00Z",
      "updated_at": "2026-05-10T14:22:00Z"
    }
  ],
  "meta": {
    "next_cursor": "eyJvIjoyMH0=",
    "prev_cursor": null,
    "has_next": true,
    "limit": 10
  }
}
```

> **Seguridad:** `password_hash`, `locked_until`, `failed_attempts` y datos de OAuth **NUNCA** se incluyen en la respuesta.

#### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `NOT_AUTHENTICATED` | 401 | Cookie ausente o expirada |
| `TOKEN_VERSION_STALE` | 401 | Sesión invalidada (cuenta deshabilitada o rol cambiado) |
| `ACCOUNT_DISABLED` | 403 | Cuenta deshabilitada |
| `MISSING_PERMISSION` | 403 | Usuario sin permiso `users:read` |
| `INVALID_STATUS` | 400 | Parámetro `status` inválido |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## User Detail

Obtiene el detalle de un usuario incluyendo sus permisos efectivos calculados.

### Request

```
GET /v1/dashboard/users/:id
```

**Path Parameters:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `id` | UUID v7 | Sí | ID del usuario |

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Cookie` | string | Sí | `__Secure-access_token` válido |

**Ejemplo:**

```bash
curl -X GET "{base_url}/users/019d5439-cb43-716d-90b5-51dcbe980908" \
  -H "Cookie: __Secure-access_token=v4.local.eyJ..."
```

### Responses

#### 200 OK

```json
{
  "user": {
    "id": "019d5439-cb43-716d-90b5-51dcbe980908",
    "email": "usuario@example.com",
    "status": "active",
    "role_id": "019d5439-cb43-716d-90b5-51dcbe980909",
    "role_name": "client",
    "email_verified": true,
    "mfa_enabled": false,
    "login_count": 42,
    "last_login_at": "2026-05-12T08:15:00Z",
    "created_at": "2026-01-15T10:30:00Z",
    "updated_at": "2026-05-12T08:15:00Z"
  },
  "effective_permissions": [
    "users:read",
    "users:write",
    "feature_limits:read"
  ]
}
```

> **Cálculo de permisos efectivos:** `(rol_permissions ∪ active_grants) − active_denies`. Overrides expirados se excluyen. Deny siempre gana.

#### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `NOT_AUTHENTICATED` | 401 | Cookie ausente o expirada |
| `MISSING_PERMISSION` | 403 | Usuario sin permiso `users:read` |
| `USER_NOT_FOUND` | 404 | ID de usuario no existe |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## Account Status

Habilita o deshabilita una cuenta de usuario. Solo acepta transiciones `active` ↔ `disabled`.

### Request

```
PUT /v1/dashboard/users/:id/status
```

**Path Parameters:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `id` | UUID v7 | Sí | ID del usuario |

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `status` | string | Sí | `"active"` o `"disabled"` | Nuevo estado de la cuenta |

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Cookie` | string | Sí | `__Secure-access_token` válido |
| `Content-Type` | string | Sí | `application/json` |

**Ejemplo:**

```bash
curl -X PUT "{base_url}/users/019d5439-cb43-716d-90b5-51dcbe980908/status" \
  -H "Content-Type: application/json" \
  -H "Cookie: __Secure-access_token=v4.local.eyJ..." \
  -d '{"status":"disabled"}'
```

### Responses

#### 200 OK

```json
{
  "user_id": "019d5439-cb43-716d-90b5-51dcbe980908",
  "previous_status": "active",
  "new_status": "disabled",
  "token_version": 3,
  "sessions_invalidated": 1
}
```

> **Efectos de deshabilitar:** `token_version++` (atómico en DB), todas las sesiones cacheadas eliminadas (`{auth}:session:*`). El próximo request del usuario con cualquier token viejo recibe 401 `TOKEN_VERSION_STALE`.

> **Efectos de habilitar:** solo cambia el estado. No rota `token_version` (el usuario puede usar sus tokens existentes).

#### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `NOT_AUTHENTICATED` | 401 | Cookie ausente o expirada |
| `MISSING_PERMISSION` | 403 | Usuario sin permiso `users:write` |
| `USER_NOT_FOUND` | 404 | ID de usuario no existe |
| `CANNOT_DISABLE_SELF` | 400 | Intentás deshabilitar tu propia cuenta |
| `INVALID_INPUT` | 400 | Estado no válido (solo `active`/`disabled`) o ya está en ese estado |
| `VALIDATION_ERROR` | 400 | Body malformado |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## Feature Limits — Usuario

CRUD de límites de feature por usuario. Cada límite mapea `(user_id, feature_key) → limit_value`.

### Listar Límites de Usuario

```
GET /v1/dashboard/users/:id/feature-limits
```

**Ejemplo:**

```bash
curl -X GET "{base_url}/users/019d5439-cb43-716d-90b5-51dcbe980908/feature-limits" \
  -H "Cookie: __Secure-access_token=v4.local.eyJ..."
```

**Response 200:**

```json
{
  "limits": [
    {
      "feature_key": "projects",
      "limit_value": 5,
      "window": "month"
    },
    {
      "feature_key": "searches",
      "limit_value": null,
      "window": "day"
    }
  ]
}
```

> **Semántica de `limit_value`:** `null` = ilimitado, `0` = bloqueado, `> 0` = cuota.

### Crear/Actualizar Límite de Usuario

```
POST /v1/dashboard/users/:id/feature-limits
```

**Body:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `feature_key` | string | Sí | Identificador del feature (ej. `"projects"`) |
| `limit_value` | int o null | Sí | `null` = ilimitado, `0` = bloqueado, `>0` = cuota |
| `window` | string | No (default: `"month"`) | Ventana: `"minute"`, `"hour"`, `"day"`, `"month"` |

**Ejemplo:**

```bash
curl -X POST "{base_url}/users/019d5439-cb43-716d-90b5-51dcbe980908/feature-limits" \
  -H "Content-Type: application/json" \
  -H "Cookie: __Secure-access_token=v4.local.eyJ..." \
  -d '{"feature_key":"projects","limit_value":10,"window":"month"}'
```

**Response 201:**

```json
{
  "feature_key": "projects",
  "limit_value": 10,
  "window": "month"
}
```

### Eliminar Límite de Usuario

```
DELETE /v1/dashboard/users/:id/feature-limits/:key
```

**Path Parameters:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `id` | UUID v7 | Sí | ID del usuario |
| `key` | string | Sí | Feature key a eliminar |

**Response 204:** Sin body.

#### Posibles Errores (Feature Limits Usuario)

| Código | HTTP | Cuándo |
|--------|------|--------|
| `MISSING_PERMISSION` | 403 | Sin permiso `feature_limits:read` (GET) o `feature_limits:write` (POST/DELETE) |
| `FEATURE_LIMIT_ALREADY_EXISTS` | 409 | Ya existe un límite para ese feature+window (POST) |
| `FEATURE_LIMIT_NOT_FOUND` | 404 | Límite no encontrado (DELETE) |
| `VALIDATION_ERROR` | 400 | Body malformado |

---

## Feature Limits — Rol

CRUD de defaults de feature por rol. Los defaults aplican a todos los usuarios con ese rol que no tengan un límite específico de usuario.

### Listar Defaults de Rol

```
GET /v1/dashboard/roles/:id/feature-limits
```

**Ejemplo:**

```bash
curl -X GET "{base_url}/roles/019d5439-cb43-716d-90b5-51dcbe980909/feature-limits" \
  -H "Cookie: __Secure-access_token=v4.local.eyJ..."
```

**Response 200:**

```json
{
  "limits": [
    {
      "feature_key": "projects",
      "limit_value": 3,
      "window": "month"
    }
  ]
}
```

### Crear/Actualizar Default de Rol

```
POST /v1/dashboard/roles/:id/feature-limits
```

**Body:** Igual que [Feature Limits Usuario](#crearactualizar-límite-de-usuario).

**Ejemplo:**

```bash
curl -X POST "{base_url}/roles/019d5439-cb43-716d-90b5-51dcbe980909/feature-limits" \
  -H "Content-Type: application/json" \
  -H "Cookie: __Secure-access_token=v4.local.eyJ..." \
  -d '{"feature_key":"projects","limit_value":5,"window":"month"}'
```

**Response 201:**

```json
{
  "feature_key": "projects",
  "limit_value": 5,
  "window": "month"
}
```

### Eliminar Default de Rol

```
DELETE /v1/dashboard/roles/:id/feature-limits/:key
```

**Response 204:** Sin body.

> **Resolución de límite efectivo:** `GetEffectiveLimit(user, feature)` → verifica límite de usuario → si no existe, verifica default del rol → si no existe, ilimitado (0 no es bloqueo). Usado internamente por `FeatureLimitService.CanConsume()`.

---

## Permission Overrides

CRUD de overrides de permisos por usuario (grants y denies).

### Listar Overrides

```
GET /v1/dashboard/users/:id/permission-overrides
```

**Ejemplo:**

```bash
curl -X GET "{base_url}/users/019d5439-cb43-716d-90b5-51dcbe980908/permission-overrides" \
  -H "Cookie: __Secure-access_token=v4.local.eyJ..."
```

**Response 200:**

```json
{
  "overrides": [
    {
      "id": "019d5439-cb43-716d-90b5-51dcbe980910",
      "permission": "users:write",
      "granted": true,
      "reason": "Acceso temporal para moderación",
      "expires_at": "2026-06-15T00:00:00Z",
      "created_at": "2026-05-10T10:00:00Z",
      "updated_at": "2026-05-10T10:00:00Z"
    }
  ]
}
```

> **PO-SPEC-002:** Los overrides expirados se incluyen en la respuesta. El cliente o el `PermissionResolver` los filtra.

### Crear Override

```
POST /v1/dashboard/users/:id/permission-overrides
```

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `permission_id` | UUID v7 | Sí | Debe existir en `permissions` | ID del permiso |
| `granted` | bool | Sí | — | `true` = grant, `false` = deny |
| `reason` | string | Sí | 1–500 caracteres, no vacío, no solo whitespace | Razón del override |
| `expires_at` | ISO 8601 | No | No puede exceder 365 días para denies | Fecha de expiración |

**Ejemplo:**

```bash
curl -X POST "{base_url}/users/019d5439-cb43-716d-90b5-51dcbe980908/permission-overrides" \
  -H "Content-Type: application/json" \
  -H "Cookie: __Secure-access_token=v4.local.eyJ..." \
  -d '{"permission_id":"019d5439-cb43-716d-90b5-51dcbe980910","granted":false,"reason":"Abuso de borrado masivo","expires_at":"2026-08-01T00:00:00Z"}'
```

**Response 201:**

```json
{
  "id": "019d5439-cb43-716d-90b5-51dcbe980910",
  "permission": "users:delete",
  "granted": false,
  "reason": "Abuso de borrado masivo",
  "expires_at": "2026-08-01T00:00:00Z",
  "created_at": "2026-05-13T12:00:00Z",
  "updated_at": "2026-05-13T12:00:00Z"
}
```

> **Efectos:** La sesión cacheada del usuario se invalida (best-effort). Próximo request: cache miss → DB fallback → permisos recalculados con el nuevo override.

### Eliminar Override

```
DELETE /v1/dashboard/users/:id/permission-overrides/:overrideId
```

**Path Parameters:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `id` | UUID v7 | Sí | ID del usuario |
| `overrideId` | UUID v7 | Sí | ID del override (permission_id) |

**Response 204:** Sin body.

> **Efectos:** Override eliminado de DB. Sesión cacheada del usuario invalidada (best-effort).

#### Posibles Errores (Permission Overrides)

| Código | HTTP | Cuándo |
|--------|------|--------|
| `MISSING_PERMISSION` | 403 | Sin permiso `permissions:read` (GET) o `permissions:write` (POST/DELETE) |
| `PERMISSION_OVERRIDE_ALREADY_EXISTS` | 409 | Ya existe un override para ese usuario+permiso |
| `PERMISSION_OVERRIDE_NOT_FOUND` | 404 | Override no encontrado (DELETE) |
| `INVALID_REASON` | 400 | Razón vacía, solo whitespace, o > 500 caracteres |
| `INVALID_BLOCK_DURATION` | 400 | Deny con expiración > 365 días |
| `VALIDATION_ERROR` | 400 | Body malformado o campos requeridos ausentes |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## Rate Limiting

Los endpoints del dashboard están protegidos por rate limiting autenticado (basado en `user_id` extraído del PASETO). Ver [AUTH_API — Rate Limiting](./AUTH_API.md#rate-limiting) para detalles de la estrategia.

---

## Cache

### Session Cache (DragonflyDB)

| Aspecto | Detalle |
|---------|---------|
| Clave | `{auth}:session:{sessionID}` |
| Tipo | Hash |
| Campos | `permissions`, `status`, `token_version`, `schema_version` |
| TTL | 5 minutos (sliding reset en cada request) |
| Invalida en | disable, role change, override CRUD, logout |
| Fallback | DB vía `PermissionResolver` (nunca bloquea requests) |

### Cache-Control

Endpoints del dashboard que retornan datos de usuario usan:

```
Cache-Control: no-store, private
```

---

## Notas de Seguridad

### Resolución de Permisos

- **Pipeline:** `(rol_permissions ∪ active_grants) − active_denies`
- **Deny siempre gana:** si un deny y un grant existen para el mismo permiso, el deny prevalece
- **Overrides expirados:** filtrados en tiempo de resolución (`expires_at < NOW()`)
- **Admin bypass:** usuarios con rol `admin` tienen acceso total (no pasan por `RequirePermission`)

### Invalidación de Sesiones

| Evento | Mecanismo | Consistencia |
|--------|-----------|-------------|
| Deshabilitar cuenta | `token_version++` atómico + DEL cache | Inmediata |
| Cambio de rol | `token_version++` + DEL cache | Inmediata |
| CRUD de overrides | DEL cache del usuario (best-effort) | Eventual (lazy refresh) |
| Override expirado | Filtrado en resolución | Inmediato |

Si el DEL de cache falla, `token_version` mismatch lo detecta en el próximo request → 401 → re-autenticación completa.

### Prevención de Ataques

| Amenaza | Mitigación |
|---------|------------|
| Escalación de privilegios | `RequirePermission` con verificación determinística (`slices.Contains`) |
| Replay de tokens viejos | `token_version` check en cada request autenticado |
| Falsificación de permisos | Permisos cacheados en DragonflyDB (server-side, no en el token) |
| Race condition en disable | `UPDATE ... RETURNING token_version` atómico en DB |
| Self-disable accidental | `ErrCannotDisableSelf` — no podés deshabilitar tu propia cuenta |
| Deny bypass | Deny se aplica AL FINAL del pipeline, después de grants |

---

## Paginación

### Formato de Cursor

Cursores opacos en base64: `{"o": offset}`. Ejemplo: `eyJvIjoyMH0=` → offset=20.

### Meta Struct

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `next_cursor` | string o null | Cursor para la página siguiente (null en última página) |
| `prev_cursor` | string o null | Cursor para la página anterior (null en primera página) |
| `has_next` | bool | `true` si hay más resultados |
| `limit` | int | Tamaño de página usado en este request |

### Ordenamiento

`ORDER BY created_at DESC, id DESC` — determinístico y estable incluso con timestamps idénticos.
