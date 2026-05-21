# Dashboard Module API Documentation (Cookie-Based)

> **Arquitectura:** Endpoints administrativos protegidos con PASETO HttpOnly cookies + RBAC granular (5 permisos). **Uso exclusivo del rol admin.** Separado del User module para mantener aislamiento de responsabilidades.
> **Alcance:** Gestión de usuarios, feature limits y verificación de documentos. 6 endpoints implementados + 3 endpoints de verificación de documentos diseñados para implementación.

---

## Índice

| Sección | Estado |
|---------|--------|
| [Arquitectura](#arquitectura) | ✅ |
| [Modelo RBAC](#modelo-rbac) | ✅ |
| [Base URLs](#base-urls) | ✅ |
| [Errores Estándar](#errores-estándar) | ✅ |
| [Autenticación](#autenticación) | ✅ |
| [List Users](#list-users) | ✅ Implementado |
| [User Detail](#user-detail) | ✅ Implementado |
| [Account Status](#account-status) | ✅ Implementado |
| [Feature Limits (User)](#feature-limits-user) | ✅ Implementado |
| [Document Verification](#document-verification) | ✅ Documentado |
| [Configuración CORS](#configuración-cors) | ✅ |
| [Rate Limiting](#rate-limiting) | ✅ |
| [Cache](#cache) | ✅ |
| [Notas de Seguridad](#notas-de-seguridad) | ✅ |

---

## Arquitectura

### Flujo de Request

```
Cliente (Frontend proactrip.com)
  │
  │ Cookie: __Secure-access_token
  ▼
┌──────────────────────────────────────────┐
│ AuthMiddleware (PASETO validation)        │  ← Verifica token, inyecta claims
├──────────────────────────────────────────┤
│ AuthenticatedRateLimitMW (10 req/min)     │  ← Rate limit por user UUID
├──────────────────────────────────────────┤
│ RequirePermission(users:read)             │  ← Grupo base: todos los endpoints
├──────────────────────────────────────────┤
│ [RequirePermission adicional]             │  ← Solo endpoints de mutación
├──────────────────────────────────────────┤
│ Handler (list_users / account_status...)  │  ← Lógica de negocio
└──────────────────────────────────────────┘
  │
  ▼
Respuesta JSON + X-Trace-Id
```

### Acceso Restringido al Admin

**Todos los endpoints de este módulo requieren permisos que solo el rol `admin` posee.** El middleware `RequirePermission` verifica que los claims del token PASETO incluyan permisos como `users:read`, `users:write` y `feature_limits:write`. El rol `admin` tiene los 5 permisos asignados en base de datos, por lo que todos los checks pasan. El rol `client` no tiene ninguno de estos permisos y recibe `403` en cualquier endpoint del dashboard.

La granularidad de permisos existe para un futuro donde otros roles puedan tener acceso parcial, pero en el diseño actual solo `admin` posee los permisos necesarios.

### Separación del User Module

El Dashboard module (`/v1/dashboard/*`) está aislado del User module (`/v1/user/*`). El User module expone endpoints de perfil para el usuario autenticado (lectura de sus propios datos, aplica solo a rol `client`. El Dashboard module expone endpoints administrativos que solo el rol `admin` puede consumir. Ambos comparten el mismo middleware de autenticación PASETO, pero difieren en los requisitos de autorización.

El grupo de rutas se define en `internal/bootstrap/app.go:524-569` con la cadena de middleware `authMiddleware → authRateLimitMW → RequirePermission(users:read)`.

---

## Modelo RBAC

El dashboard usa **autorización basada en permisos, no en roles**. El middleware `RequirePermission` verifica que el usuario autenticado tenga un permiso específico en sus claims PASETO — nunca consulta "¿es admin?". El acceso al dashboard funciona así:

1. **AuthMiddleware** valida el PASETO y carga `user_claims` en el contexto (incluye el array de permisos del usuario)
2. **RequirePermission** lee ese array y verifica que contenga el permiso requerido con `slices.Contains`
3. Si el permiso está presente → next handler. Si no → 403 Forbidden

El rol `admin` existe a nivel de base de datos como una agrupación de permisos. Cuando un admin inicia sesión, su token PASETO incluye los 5 permisos. Esa lista plana es lo único que el middleware ve.

### Permisos

| Constante | Permiso | Dashboard |
|-----------|---------|-----------|
| `PermUsersRead` | `users:read` | ✅ Grupo base — requerido en todos los endpoints |
| `PermUsersWrite` | `users:write` | ✅ Account Status |
| `PermFeatureLimitsWrite` | `feature_limits:write` | ✅ Feature Limits (crear/eliminar) |
| `PermSessionsWrite` | `sessions:write` | ✅ Account Status — requerido para invalidar sesiones al deshabilitar |
| `PermSessionsRead` | `sessions:read` | — |

### Modelo de Grupo Base + Aditivo

Cada endpoint del dashboard recibe una combinación de permisos:

1. **Grupo base**: `users:read` — aplicado a nivel de grupo (`RequirePermission` en `app.go:527`). Todo endpoint del dashboard requiere este permiso como mínimo.

2. **Permisos aditivos**: los endpoints de mutación añaden un segundo `RequirePermission` con un permiso más específico:
   - `PUT /users/:id/status` → `users:read` + `users:write` + `sessions:write`
   - `POST/DELETE /users/:id/feature-limits` → `users:read` + `feature_limits:write`

### Quién puede acceder

| Rol | Acceso al Dashboard |
|-----|---------------------|
| **admin** | ✅ **Acceso total** — el rol admin tiene los 5 permisos asignados. Todos los `RequirePermission` pasan. |
| **client** | ❌ **Sin acceso** — el rol client no tiene permisos administrativos. Cualquier request a `/v1/dashboard` recibe 403. |

---

## Base URLs

| Entorno | Base URL |
|---------|----------|
| **Production** | `https://api.proactrip.com/v1/dashboard` |
| **Development** | `http://localhost:8080/v1/dashboard` |

---

## Errores Estándar

Todos los errores usan el formato **RFC 9457 Problem Details** con `Content-Type: application/problem+json`.

### Ejemplo

```json
{
  "type": "https://api.proactrip.com/errors/forbidden",
  "title": "Forbidden",
  "status": 403,
  "detail": "Permiso denegado",
  "instance": "/v1/dashboard/users/0193c8c6-1234-7abc-8def-0123456789ab/status",
  "trace_id": "0193c8c6-5678-7def-9abc-0123456789cd"
}
```

### Headers de Error

| Header | Valor |
|--------|-------|
| `Content-Type` | `application/problem+json` |
| `X-Trace-Id` | UUID v7 para trazabilidad |
| `traceparent` | W3C Trace Context (generado por middleware) |

### Flujo de Mapeo de Errores

```
Error de dominio (ej. ErrUserNotFound)
  │
  ▼
auth/module.go: RegisterDomainErrorMapper
  │  errors.Is(err, domain.ErrUserNotFound) → serrors.ErrNotFound(...)
  │
  ▼
shared/errors/errors.go: ProblemTypeNotFound → HTTP 404
  │
  ▼
shared/http/error_mapper.go: MapError()
  │  Set X-Trace-Id, Content-Type: application/problem+json
  │  c.JSON(http.StatusNotFound, problem)
  │
  ▼
Respuesta RFC 9457 al cliente
```

### Problem Types del Dashboard

| Problem Type URI | Categoría |
|------------------|-----------|
| `https://api.proactrip.com/errors/unauthorized` | 401 — Token faltante o inválido |
| `https://api.proactrip.com/errors/forbidden` | 403 — Sin permisos |
| `https://api.proactrip.com/errors/not-found` | 404 — Recurso no encontrado |
| `https://api.proactrip.com/errors/bad-request` | 400 — Input inválido |
| `https://api.proactrip.com/errors/conflict` | 409 — Conflicto (ya existe) |
| `https://api.proactrip.com/errors/invalid-input` | 400 — Campos requeridos faltantes |
| `https://api.proactrip.com/errors/internal-error` | 500 — Error interno |

---

## Autenticación

Todos los endpoints del dashboard requieren autenticación vía **PASETO cookies HttpOnly** y permisos administrativos. Solo el rol `admin` — que tiene los 5 permisos asignados en base de datos — puede consumir estas rutas. El rol `client` no posee estos permisos y recibe `403` en cualquier endpoint del dashboard.

El middleware **no verifica roles** — verifica permisos. El admin accede porque su token PASETO incluye todos los permisos, no porque exista un `RequireRole("admin")`.

### Cookies

| Cookie | Atributos |
|--------|-----------|
| `__Secure-access_token` | `HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900` |
| `__Secure-refresh_token` | `HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800` |

### Requisitos

- El frontend debe enviar las cookies automáticamente (`credentials: 'include'` en fetch).
- El middleware `AuthMiddleware` valida el PASETO, verifica que el token no esté revocado (DragonflyDB blacklist), y rota el access token silenciosamente si está por expirar (últimos 25% de su TTL).
- Si el token es inválido o expirado, se devuelve `401 Unauthorized` con `TOKEN_INVALID` o `TOKEN_EXPIRED`.
- El middleware `RequirePermission` verifica que el usuario autenticado tenga el permiso requerido. Si no, devuelve `403 Forbidden` con `PERMISSION_DENIED`.

---

## List Users ✅

Lista usuarios del dashboard con filtros y paginación por cursor.

### Request

```
GET /v1/dashboard/users
```

**Query Params:**

| Parámetro | Tipo | Requerido | Default | Validación | Descripción |
|-----------|------|-----------|---------|------------|-------------|
| `limit` | int | No | `10` | 1–100 | Cantidad de resultados por página |
| `role` | string | No | `""` | — | Filtrar por nombre de rol |
| `status` | string | No | `""` | `unverified`, `active`, `disabled` | Filtrar por estado de cuenta |
| `search` | string | No | `""` | — | Búsqueda por email o nombre |
| `cursor` | string | No | `""` | — | Cursor opaco para paginación (base64) |
| `created_before` | string | No | `""` | — | Filtrar creados antes de fecha ISO 8601 |
| `created_after` | string | No | `""` | — | Filtrar creados después de fecha ISO 8601 |

**Headers:**

| Header | Requerido | Descripción |
|--------|-----------|-------------|
| `Cookie: __Secure-access_token` | Sí | PASETO token HttpOnly |
| `Accept: application/json` | No | Formato esperado |

**Ejemplo:**

```bash
curl -X GET "http://localhost:8080/v1/dashboard/users?limit=20&status=active&role=admin" \
  -H "Accept: application/json" \
  -b cookies.txt -c cookies.txt
```

### Responses

#### 200 OK

```json
{
  "users": [
    {
      "id": "0193c8c6-1234-7abc-8def-0123456789ab",
      "email": "admin@proactrip.com",
      "status": "active",
      "role_id": "0193c8c6-1234-7abc-8def-0123456789cd",
      "role_name": "admin",
      "email_verified": true,
      "created_at": "2026-01-15T10:30:00Z",
      "updated_at": "2026-05-18T14:22:00Z"
    }
  ],
  "meta": {
    "next_cursor": "eyJpZCI6IjAxOTNjOGM2LTEyMzQtN2FiYy04ZGVmLTAxMjM0NTY3ODllZiJ9",
    "prev_cursor": null,
    "has_next": true,
    "limit": 20
  }
}
```

**Campos de `UserResponse`:**

| Campo | Tipo | JSON key | Descripción |
|-------|------|----------|-------------|
| `ID` | UUID | `id` | Identificador único del usuario |
| `Email` | string | `email` | Email del usuario |
| `Status` | string | `status` | Estado actual de la cuenta |
| `RoleID` | UUID | `role_id` | ID del rol asignado |
| `RoleName` | string | `role_name` | Nombre del rol asignado |
| `EmailVerified` | bool | `email_verified` | Si el email fue verificado |
| `CreatedAt` | datetime | `created_at` | Fecha de creación (ISO 8601) |
| `UpdatedAt` | datetime | `updated_at` | Fecha de última actualización (ISO 8601) |

**Campos de `Meta`:**

| Campo | Tipo | JSON key | Descripción |
|-------|------|----------|-------------|
| `NextCursor` | *string | `next_cursor` | Cursor para la página siguiente (omitzero: omitido si no hay más páginas) |
| `PrevCursor` | *string | `prev_cursor` | Cursor para la página anterior (omitzero: omitido en primera página) |
| `HasNext` | bool | `has_next` | Indica si hay más resultados |
| `Limit` | int | `limit` | Límite aplicado en esta página |

#### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie ausente, token inválido o expirado |
| `TOKEN_EXPIRED` | 401 | `unauthorized` | Token PASETO expirado y no se pudo rotar |
| `NOT_AUTHENTICATED` | 401 | `unauthorized` | No hay token de acceso en la cookie |
| `PERMISSION_DENIED` | 403 | `forbidden` | El usuario no tiene el permiso `users:read` |
| `INVALID_INPUT` | 400 | `invalid-input` | `limit` fuera de rango (1–100) o `status` inválido |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado del servidor |

---

## User Detail ✅

Obtiene el detalle de un usuario específico, incluyendo sus permisos efectivos.

### Request

```
GET /v1/dashboard/users/:id
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `id` | UUID | Sí | ID del usuario a consultar |

**Permisos requeridos:** `users:read` (grupo base)

**Ejemplo:**

```bash
curl -X GET "http://localhost:8080/v1/dashboard/users/0193c8c6-1234-7abc-8def-0123456789ab" \
  -H "Accept: application/json" \
  -b cookies.txt -c cookies.txt
```

### Responses

#### 200 OK

```json
{
  "user": {
    "id": "0193c8c6-1234-7abc-8def-0123456789ab",
    "email": "admin@proactrip.com",
    "status": "active",
    "role_id": "0193c8c6-1234-7abc-8def-0123456789cd",
    "role_name": "admin",
    "email_verified": true,
    "login_count": 42,
    "last_login_at": "2026-05-20T08:15:00Z",
    "created_at": "2026-01-15T10:30:00Z",
    "updated_at": "2026-05-18T14:22:00Z"
  },
  "effective_permissions": [
    "users:read",
    "users:write",
    "feature_limits:write",
    "sessions:read",
    "sessions:write"
  ]
}
```

**Campos de `UserDetailResponse`:**

| Campo | Tipo | JSON key | Descripción |
|-------|------|----------|-------------|
| `ID` | UUID | `id` | Identificador único del usuario |
| `Email` | string | `email` | Email del usuario |
| `Status` | string | `status` | Estado actual de la cuenta |
| `RoleID` | UUID | `role_id` | ID del rol asignado |
| `RoleName` | string | `role_name` | Nombre del rol asignado |
| `EmailVerified` | bool | `email_verified` | Si el email fue verificado |
| `LoginCount` | int | `login_count` | Cantidad total de inicios de sesión |
| `LastLoginAt` | *datetime | `last_login_at` | Último inicio de sesión (omitzero: omitido si nunca inició sesión) |
| `CreatedAt` | datetime | `created_at` | Fecha de creación (ISO 8601) |
| `UpdatedAt` | datetime | `updated_at` | Fecha de última actualización (ISO 8601) |

**Campos de respuesta:**

| Campo | Tipo | JSON key | Descripción |
|-------|------|----------|-------------|
| `User` | object | `user` | Datos del usuario |
| `EffectivePermissions` | []string | `effective_permissions` | Permisos efectivos del usuario (incluye role + overrides) |

#### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie ausente, token inválido o expirado |
| `TOKEN_EXPIRED` | 401 | `unauthorized` | Token PASETO expirado |
| `NOT_AUTHENTICATED` | 401 | `unauthorized` | No hay token de acceso en la cookie |
| `PERMISSION_DENIED` | 403 | `forbidden` | El usuario no tiene el permiso `users:read` |
| `USER_NOT_FOUND` | 404 | `not-found` | El `:id` no corresponde a un usuario existente |
| `INVALID_INPUT` | 400 | `invalid-input` | El `:id` no es un UUID válido |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado del servidor |

---

## Account Status ✅

Habilita o deshabilita la cuenta de un usuario. Al deshabilitar, se invalidan todas las sesiones activas.

### Request

```
PUT /v1/dashboard/users/:id/status
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `id` | UUID | Sí | ID del usuario objetivo |

**Permisos requeridos:** `users:read` (grupo base) + `users:write` + `sessions:write` (aditivo)

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `status` | string | Sí | `"active"` o `"disabled"` | Nuevo estado de la cuenta |

**Ejemplo:**

```bash
curl -X PUT "http://localhost:8080/v1/dashboard/users/0193c8c6-1234-7abc-8def-0123456789ab/status" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -b cookies.txt -c cookies.txt \
  -d '{"status": "disabled"}'
```

### Responses

#### 200 OK

```json
{
  "user_id": "0193c8c6-1234-7abc-8def-0123456789ab",
  "previous_status": "active",
  "new_status": "disabled",
  "token_version": 3,
  "sessions_invalidated": 2
}
```

**Campos de respuesta:**

| Campo | Tipo | JSON key | Descripción |
|-------|------|----------|-------------|
| `UserID` | UUID | `user_id` | ID del usuario modificado |
| `PreviousStatus` | string | `previous_status` | Estado anterior a la modificación |
| `NewStatus` | string | `new_status` | Nuevo estado aplicado |
| `TokenVersion` | int | `token_version` | Nueva versión del token (incrementada en disable) |
| `SessionsInvalidated` | int | `sessions_invalidated` | Cantidad de sesiones activas invalidadas |

#### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie ausente, token inválido o expirado |
| `TOKEN_EXPIRED` | 401 | `unauthorized` | Token PASETO expirado |
| `NOT_AUTHENTICATED` | 401 | `unauthorized` | No hay token de acceso en la cookie |
| `PERMISSION_DENIED` | 403 | `forbidden` | El usuario no tiene `users:read` + `users:write` + `sessions:write` |
| `USER_NOT_FOUND` | 404 | `not-found` | El `:id` no existe en la DB |
| `INVALID_INPUT` | 400 | `invalid-input` | UUID inválido, `status` faltante, o valor no permitido (distinto de `active`/`disabled`) |
| `CANNOT_DISABLE_SELF` | 400 | `bad-request` | El `actorID` (extraído del token PASETO) coincide con el `:id` del path param — un admin no puede deshabilitar su propia cuenta |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado del servidor |

> **Validación de auto-deshabilitación:** el usecase compara el `actorID` (extraído del token PASETO vía `extractActorID`) con el `userID` del path param. Si son iguales, retorna `CANNOT_DISABLE_SELF`.

---

## Feature Limits (User) ✅

CRUD de límites de feature por usuario. Los límites controlan cuotas de uso por feature (ej. búsquedas por día, documentos subidos por mes).

### GET /v1/dashboard/users/:id/feature-limits — Listar Límites

Lista todos los límites de feature configurados para un usuario específico.

**Permisos requeridos:** `users:read` (grupo base)

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `id` | UUID | Sí | ID del usuario |

```bash
curl -X GET "http://localhost:8080/v1/dashboard/users/0193c8c6-1234-7abc-8def-0123456789ab/feature-limits" \
  -H "Accept: application/json" \
  -b cookies.txt -c cookies.txt
```

#### 200 OK

```json
{
  "limits": [
    {
      "feature_key": "daily_searches",
      "limit_value": 50,
      "window": "day"
    },
    {
      "feature_key": "monthly_documents",
      "limit_value": 20,
      "window": "month"
    }
  ]
}
```

### POST /v1/dashboard/users/:id/feature-limits — Crear/Actualizar Límite

Crea un nuevo límite de feature o actualiza uno existente para un usuario.

**Permisos requeridos:** `users:read` (grupo base) + `feature_limits:write` (aditivo)

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `feature_key` | string | Sí | No vacío | Identificador del feature (ej. `daily_searches`) |
| `limit_value` | *int | No | — | `nil` = ilimitado, `0` = bloqueado, `>0` = cuota |
| `window` | string | No | — | Ventana de tiempo: `"minute"`, `"hour"`, `"day"`, `"month"` |

```bash
curl -X POST "http://localhost:8080/v1/dashboard/users/0193c8c6-1234-7abc-8def-0123456789ab/feature-limits" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -b cookies.txt -c cookies.txt \
  -d '{"feature_key": "daily_searches", "limit_value": 100, "window": "day"}'
```

#### 201 Created — Límite nuevo

Si el `feature_key` no existía para este usuario, se crea y retorna `201`.

```json
{
  "feature_key": "daily_searches",
  "limit_value": 100,
  "window": "day"
}
```

#### 200 OK — Límite actualizado

Si el `feature_key` ya existía, se actualiza y retorna `200`.

```json
{
  "feature_key": "daily_searches",
  "limit_value": 100,
  "window": "day"
}
```

> **Nota:** El usecase verifica si PostgreSQL realizó un `INSERT` o un `UPDATE` (ej. vía `RowsAffected` + `ON CONFLICT`). El handler elige `201` o `200` según corresponda.

### DELETE /v1/dashboard/users/:id/feature-limits/:key — Eliminar Límite

Elimina un límite de feature de un usuario.

**Permisos requeridos:** `users:read` (grupo base) + `feature_limits:write` (aditivo)

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `id` | UUID | Sí | ID del usuario |
| `key` | string | Sí | Feature key a eliminar |

```bash
curl -X DELETE "http://localhost:8080/v1/dashboard/users/0193c8c6-1234-7abc-8def-0123456789ab/feature-limits/daily_searches" \
  -b cookies.txt -c cookies.txt
```

#### 204 No Content

Sin cuerpo de respuesta.

### Posibles Errores (User Limits)

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie ausente, token inválido o expirado |
| `TOKEN_EXPIRED` | 401 | `unauthorized` | Token PASETO expirado |
| `NOT_AUTHENTICATED` | 401 | `unauthorized` | No hay token de acceso en la cookie |
| `PERMISSION_DENIED` | 403 | `forbidden` | Falta `users:read` o `feature_limits:write` (en POST/DELETE) |
| `USER_NOT_FOUND` | 404 | `not-found` | El `:id` no existe en la DB |
| `INVALID_INPUT` | 400 | `invalid-input` | UUID inválido, `feature_key` faltante en POST |
| `FEATURE_LIMIT_ALREADY_EXISTS` | 409 | `conflict` | POST con `feature_key` que ya existe para este usuario |
| `FEATURE_LIMIT_NOT_FOUND` | 404 | `not-found` | DELETE con `:key` que no existe |
| `NOT_IMPLEMENTED` | 500 | `internal-error` | Funcionalidad aún no implementada (reservado para features futuras) |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado del servidor |

---

## Document Verification

Verificación administrativa de documentos (OCR, validación manual). Permite a un admin consultar el estado de verificación, aprobar/rechazar documentos, y disparar reprocesamiento OCR.

### GET /v1/dashboard/documents/:id/verification — Consultar Verificación

Obtiene el estado de verificación de un documento y su historial completo de cambios.

**Permisos requeridos:** `users:read` (grupo base)

#### Request

```
GET /v1/dashboard/documents/:id/verification
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `id` | UUID | Sí | ID del documento |

**Ejemplo:**

```bash
curl -X GET "http://localhost:8080/v1/dashboard/documents/0193c8c6-1234-7abc-8def-0123456789ab/verification" \
  -H "Accept: application/json" \
  -b cookies.txt -c cookies.txt
```

#### Responses

##### 200 OK

```json
{
  "document_id": "0193c8c6-1234-7abc-8def-0123456789ab",
  "status": "verified",
  "verified_by": "0193c8c6-1234-7abc-8def-0123456789cd",
  "verified_at": "2026-05-01T10:35:00Z",
  "history": [
    {
      "previous_status": "pending",
      "new_status": "verified",
      "verified_by": "0193c8c6-1234-7abc-8def-0123456789cd",
      "reason": "MRZ válido verificado",
      "changed_at": "2026-05-01T10:35:00Z"
    }
  ]
}
```

**Campos de respuesta:**

| Campo | Tipo | JSON key | Descripción |
|-------|------|----------|-------------|
| `DocumentID` | UUID | `document_id` | ID del documento |
| `Status` | string | `status` | Estado actual: `pending`, `verified`, `rejected`, `manual_review`, `suspicious` |
| `VerifiedBy` | *UUID | `verified_by` | Admin que verificó (omitzero si nunca fue verificado) |
| `VerifiedAt` | *datetime | `verified_at` | Fecha de verificación (omitzero si nunca fue verificado) |
| `History` | array | `history` | Historial completo de cambios de estado |

**Campos de `history[]`:**

| Campo | Tipo | JSON key | Descripción |
|-------|------|----------|-------------|
| `PreviousStatus` | string | `previous_status` | Estado anterior al cambio |
| `NewStatus` | string | `new_status` | Nuevo estado aplicado |
| `VerifiedBy` | UUID | `verified_by` | Admin que ejecutó el cambio |
| `Reason` | string | `reason` | Motivo del cambio |
| `ChangedAt` | datetime | `changed_at` | Timestamp del cambio (ISO 8601) |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie ausente, token inválido o expirado |
| `TOKEN_EXPIRED` | 401 | `unauthorized` | Token PASETO expirado |
| `NOT_AUTHENTICATED` | 401 | `unauthorized` | No hay token de acceso en la cookie |
| `PERMISSION_DENIED` | 403 | `forbidden` | El usuario no tiene `users:read` |
| `DOCUMENT_NOT_FOUND` | 404 | `not-found` | El `:id` no corresponde a un documento existente |
| `INVALID_INPUT` | 400 | `invalid-input` | El `:id` no es un UUID válido |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado del servidor |

---

### PATCH /v1/dashboard/documents/:id/verification — Actualizar Verificación

Actualiza el estado de verificación de un documento. El admin puede aprobar, rechazar o marcar para revisión manual.

**Permisos requeridos:** `users:read` (grupo base) + `users:write` (aditivo)

#### Request

```
PATCH /v1/dashboard/documents/:id/verification
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `id` | UUID | Sí | ID del documento |

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `status` | string | Sí | `verified`, `rejected`, `manual_review`, `suspicious` | Nuevo estado de verificación |
| `reason` | string | No | Máximo 500 caracteres | Motivo del cambio |

> **Nota:** `verified_by` se asigna automáticamente desde el `actorID` del token PASETO del admin autenticado. No se envía en el body.

**Ejemplo:**

```bash
curl -X PATCH "http://localhost:8080/v1/dashboard/documents/0193c8c6-1234-7abc-8def-0123456789ab/verification" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -b cookies.txt -c cookies.txt \
  -d '{"status": "verified", "reason": "MRZ válido verificado"}'
```

#### Responses

##### 200 OK

```json
{
  "document_id": "0193c8c6-1234-7abc-8def-0123456789ab",
  "status": "verified",
  "message": "Verificación actualizada correctamente."
}
```

**Campos de respuesta:**

| Campo | Tipo | JSON key | Descripción |
|-------|------|----------|-------------|
| `DocumentID` | UUID | `document_id` | ID del documento |
| `Status` | string | `status` | Nuevo estado aplicado |
| `Message` | string | `message` | Mensaje de confirmación |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie ausente, token inválido o expirado |
| `TOKEN_EXPIRED` | 401 | `unauthorized` | Token PASETO expirado |
| `NOT_AUTHENTICATED` | 401 | `unauthorized` | No hay token de acceso en la cookie |
| `PERMISSION_DENIED` | 403 | `forbidden` | El usuario no tiene `users:read` + `users:write` |
| `DOCUMENT_NOT_FOUND` | 404 | `not-found` | El `:id` no existe |
| `INVALID_INPUT` | 400 | `invalid-input` | UUID inválido o `status` faltante |
| `VALIDATION_ERROR` | 400 | `bad-request` | `status` no es un valor permitido |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado del servidor |

---

### POST /v1/dashboard/documents/:id/reprocess — Reprocesar Documento

Re-ejecuta el pipeline OCR para un documento específico. Útil cuando el OCR falló o produjo datos incorrectos.

**Permisos requeridos:** `users:read` (grupo base) + `users:write` (aditivo)

#### Request

```
POST /v1/dashboard/documents/:id/reprocess
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `id` | UUID | Sí | ID del documento |

**Ejemplo:**

```bash
curl -X POST "http://localhost:8080/v1/dashboard/documents/0193c8c6-1234-7abc-8def-0123456789ab/reprocess" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -b cookies.txt -c cookies.txt \
  -d '{}'
```

#### Responses

##### 202 Accepted

```json
{
  "document_id": "0193c8c6-1234-7abc-8def-0123456789ab",
  "status": "queued",
  "message": "Reprocesamiento iniciado. El documento volverá a pasar por el pipeline OCR."
}
```

**Campos de respuesta:**

| Campo | Tipo | JSON key | Descripción |
|-------|------|----------|-------------|
| `DocumentID` | UUID | `document_id` | ID del documento |
| `Status` | string | `status` | `queued` — encolado para reprocesamiento |
| `Message` | string | `message` | Mensaje de confirmación |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie ausente, token inválido o expirado |
| `TOKEN_EXPIRED` | 401 | `unauthorized` | Token PASETO expirado |
| `NOT_AUTHENTICATED` | 401 | `unauthorized` | No hay token de acceso en la cookie |
| `PERMISSION_DENIED` | 403 | `forbidden` | El usuario no tiene `users:read` + `users:write` |
| `DOCUMENT_NOT_FOUND` | 404 | `not-found` | El `:id` no existe |
| `INVALID_INPUT` | 400 | `invalid-input` | UUID inválido |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado del servidor |

---

## Configuración CORS

Configuración aplicada globalmente a todas las rutas (incluyendo `/v1/dashboard`). Definida en `internal/bootstrap/app.go:151-164`.

| Configuración | Valor |
|---------------|-------|
| **Orígenes permitidos** | `https://proactrip.com` (prod) / `http://localhost:3000` (dev) |
| **Credentials** | `true` |
| **Métodos** | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS` |
| **Headers** | `Content-Type`, `Accept`, `Authorization`, `X-Request-Id`, `Idempotency-Key`, `X-Trace-Id` |
| **Max Age** | 86400 segundos (24 horas) |

> **NUNCA se usa wildcard (`*`).** `Access-Control-Allow-Origin: *` es incompatible con `Access-Control-Allow-Credentials: true`. El origen se determina dinámicamente vía `cfg.Frontend.GetURL()` según `SERVER_ENV`.

---

## Rate Limiting

Rate limiting implementado con DragonflyDB (compatible Redis) usando scripts Lua. Tres tiers, todos implementados.

### Tiers

| Tier | Límite | Key | Estado | Headers |
|------|--------|-----|--------|---------|
| **Global** | 100 req/min | IP del cliente | ✅ Implementado | `RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset` |
| **Authenticated** | 10 req/min | User UUID (extraído del PASETO) | ✅ Implementado | `RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset` |
| **Admin** | 30 req/min | User UUID | ✅ Implementado | `RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset` |

> **Nota sobre Tier 3 (Admin):** El tier de 30 req/min para administradores reemplaza al tier Authenticated en el grupo `/v1/dashboard`. Configurable vía `RATELIMIT_ADMIN_PER_MINUTE` (default: 30). Los administradores también pasan por el tier Global (100 req/min por IP).

### Headers de Rate Limit

| Header | Descripción |
|--------|-------------|
| `RateLimit-Limit` | Límite máximo de requests en la ventana |
| `RateLimit-Remaining` | Requests restantes en la ventana actual |
| `RateLimit-Reset` | Segundos hasta que se reinicie la ventana |
| `Retry-After` | Solo en `429 Too Many Requests` — segundos para reintentar |

### Respuesta 429 Too Many Requests

```json
{
  "type": "https://api.proactrip.com/errors/rate-limit-exceeded",
  "title": "Too Many Requests",
  "status": 429,
  "detail": "rate limit exceeded: 11/10, retry after 45s",
  "instance": "/v1/dashboard/users",
  "trace_id": "0193c8c6-9abc-7def-0123-456789abcdef"
}
```

---

## Cache

Los endpoints del dashboard **no emiten headers `Cache-Control`**. El comportamiento efectivo es `no-store, private` — los proxies y browsers no cachean ninguna respuesta del dashboard.

| Endpoint | Cache-Control | Motivo |
|----------|---------------|--------|
| Todos (`/v1/dashboard/*`) | `no-store, private` (efectivo) | Sin middleware de caché — los datos administrativos son sensibles y cambian con frecuencia |

---

## Notas de Seguridad

### Autenticación
- **PASETO v4.local**: tokens simétricos con cifrado AEAD (XChaCha20-Poly1305).
- **Cookies HttpOnly**: `__Secure-access_token` y `__Secure-refresh_token` no son accesibles desde JavaScript (previene XSS).
- **SameSite=Lax**: protección contra CSRF en navegación cross-site. El prefijo `__Secure-` requiere `Secure` y HTTPS.
- **Rotación silenciosa**: el middleware de auth rota el access token automáticamente en el último 25% de su TTL (900s → rota a los 675s).

### Autorización
- **Basada en permisos, no en roles**: el middleware `RequirePermission` verifica permisos individuales (`users:read`, `users:write`, etc.), no consulta si el usuario tiene rol `admin`.
- **Admin como rol privilegiado**: el rol `admin` existe en base de datos como una agrupación de los 5 permisos. Cuando un admin se autentica, su token PASETO incluye todos los permisos, por lo que cualquier `RequirePermission` pasa. No hay un middleware `RequireRole("admin")`.
- **Default deny**: cualquier request a `/v1/dashboard/*` sin los permisos requeridos es rechazado con `403`. Solo el rol `admin` posee estos permisos en la configuración actual.
- **Granularidad futura**: los permisos individuales permitirán acceso parcial a otros roles en el futuro sin cambiar el middleware — solo se modifica la asignación de permisos en base de datos.
- **Defense in depth**: los permisos se verifican tanto en el middleware (`RequirePermission`) como en el usecase (validación de dominio).
- **Actor tracking**: el `actorID` se extrae del token PASETO en cada request de mutación. Se usa para la validación `CANNOT_DISABLE_SELF` (account status).

### Headers de Seguridad
El middleware `SecurityHeaders()` (aplicado globalmente en `app.go:167`) agrega:
- `Content-Security-Policy`
- `Strict-Transport-Security` (HSTS)
- `X-Frame-Options`
- `X-Content-Type-Options: nosniff`
- `Referrer-Policy`

### Rate Limiting
- **DragonflyDB Lua**: los contadores de rate limit se almacenan en DragonflyDB con scripts Lua atómicos.
- **Doble capa**: cada request al dashboard pasa por Global (IP) + Authenticated (user UUID). Si cualquiera de los dos excede el límite, se devuelve `429`.

### Trazabilidad
- Cada respuesta (éxito o error) incluye `X-Trace-Id` (UUID v7).
- Los errores RFC 9457 incluyen `trace_id` en el body JSON + `instance` con el path del endpoint.
- W3C Trace Context (`traceparent`) generado por middleware de tracing.
