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
| `Partitioned` | `true` | CHIPS — permite cookies en contextos de terceros sin third-party cookies |
| `Domain` | `.proactrip.com` | Compartido entre subdominios (omitir si usas `__Host-`) |

### Formatos de Producción

**Multi-subdominio (recomendado para ProacTrip):**
```
Set-Cookie: __Secure-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900; Partitioned
Set-Cookie: __Secure-refresh_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800; Partitioned
```

**Single domain (máxima seguridad):**
```
Set-Cookie: __Host-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=900; Partitioned
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

Formato **RFC 7807 Problem Details**:

```json
{
  "type": "validation_error",
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
Set-Cookie: __Secure-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900; Partitioned
Set-Cookie: __Secure-refresh_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800; Partitioned
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
    "email": "user@example.com",
    "email_verified": true,
    "role_name": "client"
  }
}
```
> **Nota:** El `environment` NO se devuelve en verify-email. El frontend debe managejarlo por separado vía `GET /v1/environment` (ver [ENVIRONMENT_API](./ENVIRONMENT_API.md)).

**Set-Cookie Headers (actualiza la sesión):**

```
Set-Cookie: __Secure-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900; Partitioned
Set-Cookie: __Secure-refresh_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800; Partitioned
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
    "email": "user@example.com",
    "email_verified": true,
    "role_name": "client"
  }
}
```

> **Nota:** El `environment` NO se devuelve en login. El frontend debe managejarlo por separado vía `GET /v1/environment` (ver [ENVIRONMENT_API](./ENVIRONMENT_API.md)).

**Set-Cookie Headers:**

```
Set-Cookie: __Secure-access_token=...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900; Partitioned
Set-Cookie: __Secure-refresh_token=...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800; Partitioned
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
  "type": "rate_limit_exceeded",
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
| Third-party cookies | `Partitioned` (CHIPS) |
| Rate limiting abuse | Multi-tier con DragonflyDB + Lua scripts atómicos (IP, usuario autenticado, cookie anónima) |

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