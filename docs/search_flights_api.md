# Flight Search API Documentation (Cookie-Based)

> **Arquitectura:** Cookie-based authentication con HttpOnly cookies.
> El frontend nunca manipula tokens.

---

## Índice

- [Arquitectura](#arquitectura)
- [Seguridad de Cookies](#seguridad-de-cookies)
- [Base URLs](#base-urls)
- [Errores Estándar](#errores-estándar)
- [Estrategia de Refresco de Tokens](#estrategia-de-refresco-de-tokens)
- [Search Flights](#search-flights)
  - [Flujo de Búsqueda](#flujo-de-búsqueda)
  - [Request](#request)
  - [Responses](#responses)
    - [Phase: outbound_selection](#phase-outbound_selection)
    - [Phase: return_selection](#phase-return_selection)
    - [Phase: complete (one_way)](#phase-complete-one_way)
    - [Sin Resultados](#sin-resultados)
  - [Response Fields Explained](#response-fields-explained)
  - [Paginación](#paginación)
  - [Tokens: departure_token vs booking_token](#tokens-departure_token-vs-booking_token)
  - [Posibles Errores](#posibles-errores-search-flights)
- [Flight Details](#flight-details)
  - [Request](#request-flight-details)
  - [Responses](#responses-flight-details)
  - [Response Fields Explained](#response-fields-explained-flight-details)
  - [Booking Options](#booking-options)
  - [Posibles Errores](#posibles-errores-flight-details)
- [Configuración CORS](#configuración-cors)
- [Rate Limiting](#rate-limiting)
- [Cache](#cache)
- [Notas de Seguridad](#notas-de-seguridad)

---

## Arquitectura

### Flujo de Búsqueda de Vuelos

```
┌─────────────┐   POST /v1/search/flights      ┌─────────────┐    ┌─────────────┐
│   Browser   │ ─────────────────────────────> │   Backend   │───>│   SerpAPI   │
│  (Frontend) │  {trip_type, departure, ...}   │             │    │  (Google    │
└─────────────┘                                └─────────────┘    │   Flights)  │
^                                                      │          └─────────────┘
│                               Set-Cookie: __Secure-access_token=...           │
│                              (si el usuario está autenticado)                 │
│                              Response: { phase, flights[], tokens }           │
└───────────────────────────────────────────────────────────────────────────────┘

Las cookies de autenticación se envían AUTOMÁTICAMENTE en cada request.
El frontend NO almacena ni lee tokens. No se requiere header Authorization.
```

### Flujo en Dos Fases para Round Trip

Para vuelos de **ida y vuelta** (`round_trip`), SerpAPI requiere dos llamadas encadenadas que el backend gestiona de forma transparente para el frontend:

```
Fase 1: outbound_selection (sin outbound_selection_token)
  Request  → { trip_type: "round_trip", departure: "MAD", arrival: "LIM", ... }
  Response → { phase: "outbound_selection", other_flights[] con departure_token en cada vuelo }
  
  El usuario elige un vuelo de ida → obtiene su departure_token

Fase 2: return_selection (con outbound_selection_token)
  Request  → { ...mismos params..., outbound_selection_token: "<token>" }
  Response → { phase: "return_selection", other_flights[] con booking_token en cada vuelo }
  
  El usuario elige un vuelo de vuelta → obtiene su booking_token
```

Para **one_way** y **multi_city**: una sola llamada devuelve `phase: "complete"` con `booking_token` directamente.

### Política de Cookies para Búsqueda

| Cookie | Nombre | TTL | Propósito |
|--------|--------|-----|-----------|
| Access Token | `__Secure-access_token` | 15 min | Sesión activa (opcional en búsqueda) |
| Refresh Token | `__Secure-refresh_token` | 7 días | Rotación de sesión (opcional en búsqueda) |

> Los endpoints de búsqueda **no requieren autenticación**. Si las cookies están presentes, el backend las usa para personalizar resultados (país, idioma, moneda desde el perfil). Si no están presentes, el frontend debe enviar `gl`, `hl`, `currency` explícitamente.

---

## Seguridad de Cookies

### Atributos Obligatorios

| Atributo | Valor | Propósito |
|----------|-------|-----------|
| `HttpOnly` | `true` | Inaccesible vía JavaScript (mitiga XSS) |
| `Secure` | `true` | Solo HTTPS en producción |
| `SameSite` | `Lax` | Protección CSRF. Permite navegación top-level |
| `Path` | `/` | Disponible en todas las rutas |
| `Partitioned` | `true` | CHIPS — permite cookies en contextos de terceros sin third-party cookies |
| `Domain` | `.proactrip.com` | Compartido entre subdominios (omitir si usas `__Host-`) |

### Formato de Producción

```
Set-Cookie: __Secure-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900; Partitioned
Set-Cookie: __Secure-refresh_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800; Partitioned
```

### Limpieza de Cookies (Logout)

```
Set-Cookie: __Secure-access_token=; Max-Age=0; Path=/; Domain=.proactrip.com; Secure; Partitioned
Set-Cookie: __Secure-refresh_token=; Max-Age=0; Path=/; Domain=.proactrip.com; Secure; Partitioned
Clear-Site-Data: "cookies"
```

---

## Base URLs

| Entorno | Base URL |
|---------|----------|
| **Production** | `https://api.proactrip.com/v1/search` |
| **Development** | `http://localhost:8080/v1/search` |

Todos los ejemplos usan `{base_url}` como placeholder.

---

## Errores Estándar

Formato **RFC 7807 Problem Details**:

```json
{
  "type": "validation_error",
  "title": "Validation Error",
  "status": 400,
  "detail": "El campo 'trip_type' es requerido",
  "instance": "/v1/search/flights",
  "trace_id": "019d5439-cb43-716d-90b5-51dcbe980908"
}
```

**Headers de respuesta en TODOS los endpoints:**

| Header | Descripción |
|--------|-------------|
| `X-Trace-Id` | UUID v7 para trazabilidad. Asignado globalmente por middleware, nunca por handlers individuales |
| `traceparent` | W3C Trace Context |

---

## Estrategia de Refresco de Tokens

El backend maneja el refresco de tokens transparentemente vía middleware.

- Si `access_token` es válido → la petición continúa
- Si `access_token` está expirado pero `refresh_token` es válido → nuevos tokens emitidos automáticamente
- Si ambos están expirados → el request continúa sin autenticación (búsqueda pública)

El frontend nunca llama manualmente a `/refresh-token`. Las cookies se gestionan solas.

---

## Search Flights

Busca vuelos de ida y vuelta, solo ida, o multi-destino.

### Flujo de Búsqueda

```
┌─────────────────────────────────────────────────────────────────────┐
│                      ROUND TRIP (2 fases)                           │
│                                                                     │
│  FASE 1: POST /v1/search/flights                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Request:  { trip_type:"round_trip", departure:"MAD",         │  │
│  │            arrival:"LIM", outbound_date, return_date, ... }  │  │
│  │ Response: { phase:"outbound_selection",                      │  │
│  │            other_flights[].departure_token ← ¡USAR ESTE! }   │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                            ↓                                        │
│        Usuario elige vuelo de ida → su departure_token              │
│                            ↓                                        │
│  FASE 2: POST /v1/search/flights                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Request:  { ...mismos params...,                             │  │
│  │            outbound_selection_token:"<departure_token>" }    │  │
│  │ Response: { phase:"return_selection",                        │  │
│  │            other_flights[].booking_token ← ¡USAR ESTE! }     │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                            ↓                                        │
│        Usuario elige vuelo de vuelta → su booking_token             │
│                            ↓                                        │
│  POST /v1/search/flight-details { booking_token }                   │
│  → Itinerario completo + booking_options                            │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                      ONE WAY (1 fase)                               │
│                                                                     │
│  POST /v1/search/flights                                            │
│  Request:  { trip_type:"one_way", departure:"MAD",                 │
│              arrival:"LIM", outbound_date, ... }                    │
│  Response: { phase:"complete",                                     │
│              best_flights[].booking_token,                         │
│              other_flights[].booking_token }                       │
│                            ↓                                        │
│  POST /v1/search/flight-details { booking_token }                   │
│  → Itinerario completo + booking_options                            │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                      MULTI CITY (1 o 2 fases)                       │
│                                                                     │
│  POST /v1/search/flights                                            │
│  Request:  { trip_type:"multi_city", legs:[...], ... }             │
│  Response: { phase:"complete",                                     │
│              best_flights[].departure_token o booking_token,       │
│              other_flights[].departure_token o booking_token }     │
│                            ↓                                        │
│  Si hay booking_token → POST /v1/search/flight-details              │
│  Si hay departure_token → segunda llamada como round_trip           │
└─────────────────────────────────────────────────────────────────────┘
```

### Máquina de Estados

| Estado | Fase | trip_type | Token en respuesta | Próximo paso |
|--------|------|-----------|--------------------|--------------|
| `outbound_selection` | Búsqueda de ida | `round_trip` | `departure_token` | Segunda llamada con `outbound_selection_token` |
| `return_selection` | Búsqueda de vuelta | `round_trip` | `booking_token` | Llamar a `/flight-details` |
| `complete` | Completo | `one_way`, `multi_city` | `booking_token` | Llamar a `/flight-details` |
| `empty` | Sin resultados | cualquiera | — | Refinar parámetros y reintentar |

### Request

```
POST /v1/search/flights
```

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Content-Type` | string | Sí | `application/json` |
| `X-Trace-Id` | string | No | UUID v7 opcional para trazabilidad. El middleware asigna uno automáticamente si no se envía |

> Las cookies `__Secure-access_token` y `__Secure-refresh_token` se envían automáticamente si existen. No se requiere header `Authorization`.

**Body: Todos los campos disponibles**

```json
{
  "trip_type": "round_trip",
  "departure": "MAD",
  "arrival": "LIM",
  "outbound_date": "2026-03-20",
  "return_date": "2026-03-30",
  "legs": [],
  "adults": 2,
  "children": 0,
  "infants_in_seat": 0,
  "infants_on_lap": 0,
  "travel_class": "economy",
  "gl": "ES",
  "hl": "es",
  "currency": "EUR",
  "bags": 0,
  "max_price": null,
  "sort_by": "top",
  "stops": "any",
  "include_airlines": [],
  "exclude_airlines": [],
  "outbound_times": null,
  "return_times": null,
  "emissions_filter": false,
  "layover_duration": null,
  "exclude_connections": [],
  "max_duration_minutes": null,
  "cursor": null,
  "limit": 10,
  "outbound_selection_token": null
}
```

**Campos Requeridos:**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `trip_type` | string | Tipo de viaje: `"round_trip"`, `"one_way"`, `"multi_city"` |
| `departure` | string | Identificador del origen. Puede ser código IATA de aeropuerto (ej: `"MAD"`, `"LIM"`), kgmid de ciudad/región (ej: `"/m/04jpl"`), o múltiples separados por coma (ej: `"CDG,ORY,/m/04jpl"`). **Requerido para `round_trip` y `one_way`. Validado como no-vacío** |
| `arrival` | string | Identificador del destino. Mismo formato que `departure`. **Requerido para `round_trip` y `one_way`. Validado como no-vacío** |
| `outbound_date` | string | Fecha de salida. Formato `YYYY-MM-DD`. **Requerido para `round_trip` y `one_way`. Validado como no-vacío** |
| `return_date` | string | Fecha de vuelta. Formato `YYYY-MM-DD`. **Requerido solo para `round_trip`. Validado como no-vacío en ese caso** |
| `legs` | array | Tramos del viaje. **Requerido solo para `multi_city`** |

**Campos Opcionales:**

| Campo | Tipo | Default | Descripción |
|-------|------|---------|-------------|
| `adults` | integer | `1` | Número de adultos. Mínimo 1 |
| `children` | integer | `0` | Número de niños (2-11 años) |
| `infants_in_seat` | integer | `0` | Bebés con asiento propio (0-1 años) |
| `infants_on_lap` | integer | `0` | Bebés en el regazo (no ocupan asiento) |
| `travel_class` | string | `"economy"` | Clase de vuelo. Ver tabla de valores |
| `gl` | string\|null | `null` | Código ISO 3166-1 alpha-2. Ej: `"ES"`, `"PE"`. Personaliza resultados al país |
| `hl` | string\|null | `null` | Código de idioma ISO 639-1. Ej: `"es"`, `"en"`, `"fr"` |
| `currency` | string | `"USD"` | Código ISO 4217. Ej: `"EUR"`, `"GBP"` |
| `bags` | integer | `0` | Número de bolsos de mano. No puede superar el total de pasajeros con derecho a equipaje |
| `max_price` | number\|null | `null` | Precio máximo total del billete |
| `sort_by` | string | `"top"` | Orden de resultados. Ver tabla de valores |
| `stops` | string | `"any"` | Número de escalas. Ver tabla de valores |
| `include_airlines` | string[] | `[]` | Incluir solo estas aerolíneas (códigos IATA de 2 letras). No usar con `exclude_airlines`. Acepta alianzas: `"STAR_ALLIANCE"`, `"SKYTEAM"`, `"ONEWORLD"` |
| `exclude_airlines` | string[] | `[]` | Excluir estas aerolíneas. No usar con `include_airlines` |
| `outbound_times` | object\|null | `null` | Rango de horas para el vuelo de ida. Ver estructura |
| `return_times` | object\|null | `null` | Rango de horas para el vuelo de vuelta. Solo para `round_trip` |
| `emissions_filter` | boolean | `false` | `true` para mostrar solo vuelos con emisiones inferiores a la media |
| `layover_duration` | object\|null | `null` | Rango de duración de escalas en minutos. Ver estructura |
| `exclude_connections` | string[] | `[]` | Aeropuertos de conexión a excluir. Ej: `["CDG", "BOG"]` |
| `max_duration_minutes` | integer\|null | `null` | Duración máxima total del viaje en minutos |
| `cursor` | string\|null | `null` | Cursor opaco para paginación. `null` para la primera página |
| `limit` | integer | `10` | Número de resultados por página. Mínimo 1, máximo 100 |
| `outbound_selection_token` | string\|null | `null` | Token del vuelo de ida seleccionado. **Solo para la segunda llamada de `round_trip`** |

### Valores de Campos Enumerados

**`trip_type`:**

| Valor | Descripción |
|-------|-------------|
| `"round_trip"` | Ida y vuelta. Requiere `outbound_date` y `return_date` |
| `"one_way"` | Solo ida. Requiere solo `outbound_date` |
| `"multi_city"` | Múltiples destinos. Requiere campo `legs` |

**`travel_class`:**

| Valor | Descripción |
|-------|-------------|
| `"economy"` | Económica (por defecto) |
| `"premium_economy"` | Económica premium |
| `"business"` | Business |
| `"first"` | Primera clase |

**`sort_by`:**

| Valor | Descripción |
|-------|-------------|
| `"top"` | Mejores vuelos (por defecto) |
| `"price"` | Precio más bajo |
| `"departure_time"` | Hora de salida |
| `"arrival_time"` | Hora de llegada |
| `"duration"` | Duración total |
| `"emissions"` | Menor emisión de CO₂ |

**`stops`:**

| Valor | Descripción |
|-------|-------------|
| `"any"` | Cualquier número de escalas (por defecto) |
| `"nonstop"` | Solo vuelos directos |
| `"max_1"` | 1 escala o menos |
| `"max_2"` | 2 escalas o menos |

### Estructuras de Campos Complejos

**`outbound_times` / `return_times`:**

Rango de horas en formato de 24 horas (0–23). Los campos de llegada son opcionales.

```json
{
  "departure_from": 6,
  "departure_to": 18,
  "arrival_from": null,
  "arrival_to": null
}
```

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `departure_from` | integer | Sí | Hora mínima de salida (0–23) |
| `departure_to` | integer | Sí | Hora máxima de salida (0–23) |
| `arrival_from` | integer | No | Hora mínima de llegada (0–23) |
| `arrival_to` | integer | No | Hora máxima de llegada (0–23) |

**`layover_duration`:**

```json
{
  "min_minutes": 90,
  "max_minutes": 330
}
```

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `min_minutes` | integer | Sí | Duración mínima de escala en minutos |
| `max_minutes` | integer | Sí | Duración máxima de escala en minutos |

**`legs` (solo para `multi_city`):**

Array de tramos en orden. Cada tramo tiene su propio origen, destino, fecha y restricción horaria opcional.

```json
[
  {
    "departure": "MAD",
    "arrival": "MIA",
    "date": "2026-11-02",
    "times": {
      "departure_from": 6,
      "departure_to": 18,
      "arrival_from": 10,
      "arrival_to": 23
    }
  },
  {
    "departure": "MIA",
    "arrival": "MAD",
    "date": "2026-11-12"
  }
]
```

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `departure` | string | Sí | Origen del tramo (códigos IATA separados por coma) |
| `arrival` | string | Sí | Destino del tramo |
| `date` | string | Sí | Fecha del tramo. Formato `YYYY-MM-DD` |
| `times` | object | No | Restricción horaria del tramo. Misma estructura que `outbound_times` |

Los aeropuertos pueden ser múltiples, separados por coma: `"departure": "LAX,SFO"` busca salidas desde cualquiera de los dos.

### Notas sobre Parámetros de Localización

La ubicación del usuario la resuelve **siempre el backend** a partir de la IP de la request. El frontend nunca llama a APIs externas de geolocalización directamente.

**Usuarios no autenticados:** Al cargar la página por primera vez, el frontend llama a `GET /v1/environment` para obtener la ubicación y el clima detectados desde la IP. Con esa respuesta, el frontend muestra un mensaje al usuario indicando la ubicación y el clima detectados y pone por defecto `gl`, `hl` y `currency` en las búsquedas posteriores pero el usuario puede cambiarlos.

```
Frontend (primera carga en cualquier página)
  │
  ├── GET /v1/environment  →  { location: {...}, weather: {...} }
  │
  └── POST /v1/search/flights  { departure:"MAD", gl:"ES", hl:"es", currency:"EUR", ... }
```

El endpoint `/v1/environment` usa ipquery.io internamente para resolver la IP y OpenWeather para el clima. 

**Usuarios autenticados:** El backend usa los datos guardados en el perfil (país, idioma, moneda) y el `environment` recibido cacheados o no. Los parámetros `gl`, `hl`, `currency` pueden omitirse o usarse para sobrescribir temporalmente las preferencias.

### Ejemplos curl

#### Round trip — primera llamada (fase outbound)

```bash
curl -X POST {base_url}/flights \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ...; __Secure-refresh_token=v4.local.eyJ..." \
  -d '{
    "trip_type": "round_trip",
    "departure": "MAD",
    "arrival": "LIM",
    "outbound_date": "2026-03-20",
    "return_date": "2026-03-30",
    "adults": 2,
    "travel_class": "economy",
    "currency": "EUR",
    "include_airlines": ["LA"],
    "stops": "nonstop",
    "emissions_filter": true,
    "max_duration_minutes": 3000,
    "sort_by": "price"
  }'
```

> **Nota:** Las cookies se envían con `-b` solo si el usuario está autenticado. Para búsquedas anónimas, omitir el flag `-b`.

#### Round trip — segunda llamada (fase return, tras seleccionar vuelo de ida)

```bash
curl -X POST {base_url}/flights \
  -H "Content-Type: application/json" \
  -d '{
    "trip_type": "round_trip",
    "departure": "MAD",
    "arrival": "LIM",
    "outbound_date": "2026-03-20",
    "return_date": "2026-03-30",
    "adults": 2,
    "travel_class": "economy",
    "currency": "EUR",
    "sort_by": "price",
    "outbound_selection_token": "WyJDalJJTVVKWE5VOTVkeTFQZFVsQlRHRldWbmRDUnkw..."
  }'
```

#### Solo ida (one_way)

```bash
curl -X POST {base_url}/flights \
  -H "Content-Type: application/json" \
  -d '{
    "trip_type": "one_way",
    "departure": "MAD",
    "arrival": "LIM",
    "outbound_date": "2026-03-20",
    "adults": 2,
    "currency": "EUR",
    "sort_by": "price"
  }'
```

#### Multi-destino (multi_city)

```bash
curl -X POST {base_url}/flights \
  -H "Content-Type: application/json" \
  -d '{
    "trip_type": "multi_city",
    "adults": 2,
    "children": 2,
    "infants_in_seat": 1,
    "infants_on_lap": 1,
    "travel_class": "economy",
    "currency": "EUR",
    "bags": 5,
    "max_price": 10000,
    "stops": "max_2",
    "include_airlines": ["UA"],
    "emissions_filter": true,
    "layover_duration": { "min_minutes": 90, "max_minutes": 330 },
    "exclude_connections": ["CDG", "AUS"],
    "max_duration_minutes": 3000,
    "sort_by": "price",
    "legs": [
      {
        "departure": "MAD",
        "arrival": "MIA",
        "date": "2026-11-02",
        "times": { "departure_from": 6, "departure_to": 18, "arrival_from": 10, "arrival_to": 23 }
      },
      {
        "departure": "MIA",
        "arrival": "MAD",
        "date": "2026-11-12",
        "times": { "departure_from": 6, "departure_to": 20, "arrival_from": 10, "arrival_to": 23 }
      }
    ]
  }'
```

#### Con filtros avanzados de horario

```bash
curl -X POST {base_url}/flights \
  -H "Content-Type: application/json" \
  -d '{
    "trip_type": "round_trip",
    "departure": "MAD",
    "arrival": "LIM",
    "outbound_date": "2026-03-20",
    "return_date": "2026-03-30",
    "adults": 2,
    "currency": "EUR",
    "outbound_times": { "departure_from": 8, "departure_to": 20 },
    "return_times": { "departure_from": 6, "departure_to": 22 },
    "exclude_connections": ["SCL", "BOG"],
    "max_duration_minutes": 900
  }'
```

### Responses

#### Phase: outbound_selection

Primera llamada de `round_trip`. Devuelve vuelos de ida con `departure_token`.

```json
{
  "trip_type": "round_trip",
  "phase": "outbound_selection",
  "results_state": "matching",
  "best_flights": [],
  "other_flights": [
    {
      "departure_token": "WyJDalJJTVVKWE5VOTVkeTFQZFVsQlRHRldWbmRDUnkwdExTMHRMUzB0TFhaMFltWnFPVUZCUVVGQlIyMHpTVTl6UzI1cWJ6SkJFZ1ZKUWpFeU5Sb0xDTkxLRGhBQ0dnTkZWVkk0SEhEdTFCQT0iLFtbIk1BRCIsIjIwMjYtMDMtMjAiLCJMSU0iLG51bGwsIklCIiwiMTI1Il1dXQ==",
      "legs": [
        {
          "departure": {
            "airport_code": "MAD",
            "airport_name": "Aeropuerto Adolfo Suárez Madrid-Barajas",
            "city": "Madrid",
            "country": "España",
            "country_code": "ES",
            "datetime": "2026-03-20 13:10"
          },
          "arrival": {
            "airport_code": "LIM",
            "airport_name": "Nuevo Aeropuerto Internacional Jorge Chávez",
            "city": "Lima",
            "country": "Perú",
            "country_code": "PE",
            "datetime": "2026-03-20 19:10"
          },
          "duration_minutes": 720,
          "aircraft": "Airbus A320",
          "airline": "Iberia",
          "airline_code": "IB",
          "airline_logo_url": "https://www.gstatic.com/flights/airline_logos/70px/IB.png",
          "flight_number": "IB 125",
          "travel_class": "Economy",
          "legroom": "79 cm",
          "legroom_quality": "average",
          "also_sold_by": ["LATAM"],
          "features": {
            "wifi": "paid",
            "power_outlets": true,
            "usb": true,
            "entertainment": "on_demand",
            "raw": [
              "Average legroom (79 cm)",
              "Wi-Fi for a fee",
              "In-seat power & USB outlets",
              "On-demand video",
              "Carbon emissions estimate: 1146 kg"
            ]
          },
          "overnight": false,
          "often_delayed": true,
          "operated_by": "Iberia for Latam Airlines Group"
        }
      ],
      "layovers": [],
      "total_duration_minutes": 720,
      "price": {
        "amount": 2390,
        "currency": "EUR"
      },
      "carbon_emissions": {
        "this_flight_grams": 1146000,
        "typical_route_grams": 1242000,
        "difference_percent": -8
      },
      "type": "Round trip",
      "airline_logo_url": "https://www.gstatic.com/flights/airline_logos/70px/IB.png"
    }
  ],
  "airports": [
    {
      "role": "departure",
      "airport_code": "MAD",
      "airport_name": "Aeropuerto Adolfo Suárez Madrid-Barajas",
      "city": "Madrid",
      "country": "España",
      "country_code": "ES",
      "image_url": "https://encrypted-tbn0.gstatic.com/images?q=tbn:ANd9GcQ...",
      "thumbnail_url": "https://encrypted-tbn0.gstatic.com/images?q=tbn:ANd9GcQ..."
    },
    {
      "role": "arrival",
      "airport_code": "LIM",
      "airport_name": "Nuevo Aeropuerto Internacional Jorge Chávez",
      "city": "Lima",
      "country": "Perú",
      "country_code": "PE",
      "image_url": "https://encrypted-tbn2.gstatic.com/images?q=tbn:ANd9GcQ...",
      "thumbnail_url": "https://encrypted-tbn2.gstatic.com/images?q=tbn:ANd9GcQ..."
    }
  ],
  "price_insights": {
    "lowest_price": { "amount": 2390, "currency": "EUR" },
    "price_level": "high",
    "typical_range": {
      "min": 1500,
      "max": 1900,
      "currency": "EUR"
    },
    "price_history": [
      { "timestamp": 1691424000, "price": 1877 },
      { "timestamp": 1696176000, "price": 2390 }
    ]
  },
  "meta": {
    "next_cursor": "eyJvZmZzZXQiOjEwfQ==",
    "prev_cursor": null,
    "has_next": true,
    "limit": 10
  },
  "from_cache": false,
  "cached_at": null
}
```

**Response Headers:**

```
X-Trace-Id: 019ef5439-cb43-716d-90b5-51dcbe980908
traceparent: 00-019ef5439cb43716d90b551dcbe980908-a1b2c3d4e5f67890-01
```

> Si el usuario está autenticado y la sesión fue refrescada, se incluyen nuevos `Set-Cookie` headers con los tokens rotados.

#### Phase: return_selection

Segunda llamada de `round_trip` (con `outbound_selection_token`). Devuelve vuelos de vuelta con `booking_token`.

```json
{
  "trip_type": "round_trip",
  "phase": "return_selection",
  "results_state": "matching",
  "best_flights": [],
  "other_flights": [
    {
      "booking_token": "WyJDalJJT1ZSSmJGQnFRMVpwUVZWQlQwcFFYMmRDUnkwdExTMHRMUzB0YjNsaWFIazBORUZCUVVGQlIyMHpTV1ZyUlhGMmMwdEJFZ1ZKUWpFeU5ob0xDTkxLRGhBQ0dnTkZWVkk0SEhDUzFCQT0=",
      "legs": [
        {
          "departure": {
            "airport_code": "LIM",
            "airport_name": "Nuevo Aeropuerto Internacional Jorge Chávez",
            "city": "Lima",
            "country": "Perú",
            "country_code": "PE",
            "datetime": "2026-03-30 20:00"
          },
          "arrival": {
            "airport_code": "MAD",
            "airport_name": "Aeropuerto Adolfo Suárez Madrid-Barajas",
            "city": "Madrid",
            "country": "España",
            "country_code": "ES",
            "datetime": "2026-03-31 14:20"
          },
          "duration_minutes": 680,
          "aircraft": "Airbus A350",
          "airline": "Iberia",
          "airline_code": "IB",
          "airline_logo_url": "https://www.gstatic.com/flights/airline_logos/70px/IB.png",
          "flight_number": "IB 126",
          "travel_class": "Economy",
          "legroom": "79 cm",
          "legroom_quality": "average",
          "also_sold_by": ["LATAM"],
          "features": {
            "wifi": "paid",
            "power_outlets": true,
            "usb": true,
            "entertainment": "on_demand",
            "raw": [
              "Average legroom (79 cm)",
              "Wi-Fi for a fee",
              "In-seat power & USB outlets",
              "On-demand video",
              "Carbon emissions estimate: 1146 kg"
            ]
          },
          "overnight": true,
          "often_delayed": false,
          "operated_by": "Iberia for Latam Airlines Group"
        }
      ],
      "layovers": [],
      "total_duration_minutes": 680,
      "price": {
        "amount": 2390,
        "currency": "EUR"
      },
      "carbon_emissions": {
        "this_flight_grams": 1147000,
        "typical_route_grams": 1276000,
        "difference_percent": -10
      },
      "type": "Round trip",
      "airline_logo_url": "https://www.gstatic.com/flights/airline_logos/70px/IB.png"
    }
  ],
  "airports": [
    {
      "role": "departure",
      "airport_code": "LIM",
      "airport_name": "Nuevo Aeropuerto Internacional Jorge Chávez",
      "city": "Lima",
      "country": "Perú",
      "country_code": "PE",
      "image_url": "https://encrypted-tbn2.gstatic.com/images?q=tbn:ANd9GcQ...",
      "thumbnail_url": "https://encrypted-tbn2.gstatic.com/images?q=tbn:ANd9GcQ..."
    },
    {
      "role": "arrival",
      "airport_code": "MAD",
      "airport_name": "Aeropuerto Adolfo Suárez Madrid-Barajas",
      "city": "Madrid",
      "country": "España",
      "country_code": "ES",
      "image_url": "https://encrypted-tbn0.gstatic.com/images?q=tbn:ANd9GcQ...",
      "thumbnail_url": "https://encrypted-tbn0.gstatic.com/images?q=tbn:ANd9GcQ..."
    }
  ],
  "price_insights": null,
  "meta": {
    "next_cursor": "eyJvZmZzZXQiOjEwfQ==",
    "prev_cursor": null,
    "has_next": true,
    "limit": 10
  },
  "from_cache": false,
  "cached_at": null
}
```

#### Phase: complete (one_way)

Vuelos de solo ida. `best_flights` y `other_flights` contienen `booking_token` directamente.

```json
{
  "trip_type": "one_way",
  "phase": "complete",
  "results_state": "matching",
  "best_flights": [
    {
      "booking_token": "WyJDalJJTVVKWE5VOTVkeTFQZFVsQlRHRldWbmRDUnkwdExTMHRMUzB0Um5GTlIyMWhNRFZFUVVGQlIyMHpTVTl6UzI1cWJ6SkJFZ1ZKUWpFeU5SUnJURVJSVGxWb1JFY3hjbFY0VmpWeFdETmhSM2t3TlRSVlNGVXROa2xaUTBoQlIzZFZSMWxEUlhrMU1XTkRaV3h3UkcweU1XTjJabmxqUVhwM1R6TnFNRXA2TnpOUldsbHJXRUZGVkdOTFJqRk5VbFZCVUZwclQwOXpRVGd5VFZoUWVsZFljMms5UFElM0QlM0Q=",
      "legs": [
        {
          "departure": {
            "airport_code": "MAD",
            "airport_name": "Aeropuerto Adolfo Suárez Madrid-Barajas",
            "city": "Madrid",
            "country": "España",
            "country_code": "ES",
            "datetime": "2026-03-20 13:10"
          },
          "arrival": {
            "airport_code": "LIM",
            "airport_name": "Nuevo Aeropuerto Internacional Jorge Chávez",
            "city": "Lima",
            "country": "Perú",
            "country_code": "PE",
            "datetime": "2026-03-20 19:10"
          },
          "duration_minutes": 720,
          "aircraft": "Airbus A350-900",
          "airline": "Iberia",
          "airline_code": "IB",
          "airline_logo_url": "https://www.gstatic.com/flights/airline_logos/70px/IB.png",
          "flight_number": "IB 125",
          "travel_class": "Economy",
          "legroom": "79 cm",
          "legroom_quality": "average",
          "also_sold_by": ["LATAM"],
          "features": {
            "wifi": "paid",
            "power_outlets": true,
            "usb": true,
            "entertainment": "on_demand",
            "raw": [
              "Average legroom (79 cm)",
              "Wi-Fi for a fee",
              "In-seat power & USB outlets",
              "On-demand video",
              "Carbon emissions estimate: 1146 kg"
            ]
          },
          "overnight": false,
          "often_delayed": true,
          "operated_by": "Iberia for Latam Airlines Group"
        }
      ],
      "layovers": [],
      "total_duration_minutes": 720,
      "price": {
        "amount": 1877,
        "currency": "EUR"
      },
      "carbon_emissions": {
        "this_flight_grams": 1146000,
        "typical_route_grams": 1242000,
        "difference_percent": -8
      },
      "type": "One way",
      "airline_logo_url": "https://www.gstatic.com/flights/airline_logos/70px/IB.png"
    }
  ],
  "other_flights": [
    {
      "booking_token": "WyJDalJJTVVKWE5VOTVkeTFQZFVsQlRHRldWbmRDUnkwdExTMHRMUzB0VFhaMFltWnFPVUZCUVVGQlIyMHpTVTl6UzI1cWJ6SkJFZ1ZKUWpFeU5Sb0xDTkxLRGhBQ0dnTkZWVkk0SEhEdTFCQT0...",
      "legs": [
        {
          "departure": {
            "airport_code": "MAD",
            "airport_name": "Aeropuerto Adolfo Suárez Madrid-Barajas",
            "city": "Madrid",
            "country": "España",
            "country_code": "ES",
            "datetime": "2026-03-21 08:00"
          },
          "arrival": {
            "airport_code": "LIM",
            "airport_name": "Nuevo Aeropuerto Internacional Jorge Chávez",
            "city": "Lima",
            "country": "Perú",
            "country_code": "PE",
            "datetime": "2026-03-21 14:00"
          },
          "duration_minutes": 720,
          "aircraft": "Boeing 787",
          "airline": "LATAM",
          "airline_code": "LA",
          "airline_logo_url": "https://www.gstatic.com/flights/airline_logos/70px/LA.png",
          "flight_number": "LA 2485",
          "travel_class": "Economy",
          "legroom": "81 cm",
          "legroom_quality": "above_average",
          "also_sold_by": ["Iberia", "British Airways"],
          "features": {
            "wifi": "free",
            "power_outlets": true,
            "usb": true,
            "entertainment": "stream",
            "raw": [
              "Above average legroom (81 cm)",
              "Free Wi-Fi",
              "In-seat power & USB outlets",
              "Stream media to your device",
              "Carbon emissions estimate: 1200 kg"
            ]
          },
          "overnight": false,
          "often_delayed": false,
          "operated_by": null
        }
      ],
      "layovers": [],
      "total_duration_minutes": 720,
      "price": {
        "amount": 2100,
        "currency": "EUR"
      },
      "carbon_emissions": {
        "this_flight_grams": 1200000,
        "typical_route_grams": 1242000,
        "difference_percent": -3
      },
      "type": "One way",
      "airline_logo_url": "https://www.gstatic.com/flights/airline_logos/70px/LA.png"
    }
  ],
  "airports": [
    {
      "role": "departure",
      "airport_code": "MAD",
      "airport_name": "Aeropuerto Adolfo Suárez Madrid-Barajas",
      "city": "Madrid",
      "country": "España",
      "country_code": "ES",
      "image_url": "https://encrypted-tbn0.gstatic.com/images?q=tbn:ANd9GcQ...",
      "thumbnail_url": "https://encrypted-tbn0.gstatic.com/images?q=tbn:ANd9GcQ..."
    },
    {
      "role": "arrival",
      "airport_code": "LIM",
      "airport_name": "Nuevo Aeropuerto Internacional Jorge Chávez",
      "city": "Lima",
      "country": "Perú",
      "country_code": "PE",
      "image_url": "https://encrypted-tbn2.gstatic.com/images?q=tbn:ANd9GcQ...",
      "thumbnail_url": "https://encrypted-tbn2.gstatic.com/images?q=tbn:ANd9GcQ..."
    }
  ],
  "price_insights": {
    "lowest_price": { "amount": 1877, "currency": "EUR" },
    "price_level": "low",
    "typical_range": {
      "min": 1500,
      "max": 1900,
      "currency": "EUR"
    },
    "price_history": [
      { "timestamp": 1691424000, "price": 1877 },
      { "timestamp": 1691500000, "price": 1790 },
      { "timestamp": 1691600000, "price": 1850 },
      { "timestamp": 1696176000, "price": 2390 },
      { "timestamp": 1696262400, "price": 2200 }
    ]
  },
  "meta": {
    "next_cursor": "eyJvZmZzZXQiOjEwfQ==",
    "prev_cursor": null,
    "has_next": true,
    "limit": 10
  },
  "from_cache": false,
  "cached_at": null
}
```

#### Sin Resultados

Cuando no se encuentran vuelos para la búsqueda.

```json
{
  "trip_type": "multi_city",
  "phase": "complete",
  "results_state": "empty",
  "best_flights": [],
  "other_flights": [],
  "airports": [
    {
      "role": "departure",
      "airport_code": "MAD",
      "airport_name": "Aeropuerto Adolfo Suárez Madrid-Barajas",
      "city": "Madrid",
      "country": "España",
      "country_code": "ES",
      "image_url": null,
      "thumbnail_url": null
    },
    {
      "role": "arrival",
      "airport_code": "LIM",
      "airport_name": "Nuevo Aeropuerto Internacional Jorge Chávez",
      "city": "Lima",
      "country": "Perú",
      "country_code": "PE",
      "image_url": null,
      "thumbnail_url": null
    }
  ],
  "price_insights": null,
  "meta": {
    "next_cursor": null,
    "prev_cursor": null,
    "has_next": false,
    "limit": 10
  },
  "from_cache": false,
  "cached_at": null
}
```

### Response Fields Explained

#### Nivel Raíz

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `trip_type` | string | Tipo de viaje: `"round_trip"`, `"one_way"`, `"multi_city"` |
| `phase` | string | `"outbound_selection"` (primera llamada de round_trip), `"return_selection"` (segunda llamada de round_trip), `"complete"` (one_way y multi_city) |
| `results_state` | string | `"matching"` = resultados encontrados, `"empty"` = sin resultados |
| `best_flights` | array | Vuelos destacados/recomendados por el proveedor. Puede ser `[]` con filtros específicos. Aparece en `one_way` y `multi_city` con resultados. **No aparece** en `round_trip` |
| `other_flights` | array | Resto de vuelos disponibles. Aparece en todas las fases excepto `empty` |
| `airports` | array | Información completa de los aeropuertos involucrados en la búsqueda. **Siempre presente**, incluso con 0 resultados |
| `price_insights` | object\|null | Análisis de precios históricos para la ruta. Solo en primera llamada de `round_trip` y en `one_way`. `null` en fase `return_selection` y `multi_city` |
| `from_cache` | boolean | `true` si la respuesta vino de caché |
| `cached_at` | string\|null | Timestamp ISO 8601 del momento en que se cacheó. `null` si no es de caché |
| `meta` | object\|null | Metadatos de paginación. Ver sección [Paginación](#paginación). `null` cuando no hay suficientes resultados para paginar |

#### Flight Object (elemento de `best_flights[]` u `other_flights[]`)

| Campo | Tipo | Aparece en | Descripción |
|-------|------|------------|-------------|
| `departure_token` | string | `phase: "outbound_selection"` | **Token en RESPUESTA.** Se usa para la segunda llamada de round_trip como `outbound_selection_token` en el body del request |
| `booking_token` | string | `phase: "return_selection"`, `"complete"` | **Token en RESPUESTA.** Se usa en `POST /v1/search/flight-details` para obtener opciones de reserva |
| `legs` | array | Todas las fases con resultados | Segmentos de vuelo. Un vuelo directo tiene 1 leg. Con escala, 2 o más |
| `layovers` | array | Todas las fases con resultados | Escalas entre legs. Vacío `[]` si el vuelo es directo |
| `total_duration_minutes` | integer | Todas las fases con resultados | Duración total incluyendo esperas en escalas |
| `price.amount` | number | Todas excepto fase de detalles | Precio total del billete (todos los pasajeros) |
| `price.currency` | string | Todas excepto fase de detalles | Moneda del precio |
| `carbon_emissions` | object | Todas las fases con resultados | Datos de emisiones de CO₂ |
| `type` | string | Todas las fases con resultados | `"Round trip"` o `"One way"` |
| `airline_logo_url` | string | Todas las fases con resultados | Logo de la aerolínea principal. Para vuelos multi-aerolínea, puede ser un logo genérico "multi" |

#### Leg (segmento de vuelo)

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `departure.airport_code` | string | Código IATA del aeropuerto de salida |
| `departure.airport_name` | string | Nombre completo del aeropuerto de salida |
| `departure.city` | string | Ciudad de salida |
| `departure.country` | string | País de salida en el idioma de la búsqueda |
| `departure.country_code` | string | Código ISO 3166-1 alpha-2 del país de salida |
| `departure.datetime` | string | Fecha y hora de salida. Formato `"YYYY-MM-DD HH:MM"` |
| `arrival.*` | object | Igual estructura que `departure` pero para la llegada |
| `duration_minutes` | integer | Duración del segmento en minutos |
| `aircraft` | string | Modelo del avión. Ej: `"Airbus A350"`, `"Boeing 787"` |
| `airline` | string | Nombre de la aerolínea operadora |
| `airline_code` | string | Código IATA de 2 letras. Ej: `"IB"`, `"LA"` |
| `airline_logo_url` | string | URL del logo de la aerolínea |
| `flight_number` | string | Número de vuelo. Ej: `"IB 125"` |
| `travel_class` | string | Clase del billete. Ej: `"Economy"`, `"Business"` |
| `legroom` | string | Espacio entre asientos. Ej: `"79 cm"`, `"31 in"` |
| `legroom_quality` | string | Calidad del espacio: `"average"`, `"above_average"`, `"below_average"` |
| `also_sold_by` | string[] | Aerolíneas que también venden este vuelo (codeshare). Vacío `[]` si no hay codeshare |
| `features.wifi` | string\|null | `"free"` = WiFi gratis, `"paid"` = WiFi de pago, `null` = sin WiFi |
| `features.power_outlets` | boolean | Enchufes eléctricos disponibles en el asiento |
| `features.usb` | boolean | Puertos USB disponibles en el asiento |
| `features.entertainment` | string\|null | `"on_demand"` = video bajo demanda, `"stream"` = streaming al dispositivo, `"live_tv"` = TV en vivo, `null` = sin entretenimiento |
| `features.raw` | string[] | Lista original de características sin procesar del proveedor |
| `overnight` | boolean | `true` si el vuelo cruza medianoche |
| `often_delayed` | boolean | **Alerta proactiva.** `true` si este vuelo habitualmente se retrasa más de 30 min. Dato del historial real |
| `operated_by` | string\|null | Operador real si difiere de la aerolínea que vende (codeshare/ wet-lease). `null` si la aerolínea opera su propio vuelo |

#### Layover (escala)

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `airport_code` | string | Código IATA del aeropuerto de escala |
| `airport_name` | string | Nombre del aeropuerto de escala |
| `duration_minutes` | integer | Duración de la escala en minutos |
| `overnight` | boolean | `true` si la escala dura toda la noche |

#### Airport (aeropuerto en el array `airports[]`)

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `role` | string | `"departure"` o `"arrival"` |
| `airport_code` | string | Código IATA |
| `airport_name` | string | Nombre completo |
| `city` | string | Ciudad |
| `country` | string | País en el idioma de la búsqueda |
| `country_code` | string | Código ISO 3166-1 alpha-2 |
| `image_url` | string\|null | Foto del aeropuerto. `null` si no disponible |
| `thumbnail_url` | string\|null | Miniatura del aeropuerto. `null` si no disponible |

#### Price Insights

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `lowest_price` | object | Precio más bajo encontrado en la búsqueda actual: `{ amount, currency }` |
| `price_level` | string | Nivel actual del precio: `"low"`, `"typical"`, `"high"` |
| `typical_range.min` | number | Extremo inferior del rango de precios típico para esta ruta |
| `typical_range.max` | number | Extremo superior del rango de precios típico |
| `typical_range.currency` | string | Moneda del rango típico |
| `price_history` | array | Historial de precios. Cada elemento: `{ timestamp: unix_epoch, price: number }` |

### Paginación

Cuando una búsqueda devuelve más resultados de los que caben en una página, la respuesta incluye metadatos de paginación en el campo `meta`. Cada página se obtiene enviando el `next_cursor` en el siguiente request.

**Campos de `meta`:**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `meta.next_cursor` | string\|null | Cursor opaco para obtener la siguiente página. `null` si no hay más resultados |
| `meta.prev_cursor` | string\|null | Cursor opaco para obtener la página anterior. `null` en la primera página |
| `meta.has_next` | boolean | `true` si hay más resultados disponibles |
| `meta.limit` | integer | Número de resultados por página |

**Uso:**

```
1. Primera página: POST /v1/search/flights { ..., "limit": 10 }
2. Siguiente página: POST /v1/search/flights { ..., "limit": 10, "cursor": "<next_cursor>" }
```

> **Nota:** El cursor es opaco y no debe ser interpretado ni construido por el cliente. Su formato puede cambiar sin previo aviso.

### Tokens: departure_token vs booking_token

Esta es la parte más confusa del API. Leela con atención.

#### Tabla de Nombres

| Contexto | Nombre del campo | Dirección |
|----------|------------------|-----------|
| **RESPUESTA** (fase outbound_selection) | `departure_token` | Backend → Frontend |
| **REQUEST** (fase return_selection) | `outbound_selection_token` | Frontend → Backend |
| **RESPUESTA** (fase return_selection/complete) | `booking_token` | Backend → Frontend |
| **REQUEST** (flight-details) | `booking_token` | Frontend → Backend |

> **Regla mnemotécnica:** El backend SIEMPRE devuelve `departure_token` y `booking_token`. El frontend SIEMPRE envía `outbound_selection_token` y `booking_token`. El `departure_token` de la respuesta se convierte en el `outbound_selection_token` del siguiente request.

#### Ciclo de Vida

```
departure_token (respuesta)
    │
    │  El frontend lo toma tal cual, sin modificarlo
    │
    ▼
outbound_selection_token (request) ──── se envía en la segunda llamada
    │
    │  El backend lo reenvía a SerpAPI
    │
    ▼
booking_token (respuesta) ──── se obtiene en return_selection o complete
    │
    │  El frontend lo toma tal cual
    │
    ▼
booking_token (request) ──── se envía a /flight-details
    │
    ▼
Itinerario completo + booking_options (respuesta)
```

#### Cuándo Aparece Cada Token

| `trip_type` | Fase | `departure_token` | `booking_token` |
|-------------|------|-------------------|-----------------|
| `round_trip` | Primera llamada | En `other_flights[]` | No aparece |
| `round_trip` | Segunda llamada | No aparece | En `other_flights[]` |
| `one_way` | Única llamada | No aparece | En `best_flights[]` y `other_flights[]` |
| `multi_city` | Con resultados | En `best_flights[]` y `other_flights[]` | No aparece (puede haber `booking_token` si el proveedor lo permite) |
| Cualquiera | Sin resultados | No aparece | No aparece |

### Posibles Errores (Search Flights)

| Código | HTTP | Cuándo |
|--------|------|--------|
| `VALIDATION_ERROR` | 400 | Body inválido, `trip_type` no reconocido, faltan campos requeridos según el tipo de viaje, `include_airlines` y `exclude_airlines` usados simultáneamente, `return_date` ausente en `round_trip` |
| `INVALID_PARAM_RANGE` | 422 | Parámetros fuera de rango: `bags` supera el número de pasajeros, horas de rango inválidas, `outbound_selection_token` expirado o inválido |
| `PROVIDER_UNAVAILABLE` | 503 | El proveedor externo (SerpAPI) no está disponible |
| `TOKEN_INVALID` | 401 | Cookie de sesión inválida o expirada (solo si el usuario estaba autenticado) |
| `RATE_LIMIT_EXCEEDED` | 429 | Demasiadas peticiones (RFC 7807 Problem JSON). Ver [Rate Limiting](#rate-limiting) |
| `INTERNAL_ERROR` | 500 | Error inesperado del servidor |

---

## Flight Details

Devuelve el itinerario completo y las opciones de reserva disponibles para un vuelo seleccionado, usando el `booking_token` obtenido de `POST /v1/search/flights`.

### Request (Flight Details)

```
POST /v1/search/flight-details
```

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Content-Type` | string | Sí | `application/json` |

> Las cookies se envían automáticamente si existen. No se requiere header `Authorization`.

**Body:**

```json
{
  "booking_token": "WyJDalJJT1ZSSmJGQnFRMVpwUVZWQlQwcFFYMmRDUnkwdExTMHRMUzB0YjNsaWFIazBORUZCUVVGQlIyMHpTV1ZyUlhGMmMwdEJFZ1ZKUWpFeU5ob0xDTkxLRGhBQ0dnTkZWVkk0SEhDUzFCQT0=",
  "adults": 2,
  "departure": "MAD",
  "arrival": "LIM",
  "outbound_date": "2026-03-20",
  "return_date": "2026-03-30",
  "gl": "ES",
  "hl": "es",
  "currency": "EUR"
}
```

**Campos:**

| Campo | Tipo | Requerido | Default | Descripción |
|-------|------|-----------|---------|-------------|
| `booking_token` | string | Sí | — | Token opaco obtenido de `POST /v1/search/flights` |
| `adults` | integer | No | `1` | Número de adultos. Debe coincidir con la búsqueda original |
| `departure` | string | Sí* | — | Código IATA del origen. Requerido por SerpAPI para validar el token |
| `arrival` | string | Sí* | — | Código IATA del destino. Requerido por SerpAPI para validar el token |
| `outbound_date` | string | Sí* | — | Fecha de ida (`YYYY-MM-DD`). Requerido por SerpAPI para validar el token |
| `return_date` | string | No | — | Fecha de vuelta (`YYYY-MM-DD`). Solo para `round_trip` |
| `gl` | string\|null | No | `null` | Código ISO 3166-1 alpha-2. Ej: `"ES"`, `"PE"` |
| `hl` | string\|null | No | `null` | Código de idioma ISO 639-1. Ej: `"es"`, `"en"` |
| `currency` | string | No | `"USD"` | Código ISO 4217 |

> **Nota:** Aunque el `booking_token` contiene la información del itinerario codificada internamente, SerpAPI requiere los parámetros de ruta (`departure`, `arrival`, `outbound_date`) para validar la solicitud. El frontend debe conservar estos valores de la búsqueda original y enviarlos junto con el token.

### Ejemplos curl

#### Detalle de vuelo ida y vuelta (round_trip)

```bash
curl -X POST {base_url}/flight-details \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ...; __Secure-refresh_token=v4.local.eyJ..." \
  -d '{
    "booking_token": "WyJDalJJT1ZSSmJGQnFRMVpwUVZWQlQwcFFYMmRDUnkw...",
    "adults": 2,
    "departure": "MAD",
    "arrival": "LIM",
    "outbound_date": "2026-03-20",
    "return_date": "2026-03-30",
    "currency": "EUR"
  }'
```

#### Detalle de vuelo solo ida (one_way)

```bash
curl -X POST {base_url}/flight-details \
  -H "Content-Type: application/json" \
  -d '{
    "booking_token": "WyJDalJJTVVKWE5VOTVkeTFQZFVsQlRHRldWbmRDUnkw...",
    "adults": 1,
    "departure": "MAD",
    "arrival": "LIM",
    "outbound_date": "2026-03-20",
    "currency": "USD"
  }'
```

### Responses (Flight Details)

#### 200 OK — Itinerario completo con opciones de reserva

```json
{
  "itinerary": {
    "trip_type": "round_trip",
    "outbound": {
      "legs": [
        {
          "departure": {
            "airport_code": "MAD",
            "airport_name": "Aeropuerto Adolfo Suárez Madrid-Barajas",
            "city": "Madrid",
            "country": "España",
            "country_code": "ES",
            "datetime": "2026-03-20 13:10"
          },
          "arrival": {
            "airport_code": "LIM",
            "airport_name": "Nuevo Aeropuerto Internacional Jorge Chávez",
            "city": "Lima",
            "country": "Perú",
            "country_code": "PE",
            "datetime": "2026-03-20 19:10"
          },
          "duration_minutes": 720,
          "aircraft": "Airbus A350",
          "airline": "Iberia",
          "airline_code": "IB",
          "airline_logo_url": "https://www.gstatic.com/flights/airline_logos/70px/IB.png",
          "flight_number": "IB 125",
          "travel_class": "Economy",
          "legroom": "79 cm",
          "legroom_quality": "average",
          "also_sold_by": ["LATAM"],
          "features": {
            "wifi": "paid",
            "power_outlets": true,
            "usb": true,
            "entertainment": "on_demand",
            "raw": [
              "Average legroom (79 cm)",
              "Wi-Fi for a fee",
              "In-seat power & USB outlets",
              "On-demand video",
              "Carbon emissions estimate: 1146 kg"
            ]
          },
          "overnight": false,
          "often_delayed": true,
          "operated_by": "Iberia for Latam Airlines Group"
        }
      ],
      "layovers": [],
      "total_duration_minutes": 720,
      "carbon_emissions": {
        "this_flight_grams": 1146000,
        "typical_route_grams": 1242000,
        "difference_percent": -8
      }
    },
    "return": {
      "legs": [
        {
          "departure": {
            "airport_code": "LIM",
            "airport_name": "Nuevo Aeropuerto Internacional Jorge Chávez",
            "city": "Lima",
            "country": "Perú",
            "country_code": "PE",
            "datetime": "2026-03-30 20:00"
          },
          "arrival": {
            "airport_code": "MAD",
            "airport_name": "Aeropuerto Adolfo Suárez Madrid-Barajas",
            "city": "Madrid",
            "country": "España",
            "country_code": "ES",
            "datetime": "2026-03-31 14:20"
          },
          "duration_minutes": 680,
          "aircraft": "Airbus A350",
          "airline": "Iberia",
          "airline_code": "IB",
          "airline_logo_url": "https://www.gstatic.com/flights/airline_logos/70px/IB.png",
          "flight_number": "IB 126",
          "travel_class": "Economy",
          "legroom": "79 cm",
          "legroom_quality": "average",
          "also_sold_by": ["LATAM"],
          "features": {
            "wifi": "paid",
            "power_outlets": true,
            "usb": true,
            "entertainment": "on_demand",
            "raw": [
              "Average legroom (79 cm)",
              "Wi-Fi for a fee",
              "In-seat power & USB outlets",
              "On-demand video",
              "Carbon emissions estimate: 1146 kg"
            ]
          },
          "overnight": true,
          "often_delayed": false,
          "operated_by": "Iberia for Latam Airlines Group"
        }
      ],
      "layovers": [],
      "total_duration_minutes": 680,
      "carbon_emissions": {
        "this_flight_grams": 1147000,
        "typical_route_grams": 1276000,
        "difference_percent": -10
      }
    }
  },
  "booking_options": [
    {
      "trip_type": "round_trip",
      "separate_tickets": false,
      "together": {
        "book_with": "Iberia",
        "airline": true,
        "airline_logos": [
          "https://www.gstatic.com/flights/airline_logos/70px/IB.png"
        ],
        "marketed_as": ["IB 125", "IB 126"],
        "price": 2372,
        "option_title": "Turista Básica",
        "baggage_prices": ["1 free carry-on"],
        "booking_request": {
          "url": "https://www.google.com/flights/booking?...",
          "post_data": "eJxdkEFPg0AQhf8Kmb...
        },
        "booking_phone": "+34 900 111 500"
      }
    },
    {
      "trip_type": "round_trip",
      "separate_tickets": true,
      "departing": {
        "book_with": "Iberia",
        "airline": true,
        "airline_logos": ["https://www.gstatic.com/flights/airline_logos/70px/IB.png"],
        "marketed_as": ["IB 125"],
        "price": 1200,
        "option_title": "Turista Básica",
        "booking_phone": "+34 900 111 500"
      },
      "returning": {
        "book_with": "LATAM",
        "airline": true,
        "airline_logos": ["https://www.gstatic.com/flights/airline_logos/70px/LA.png"],
        "marketed_as": ["LA 2485"],
        "price": 1300,
        "option_title": "Económica",
        "booking_phone": "+51 1 213 4343"
      },
      "together": {
        "book_with": "Combinado",
        "airline": true,
        "airline_logos": [
          "https://www.gstatic.com/flights/airline_logos/70px/IB.png",
          "https://www.gstatic.com/flights/airline_logos/70px/LA.png"
        ],
        "marketed_as": ["IB 125", "LA 2485"],
        "price": 2500,
        "local_prices": [
          {"currency": "USD", "price": 2650},
          {"currency": "PEN", "price": 9800}
        ]
      }
    }
  ],
  "from_cache": false,
  "cached_at": null
}
```

**Response Headers:**

```
X-Trace-Id: 019ef5439-cb43-716d-90b5-51dcbe980908
traceparent: 00-019ef5439cb43716d90b551dcbe980908-a1b2c3d4e5f67890-01
```

### Response Fields Explained (Flight Details)

#### Nivel Raíz

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `itinerary` | object | Itinerario completo con todos los tramos confirmados |
| `itinerary.trip_type` | string | `"round_trip"` o `"one_way"` |
| `itinerary.outbound` | object | Tramo de ida: legs, layovers, duración y emisiones |
| `itinerary.return` | object\|null | Tramo de vuelta. `null` para vuelos solo ida |
| `booking_options` | array | Opciones de reserva disponibles. Cada elemento representa un proveedor (aerolínea u OTA) |

La estructura de cada `leg` y `layover` es idéntica a la documentada en [Response Fields Explained](#response-fields-explained). Consultar las secciones "Leg (segmento de vuelo)" y "Layover (escala)".

### Booking Options

Cada opción de reserva en el array `booking_options[]` representa un proveedor (aerolínea u OTA) que puede vender este itinerario.

#### Estructura de BookingOption

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `trip_type` | string | `"round_trip"` o `"one_way"` |
| `separate_tickets` | boolean | `true` si ida y vuelta se reservan por separado (proveedores distintos) |
| `together` | BookingDetail | Detalle de la reserva conjunta (ida+vuelta con el mismo proveedor). **Siempre presente** |
| `departing` | BookingDetail\|null | Detalle del tramo de ida. Solo presente cuando `separate_tickets: true` |
| `returning` | BookingDetail\|null | Detalle del tramo de vuelta. Solo presente cuando `separate_tickets: true` |

#### Estructura de BookingDetail

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `book_with` | string | Nombre del proveedor. Ej: `"Iberia"`, `"LATAM"`, `"lastminute.com"` |
| `airline` | boolean | `true` si el proveedor es la aerolínea directamente (no una OTA) |
| `airline_logos` | string[] | URLs de los logos del proveedor. Para `separate_tickets`, pueden aparecer dos logos |
| `marketed_as` | string[] | Números de vuelo tal como aparecen en el billete del proveedor |
| `price` | number | Precio en la moneda solicitada (`currency` del request) |
| `local_prices` | LocalPrice[] | Precios en otras monedas locales. Opcional |
| `option_title` | string | Título de la tarifa. Ej: `"Turista Básica"`, `"Económica"` |
| `baggage_prices` | string[] | Descripción del equipaje incluido. Ej: `["1 free carry-on"]` |
| `booking_request` | BookingRequest\|null | Datos para completar la reserva externa. `null` si no hay booking online directo |
| `booking_phone` | string | Teléfono del proveedor para reserva telefónica |
| `estimated_service_fee` | number\|null | Tarifa estimada por servicio telefónico |

#### Estructura de BookingRequest

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `url` | string | URL de reserva externa (Google Flights redirect) |
| `post_data` | string | Datos POST codificados para enviar a la URL de reserva. Enviar como body sin modificar |

#### Nota sobre `separate_tickets`

Cuando `separate_tickets: true`, el proveedor ofrece reservar cada tramo con aerolíneas distintas. El campo `together` contiene el precio combinado, mientras que `departing` y `returning` muestran los precios por tramo. El frontend puede usar esto para ofrecer al usuario la opción de "Reservar juntos" vs "Reservar por separado".

### Uso Proactivo de `often_delayed`

El campo `often_delayed: true` en cualquier leg es la señal que el sistema usa para alertas proactivas. Cuando un usuario tiene una reserva que incluye un vuelo con `often_delayed: true`, el sistema puede:

1. Enviar una notificación push con antelación al vuelo: *"El vuelo IB 125 suele retrasarse. Considerá salir al aeropuerto con más margen."*
2. Revisar si el retraso podría afectar vuelos de conexión o reservas de hotel asociadas.
3. Sugerir opciones alternativas si el impacto sería significativo.

Este dato viene directamente del proveedor y refleja el historial real del vuelo.

### Nota sobre `operated_by` y Codeshare

El campo `operated_by` indica que el vuelo es operado físicamente por una aerolínea distinta a la que vende el billete (codeshare o wet-lease). Ejemplos:

- `"Iberia for Latam Airlines Group"` — Iberia opera el vuelo, pero LATAM lo vende como propio
- `null` — la aerolínea opera su propio vuelo (no hay codeshare)

El frontend debe mostrar esta información al usuario para que sepa qué aerolínea opera realmente el vuelo. Ejemplo de UI: *"Operado por Iberia para LATAM"*.

### Posibles Errores (Flight Details)

| Código | HTTP | Cuándo |
|--------|------|--------|
| `VALIDATION_ERROR` | 400 | `booking_token` vacío o ausente, `departure` o `arrival` no proporcionados, `outbound_date` no proporcionado |
| `BOOKING_TOKEN_EXPIRED` | 404 | El `booking_token` no devuelve resultados — puede haber caducado. Los tokens del proveedor expiran después de un tiempo variable (generalmente horas). Si esto ocurre, el frontend debe volver a llamar `POST /v1/search/flights` con los mismos parámetros para obtener un token nuevo |
| `INVALID_PARAM_RANGE` | 422 | Parámetros de pasajeros incoherentes con el token |
| `PROVIDER_UNAVAILABLE` | 503 | El proveedor externo no está disponible |
| `TOKEN_INVALID` | 401 | Cookie de sesión inválida o expirada (solo si el usuario estaba autenticado) |
| `RATE_LIMIT_EXCEEDED` | 429 | Demasiadas peticiones (RFC 7807 Problem JSON). Ver [Rate Limiting](#rate-limiting) |
| `INTERNAL_ERROR` | 500 | Error inesperado del servidor |

---

## Configuración CORS

| Setting | Valor |
|---------|-------|
| Allowed Origins | `https://proactrip.com`, `http://localhost:3000` |
| Allowed Methods | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS` |
| Allowed Headers | `Content-Type`, `Accept`, `X-Request-Id`, `X-Trace-Id`, `Idempotency-Key` |
| Allow Credentials | `true` |
| Max Age | `86400` |

> **Crítico:** NUNCA usar `Access-Control-Allow-Origin: *` cuando se envían cookies. Debe ser origen explícito.

---

## Rate Limiting

Rate limiting multi-tier con DragonflyDB y scripts Lua atómicos. Distribuido y seguro en entornos multi-instancia. Todos los límites son configurables vía variables de entorno.

### Tiers

| Tier | Scope | Límite | Aplica a |
|------|-------|--------|----------|
| **Tier 1 — Global** | IP | 100 req/min | Todos los endpoints (DDoS shield) |
| **Tier 2 — Authenticated** | UUID del usuario | 10 req/min | Usuarios autenticados que realizan búsquedas |
| **Tier 3 — Anonymous** | Cookie `__Secure-anon_token` | 5 req/min | Usuarios no autenticados (la mayoría de las búsquedas) |

### Provider-Aware Rate Limiting

| Proveedor | Límite | Descripción |
|-----------|--------|-------------|
| SerpAPI | 50/hour | Límite por IP para llamadas al proveedor externo. El backend cachea resultados para reducir consumo |
| Resend (email) | 100/day | Límite del plan gratuito de Resend |

### Cookie Anónima (`__Secure-anon_token`)

Para búsquedas sin autenticación, el backend establece una cookie anónima para rate limiting:

```
Set-Cookie: __Secure-anon_token=019d5439-cb43-716d-90b5-51dcbe980908; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=315360000
```

| Atributo | Valor | Propósito |
|----------|-------|-----------|
| Nombre | `__Secure-anon_token` | Identificador anónimo (UUID v7) |
| TTL | 10 años (Max-Age=315360000) | Persiste entre sesiones — permite rate limiting consistente en usuarios no autenticados |
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
  "instance": "/v1/search/flights",
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

El backend cachea los resultados de búsqueda en DragonflyDB para reducir llamadas al proveedor externo y mejorar tiempos de respuesta.

### Estrategia de Caché

El backend **siempre obtiene datos frescos del proveedor** (SerpAPI). La caché es una capa interna gestionada por el servidor con DragonflyDB, totalmente transparente para el frontend.

| Aspecto | Valor |
|---------|-------|
| TTL de caché | 5 minutos (300 segundos) |
| Backend de caché | DragonflyDB (Redis-compatible) |
| Clave de caché | Hash con Blake3 de los parámetros de búsqueda (ver campos abajo) |
| Invalidación | Por TTL únicamente. No se invalida manualmente |

- Si un usuario busca sin autenticarse, se registra, y vuelve a buscar con los mismos parámetros dentro de la ventana de caché → se reutilizan los resultados cacheados (sin nueva llamada a SerpAPI)
- `from_cache` es **siempre `false`** en todas las respuestas. La caché no se expone al frontend
- `cached_at` es **siempre `null`** en todas las respuestas

> **Motivo:** El manejo interno de caché evita llamadas redundantes al proveedor externo (ahorro de créditos de API) sin exponer detalles de implementación al frontend.

### Campos que Forman la Clave de Caché

La clave de caché se genera haciendo hash de los siguientes campos del request:

| Campo | Incluido en clave |
|-------|-------------------|
| `trip_type` | Sí |
| `departure` | Sí |
| `arrival` | Sí |
| `outbound_date` | Sí |
| `return_date` | Sí (si aplica) |
| `legs` | Sí (para multi_city) |
| `adults` | Sí |
| `children` | Sí |
| `infants_in_seat` | Sí |
| `infants_on_lap` | Sí |
| `travel_class` | Sí |
| `gl` | Sí |
| `hl` | Sí |
| `currency` | Sí |
| `bags` | Sí |
| `max_price` | Sí |
| `sort_by` | Sí |
| `stops` | Sí |
| `include_airlines` | Sí |
| `exclude_airlines` | Sí |
| `outbound_times` | Sí |
| `return_times` | Sí |
| `emissions_filter` | Sí |
| `layover_duration` | Sí |
| `exclude_connections` | Sí |
| `max_duration_minutes` | Sí |
| `outbound_selection_token` | **No** (los tokens son opacos y únicos por vuelo) |
| `cursor` | **No** — la paginación ocurre post-caché en el servidor |
| `limit` | **No** — la paginación ocurre post-caché en el servidor |

> Dos requests con exactamente los mismos parámetros (excepto `outbound_selection_token`) se benefician de la caché interna durante la ventana de TTL, evitando llamadas redundantes a SerpAPI.

---

## Notas de Seguridad

### Headers de Seguridad

Todas las respuestas incluyen:

```
Content-Security-Policy: default-src 'self'
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
Strict-Transport-Security: max-age=31536000
```

### Tokens PASETO v4

Todos los tokens internos son **PASETO v4 symmetric**. Son opacos para el cliente.

| Token | TTL | Propósito |
|-------|-----|-----------|
| `access_token` (cookie `__Secure-access_token`) | 15 min | Autenticar requests |
| `refresh_token` (cookie `__Secure-refresh_token`) | 7 días | Rotación de sesión |

### Rotación de Refresh Tokens

Cada vez que el backend refresca un `__Secure-access_token`, rota también el `__Secure-refresh_token` (token rotation). Si un `__Secure-refresh_token` revocado es reutilizado, **todas las sesiones del usuario se invalidan** automáticamente (detección de robo).

### Comportamiento de Tokens en Búsqueda

- Los endpoints de búsqueda **no requieren autenticación**. Funcionan con o sin cookies.
- Si las cookies están presentes y son válidas, el backend personaliza resultados con las preferencias del usuario.
- Si las cookies expiraron, el backend intenta refrescarlas transparentemente. Si el refresh también falló, el request continúa sin autenticación.
- Los `departure_token` y `booking_token` **no** son tokens de autenticación. Son tokens opacos de SerpAPI que codifican selecciones de vuelo.

### Prevención de Ataques

| Amenaza | Mitigación |
|---------|------------|
| XSS | `HttpOnly cookies` + `Content-Security-Policy` |
| CSRF | `SameSite=Lax` + cookies automáticas (sin `Authorization` manual) |
| Token Exposure | Cookies HttpOnly — JavaScript no puede leerlas |
| Replay de refresh | Rotación continua + invalidación total ante reúso |
| Third-party cookies | `Partitioned` (CHIPS) |
| Rate limiting abuse | Multi-tier con DragonflyDB + Lua scripts atómicos (IP, usuario autenticado, cookie anónima) |
| Cache poisoning | Clave de caché basada en hash de parámetros validados |

### `operated_by` y Codeshare

El campo `operated_by` es importante para el usuario: indica que el vuelo es operado físicamente por una aerolínea distinta a la que vende el billete. Esto es común en alianzas (oneworld, Star Alliance, SkyTeam) y acuerdos de código compartido.

El frontend debe mostrar esta información de forma visible. Ejemplo:

> *"Vuelo IB 125 — Operado por Iberia para LATAM Airlines Group"*

Si `operated_by` es `null`, significa que la aerolínea que vende el billete también opera el vuelo. No se necesita mostrar ningún aviso adicional.

### `often_delayed` — Alerta Proactiva

Cuando un vuelo tiene `often_delayed: true`, el frontend debe mostrar un indicador visual de advertencia (ícono amarillo/naranja) junto con un tooltip: *"Este vuelo suele retrasarse más de 30 minutos"*.

Este dato se usa también para:

- Alertas push antes del vuelo
- Verificación de impacto en conexiones
- Sugerencias de vuelos alternativos
