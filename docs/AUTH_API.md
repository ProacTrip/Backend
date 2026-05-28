# Auth Module API Documentation (Cookie-Based)

> **Arquitectura:** Cookie-based authentication con HttpOnly cookies. El frontend nunca manipula tokens.
> Auth endpoints devuelven identidad mínima (`id`, `email`, `role_name`). Datos completos del usuario
> se obtienen vía `GET /v1/user/profile` y preferencias de viaje vía `GET /v1/user/travel-preferences`.
> Ver [USER_API](./USER_API.md).

---

## Índice

| Sección | Estado |
|----------|--------|
| [Register](#register) | ✅ Implementado |
| [Resend Verification Email](#resend-verification-email) | ✅ Implementado |
| [Verify Email](#verify-email) | ✅ Implementado |
| [Login](#login) | ✅ Implementado |
| [Logout](#logout) | ✅ Implementado |
| [OAuth Google](#oauth-google) | ✅ Implementado |
| [OAuth Google Callback](#oauth-google-callback) | ✅ Implementado |
---

## Arquitectura

### Flujo de Autenticación por Email

```
┌──────────┐  POST /register            ┌──────────┐  evento: auth.user.registered
│ Frontend │ ──────────────────────────> │   Auth   │ ────────────────────────────> Módulo Notificaciones
└──────────┘  {email, password, name}    └──────────┘                                (envía email verificación)
                                                 │
                                                 │  evento: auth.user.registered
                                                 │  ────────────────────────────> Módulo User
                                                 │                                  (crea perfil con first_name y email)
                                                 │
  (usuario hace clic en enlace)                  │
┌──────────┐  POST /verify-email          ┌──────────┐  evento: auth.user.verified
│ Frontend │ ──────────────────────────>  │   Auth   │ ────────────────────────────> Módulo User
└──────────┘  {token}                     └──────────┘                                (setea preferencias:
       │                                         │                                     language de Accept-Language header)
       │  200 { user: { id, email,               │                                     
       │          role_name } }                  │
       │  Set-Cookie: access_token               │
       │  Set-Cookie: refresh_token              │
       └─────────────────────────────────────────┘
```

### Flujo OAuth (Google)

```
┌──────────┐  GET /oauth/google     ┌──────────┐
│ Frontend │ ──────────────────────> │   Auth   │
└──────────┘  { auth_url }          └──────────┘
     │                                    │
     │  window.location.href = auth_url   │
     ▼                                    │
┌──────────┐                              │
│  Google  │  (usuario autoriza)          │
└──────────┘                              │
     │  GET /oauth/google/callback        │
     │  ?code=xxx&state=yyy               │
     └───────────────────────────────────>│
                                          │  valida state, intercambia code,
                                          │  crea/vincula usuario, emite cookies.
                                          │  Si es nuevo → evento auth.user.registered
                                          │    └─> Módulo User (crea perfil con datos de Google)
                                          │
     302 → /auth/callback?status=success  │
     Set-Cookie: access_token             │
     Set-Cookie: refresh_token            │
┌──────────┐<─────────────────────────────│
│ Frontend │  GET /v1/user/profile       │  → session bootstrap
└──────────┘  GET /v1/realtime/events    │  → realtime sync
```

### Política de Cookies

| Cookie | Nombre | TTL | Propósito |
|--------|--------|-----|-----------|
| Access Token | `__Secure-access_token` | 15 min | Sesión activa |
| Refresh Token | `__Secure-refresh_token` | 7 días | Rotación de sesión |

> En desarrollo local (sin HTTPS), las cookies usan los nombres `access_token` y `refresh_token` sin el prefijo `__Secure-`.

---

## Seguridad de Cookies

### Atributos Obligatorios

| Atributo | Valor | Propósito |
|----------|-------|-----------|
| `HttpOnly` | `true` | Inaccesible vía JavaScript (mitiga XSS) |
| `Secure` | `true` | Solo HTTPS en producción |
| `SameSite` | `Lax` | Protección CSRF. Permite navegación top-level (OAuth callbacks) |
| `Path` | `/` | Disponible en todas las rutas |
| `Domain` | `.proactrip.com` | Compartido entre subdominios (solo en producción) |

### Formatos de Producción

```
Set-Cookie: __Secure-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900
Set-Cookie: __Secure-refresh_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800
```

### Limpieza de Cookies (Logout)

Además de `Max-Age=0`, el backend envía:

```
Clear-Site-Data: "cookies"
```

Esto fuerza al navegador a limpiar todas las cookies del origen inmediatamente (soportado por todos los navegadores modernos en 2026).

---

## Base URLs

| Entorno | Base URL |
|---------|----------|
| **Production** | `https://api.proactrip.com/v1/auth` |
| **Development** | `http://localhost:8080/v1/auth` |

Todos los ejemplos usan `{base_url}` como placeholder.

---

## Errores Estándar

Formato **RFC 9457 Problem Details**. Todas las respuestas de error usan `Content-Type: application/problem+json`.

```json
{
  "type": "https://api.proactrip.com/errors/validation-error",
  "title": "Validation Error",
  "status": 400,
  "detail": "Specific detail about what went wrong",
  "instance": "/v1/auth/register",
  "trace_id": "019d5439-cb43-716d-90b5-51dcbe980908"
}
```

**Headers de respuesta en TODOS los endpoints:**

| Header        | Descripción                                             |
| ------------- | ------------------------------------------------------- |
| `X-Trace-Id`  | UUID v7 para trazabilidad. Asignado globalmente por middleware, nunca por handlers individuales |
| `traceparent` | W3C Trace Context                                       |

---

## Estrategia de Refresco de Tokens

El backend maneja el refresco de tokens transparentemente vía middleware.

- Si `access_token` es válido → la petición continúa
- Si `access_token` está expirado pero `refresh_token` es válido → nuevos tokens emitidos
- Si ambos están expirados → 401 Unauthorized

---

## Session Bootstrap

El frontend reconstruye el estado del usuario al autenticarse o al recargar la página (F5 / SSR) con:

1. `GET /v1/user/profile` → identidad, avatar y configuración regional.
2. `GET /v1/user/travel-preferences` → preferencias de viaje.
3. `GET /v1/environment` → ubicación actual como placeholder en la UI.
4. `GET /v1/realtime/events` → conexión SSE para sincronización en tiempo real.

`GET /v1/auth/me` existe como endpoint de identidad ligera. Devuelve `id`, `email`, `role_name` y `permissions` (RBAC). Es usado por el frontend para conocer los permisos del usuario sin llamar al perfil completo.

---

## Register

Crea una nueva cuenta. El backend:

1. Obtiene la IP del cliente (de la conexión en producción o del header `X-Real-IP` en desarrollo).
2. Valida los datos de entrada.
3. Aplica rate limiting por IP.
4. Crea el usuario con email sin verificar.
5. Publica evento `auth.user.registered` con `first_name` y `email` para que:
   - El módulo de notificaciones envíe el email de verificación.
   - El módulo user cree el perfil inicial.
6. No existe sesión hasta verificación (sin cookies de acceso/refresh).

### Request

```
POST /v1/auth/register
```

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Content-Type` | string | Sí | `application/json` |
| `Idempotency-Key` | string | Sí | UUID v7. Previene registros duplicados por retries de red. El backend cachea la respuesta por 24h |
| `X-Real-IP` | string | No | IP del cliente (override de auto-detección). Útil para testing y desarrollo |

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `email` | string | Sí | Email válido | Correo del usuario |
| `password` | string | Sí | Mínimo 8 caracteres, al menos una mayúscula, una minúscula, un dígito y un carácter especial | Contraseña |
| `first_name` | string | Sí | Máximo 100 caracteres | Nombre del usuario. Se usa en el email de verificación y se guarda en el perfil |

**Ejemplo:**

```bash
curl -X POST {base_url}/register \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: 019d5439-cb43-716d-90b5-51dcbe980908" \
  -d '{"email":"user@example.com","password":"SecurePass123!","first_name":"María"}'
```

### Responses

#### 201 Created

```json
{
  "message": "Registro exitoso. Por favor verificá tu email."
}
```

#### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `ErrEmailAlreadyExists` | 409 | `conflict` | Email ya registrado |
| `ErrInvalidEmail` | 400 | `invalid-email` | Email vacío o con formato inválido |
| `ErrInvalidInput` | 400 | `invalid-input` | Faltan campos requeridos (`email`, `password` o `first_name`) |
| `ErrPasswordTooShort` | 400 | `weak-password` | Contraseña con menos de 8 caracteres |
| `ErrInvalidPassword` | 400 | `weak-password` | Contraseña no cumple los requisitos de complejidad |
| `ErrRoleNotFound` | 404 | `not-found` | Error de configuración del servidor |

> Errores de rate limiting (429 `rate-limit-exceeded`) e internos (500 `internal-error`) aplican a todos los endpoints. Son manejados por middleware global. Ver [Rate Limiting](#rate-limiting).

---

## Resend Verification Email

Solicita un nuevo email de verificación. Siempre retorna 200 para prevenir enumeración de usuarios.

### Request

```
POST /v1/auth/resend-verification
```

**Body:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | Sí | Email usado en el registro |

**Ejemplo:**

```bash
curl -X POST {base_url}/resend-verification \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com"}'
```

### Responses

#### 200 OK

```json
{
  "message": "Si el email existe y no está verificado, se enviará un nuevo email de verificación."
}
```

#### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `ErrInvalidEmail` | 400 | `invalid-email` | Email vacío o con formato inválido |
| `ErrInvalidInput` | 400 | `invalid-input` | Body malformado (JSON inválido) |

---

## Verify Email

Verifica el email usando el token del enlace. Este es el paso donde se activa la cuenta y se establecen las preferencias iniciales del perfil.

El backend:

1. Extrae language de Accept-Language header.
2. Valida el token de verificación.
3. Marca el email como verificado y activa la cuenta.
4. Resuelve defaults de entorno (language) desde Accept-Language header.
5. Publica evento `auth.user.verified` con los defaults para que el user module los persista como preferencias iniciales del perfil.
6. Genera tokens de sesión y los envía como cookies.

> **Nota:** El enlace del email apunta al frontend (`{FRONTEND_URL}/auth/verify-email?token=xxx`), no al backend. El frontend extrae el token de la URL y llama a este endpoint.

### Request

```
POST /v1/auth/verify-email
```

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Content-Type` | string | Sí | `application/json` |

**Body:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `token` | string | Sí | Token de verificación del email |

**Ejemplo:**

```bash
curl -X POST {base_url}/verify-email \
  -H "Content-Type: application/json" \
  -d '{"token":"verification-token-here"}'
```

### Responses

#### 200 OK

```json
{
  "user": {
    "id": "019d5439-cb43-716d-90b5-51dcbe980908",
    "email": "user@example.com",
    "role_name": "client"
  }
}
```

**Set-Cookie Headers:**

```
Set-Cookie: __Secure-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900
Set-Cookie: __Secure-refresh_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800
```

> Para obtener datos completos del usuario, el frontend debe llamar a `GET /v1/user/profile` (identidad y avatar) y `GET /v1/user/travel-preferences` (preferencias de viaje).

#### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `ErrInvalidInput` | 400 | `invalid-input` | Falta el campo `token` |
| `ErrTokenInvalid` | 401 | `unauthorized` | Token inválido, expirado o malformado |
| `ErrUserNotFound` | 404 | `not-found` | El email del token no coincide con ningún usuario |

---

## Login

Autentica con email y password. El backend valida credenciales, verifica el estado de la cuenta y emite cookies de sesión.

> **Frontend:** Después del login, el frontend debería:
> 1. Llamar a `GET /v1/user/profile` para obtener identidad y avatar.
> 2. Llamar a `GET /v1/user/travel-preferences` para preferencias de viaje.
> 3. Llamar a `GET /v1/environment` para obtener la ubicación actual como placeholder en la UI. Ver [ENVIRONMENT_API](./ENVIRONMENT_API.md).

### Request

```
POST /v1/auth/login
```

**Body:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | Sí | Email del usuario |
| `password` | string | Sí | Contraseña |

**Ejemplo:**

```bash
curl -X POST {base_url}/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"SecurePass123!"}'
```

### Responses

#### 200 OK 

```json
{
  "user": {
    "id": "019d5439-cb43-716d-90b5-51dcbe980908",
    "email": "user@example.com",
    "role_name": "client"
  }
}
```

**Set-Cookie Headers:**

```
Set-Cookie: __Secure-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900
Set-Cookie: __Secure-refresh_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800
```

#### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `ErrInvalidCredentials` | 401 | `unauthorized` | Email o password incorrectos |
| `ErrEmailNotVerified` | 401 | `unauthorized` | Email no verificado |
| `ErrAccountLocked` | 429 | `rate-limit-exceeded` | Cuenta bloqueada por demasiados intentos fallidos |
| `ErrAccountSuspended` | 403 | `forbidden` | Cuenta suspendida |
| `ErrAccountInactive` | 403 | `forbidden` | Cuenta inactiva o deshabilitada |
| `ErrInvalidEmail` | 400 | `invalid-email` | Email vacío o con formato inválido |
| `ErrInvalidInput` | 400 | `invalid-input` | Falta email o password |
| `ErrPasswordTooShort` | 400 | `weak-password` | Contraseña con menos de 8 caracteres |

---

## Logout

Revoca la sesión actual. El backend lee las cookies automáticamente — no se pasa ningún token en el body.

### Request

```
POST /v1/auth/logout
```

> El navegador envía las cookies automáticamente. No enviar body ni headers adicionales.

**Ejemplo:**

```bash
curl -X POST {base_url}/logout \
  -c cookies.txt -b cookies.txt
```

### Responses

#### 200 OK

```json
{
  "message": "Sesión cerrada correctamente."
}
```

**Headers de limpieza:**

```
Set-Cookie: __Secure-access_token=; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=0
Set-Cookie: __Secure-refresh_token=; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=0
Clear-Site-Data: "cookies"
```

#### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `ErrNotAuthenticated` | 401 | `unauthorized` | No hay cookie de sesión (rechazado por middleware de auth) |
| `ErrTokenInvalid` | 401 | `unauthorized` | Token inválido o expirado (rechazado por middleware de auth) |

---

## OAuth Google

Inicia el flujo de autenticación OAuth con Google. El frontend llama a este endpoint, obtiene la `auth_url` y redirige al usuario.

### Request

```
GET /v1/auth/oauth/:provider
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `provider` | string | Sí | Proveedor OAuth. Actualmente solo `google` |

**Ejemplo:**

```bash
curl -X GET {base_url}/oauth/google
```

### Responses

#### 200 OK

```json
{
  "auth_url": "https://accounts.google.com/o/oauth2/v2/auth?..."
}
```

> **Flujo:** El frontend redirige al navegador con `window.location.href = data.auth_url`. El backend genera un `state` anti-CSRF one-time en cada llamada — **no cachear esta respuesta**.

#### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `ErrOAuthProviderNotFound` | 400 | `bad-request` | Proveedor no soportado (ej: `facebook`) |

---

## OAuth Google Callback

Callback llamado por Google después de que el usuario autoriza. **Este endpoint no debe ser llamado directamente por el frontend** — el navegador sigue la redirección automáticamente desde Google.

El backend:

1. Valida el `state` anti-CSRF (one-time, se elimina del cache al validarse).
2. Intercambia el `code` por tokens con Google.
3. Crea el usuario si es la primera vez, o vincula la cuenta si ya existe.
4. Si es un **nuevo usuario**: publica el evento `auth.user.registered` para que el módulo user cree el perfil inicial con given_name, family_name, locale(para language) y avatar de google.
5. Emite cookies de sesión y redirige al frontend.

> **Frontend:** Después del redirect exitoso (`status=success`):
> 1. Llamar a `GET /v1/user/profile` para obtener identidad y avatar.
> 2. Llamar a `GET /v1/user/travel-preferences` para preferencias de viaje.
> 3. Llamar a `GET /v1/environment` para ubicación actual como placeholder en la UI.
>
> Ver [ENVIRONMENT_API](./ENVIRONMENT_API.md).

### Flujo Completo

```
┌──────────┐   GET /oauth/:provider    ┌──────────┐
│ Frontend │ ────────────────────────> │ Backend  │
└──────────┘                           └──────────┘
     │   { auth_url }                       │
     │<─────────────────────────────────────│
     │                                      │
     │   window.location.href = auth_url    │
     ▼                                      │
┌──────────┐                               │
│  Google  │ (usuario autoriza)            │
└──────────┘                               │
     │  GET /oauth/google/callback          │
     │  ?code=xxx&state=yyy                 │
     └─────────────────────────────────────>│
                                            │ (valida state, intercambia código,
                                            │  crea/vincula usuario, genera tokens.
                                            │  Si es nuevo → evento auth.user.registered)
                                            │
     302 → /auth/callback?status=success    │
     Set-Cookie: __Secure-access_token      │
     Set-Cookie: __Secure-refresh_token     │
┌──────────┐<───────────────────────────────│
│ Frontend │  GET /v1/user/profile         │  → session bootstrap
│          │  GET /v1/environment          │  → ubicación placeholder
└──────────┘
```

### Request

```
GET /v1/auth/oauth/:provider/callback
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `provider` | string | Sí | Proveedor OAuth. Debe coincidir con el usado en `/oauth/:provider` |

**Query Params (enviados por Google, no por el frontend):**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `code` | string | Sí | Código de autorización de Google |
| `state` | string | Sí | Token anti-CSRF generado por el backend en `/oauth/:provider` |

### Responses

#### 302 Found — Éxito

```
Location: {FRONTEND_URL}/auth/callback?status=success
```

**Set-Cookie Headers:**

```
Set-Cookie: __Secure-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900
Set-Cookie: __Secure-refresh_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800
```

#### 302 Found — Error

```
Location: {FRONTEND_URL}/auth/callback?status=error&code=OAUTH_EXCHANGE_FAILED
```

> El frontend lee `status` y `code` de los query params para mostrar el error adecuado. Si `status=success`, las cookies ya están disponibles.

#### Posibles Errores (vía redirect)

Todos los errores se devuelven como `302 Found` con `status=error&code=XXX`. El frontend nunca recibe un JSON de error en este endpoint.

| Código | HTTP | Problem Type (interno) | Cuándo |
|--------|------|------------------------|--------|
| `OAUTH_CODE_MISSING` | 302 | `bad-request` | Falta el parámetro `code` en el callback |
| `OAUTH_STATE_MISSING` | 302 | `bad-request` | Falta el parámetro `state` en el callback |
| `OAUTH_STATE_INVALID` | 302 | `bad-request` | State inválido, expirado o reutilizado (posible CSRF) |
| `OAUTH_ACCESS_DENIED` | 302 | `bad-request` | Usuario denegó el acceso en Google |
| `OAUTH_EXCHANGE_FAILED` | 302 | `unauthorized` | Error al intercambiar código con Google |
| `OAUTH_PROVIDER_NOT_FOUND` | 302 | `bad-request` | Proveedor no soportado |
| `OAUTH_EXCHANGE_FAILED` | 302 | `unauthorized` | Email de Google no verificado |
| `OAUTH_EXCHANGE_FAILED` | 302 | `rate-limit-exceeded` | Cuenta bloqueada |
| `OAUTH_EXCHANGE_FAILED` | 302 | `forbidden` | Cuenta suspendida |
| `OAUTH_EXCHANGE_FAILED` | 302 | `forbidden` | Cuenta deshabilitada |

> La columna HTTP muestra `302` porque todos los errores son redirects. La columna Problem Type indica el tipo RFC 9457 que se usaría si el error fuera devuelto como JSON (valor interno de referencia).

---

## Configuración CORS

Configuración global aplicada a todos los endpoints. El origen permitido se resuelve dinámicamente desde la configuración del servidor (`FRONTEND_URL_DEV` o `FRONTEND_URL_PROD` según `SERVER_ENV`).

| Setting | Valor |
|---------|-------|
| Allowed Origins | Un solo origen, resuelto dinámicamente: `http://localhost:3000` (dev) o `https://proactrip.com` (prod) |
| Allowed Methods | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS` |
| Allowed Headers | `Content-Type`, `Accept`, `Authorization`, `X-Request-Id`, `X-Trace-Id`, `Idempotency-Key` |
| Allow Credentials | `true` |
| Max Age | `86400` (24h) |

> **Crítico:** Con `AllowCredentials: true`, el spec CORS exige UN solo origen explícito. Nunca se usa `Access-Control-Allow-Origin: *`.

---

## Rate Limiting

Rate limiting multi-tier con DragonflyDB y scripts Lua atómicos. Distribuido y seguro en entornos multi-instancia. Todos los límites son configurables vía variables de entorno.

### Tiers

| Tier | Scope | Límite | Aplica a |
|------|-------|--------|----------|
| **Tier 1 — Global** | IP | 100 req/min | Todos los endpoints (escudo anti-DDoS) |
| **Tier 2 — Authenticated** | UUID del usuario | 10 req/min | Endpoints protegidos con auth |
| **Tier 3 — Anonymous** | Cookie `__Secure-anon_token` | 5 req/min | Endpoints públicos sin autenticación |

### Provider-Aware Rate Limiting

| Proveedor | Límite | Descripción |
|-----------|--------|-------------|
| Resend (email) | 100/día | Límite del plan de Resend. Se aplica por IP |
| SerpAPI | 50/hora | Límite por IP para llamadas al proveedor externo de búsqueda |

### Cookie Anónima (`__Secure-anon_token`)

Para endpoints públicos donde no hay sesión de usuario, el backend establece una cookie anónima con UUID v7 para rate limiting:

```
Set-Cookie: __Secure-anon_token=019d5439-cb43-716d-90b5-51dcbe980908; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=315360000
```

| Atributo | Valor | Propósito |
|----------|-------|-----------|
| Nombre | `__Secure-anon_token` | Identificador anónimo para rate limiting |
| TTL | 10 años (`Max-Age=315360000`) | Persiste entre sesiones del navegador para rate limiting consistente |
| `HttpOnly` | `true` | Inaccesible vía JavaScript |
| `Secure` | `true` | Solo HTTPS en producción |
| `SameSite` | `Lax` | Se envía en navegación top-level |

> El frontend no necesita hacer nada con esta cookie. El navegador la envía automáticamente. Si la cookie no existe, el backend la establece en la primera respuesta.

### Response en 429

```json
{
  "type": "https://api.proactrip.com/errors/rate-limit-exceeded",
  "title": "Too Many Requests",
  "status": 429,
  "detail": "Demasiadas peticiones. Esperá 60 segundos antes de reintentar.",
  "instance": "/v1/auth/login",
  "trace_id": "019d5439-cb43-716d-90b5-51dcbe980908"
}
```

### Rate Limit Headers

| Header | Descripción |
|--------|-------------|
| `RateLimit-Limit` | Máximo permitido en la ventana actual |
| `RateLimit-Remaining` | Peticiones restantes en la ventana actual |
| `RateLimit-Reset` | Segundos hasta que se reinicia la ventana |
| `Retry-After` | Segundos a esperar antes de reintentar (solo en respuestas 429) |

---

## Cache

Endpoints de auth que retornan datos sensibles usan:

```
Cache-Control: no-store, private
```

Esto previene almacenamiento en caches compartidos o del navegador.

| Endpoint | Cache-Control | Motivo |
|----------|---------------|--------|
| `POST /v1/auth/register` | `no-store, private` | Datos de registro sensibles |
| `POST /v1/auth/resend-verification` | `no-store, private` | Anti-enumeración |
| `POST /v1/auth/verify-email` | `no-store, private` | Datos de usuario + emisión de sesión |
| `POST /v1/auth/login` | `no-store, private` | Datos de sesión sensibles |
| `POST /v1/auth/logout` | `no-store, private` | Invalidación de sesión |
| `GET /v1/auth/oauth/:provider` | `no-store` | State anti-CSRF one-time — nunca cachear |

---

## Notas de Seguridad

### Tokens PASETO v4

Todos los tokens internos son **PASETO v4 symmetric**. Son opacos para el cliente — el frontend nunca los lee ni los decodifica.

| Token | TTL | Propósito |
|-------|-----|-----------|
| `access_token` | 15 min | Autenticar requests |
| `refresh_token` | 7 días | Rotación de sesión |
| Email verification | 24 horas | Verificar email y activar cuenta |
| Password reset | 1 hora | Reset de contraseña |
| OAuth `state` | 5-10 min | Anti-CSRF OAuth (one-time) |

### Hashing de Contraseñas

**Argon2id** con parámetros recomendados por OWASP.

### Rotación de Refresh Tokens

Cada vez que el middleware refresca un `access_token`, rota también el `refresh_token`. Si un `refresh_token` revocado es reutilizado, se invalida la sesión del usuario (detección de robo de sesión).

### Flujo de Eventos y Preferencias

El módulo auth publica eventos para que otros módulos actúen. Los datos de environment se resuelven en momentos específicos:

| Evento | Publicado en | Datos incluidos | Consumidor |
|--------|-------------|-----------------|------------|
| `auth.user.registered` | Register + OAuth (nuevo usuario) | `user_id`, `email`, `first_name` | Módulo Notificaciones (envía email). Módulo User (crea perfil inicial). |
| `auth.user.verified` | Verify Email (primera verificación) | `user_id`, `email`, `language_code` | Módulo User (setea preferencias iniciales desde Accept-Language header). |

Las preferencia (language) se establece **una sola vez** durante la verificación del email, usando Accept-Language header. Para OAuth, el perfil se crea con los datos disponibles del proveedor (given_name, avatar_url, family_name, locale) y las preferencias se configuran posteriormente.

### OAuth PKCE

El backend genera y almacena el `code_verifier`. El `state` es one-time: se valida y se elimina del cache inmediatamente para prevenir replay attacks.

### Prevención de Ataques

| Amenaza | Mitigación |
|---------|------------|
| XSS | `HttpOnly` cookies + CSP |
| CSRF | `SameSite=Lax` + cookies automáticas |
| Enumeración de usuarios | `POST /resend-verification` siempre retorna 200 |
| Replay de refresh | Rotación continua + invalidación total ante reúso |
| OAuth CSRF | State anti-CSRF one-time eliminado del cache al validarse |
| Rate limiting abuse | Multi-tier con DragonflyDB + Lua scripts atómicos (IP, usuario autenticado, cookie anónima) |

### Compartición de Cookies entre Subdominios

Las cookies usan `Domain=.proactrip.com` con `SameSite=Lax`, lo que las comparte entre `api.proactrip.com` y `app.proactrip.com`. No se usa `Partitioned` (CHIPS) porque está diseñado para iframes cross-site, no para subdominios propios.

### Headers de Seguridad

Todas las respuestas incluyen:

```
Content-Security-Policy: default-src 'self'
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
Strict-Transport-Security: max-age=31536000
```