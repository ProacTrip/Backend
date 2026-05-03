# Auth Module API Documentation (Cookie-Based)

> **Arquitectura:** Cookie-based authentication con HttpOnly cookies. El frontend nunca manipula tokens.  
> **Migración:** Desde v1 (token-based), ver [Apéndice de Migración](#apéndice-migración-desde-v1).

---

## Índice

- [Arquitectura](#arquitectura)
- [Seguridad de Cookies](#seguridad-de-cookies)
- [Base URLs](#base-urls)
- [Errores Estándar](#errores-estándar)
- [Register](#register)
- [Resend Verification Email](#resend-verification-email)
- [Verify Email](#verify-email)
- [Login](#login)
- [Login MFA](#login-mfa)
- [Logout](#logout)
- [Logout All Sessions](#logout-all-sessions)
- [Refresh Token](#refresh-token)
- [Change Password](#change-password)
- [Forgot Password](#forgot-password)
- [Reset Password](#reset-password)
- [OAuth Google](#oauth-google)
- [OAuth Google Callback](#oauth-google-callback)
- [Current User (Me)](#current-user-me)
- [MFA — List Active Methods](#mfa--list-active-methods)
- [MFA — Setup TOTP](#mfa--setup-totp)
- [MFA — Verify TOTP Setup](#mfa--verify-totp-setup)
- [MFA — Setup Email](#mfa--setup-email)
- [MFA — Verify Email Setup](#mfa--verify-email-setup)
- [MFA — Setup SMS](#mfa--setup-sms)
- [MFA — Verify SMS Setup](#mfa--verify-sms-setup)
- [MFA — Disable Method](#mfa--disable-method)
- [MFA — Disable All Methods](#mfa--disable-all-methods)
- [SSE Token](#sse-token)
- [Configuración CORS](#configuración-cors)
- [Rate Limiting](#rate-limiting)
- [Cache](#cache)
- [Apéndice: Migración desde v1](#apéndice-migración-desde-v1)
- [Notas de Seguridad](#notas-de-seguridad)

---

## Arquitectura

### Flujo de Autenticación

```
┌─────────────┐       POST /login       ┌─────────────┐
│   Browser   │ ──────────────────────> │   Backend   │
│  (Frontend) │    {email, password}    │             │
└─────────────┘                         └─────────────┘
^                                                     │
│         Set-Cookie: __Secure-access_token=...       │
│         Set-Cookie: __Secure-refresh_token=...      │
└─────────────────────────────────────────────────────┘
Las cookies se envían AUTOMÁTICAMENTE en cada request subsiguiente.
El frontend NO almacena ni lee tokens.
```

### Política de Cookies

| Cookie | Nombre | TTL | Propósito |
|--------|--------|-----|-----------|
| Access Token | `__Secure-access_token` | 15 min | Sesión activa |
| Refresh Token | `__Secure-refresh_token` | 7 días | Rotación de sesión |

> En despliegues single-domain sin subdominios cruzados, usar `__Host-access_token` y `__Host-refresh_token` para máxima seguridad (impide el atributo `Domain`).

---

## Seguridad de Cookies

### Atributos Obligatorios

| Atributo | Valor | Propósito |
|----------|-------|-----------|
| `HttpOnly` | `true` | Inaccesible vía JavaScript (mitiga XSS) |
| `Secure` | `true` | Solo HTTPS en producción |
| `SameSite` | `Lax` | Protección CSRF. Permite navegación top-level (OAuth callbacks) |
| `Path` | `/` | Disponible en todas las rutas |
| `Domain` | `.proactrip.com` | Compartido entre subdominios (omitir si usas `__Host-`) |

### Formatos de Producción

**Multi-subdominio (recomendado para ProacTrip):**
```
Set-Cookie: __Secure-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900
Set-Cookie: __Secure-refresh_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800
```

**Single domain (máxima seguridad):**
```
Set-Cookie: __Host-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=900
```

### Limpieza de Cookies (Logout)

Además de `Max-Age=0`, el backend puede enviar:

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

El frontend nunca llama manualmente a `/refresh-token`.

---

## Register

Crea una nueva cuenta. El backend:

1. Obtiene la IP del cliente (de la conexión o proxy).
2. Cachea la IP asociada al registro (TTL: 24h, mismo tiempo que el token de verificación).
3. Aplica rate limiting por IP.
4. Envía email de verificación vía Resend.
5. Establece cookies `__Secure-access_token` + `__Secure-refresh_token` inmediatamente (sesión pre-verificada con privilegios limitados).

### Request

```
POST /v1/auth/register
```

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Idempotency-Key` | string | No | UUID v7. Previene registros duplicados por retries de red. El backend cachea la respuesta por 24h. |
| `Content-Type` | string | Sí | `application/json` |

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `email` | string | Sí | Email válido | Correo del usuario |
| `password` | string | Sí | Mínimo 8 caracteres | Contraseña |

**Ejemplo:**

```bash
curl -X POST {base_url}/register \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: 019d5439-cb43-716d-90b5-51dcbe980908" \
  -d '{"email":"user@example.com","password":"SecurePass123!"}'
```

### Responses

#### 201 Created

> **Seguridad:** En `EMAIL_ALREADY_EXISTS` (409), la respuesta es genérica y **no incluye cookies ni datos de usuario**.

```json
{
  "message": "Registration successful. Please verify your email."
}
```

**Set-Cookie Headers:**

```
Set-Cookie: __Secure-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900
Set-Cookie: __Secure-refresh_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800
```

#### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `EMAIL_ALREADY_EXISTS` | 409 | Email ya registrado |
| `INVALID_EMAIL` | 400 | Formato inválido |
| `WEAK_PASSWORD` | 400 | No cumple requisitos |
| `VALIDATION_ERROR` | 400 | Body malformado |
| `RATE_LIMIT_EXCEEDED` | 429 | Demasiadas peticiones (RFC 7807 Problem JSON). Ver [Rate Limiting](#rate-limiting) |
| `INTERNAL_ERROR` | 500 | Error inesperado |

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

**Example:**

```bash
curl -X POST {base_url}/resend-verification \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com"}'
```

### Responses

#### 200 OK

```json
{
  "message": "If the email exists and is unverified, a new verification email will be sent."
}
```

#### Possible Errors

| Code | HTTP | When |
|------|------|------|
| `VALIDATION_ERROR` | 400 | Missing or malformed body |
| `INTERNAL_ERROR` | 500 | Unexpected server error |

---

## Verify Email

Verifica el email usando el token del enlace.
El backend usa la IP cacheada del registro. Si el cache expiró, obtiene la IP de la conexión actual.

> **Nota:** El enlace del email apunta al frontend (`{FRONTEND_URL}/auth/verify-email?token=xxx`), no al backend. El frontend llama a este endpoint.

### Request

```
POST /v1/auth/verify-email
```

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
    "email_verified": true,
    "role_name": "client"
  }
}
```
> **Nota:** El `environment` NO se devuelve en verify-email. El frontend debe managejarlo por separado vía `GET /v1/environment` (ver [ENVIRONMENT_API](./ENVIRONMENT_API.md)).

**Set-Cookie Headers (actualiza la sesión):**

```
Set-Cookie: __Secure-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900
Set-Cookie: __Secure-refresh_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800
```

#### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `TOKEN_INVALID` | 401 | Token malformado o inexistente |
| `TOKEN_EXPIRED` | 401 | Token expiró (24h) |
| `VALIDATION_ERROR` | 400 | Falta el campo `token` |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## Login

Autentica con email y password.  
**No requiere `X-Real-IP`**: la IP se obtiene de la conexión y se usa para GeoIP/Weather.

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

#### 200 OK — Sin MFA

```json
{
  "user": {
    "id": "019d5439-cb43-716d-90b5-51dcbe980908",
    "email": "user@example.com",
    "email_verified": true,
    "role_name": "client"
  }
}
```

> **Nota:** El `environment` NO se devuelve en login. El frontend debe managejarlo por separado vía `GET /v1/environment` (ver [ENVIRONMENT_API](./ENVIRONMENT_API.md)).

**Set-Cookie Headers:**

```
Set-Cookie: __Secure-access_token=...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900
Set-Cookie: __Secure-refresh_token=...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800
```

#### 200 OK — MFA Requerido

Cuando `mfa_required` es `true`, **no se establecen cookies** hasta completar `/login/mfa`.

```json
{
  "user": {
    "email": "user@example.com"
  },
  "mfa_required": true,
  "mfa_methods": ["totp", "email", "sms"],
  "session_id": "019d5439-cb43-716d-90b5-51dcbe980908"
}
```

> **Cambios aplicados:** El `session_id` expira en 5 minutos.

#### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `INVALID_CREDENTIALS` | 401 | Email o password incorrectos |
| `EMAIL_NOT_VERIFIED` | 401 | Email no verificado |
| `ACCOUNT_LOCKED` | 429 | Demasiados intentos fallidos |
| `ACCOUNT_SUSPENDED` | 403 | Cuenta suspendida |
| `ACCOUNT_INACTIVE` | 403 | Cuenta inactiva |
| `VALIDATION_ERROR` | 400 | Body malformado |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## Login MFA sera futuro

## Logout

Revoca la sesión actual. **No se pasa token en el body**: el backend lee las cookies automáticamente.

### Request

```
POST /v1/auth/logout
```

> El navegador envía las cookies automáticamente. No enviar body.

### Responses

#### 200 OK

```json
{
  "message": "Logged out successfully."
}
```

**Headers de limpieza:**

```
Set-Cookie: __Secure-access_token=; Max-Age=0; Path=/; Domain=.proactrip.com; Secure
Set-Cookie: __Secure-refresh_token=; Max-Age=0; Path=/; Domain=.proactrip.com; Secure
Clear-Site-Data: "cookies"
```

#### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## Logout All Sessions

Revoca **todas** las sesiones activas del usuario. Requiere cookie `__Secure-access_token` válida. **No se pasa token en el body**.

### Request

```
POST /v1/auth/logout/all
```

### Responses

#### 200 OK

```json
{
  "message": "All sessions have been revoked."
}
```

**Headers de limpieza:**

```
Set-Cookie: __Secure-access_token=; Max-Age=0; Path=/; Domain=.proactrip.com; Secure
Set-Cookie: __Secure-refresh_token=; Max-Age=0; Path=/; Domain=.proactrip.com; Secure
Clear-Site-Data: "cookies"
```

#### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## OAuth Google

Inicia el flujo de autenticación OAuth con Google. El frontend debe llamar este endpoint y redirigir al usuario a la URL de autorización devuelta por el backend.

### Request

```
GET /v1/auth/oauth/:provider
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `provider` | string | Sí | Proveedor OAuth. Actualmente solo `google`. |

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| (ninguno requerido) | — | — | Endpoint público sin autenticación |

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

> **Flujo:** El frontend debe redirigir al navegador a `auth_url` con `window.location.href = data.auth_url`. El backend genera un `state` anti-CSRF one-time en cada llamada — **no cachear esta respuesta**.

#### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `OAUTH_PROVIDER_NOT_FOUND` | 400 | Proveedor no soportado (ej: `facebook`) |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## OAuth Google Callback

Callback llamado por Google después de que el usuario autoriza la aplicación. **Este endpoint NO debe ser llamado directamente por el frontend.** El navegador sigue la redirección automáticamente desde Google.

### Flujo Completo

```
┌──────────┐   GET /oauth/:provider    ┌──────────┐
│ Frontend │ ────────────────────────> │ Backend  │
└──────────┘                           └──────────┘
     │                                      │
     │  { auth_url }                        │
     │<─────────────────────────────────────│
     │                                      │
     │  window.location.href = auth_url     │
     │─────────────────────────────────────>│
     │                                      │
     │           ┌──────────┐               │
     │           │  Google  │               │
     │           └──────────┘               │
     │                │                     │
     │  (usuario autoriza)                  │
     │                │                     │
     │   Google redirige al backend         │
     │   GET /oauth/google/callback         │
     │   ?code=xxx&state=yyy                │
     │                │────────────────────>│
     │                │                     │ (intercambia código,
     │                │                     │  crea/víncula usuario,
     │                │                     │  genera tokens)
     │                │                     │
     │  302 → /auth/callback?status=success │
     │  + cookies: __Secure-access_token    │
     │  + cookies: __Secure-refresh_token   │
     │<─────────────────────────────────────│
```

### Request

```
GET /v1/auth/oauth/:provider/callback
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `provider` | string | Sí | Proveedor OAuth. Debe coincidir con el usado en `/oauth/:provider`. |

**Query Params (enviados por Google):**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `code` | string | Sí | Código de autorización de Google |
| `state` | string | Sí | Token anti-CSRF generado por el backend en `/oauth/:provider` |

> **Nota:** El frontend no necesita leer estos parámetros. Google los añade automáticamente a la URL de callback registrada.

### Responses

#### 302 Found — Éxito

El backend redirige al frontend:

```
Location: {FRONTEND_URL}/auth/callback?status=success
```

**Set-Cookie Headers:**

```
Set-Cookie: __Secure-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900
Set-Cookie: __Secure-refresh_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800
```

| Cookie | HttpOnly | TTL | Contenido |
|--------|----------|-----|-----------|
| `__Secure-access_token` | Sí | 15 min | PASETO v4 access token (opaco) |
| `__Secure-refresh_token` | Sí | 7 días | PASETO v4 refresh token (opaco) |

> **Importante:** Después del callback OAuth, el frontend debe llamar a `GET /v1/auth/me` para obtener los datos del usuario (`id`, `email`, `email_verified`, `role_name`). Login, register y verify-email ya incluyen estos datos en su respuesta.

#### 302 Found — Error

El backend redirige al frontend con el código de error:

```
Location: {FRONTEND_URL}/auth/callback?status=error&code=OAUTH_EXCHANGE_FAILED
```

> El frontend debe leer `status` y `code` de los query params en la URL de callback para mostrar el error adecuado al usuario. Si `status=success`, el usuario ya está autenticado y las cookies están disponibles.

#### Posibles Errores (vía redirect)

Todos los errores se devuelven como `302 Found` con `status=error&code=XXX`. El frontend nunca recibe un JSON de error en este endpoint.

| Código | Cuándo |
|--------|--------|
| `OAUTH_CODE_MISSING` | Falta el parámetro `code` en el callback (Google no lo envió) |
| `OAUTH_STATE_MISSING` | Falta el parámetro `state` en el callback (Google no lo envió) |
| `OAUTH_STATE_INVALID` | State inválido, expirado o reutilizado (posible ataque CSRF o replay) |
| `OAUTH_ACCESS_DENIED` | El usuario denegó el acceso en Google o hubo un error del proveedor |
| `OAUTH_EXCHANGE_FAILED` | Error al intercambiar el código por tokens con Google, o error interno inesperado |
| `OAUTH_PROVIDER_NOT_FOUND` | Proveedor no soportado |
| `EMAIL_NOT_VERIFIED` | El email de la cuenta de Google no está verificado |
| `ACCOUNT_LOCKED` | Cuenta bloqueada por intentos fallidos |
| `ACCOUNT_SUSPENDED` | Cuenta suspendida |
| `ACCOUNT_INACTIVE` | Cuenta inactiva |

---

## Current User (Me)

Retorna los datos del usuario autenticado. Usa la cookie `__Secure-access_token` (o `access_token` en dev) para identificar al usuario.

> **Cuándo llamarlo:** Este endpoint se usa después de OAuth callback para obtener los datos del usuario. Login, register y verify-email ya devuelven los datos del usuario en su respuesta.

### Request

```
GET /v1/auth/me
```

> El navegador envía las cookies automáticamente. No requiere body ni headers adicionales.

### Responses

#### 200 OK

```json
{
  "user": {
    "id": "019d5439-cb43-716d-90b5-51dcbe980908",
    "email": "user@example.com",
    "email_verified": true,
    "role_name": "client"
  }
}
```

**Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

#### Posibles Errores

| HTTP | Cuándo |
|------|--------|
| 401 | No autenticado — falta cookie de access token |
| 500 | Usuario no encontrado en base de datos |

---

## Configuración CORS

| Setting | Valor |
|---------|-------|
| Allowed Origins | `https://proactrip.com`, `http://localhost:3000` |
| Allowed Methods | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS` |
| Allowed Headers | `Content-Type`, `Accept`, `Authorization`, `X-Request-Id`, `X-Trace-Id`, `Idempotency-Key` |
| Allow Credentials | `true` |
| Max Age | `86400` |

> **Crítico:** NUNCA usar `Access-Control-Allow-Origin: *` cuando se envían cookies. Debe ser origen explícito. Max Age varia según el contexto.

---

## Rate Limiting

Rate limiting multi-tier con DragonflyDB y scripts Lua atómicos. Distribuido y seguro en entornos multi-instancia. Todos los límites son configurables vía variables de entorno.

### Tiers

| Tier | Scope | Límite | Aplica a |
|------|-------|--------|----------|
| **Tier 1 — Global** | IP | 100 req/min | Todos los endpoints (DDoS shield) |
| **Tier 2 — Authenticated** | UUID del usuario | 10 req/min | Endpoints protegidos con auth (`/v1/auth/*` autenticados) |
| **Tier 3 — Anonymous** | Cookie `__Secure-anon_token` | 5 req/min | Endpoints públicos sin autenticación |

### Provider-Aware Rate Limiting

| Proveedor | Límite | Descripción |
|-----------|--------|-------------|
| Resend (email) | 100/day | Límite del plan gratuito de Resend. Se aplica por IP |
| SerpAPI | 50/hour | Límite por IP para llamadas al proveedor externo de búsqueda |

### Cookie Anónima (`__Secure-anon_token`)

Para endpoints públicos donde no hay sesión de usuario, el backend establece una cookie anónima con UUID v7 para rate limiting:

```
Set-Cookie: __Secure-anon_token=019d5439-cb43-716d-90b5-51dcbe980908; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=315360000
```

| Atributo | Valor | Propósito |
|----------|-------|-----------|
| Nombre | `__Secure-anon_token` | Identificador anónimo para rate limiting |
| TTL | 10 años (Max-Age=315360000) | Persiste entre sesiones del navegador — permite rate limiting consistente en usuarios no autenticados |
| `HttpOnly` | `true` | Inaccesible vía JavaScript |
| `Secure` | `true` | Solo HTTPS en producción |
| `SameSite` | `Lax` | Se envía en navegación top-level |

> El frontend no necesita hacer nada con esta cookie. El navegador la envía automáticamente. Si la cookie no existe, el backend la establece en la primera respuesta.

### Response on 429 (Rate Limit Exceeded)

Formato **RFC 7807 Problem Details**:

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

Todas las respuestas incluyen estos headers (independientemente del status code):

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
| `POST /v1/auth/login` | `no-store, private` | Datos de sesión sensibles |
| `POST /v1/auth/register` | `no-store, private` | Datos de registro sensibles |
| `POST /v1/auth/verify-email` | `no-store, private` | Datos de usuario + sesión |
| `POST /v1/auth/logout` | `no-store, private` | Invalidación de sesión |
| `POST /v1/auth/logout/all` | `no-store, private` | Invalidación masiva de sesiones |
| `GET /v1/auth/me` | `no-store, private` | Datos de usuario sensibles |

---

## Notas de Seguridad

### Tokens PASETO v4

Todos los tokens internos son **PASETO v4 symmetric**. Son opacos para el cliente.

| Token | TTL | Propósito |
|-------|-----|-----------|
| `access_token` (cookie `__Secure-access_token`) | 15 min | Autenticar requests |
| `refresh_token` (cookie `__Secure-refresh_token`) | 7 días | Rotación de sesión |
| `session_id` (MFA) | 5 min | Completar MFA |
| Email verification | 24 horas | Verificar email |
| Password reset | 1 hora | Reset de contraseña |
| `sse_token` | 30 seg | Conexiones SSE |
| OAuth `state` | 5-10 min | Anti-CSRF OAuth |

### Hashing de Contraseñas

**Argon2id** con parámetros recomendados por OWASP (2026).

### Rotación de Refresh Tokens

Cada vez que el backend refresca un `__Secure-access_token`, rota también el `__Secure-refresh_token` (token rotation). Si un `__Secure-refresh_token` revocado es reutilizado, **todas las sesiones del usuario se invalidan** automáticamente (detección de robo).

### GeoIP y Weather

- Resueltos desde la IP de conexión vía `GET /v1/environment` (ver [ENVIRONMENT_API](./ENVIRONMENT_API.md)).
- No se requiere header `X-Real-IP` manual.
- El backend cachea estos datos 10 min para evitar llamadas repetidas a APIs externas.
- Los endpoints de auth (`/v1/auth/*`) NO devuelven `environment` — es responsabilidad del frontend.

### MFA

- Códigos de un solo uso, TTL 5 minutos en Redis.
- Un código verificado se elimina inmediatamente.
- Recovery codes de 8 caracteres hexadecimales, mostrados una sola vez.

### OAuth PKCE

- El backend genera y almacena el `code_verifier`.
- Google valida el hash PKCE internamente durante el exchange.
- El `state` es one-time: se valida y elimina del cache inmediatamente.

### Prevención de Ataques

| Amenaza | Mitigación |
|---------|------------|
| XSS | `HttpOnly cookies + CSP`  |
| CSRF | `SameSite=Lax` + cookies automáticas |
| Token Exposure in SSE | SSE authenticated via HttpOnly cookies (no tokens in URL or storage) |
| Replay de refresh | Rotación continua + invalidación total ante reúso |
| Third-party cookies | No se usa Partitioned (CHIPS) — SameSite=Lax + Domain=.proactrip.com es suficiente para subdominios |
| Rate limiting abuse | Multi-tier con DragonflyDB + Lua scripts atómicos (IP, usuario autenticado, cookie anónima) |

### Compartición de Cookies entre Subdominios

Las cookies de autenticación se comparten entre `api.proactrip.com` y `app.proactrip.com` usando `Domain=.proactrip.com` con `SameSite=Lax`. NO se usa `Partitioned` (CHIPS) porque CHIPS está diseñado para iframes y cross-site embedding, no para subdominios. Con `Partitioned` + un `Domain` amplio, las cookies no se envían entre subdominios, rompiendo la autenticación cruzada.

### Headers de Seguridad

Todas las respuestas incluyen:

```
Content-Security-Policy: default-src 'self'
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
Strict-Transport-Security: max-age=31536000
```

---