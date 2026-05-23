# Environment API Documentation

> **Arquitectura:** Endpoint público (sin auth). Devuelve ubicación GeoIP + clima actual detectado de la IP del cliente. El frontend lo llama al entrar a la web para poblar placeholders y defaults de ubicación, idioma y moneda.

---

## Índice

| Endpoint | Estado |
|----------|--------|
| [Arquitectura](#arquitectura) | ✅ |
| [Base URLs](#base-urls) | ✅ |
| [Errores Estándar](#errores-estándar) | ✅ |
| [Get Environment](#get-environment) | ✅ Implementado |
| [Configuración CORS](#configuración-cors) | ✅ |
| [Rate Limiting](#rate-limiting) | ✅ |
| [Cache](#cache) | ✅ |
| [Notas Técnicas](#notas-técnicas) | ✅ |

---

## Arquitectura

### Flujo de Datos

```
┌─────────────┐     GET /v1/environment    ┌─────────────────────────┐
│   Browser   │ ──────────────────────────>│   Backend               │
│  (Frontend) │    Accept-Language: es     │                         │
└─────────────┘                            │  ┌───────────────────┐  │
       ^                                   │  │ Cache (Dragonfly)?│  │
       │                                   │  │ HIT → devolver    │  │
       │                                   │  │ MISS ↓            │  │
       │                                   │  └───────────────────┘  │
       │                                   │         │               │
       │                                   │    ┌────┴────┐          │
       │                                   │    ▼         ▼          │
       │                                   │ ipquery.io  OpenWeather    │
       │                                   │ (GeoIP)    (clima)        │
       │                                   │    │         │          │
       │                                   │    └────┬────┘          │
       │                                   │         ▼               │
       │                                   │    Cache (DragonflyDB)  │
       │                                   │    TTL: configurable    │
       │                                   └─────────────────────────┘
       │  200 OK: Cache-Control: public, max-age=600                 │
       │  { location: {...}, weather: {...} }                        │
       └─────────────────────────────────────────────────────────────┘
```

### Módulos involucrados

| Archivo | Responsabilidad |
|--------|----------------|
| `internal/modules/environment/features/get_environment/handler.go` | HTTP handler: resuelve IP, extrae idioma, delega al usecase |
| `internal/modules/environment/features/get_environment/usecase.go` | Lógica: cachea, resuelve IP → GeoIP → weather |
| `internal/modules/environment/features/get_environment/response.go` | Type alias: `Response = domain.EnvironmentResponse` |
| `internal/modules/environment/adapters/ipquery/` | Cliente ipquery.io (geolocalización externa) |
| `internal/modules/environment/adapters/openweather/` | Cliente OpenWeather (clima externo) |
| `internal/modules/environment/domain/environment.go` | Tipo principal: `EnvironmentResponse` |
| `internal/modules/environment/domain/location.go` | `LocationData` |
| `internal/modules/environment/domain/weather.go` | `WeatherData` |
| `internal/modules/environment/domain/errors.go` | Errores de dominio |
| `internal/modules/environment/domain/defaults.go` | `DefaultLocation()`, `CountryMetadata` (229 países) |

### Responsabilidades de caché

| Capa | Quién | Qué cachea | TTL |
|------|-------|-----------|-----|
| **Backend** | DragonflyDB | `ipquery:{ip}` → `LocationData` (ubicación GeoIP) | 24h (estable por IP) |
| **Backend** | DragonflyDB | `weather:{lat_2d}:{lon_2d}:{lang}` → `WeatherData` (clima) | Configurable via `ENVIRONMENT_WEATHER_CACHE_TTL` (default: 30 min) |
| **Backend** | DragonflyDB | `env:{ip}` → `CountryInfo` (compatibilidad con módulo search) | 24h |

> Auth y environment son módulos **zero-knowledge** — no se importan entre sí. El frontend orquesta ambas llamadas por separado.

---

## Base URLs

| Entorno | Base URL |
|---------|----------|
| **Production** | `https://api.proactrip.com/v1/environment` |
| **Development** | `http://localhost:8080/v1/environment` |

Todos los ejemplos usan `{base_url}` como placeholder.

---

## Errores Estándar

Formato **RFC 9457 Problem Details**. Todas las respuestas de error usan `Content-Type: application/problem+json`.

```json
{
  "type": "https://api.proactrip.com/errors/bad-request",
  "title": "Bad Request",
  "status": 400,
  "detail": "dirección IP inválida",
  "instance": "/v1/environment",
  "trace_id": "019d5439-cb43-716d-90b5-51dcbe980908"
}
```

**Headers de respuesta en TODOS los endpoints:**

| Header | Descripción |
|--------|-------------|
| `X-Trace-Id` | UUID v7 para trazabilidad. Asignado globalmente por middleware |
| `traceparent` | W3C Trace Context |

---

## Get Environment

Devuelve la ubicación GeoIP y el clima actual del cliente, detectados automáticamente de la IP de la conexión.

**Comportamiento por entorno:**
- **Producción:** IPs privadas/localhost son rechazadas con HTTP 400 (`ErrInvalidIP`). Solo se procesan IPs públicas.
- **Desarrollo (`SERVER_ENV=dev`):** IPs privadas pasan al usecase, que intenta auto-detección vía ipquery.io (IP vacía). Si falla, devuelve ubicación por defecto según `DEFAULT_COUNTRY_CODE`.

> **Degradación elegante:** Si `OPENWEATHER_API_KEY` no está configurada, `weather` viene `null` (no falla). Si el rate limit del proveedor de clima se excede, se devuelve HTTP 429 (`ErrRateLimitExceeded`). El frontend debe handlejar `weather: null`.

### Request

```
GET /v1/environment
```

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Accept-Language` | string | No | Idioma para la descripción del clima y `location.language`. Fallback: `"es"`. Acepta locales completos (ej: `en-US`, `es-AR`) y los normaliza al código ISO 639-1 de 2 caracteres (ej: `en`, `es`) |
| `X-Real-IP` | string | No | IP del cliente (override de auto-detección). Útil para testing |

**Ejemplo:**

```bash
curl -X GET {base_url}/environment \
  -H "Accept-Language: es"
# Detecta IP automáticamente, devuelve clima en español
```

### Responses

#### 200 OK

**Headers:**
| Header | Valor |
|--------|-------|
| `Cache-Control` | `public, max-age=600` |

**Body:**

```json
{
  "location": {
    "country": "Spain",
    "country_code": "ES",
    "city": "Móstoles",
    "state": "Madrid",
    "zipcode": "28938",
    "timezone": "Europe/Madrid",
    "currency": "EUR",
    "language": "es",
    "latitude": 40.30796034467124,
    "longitude": -3.8797081062333523
  },
  "weather": {
    "temp": 18.5,
    "feels_like": 17.2,
    "description": "cielo claro",
    "icon": "01d",
    "icon_url": "https://openweathermap.org/img/wn/01d@4x.png",
    "humidity": 45,
    "wind_speed": 3.5
  }
}
```

#### 200 OK (weather no disponible)

Cuando el clima no está disponible (API key no configurada, rate limit interno, error del proveedor), `weather` es `null`:

```json
{
  "location": {
    "country": "Spain",
    "country_code": "ES",
    "city": "Móstoles",
    "state": "Madrid",
    "zipcode": "28938",
    "timezone": "Europe/Madrid",
    "currency": "EUR",
    "language": "es",
    "latitude": 40.30796034467124,
    "longitude": -3.8797081062333523
  },
  "weather": null
}
```

**Campos `location`:**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `country` | string | Nombre completo del país (ej: `"Spain"`) |
| `country_code` | string | Código ISO 3166-1 alpha-2 (ej: `"ES"`) |
| `city` | string | Ciudad (ej: `"Móstoles"`) |
| `state` | string | Estado/provincia (ej: `"Madrid"`) |
| `zipcode` | string | Código postal (ej: `"28938"`) |
| `timezone` | string | Timezone IANA (ej: `"Europe/Madrid"`) |
| `currency` | string | Código ISO 4217 del país, resuelto desde `CountryMetadata` |
| `language` | string | Código ISO 639-1. Fuente primaria: `Accept-Language` del navegador. Fallback: `CountryMetadata` (🇦🇷 AR→es, 🇺🇸 US→en, 🇫🇷 FR→fr, etc.) |
| `latitude` | float64 | Latitud geográfica |
| `longitude` | float64 | Longitud geográfica |

**Campos `weather` (el objeto completo puede ser `null`):**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `temp` | float64 | Temperatura actual en °C. (al proveedor se pasa por param units=metric, para Farenheit pasar units=imperial). Ajustar internamente los estandares que cada País tiene en `CountryMetadata` (ej: ES:Celsius, USA:Farenheit) |
| `feels_like` | float64 | Sensación térmica en °C. (al proveedor se pasa por param units=metric, para Farenheit pasar units=imperial). Ajustar internamente los estandares que cada País tiene en `CountryMetadata` (ej: ES:Celsius, USA:Farenheit) |
| `description` | string | Descripción textual en el idioma solicitado |
| `icon` | string | Código del icono OpenWeather (ej: `"01d"`) |
| `icon_url` | string | URL del icono en resolución 4x |
| `humidity` | int | Humedad relativa (%) |
| `wind_speed` | float64 | Velocidad del viento en m/s |

#### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `ErrInvalidIP` | 400 | `bad-request` | IP malformada, privada, de loopback, o no especificada (`0.0.0.0`, `::`) en producción |
| `ErrLocationProvider` | 502 | `bad-gateway` | ipquery.io no responde después de agotar reintentos |
| `ErrRateLimitExceeded` | 429 | `rate-limit-exceeded` | Rate limit del proveedor de clima (OpenWeather) excedido |
| `ErrInternal` | 500 | `internal-error` | Error interno inesperado del servidor |

---

## Configuración CORS

Configuración compartida a nivel aplicación (`internal/bootstrap/app.go`):

| Setting | Valor |
|---------|-------|
| Allowed Origins | Configurable vía `FRONTEND_URL_PROD` y `FRONTEND_URL_DEV` (resuelto por `FrontendConfig.GetURL()` según `SERVER_ENV`) |
| Allowed Methods | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS` |
| Allow Credentials | `true` |
| Allowed Headers | `Content-Type`, `Accept`, `Authorization`, `X-Request-Id`, `Idempotency-Key`, `X-Trace-Id` |
| Max Age | 86400s (24h) |

---

## Rate Limiting

### A nivel HTTP

El endpoint `/v1/environment` **no tiene middleware de rate limiting específico**. Aplica el rate limiting global compartido por todas las rutas (`GlobalRateLimitMiddleware`).

### A nivel proveedor externo

El usecase controla el rate limiting contra OpenWeather internamente:

| Provider | Rate Limit | Mecanismo |
|----------|-----------|-----------|
| ipquery.io | 45 req/min (plan gratuito) | Mitigado por caché |
| OpenWeather | 60 req/min (plan gratuito) | Dos niveles: **interno** (`RateLimiter.ProviderAllow`) → si se excede, `weather` viene `null` (degradación elegante). **Externo** (HTTP 429 del provider) → `ErrRateLimitExceeded` (429) al cliente |

### Rate Limit Headers (solo en respuestas 429)

Cuando el rate limiter global rechaza una petición con HTTP 429, se incluyen los siguientes headers:

| Header | Descripción |
|--------|-------------|
| `RateLimit-Limit` | Máximo permitido en la ventana actual |
| `RateLimit-Remaining` | Peticiones restantes (0 cuando se rechaza) |
| `RateLimit-Reset` | Segundos hasta reinicio de ventana |
| `Retry-After` | Segundos a esperar antes de reintentar |

---

## Cache

| Endpoint | Cache-Control | Motivo |
|----------|--------------|--------|
| `GET /v1/environment` | `public, max-age=600` | La ubicación y clima no cambian en intervalos cortos. 10 min de caché en backend reducen llamadas a APIs externas |

**Backend (DragonflyDB v1.38):**

| Key pattern | Datos almacenados | TTL | Estrategia |
|-------------|-------------------|-----|------------|
| `ipquery:{client_ip}` | `LocationData` completo (country, city, lat, lon, etc.) | 24h | Cache-aside. ipquery es gratuito, TTL largo para reducir latencia. |
| `weather:{lat_2d}:{lon_2d}:{lang}` | `WeatherData` (temp, description, humidity, etc.) | Configurable via `ENVIRONMENT_WEATHER_CACHE_TTL` (default: 30 min) | Cache-aside. Lat/lon redondeado a 2 decimales (≈1.1km) para que usuarios de la misma ciudad compartan entrada. Ahorra costos de OpenWeather. |
| `env:{client_ip}` | `CountryInfo` (country, currency, language) | 24h | Clave compartida con el módulo search (backward compat). Escritura asíncrona. |

Si el cache HIT, se devuelve inmediatamente sin llamar a ningún provider externo.
Si el cache MISS, se resuelve IP → GeoIP → Weather, se cachea cada capa por separado (escritura asíncrona, fire-and-forget).

---

## Notas Técnicas

### Resolución de idioma

El backend **normaliza** el header `Accept-Language`: acepta locales completos (ej: `en-US`, `es-AR`, `fr-CA`) y extrae el código primario ISO 639-1 de 2 caracteres (ej: `en`, `es`, `fr`). También maneja parámetros de calidad (`;q=...`).

`location.language` se resuelve en este orden:
1. `Accept-Language` del navegador, normalizado a código ISO 639-1 de 2 caracteres
2. Fallback: `CountryMetadata[country_code].Language` (257 países mapeados)
3. Si el país no está en CountryMetadata: se mantiene el valor normalizado de `Accept-Language` (con fallback a `"es"`)

`weather.description` usa el mismo idioma normalizado del `Accept-Language`, con fallback a `"es"`.

### Moneda

`location.currency` se resuelve desde `CountryMetadata[country_code].Currency`. Fallback configurable vía `DEFAULT_CURRENCY` (default: `"USD"`).

### Weather puede ser null — con trazabilidad

| Condición | Log nivel | `weather` en respuesta | Acción backend |
|-----------|-----------|------------------------|----------------|
| `OPENWEATHER_API_KEY` no configurada | `WARN` — "weather provider returned nil (no API key?)" | `null` | No llama a OpenWeather |
| Rate limit interno excedido (Dragonfly `ProviderAllow`) | `WARN` | `null` | No llama a OpenWeather, continúa sin clima |
| Rate limit externo de OpenWeather (HTTP 429 del provider) | `WARN` | — | Devuelve error 429 (`ErrRateLimitExceeded`) |
| OpenWeather devuelve error | `WARN` — "weather fetch failed, continuing without weather" | `null` | Loguea el error exacto, continúa sin clima |
| OpenWeather OK | `DEBUG` — "weather provider OK" | Datos completos | Cachea y devuelve |

### Separación de auth

- `/v1/auth/*` **no** devuelven `environment`
- El frontend es responsable de obtener el environment por separado al entrar a la web
- Valores de `environment` se usan como defaults/placeholders hasta que el usuario configure preferencias explícitas en su perfil

### Extracción de IP (Echo v5.1.0)

El backend usa `echo.ExtractIPFromXFFHeader(TrustLoopback, TrustPrivateNet)` para extraer la IP del cliente del header `X-Forwarded-For`. Esto es seguro con proxies de confianza (Cloudflare reescribe XFF). **No** se usa el deprecado `LegacyIPExtractor`.

El handler también verifica manualmente el header `X-Real-IP` antes del fallback `c.RealIP()`, asegurando compatibilidad con proxies que solo setean `X-Real-IP` sin `X-Forwarded-For`. En entornos sin proxy, `c.RealIP()` devuelve la IP del socket TCP.

### Desarrollo local

#### Variables de entorno del módulo environment

| Variable | Default | Descripción |
|----------|---------|-------------|
| `SERVER_ENV` | `production` | Entorno: `dev` o `production`. En `dev`, IPs privadas pasan al usecase con auto-detección |
| `DEFAULT_COUNTRY_CODE` | `"ES"` | Código de país por defecto (ISO 3166-1 alpha-2) para `DefaultLocation()` |
| `DEFAULT_CURRENCY` | `"USD"` | Moneda por defecto cuando `CountryMetadata` no tiene el país |
| `OPENWEATHER_API_KEY` | (vacío) | API key de OpenWeather. Si está vacía, `weather` viene `null` |
| `IPQUERY_TIMEOUT` | `5` | Timeout HTTP por intento contra ipquery.io (en segundos) |
| `OPENWEATHER_TIMEOUT` | `10` | Timeout HTTP por intento contra OpenWeather (en segundos) |
| `IPQUERY_MAX_RETRIES` | `3` | Reintentos máximos contra ipquery.io (0 = sin reintentos) |
| `IPQUERY_BASE_URL` | — | URL base de ipquery.io (ej: `https://api.ipquery.io`) |

#### .env de ejemplo

Para simular una ubicación específica en desarrollo:
```bash
# .env
SERVER_ENV=dev
DEFAULT_COUNTRY_CODE=ES
DEFAULT_CURRENCY=EUR
```
Con `SERVER_ENV=dev`, IPs locales (`127.0.0.1`, `192.168.x.x`) pasan al usecase que intenta auto-detección. Si falla, usa `DEFAULT_COUNTRY_CODE`.

También podés usar `X-Real-IP` para simular una IP pública específica:
```bash
curl -H "X-Real-IP: 8.8.8.8" http://localhost:8080/v1/environment
```
