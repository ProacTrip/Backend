# Environment API Documentation

> **Environment** = ubicación GeoIP + clima actual de la IP del usuario.  
> **Caché:** 10 minutos en backend (Redis) y frontend (localStorage).  
> **Independencia:** Separado de auth — `/v1/auth/*` NO devuelven `environment`.

---

## Índice

- [Arquitectura](#arquitectura)
- [Caché](#caché)
- [Base URLs](#base-urls)
- [Errores Estándar](#errores-estándar)
- [Get Environment](#get-environment)
- [Flujo Frontend](#flujo-frontend)
- [Configuración CORS](#configuración-cors)
- [Rate Limiting](#rate-limiting)
- [Notas Técnicas](#notas-técnicas)

---

## Arquitectura

### Flujo de Datos

```
┌─────────────┐     GET /v1/environment    ┌─────────────────────────┐
│   Browser   │ ──────────────────────────>│   Backend               │
│  (Frontend) │    Credentials: include    │                         │
└─────────────┘                            │  ┌───────────────────┐  │
       ^                                   │  │ Cache (Redis)?    │  │
       │                                   │  │ HIT → devolver    │  │
       │                                   │  │ MISS ↓            │  │
       │                                   │  └───────────────────┘  │
       │                                   │         │               │
       │                                   │    ┌────┴────┐          │
       │                                   │    ▼         ▼          │
       │                                   │ IP-API   OpenWeather    │
       │                                   │ (GeoIP)  (clima)        │
       │                                   │    │         │          │
       │                                   │    └────┬────┘          │
       │                                   │         ▼               │
       │                                   │    Cache (Redis)        │
       │                                   │    TTL: 10 min          │
       │                                   └─────────────────────────┘
       │        200 OK: location + weather                           │
       └─────────────────────────────────────────────────────────────┘
```

### Módulos involucrados

| Archivo | Responsabilidad |
|--------|----------------|
| `environment/features/get_environment/handler.go` | HTTP handler: parsea request, delega al usecase |
| `environment/features/get_environment/usecase.go` | Lógica: resuelve IP → GeoIP → weather, cachea |
| `environment/adapters/ipquery/` | Cliente IP-API (proveedor externo de geolocalización) |
| `environment/adapters/openweather/` | Cliente OpenWeather (proveedor externo de clima) |
| `environment/domain/context.go` | Tipos: `EnvironmentResponse`, `LocationData`, `WeatherData` |

### Responsabilidades

| Capa | Quién | Qué cachea | TTL |
|------|-------|-----------|-----|
| **Backend** | Redis/DragonflyDB | `ip:{client_ip}` + `weather:{lat}:{lon}:{lang}` | 10 min |
| **Frontend** | localStorage/React state | Environment response completo | 10 min |

> Auth y environment son módulos **zero-knowledge** — no se importan entre sí. La conexión es vía el frontend que orquesta ambas llamadas.

---

## Caché

### Backend (Redis/DragonflyDB)

Se cachea la respuesta completa `EnvironmentResponse` por IP para evitar llamadas repetidas a todos los providers externos (IP-API + OpenWeather).

| Key pattern | Datos almacenados | TTL |
|-------------|-------------------|-----|
| `env:{client_ip}` | `EnvironmentResponse` completo (location + weather) | 10 min |

Si el cache HIT, se devuelve inmediatamente sin llamar a ningún provider externo.  
Si el cache MISS, se resuelve IP → GeoIP → Weather, se ensambla y cachea la respuesta completa.

### Frontend (localStorage)

| Key | Valor |
|-----|-------|
| `user_environment` | JSON del `EnvironmentResponse` completo |
| `user_environment_stored_at` | Timestamp ISO 8601 de cuando se cacheó |

**Lógica de validación:**

```typescript
function isEnvCacheValid(): boolean {
  const storedAt = localStorage.getItem('user_environment_stored_at');
  if (!storedAt) return false;
  const age = Date.now() - new Date(storedAt).getTime();
  return age < 10 * 60 * 1000; // 10 minutos
}
```

---

## Base URLs

| Entorno | Base URL |
|---------|----------|
| **Production** | `https://api.proactrip.com/v1/environment` |
| **Development** | `http://localhost:8080/v1/environment` |

Todos los ejemplos usan `{base_url}` como placeholder.

---

## Errores Estándar

Formato **RFC 7807 Problem Details**:

```json
{
  "type": "internal_error",
  "title": "Internal Server Error",
  "status": 500,
  "detail": "Failed to resolve IP address",
  "instance": "/v1/environment",
  "trace_id": "019d5439-cb43-716d-90b5-51dcbe980908"
}
```

**Headers de respuesta en TODOS los endpoints:**

| Header        | Descripción                                             |
| ------------- | ------------------------------------------------------- |
| `X-Trace-Id`  | UUID v7 para trazabilidad. Asignado globalmente por middleware |
| `traceparent` | W3C Trace Context                                       |

---

## Get Environment

Devuelve la ubicación GeoIP y el clima actual del cliente.

La IP se detecta automáticamente de la conexión. Si la IP es local/privada:
1. Intenta auto-detección usando el mismo proveedor IP con IP vacía
2. Si falla, devuelve ubicación por defecto (configurable vía `DEFAULT_COUNTRY_CODE`)

> **Degradación elegante:** Si `OPENWEATHER_API_KEY` no está configurada, `weather` viene `null` (no falla). Si el rate limit del proveedor se excede, `weather` viene `null`. El frontend debe handlejar `weather: null`.

### Request

```
GET /v1/environment
```

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `X-Real-IP` | string | No | IP del cliente (override de auto-detección). Útil para testing. |
| `Content-Type` | string | Sí | `application/json` |

> El idioma de la descripción del clima se obtiene del header `Accept-Language` del navegador, con fallback a `"en"`. `location.language` también usa `Accept-Language` como fuente primaria, con fallback a `CountryMetadata` (mapeo país→idioma).

**Ejemplo:**

```bash
curl -X GET {base_url}/environment \
  -H "Content-Type: application/json"
# Detecta IP automáticamente, location.language determina el idioma del clima
```

### Responses

#### 200 OK

```json
{
  "location": {
    "country": "Argentina",
    "country_code": "AR",
    "city": "Buenos Aires",
    "state": "Buenos Aires",
    "zipcode": "",
    "timezone": "America/Argentina/Buenos_Aires",
    "currency": "ARS",
    "language": "es",
    "latitude": -34.6037,
    "longitude": -58.3816
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

**Campos `location`:**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `country` | string | Nombre completo del país (ej: `"Argentina"`) |
| `country_code` | string | Código ISO 3166-1 alpha-2 (ej: `"AR"`) |
| `city` | string | Ciudad (ej: `"Buenos Aires"`) |
| `state` | string | Estado/provincia (ej: `"Buenos Aires"`) |
| `zipcode` | string | Código postal (puede ser vacío) |
| `timezone` | string | Timezone IANA (ej: `"America/Argentina/Buenos_Aires"`) |
| `currency` | string | Código ISO 4217 del país (ej: `"ARS"`) |
| `language` | string | Código ISO 639-1 del idioma. Fuente primaria: `Accept-Language` del navegador. Fallback: CountryMetadata (🇦🇷 AR→es, 🇺🇸 US→en, 🇫🇷 FR→fr, etc.) |
| `latitude` | float64 | Latitud geográfica |
| `longitude` | float64 | Longitud geográfica |

**Campos `weather` (puede ser `null`):**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `temp` | float64 | Temperatura actual en °C |
| `feels_like` | float64 | Sensación térmica en °C |
| `description` | string | Descripción textual en el idioma solicitado |
| `icon` | string | Código del icono OpenWeather (ej: `"01d"`) |
| `icon_url` | string | URL del icono en resolución 4x |
| `humidity` | int | Humedad relativa (%) |
| `wind_speed` | float64 | Velocidad del viento en m/s |

#### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `INVALID_IP` | 400 | IP malformada en `X-Real-IP` |
| `LOCATION_PROVIDER_ERROR` | 502 | IP-API no responde o devuelve error |
| `INTERNAL_ERROR` | 500 | Error inesperado del backend |
| `RATE_LIMIT_EXCEEDED` | 429 | Rate limit del proveedor externo excedido. Ver [Rate Limiting](#rate-limiting) |

---

## Configuración CORS

Misma configuración que auth (compartida a nivel aplicación en `bootstrap/app.go`):

| Setting | Valor |
|---------|-------|
| Allowed Origins | `https://proactrip.com`, `http://localhost:3000` |
| Allowed Methods | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS` |
| Allow Credentials | `true` |

---

## Rate Limiting

Aplica Tier 1 — Global (por IP): **100 req/min** via DragonflyDB + scripts Lua atómicos.

| Provider | Rate Limit | Descripción |
|----------|-----------|-------------|
| IP-API | 45 req/min | Límite del plan gratuito de IP-API. Cache de 10 min reduce esto drásticamente |
| OpenWeather | 60 req/min | Límite del plan gratuito. Cache de 10 min reduce a ~6 req/hora por coordenada única |

### Rate Limit Headers

| Header | Descripción |
|--------|-------------|
| `RateLimit-Limit` | Máximo permitido en la ventana actual |
| `RateLimit-Remaining` | Peticiones restantes |
| `RateLimit-Reset` | Segundos hasta reinicio de ventana |
| `Retry-After` | Segundos a esperar (solo en 429) |

---

## Notas Técnicas

### Resolución de IP

- Si la IP es local/privada (`127.0.0.1`, `192.168.x.x`, `10.x.x.x`, `::1`), el backend intenta auto-detección usando IP-API con IP vacía.
- Si la auto-detección falla, usa `DEFAULT_COUNTRY_CODE`.
- Para desarrollo local, setear `DEFAULT_COUNTRY_CODE=ES` (o el país que quieras simular) en `.env`.

### Weather puede ser null — con trazabilidad completa

El backend maneja cada caso con logs claros para diagnosticar el problema real:

| Condición | Log nivel | `weather` en respuesta | Acción backend |
|-----------|-----------|------------------------|----------------|
| `OPENWEATHER_API_KEY` no configurada | `WARN` — "weather provider returned nil (no API key?)" | `null` | No llama a OpenWeather |
| Rate limit de OpenWeather excedido | `WARN` — "openweather rate limit exceeded: X/Y" | `null` | Devuelve error 429 al frontend |
| OpenWeather devuelve error | `ERROR` — "weather provider failed: ..." | `null` | Loguea el error exacto |
| OpenWeather OK | `DEBUG` — "weather provider OK" con temp/desc | Datos completos | Cachea y devuelve |

El frontend debe handlejar `weather: null` mostrando la ubicación sin clima. El backend **siempre** loguea la causa raíz para que el equipo pueda diagnosticar sin adivinar.

### Separación de auth

- `/v1/auth/login` **no** devuelve `environment`
- `/v1/auth/register` **no** devuelve `environment`
- `/v1/auth/verify-email` **no** devuelve `environment`
- El frontend es responsable de obtener el environment por separado y cachearlo

### Event Bus interno

| Evento | Publicador | Consumidor | Acción |
|--------|-----------|------------|--------|
| `auth.user.registered` | `auth/features/register` | `user/consumer` | Crea perfil de usuario |
| `auth.user.registered` | `auth/features/register` | `notification/consumer` | Envía email de verificación |

El perfil de usuario se crea con los datos cacheados de `/v1/environment`:
- `timezone` = `location.timezone` (ej: `"America/Argentina/Buenos_Aires"`)
- `language` = `location.language` (ej: `"es"`)
- `currency` = `location.currency` (ej: `"ARS"`)

Campos opcionales (`first_name`, `last_name`, `phone`, etc.) se crean en `null` — el usuario los completa después desde la UI.

---

## Beneficios de esta arquitectura

- **Separación clara:** Auth solo maneja autenticación. Environment solo maneja ubicación/clima.
- **Menor acoplamiento:** Cambiar el environment no rompe auth. Agregar campos al environment no afecta auth.
- **Reducción de tráfico:** Cache de 10 min en backend + frontend = mínimo 6x menos llamadas a APIs externas.
- **Mejor UX:** Paralelismo con `Promise.all` en lugar de secuencial = menor tiempo percibido por el usuario.
- **Escalabilidad:** Agregar más datos al environment (ej: eventos locales, festividades, tipo de cambio) no requiere modificar auth.
