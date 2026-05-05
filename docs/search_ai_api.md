# AI Search API Documentation (Cookie-Based)

> **Arquitectura:** Unified search endpoint con interpretación de lenguaje natural.
> El usuario envía un mensaje conversacional y el backend interpreta la intención,
> resuelve parámetros, y ejecuta búsquedas de vuelos y/o hoteles automáticamente.

---

## Índice

- [Arquitectura](#arquitectura)
- [Seguridad de Cookies](#seguridad-de-cookies)
- [Base URLs](#base-urls)
- [Errores Estándar](#errores-estándar)
- [Estrategia de Refresco de Tokens](#estrategia-de-refresco-de-tokens)
- [AI Search](#ai-search)
  - [Flujo de Conversación](#flujo-de-conversación)
  - [Modelo Multi-Turno](#modelo-multi-turno)
  - [Request](#request)
  - [Responses](#responses)
    - [Intento Incompleto (incomplete)](#intento-incompleto-incomplete)
    - [Intento Ambiguo (ambiguous)](#intento-ambiguo-ambiguous)
    - [Vuelos (flights)](#vuelos-flights)
    - [Hoteles (hotels)](#hoteles-hotels)
    - [Ambos (both)](#ambos-both)
    - [Partial Failure en Both](#partial-failure-en-both)
  - [Response Fields Explained](#response-fields-explained)
  - [Tipos de Intento](#tipos-de-intento)
  - [Resolución IATA](#resolución-iata)
  - [Posibles Errores](#posibles-errores-ai-search)
- [Rate Limiting](#rate-limiting)
- [Cache](#cache)
- [Notas de Autenticación](#notas-de-autenticación)
- [Configuración CORS](#configuración-cors)

---

## Arquitectura

### Flujo de AI Search

```
┌─────────────┐   POST /v1/search/ai            ┌─────────────┐    ┌─────────────┐
│   Browser   │ ────────────────────────────────>│   Backend   │───>│  AI Provider│
│  (Frontend) │  {message:"Busco vuelos a..."}   │             │    │ (DeepSeek/  │
└─────────────┘                                  └─────────────┘    │  Ollama/    │
^                                                      │            │  OpenAI)    │
│                               Set-Cookie: __Secure-access_token=..│            │
│                              (si el usuario está autenticado)     └─────────────┘
│                              Response: { intent, message,               │
│                                flights?, hotels?, conversation_id }    │
│                                                                        │
│                     ┌──────────────────────────────────────────┐       │
│                     │      Si intent = "flights" o "both"      │       │
│                     │          ┌─────────────┐                 │       │
│                     │          │   SerpAPI   │ <─── llamada    │       │
│                     │          │  (Google    │      interna    │       │
│                     │          │   Flights)  │                 │       │
│                     │          └─────────────┘                 │       │
│                     └──────────────────────────────────────────┘       │
│                     ┌──────────────────────────────────────────┐       │
│                     │      Si intent = "hotels" o "both"       │       │
│                     │          ┌─────────────┐                 │       │
│                     │          │   SerpAPI   │ <─── llamada    │       │
│                     │          │  (Google    │      interna    │       │
│                     │          │   Hotels)   │                 │       │
│                     │          └─────────────┘                 │       │
│                     └──────────────────────────────────────────┘       │
└────────────────────────────────────────────────────────────────────────┘

Las cookies de autenticación se envían AUTOMÁTICAMENTE en cada request.
El frontend NO almacena ni lee tokens. No se requiere header Authorization.
```

### Tres Capas de Procesamiento

```
Mensaje del usuario
    │
    ▼
┌──────────────────────────────────────────────────────────────────┐
│ CAPA 1: Interpretación AI (DeepSeek/Ollama/OpenAI)               │
│   - Analiza lenguaje natural                                    │
│   - Extrae parámetros: origen, destino, fechas, pasajeros...    │
│   - Determina intención: flights | hotels | both                 │
│   - Si faltan datos → incomplete / ambiguous                     │
│   - Genera pregunta de seguimiento                               │
└──────────────────────────────────────────────────────────────────┘
    │
    ▼
┌──────────────────────────────────────────────────────────────────┐
│ CAPA 2: Orquestación (Go — UseCase)                              │
│   - Valida campos extraídos                                      │
│   - Convierte TravelIntent → FlightCommand / HotelCommand        │
│   - Ejecuta búsqueda(s) vía SerpAPI (paralelo si "both")         │
│   - Aplica FilterCriteria determinísticos en Go                  │
└──────────────────────────────────────────────────────────────────┘
    │
    ▼
┌──────────────────────────────────────────────────────────────────┐
│ CAPA 3: Respuesta Unificada                                      │
│   - Estado de conversación (turn_count, max_turns)               │
│   - Intención interpretada + confianza                           │
│   - Mensaje de respuesta (resultados o pregunta de seguimiento)  │
│   - Resultados de vuelos (formato search_flights completo)       │
│   - Resultados de hoteles (formato search_hotels completo)       │
└──────────────────────────────────────────────────────────────────┘
```

### Política de Cookies para AI Search

| Cookie | Nombre | TTL | Propósito |
|--------|--------|-----|-----------|
| Access Token | `__Secure-access_token` | 15 min | Sesión activa (opcional en búsqueda) |
| Refresh Token | `__Secure-refresh_token` | 7 días | Rotación de sesión (opcional en búsqueda) |

> Este endpoint **no requiere autenticación**. Si las cookies están presentes, el backend usa las preferencias del perfil (país, idioma, moneda) para personalizar los resultados. Si no están presentes, se usan los defaults de configuración del servidor.
>
> **Usuarios autenticados** tienen 10 turnos por conversación y las búsquedas se guardan en PostgreSQL para historial.
> **Usuarios anónimos** tienen 5 turnos por conversación y los datos se guardan solo en DragonflyDB (volátil).

---

## Seguridad de Cookies

### Atributos Obligatorios

| Atributo | Valor | Propósito |
|----------|-------|-----------|
| `HttpOnly` | `true` | Inaccesible vía JavaScript (mitiga XSS) |
| `Secure` | `true` | Solo HTTPS en producción |
| `SameSite` | `Lax` | Protección CSRF. Permite navegación top-level |
| `Path` | `/` | Disponible en todas las rutas |
| `Domain` | `.proactrip.com` | Compartido entre subdominios (omitir si usas `__Host-`) |

### Formato de Producción

```
Set-Cookie: __Secure-access_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=900
Set-Cookie: __Secure-refresh_token=v4.local.eyJ...; HttpOnly; Secure; SameSite=Lax; Path=/; Domain=.proactrip.com; Max-Age=604800
```

### Limpieza de Cookies (Logout)

```
Set-Cookie: __Secure-access_token=; Max-Age=0; Path=/; Domain=.proactrip.com; Secure
Set-Cookie: __Secure-refresh_token=; Max-Age=0; Path=/; Domain=.proactrip.com; Secure
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

Formato **RFC 9457 Problem Details**:

```json
{
  "type": "validation_error",
  "title": "Validation Error",
  "status": 400,
  "detail": "El campo 'message' es requerido",
  "instance": "/v1/search/ai",
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

## AI Search

Busca vuelos y/o hoteles mediante interpretación de lenguaje natural. Un solo endpoint que reemplaza preguntas del tipo "¿a dónde querés ir?" con una conversación inteligente que extrae los parámetros automáticamente.

### Flujo de Conversación

```
┌────────────────────────────────────────────────────────────────────┐
│                  CONVERSACIÓN MULTI-TURNO                          │
│                                                                    │
│  TURNO 1: Mensaje incompleto                                       │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Request:  { "message": "Quiero viajar a Madrid" }            │  │
│  │ Response: { intent:"incomplete",                              │  │
│  │            missing_fields:["outbound_date","return_date"],    │  │
│  │            message:"¿Desde qué ciudad salís y en qué fechas?"}│  │
│  │            conversation_id:"019ef..." }                       │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                            ↓                                        │
│  TURNO 2: El usuario completa datos                                 │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Request:  { "message":"Desde Buenos Aires, del 20 al 30",    │  │
│  │            "conversation_id":"019ef..." }                     │  │
│  │ Response: { intent:"flights", confidence:0.95,                │  │
│  │            message:"Encontré 15 vuelos...",                   │  │
│  │            flights:{ phase:"outbound_selection", ... },       │  │
│  │            turn_count:2, max_turns:10 }                       │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                            ↓                                        │
│  El frontend muestra los resultados de vuelos normalmente           │
│  (mismos componentes que POST /v1/search/flights)                   │
└────────────────────────────────────────────────────────────────────┘
```

### Modelo Multi-Turno

El endpoint mantiene una conversación con estado. Cada turno agrega contexto que la AI usa para refinar la interpretación.

| Aspecto | Valor |
|---------|-------|
| **Duración de conversación** | 10 minutos desde la creación |
| **Turnos anónimos** | Máximo 5 |
| **Turnos autenticados** | Máximo 10 |
| **Persistencia** | DragonflyDB para todos; PostgreSQL adicional para autenticados |
| **Nuevo conversation_id** | Se genera al primer mensaje (`POST` sin `conversation_id`) |
| **Reanudar conversación** | Se envía `conversation_id` en requests subsiguientes |

### Request

```
POST /v1/search/ai
```

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Content-Type` | string | Sí | `application/json` |
| `X-Trace-Id` | string | No | UUID v7 opcional para trazabilidad. El middleware asigna uno automáticamente si no se envía |

> Las cookies `__Secure-access_token` y `__Secure-refresh_token` se envían automáticamente si existen. No se requiere header `Authorization`.

**Body:**

```json
{
  "message": "Quiero viajar a Madrid desde Buenos Aires del 15 al 22 de marzo, 2 adultos",
  "conversation_id": "019ef5439-cb43-716d-90b5-51dcbe980908"
}
```

**Campos:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `message` | string | Sí | Mensaje en lenguaje natural describiendo la búsqueda. No puede estar vacío ni ser solo espacios. Ej: `"Busco vuelos baratos a Lima en marzo"`, `"hoteles en Bali con pileta"`, `"vuelo y hotel a Cancún para 2"` |
| `conversation_id` | string | No | UUID v7 de una conversación existente. Omitir en el primer mensaje — el backend genera uno nuevo y lo devuelve en la respuesta. Usar el mismo ID en turnos subsiguientes |
| `stream` | boolean | No | `true` para recibir la respuesta como SSE (Server-Sent Events) con `Content-Type: text/event-stream`. Cada evento contiene los campos de la respuesta de forma incremental. `false` o ausente → respuesta JSON estándar |

### Ejemplos curl

#### Primer mensaje — sin conversation_id (el backend lo genera)

```bash
curl -X POST {base_url}/ai \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Quiero viajar a Barcelona desde Buenos Aires la semana que viene"
  }'
```

#### Turno siguiente — con conversation_id

```bash
curl -X POST {base_url}/ai \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Salgo el 20 de marzo y vuelvo el 30, 2 adultos en económica",
    "conversation_id": "019ef5439-cb43-716d-90b5-51dcbe980908"
  }'
```

#### Usuario autenticado (cookies)

```bash
curl -X POST {base_url}/ai \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ...; __Secure-refresh_token=v4.local.eyJ..." \
  -d '{
    "message": "Vuelos baratos a Madrid desde Ezeiza en abril"
  }'
```

> **Nota:** Las cookies se envían con `-b` solo si el usuario está autenticado. Para búsquedas anónimas, omitir el flag `-b`.

#### Búsqueda combinada (vuelos + hotel)

```bash
curl -X POST {base_url}/ai \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Vuelo y hotel en Cancún del 10 al 17 de junio para 2 personas, all inclusive",
    "conversation_id": "019ef5439-cb43-716d-90b5-51dcbe980908"
  }'
```

#### Búsqueda de hotel con filtros

```bash
curl -X POST {base_url}/ai \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Hotel 5 estrellas en Bali con spa, del 1 al 7 de agosto, menos de 200 EUR por noche"
  }'
```

### Responses

#### Intento Incompleto (incomplete)

Cuando el mensaje no contiene suficiente información para ejecutar una búsqueda. El backend devuelve una pregunta de seguimiento generada por la AI y los campos faltantes.

```json
{
  "conversation_id": "019ef5439-cb43-716d-90b5-51dcbe980908",
  "turn_count": 1,
  "max_turns": 5,
  "intent": "incomplete",
  "confidence": 0.0,
  "message": "¿Desde qué ciudad salís y en qué fechas te gustaría viajar?",
  "missing_fields": [
    "departure",
    "outbound_date",
    "return_date"
  ],
  "from_cache": false
}
```

> **Nota para el frontend:** Cuando `intent` es `"incomplete"`, mostrar el `message` como pregunta de seguimiento al usuario. Los campos `missing_fields` indican qué datos faltan. No hay `flights` ni `hotels` en la respuesta.

#### Intento Ambiguo (ambiguous)

Cuando la AI entiende parcialmente la consulta pero necesita una aclaración. Similar a `incomplete` pero con al menos algunos parámetros interpretados.

```json
{
  "conversation_id": "019ef5439-cb43-716d-90b5-51dcbe980908",
  "turn_count": 2,
  "max_turns": 5,
  "intent": "ambiguous",
  "confidence": 0.45,
  "message": "Entiendo que querés viajar a Madrid. ¿Buscás vuelos, hoteles, o ambos? ¿Desde qué fecha?",
  "missing_fields": [
    "departure",
    "outbound_date"
  ],
  "from_cache": false
}
```

#### Vuelos (flights)

Cuando la AI interpreta que el usuario busca vuelos y tiene todos los parámetros necesarios. La respuesta incluye la estructura completa de `search_flights` igual que `POST /v1/search/flights`.

```json
{
  "conversation_id": "019ef5439-cb43-716d-90b5-51dcbe980908",
  "turn_count": 2,
  "max_turns": 10,
  "intent": "flights",
  "confidence": 0.95,
  "message": "Encontré 15 vuelos de Buenos Aires a Madrid. Acá están los resultados.",
  "flights": {
    "trip_type": "round_trip",
    "phase": "outbound_selection",
    "results_state": "matching",
    "best_flights": [],
    "other_flights": [
      {
        "departure_token": "WyJDalJJTVVKWE5VOTVkeTFQZFVsQlRHRldWbmRDUnkw...",
        "legs": [
          {
            "departure": {
              "airport_code": "EZE",
              "airport_name": "Aeropuerto Internacional Ministro Pistarini",
              "city": "Buenos Aires",
              "country": "Argentina",
              "country_code": "AR",
              "datetime": "2026-03-20 23:55"
            },
            "arrival": {
              "airport_code": "MAD",
              "airport_name": "Aeropuerto Adolfo Suárez Madrid-Barajas",
              "city": "Madrid",
              "country": "España",
              "country_code": "ES",
              "datetime": "2026-03-21 16:10"
            },
            "duration_minutes": 795,
            "aircraft": "Airbus A350",
            "airline": "Iberia",
            "airline_code": "IB",
            "airline_logo_url": "https://www.gstatic.com/flights/airline_logos/70px/IB.png",
            "flight_number": "IB 6844",
            "travel_class": "Economy",
            "legroom": "81 cm",
            "legroom_quality": "above_average",
            "also_sold_by": [],
            "features": {
              "wifi": "paid",
              "power_outlets": true,
              "usb": true,
              "entertainment": "on_demand",
              "raw": [
                "Above average legroom (81 cm)",
                "Wi-Fi for a fee",
                "In-seat power & USB outlets",
                "On-demand video"
              ]
            },
            "overnight": false,
            "often_delayed": false,
            "operated_by": null
          }
        ],
        "layovers": [],
        "total_duration_minutes": 795,
        "price": {
          "amount": 1245000,
          "currency": "ARS"
        },
        "carbon_emissions": {
          "this_flight_grams": 850000,
          "typical_route_grams": 920000,
          "difference_percent": -8
        },
        "type": "Round trip",
        "airline_logo_url": "https://www.gstatic.com/flights/airline_logos/70px/IB.png"
      }
    ],
    "airports": [
      {
        "role": "departure",
        "airport_code": "EZE",
        "airport_name": "Aeropuerto Internacional Ministro Pistarini",
        "city": "Buenos Aires",
        "country": "Argentina",
        "country_code": "AR",
        "image_url": null,
        "thumbnail_url": null
      },
      {
        "role": "arrival",
        "airport_code": "MAD",
        "airport_name": "Aeropuerto Adolfo Suárez Madrid-Barajas",
        "city": "Madrid",
        "country": "España",
        "country_code": "ES",
        "image_url": null,
        "thumbnail_url": null
      }
    ],
    "price_insights": {
      "lowest_price": { "amount": 1245000, "currency": "ARS" },
      "price_level": "typical",
      "typical_range": {
        "min": 900000,
        "max": 1500000,
        "currency": "ARS"
      },
      "price_history": []
    },
    "meta": {
      "next_cursor": "eyJvZmZzZXQiOjEwfQ==",
      "prev_cursor": null,
      "has_next": true,
      "limit": 10
    },
    "from_cache": false,
    "cached_at": null
  },
  "from_cache": false
}
```

**Response Headers:**

```
X-Trace-Id: 019ef5439-cb43-716d-90b5-51dcbe980908
traceparent: 00-019ef5439cb43716d90b551dcbe980908-a1b2c3d4e5f67890-01
```

> Si el usuario está autenticado y la sesión fue refrescada, se incluyen nuevos `Set-Cookie` headers con los tokens rotados.

> **Nota:** La estructura del campo `flights` es **idéntica** a la respuesta de `POST /v1/search/flights`. El frontend reutiliza los mismos componentes de UI para mostrar los resultados. Ver [Search Flights API](search_flights_api.md#responses) para la documentación completa de cada campo.

#### Hoteles (hotels)

Cuando la AI interpreta que el usuario solo busca hoteles. La respuesta incluye la estructura completa de `search_hotels`.

```json
{
  "conversation_id": "019ef5439-cb43-716d-90b5-51dcbe980908",
  "turn_count": 3,
  "max_turns": 10,
  "intent": "hotels",
  "confidence": 0.92,
  "message": "Encontré 8 alojamientos en Bali. Acá están los resultados.",
  "hotels": {
    "type": "hotels",
    "results_state": "matching",
    "properties": [
      {
        "id": "ChcIquf58YLhjLVOGgsvZy8xdzB6cWNmbRAB",
        "type": "hotel",
        "name": "Pullman Bali Legian Beach",
        "description": "Hotel de alta gama con 2 restaurantes, bar, spa y piscina infinita.",
        "booking_url": "https://proactrip.com/book/hotel/ChcIquf58YLhjLVOGgsvZy8xdzB6cWNmbRAB",
        "gps": { "lat": -8.7097252, "lng": 115.1672141 },
        "hotel_class": 5,
        "check_in": "15:00",
        "check_out": "12:00",
        "rating": { "overall": 4.6, "location": 4.4 },
        "total_reviews": 9434,
        "price": {
          "currency": "EUR",
          "per_night": { "amount": 205.0, "before_taxes": 169.0 },
          "total": { "amount": 820.0, "before_taxes": 677.0 }
        },
        "images": [],
        "amenities": ["Free Wi-Fi", "Outdoor pool", "Spa"],
        "nearby_places": [],
        "free_cancellation": false,
        "special_offer": true,
        "eco_certified": true,
        "ratings": [],
        "reviews_breakdown": []
      }
    ],
    "brands": [],
    "pagination": { "next_token": "CBI=", "has_more": true },
    "from_cache": false,
    "cached_at": null
  },
  "from_cache": false
}
```

> **Nota:** La estructura del campo `hotels` es **idéntica** a la respuesta de `POST /v1/search/hotels`. Ver [Search Hotels API](search_hotels_api.md#responses) para la documentación completa.

#### Ambos (both)

Cuando la AI interpreta que el usuario quiere vuelos Y hoteles. Ambos buscadores se ejecutan **en paralelo** con errgroup. La respuesta incluye ambos campos.

```json
{
  "conversation_id": "019ef5439-cb43-716d-90b5-51dcbe980908",
  "turn_count": 1,
  "max_turns": 5,
  "intent": "both",
  "confidence": 0.88,
  "message": "Acá tenés los resultados de vuelos y hoteles que encontré.",
  "flights": {
    "trip_type": "one_way",
    "phase": "complete",
    "results_state": "matching",
    "best_flights": [],
    "other_flights": [ "..." ],
    "airports": [ "..." ],
    "price_insights": null,
    "meta": null,
    "from_cache": false,
    "cached_at": null
  },
  "hotels": {
    "type": "hotels",
    "results_state": "matching",
    "properties": [ "..." ],
    "brands": [],
    "pagination": { "next_token": null, "has_more": false },
    "from_cache": false,
    "cached_at": null
  },
  "from_cache": false
}
```

#### Partial Failure en Both

Cuando el intent es `"both"` y uno de los dos buscadores falla pero el otro tiene éxito, el backend devuelve resultados parciales. El campo `flights_error` o `hotels_error` contiene el mensaje de error del buscador fallido.

```json
{
  "conversation_id": "019ef5439-cb43-716d-90b5-51dcbe980908",
  "turn_count": 1,
  "max_turns": 5,
  "intent": "both",
  "confidence": 0.88,
  "message": "Encontré 5 alojamientos en Cancún. Los vuelos no se pudieron obtener en este momento.",
  "hotels": {
    "type": "hotels",
    "results_state": "matching",
    "properties": [ "..." ],
    "brands": [],
    "pagination": { "next_token": null, "has_more": false },
    "from_cache": false,
    "cached_at": null
  },
  "flights_error": "flight search: provider unavailable",
  "from_cache": false
}
```

> **Nota para el frontend:** Cuando `flights_error` o `hotels_error` están presentes, significa que esa búsqueda falló. Mostrar un mensaje informativo al usuario indicando que esa parte no está disponible. La otra búsqueda (`flights` u `hotels`) contiene resultados válidos.

### Response Fields Explained

#### Nivel Raíz

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `conversation_id` | string | UUID v7 de la conversación. Usar en requests subsiguientes para continuar el multi-turno |
| `turn_count` | integer | Número de turno actual (1-indexado, incrementa en cada request) |
| `max_turns` | integer | Límite máximo de turnos: 5 para anónimos, 10 para autenticados |
| `intent` | string | Tipo de intención interpretada. Ver [Tipos de Intento](#tipos-de-intento) |
| `confidence` | float | Nivel de confianza de la AI en la interpretación (0.0 a 1.0). 0.0 para `incomplete` |
| `message` | string | Texto de respuesta. Resultados en lenguaje natural o pregunta de seguimiento |
| `missing_fields` | string[] | Campos que faltan para completar la búsqueda. Vacío `[]` para intents completos |
| `flights` | object\|null | Resultados de búsqueda de vuelos (formato `search_flights`). `null` si no hay búsqueda de vuelos |
| `hotels` | object\|null | Resultados de búsqueda de hoteles (formato `search_hotels`). `null` si no hay búsqueda de hoteles |
| `flights_error` | string | Mensaje de error del buscador de vuelos. Solo presente en partial failure de `"both"` |
| `hotels_error` | string | Mensaje de error del buscador de hoteles. Solo presente en partial failure de `"both"` |
| `from_cache` | boolean | `true` si la **interpretación** de la AI vino de caché (blake3 hash). `false` si fue fresca. **No** indica si los resultados de búsqueda son cacheados |

### Tipos de Intento

| Tipo | Significado | Resultados incluidos | Acción del frontend |
|------|-------------|---------------------|---------------------|
| `"incomplete"` | Faltan datos esenciales para cualquier búsqueda | `flights: null`, `hotels: null` | Mostrar `message` como pregunta de seguimiento. Usar `missing_fields` para guiar al usuario |
| `"ambiguous"` | Hay parámetros pero la intención no es clara (¿vuelos u hoteles?) | `flights: null`, `hotels: null` | Mostrar `message` como pregunta de aclaración. La AI pide que el usuario especifique |
| `"flights"` | Búsqueda de vuelos completa | `flights: {...}`, `hotels: null` | Renderizar resultados de vuelos con los mismos componentes de `POST /v1/search/flights` |
| `"hotels"` | Búsqueda de hoteles completa | `flights: null`, `hotels: {...}` | Renderizar resultados de hoteles con los mismos componentes de `POST /v1/search/hotels` |
| `"both"` | Búsqueda combinada de vuelos y hoteles | `flights: {...}`, `hotels: {...}` | Renderizar ambos resultados. Si hay `flights_error` o `hotels_error`, mostrar mensaje de partial failure |

### Resolución IATA

Cuando la AI interpreta un mensaje, puede devolver nombres de ciudad en lugar de códigos IATA. El backend resuelve estos nombres a códigos de aeropuerto usando un sistema de 3 niveles:

```
Nivel 1: Coincidencia exacta (embedded JSON ~300 aeropuertos)
  ├── IATA code:           "EZE" → Aeropuerto de Ezeiza
  ├── Ciudad:              "Buenos Aires" → EZE
  └── Alias:               "Madrid-Barajas" → MAD

Nivel 2: Fuzzy matching (sahilm/fuzzy — corrección de typos)
  ├── "bueno aires"        → EZE (score alto)
  ├── "mdrid"              → MAD (score alto)
  └── "barselona"          → BCN (score alto)

Nivel 3: AI fallback (cacheado 24h en DragonflyDB)
  └── Ciudades no cubiertas por el dataset → la AI las resuelve
      y el resultado se cachea por 24 horas
```

> **Dataset:** ~300 aeropuertos principales del mundo embebidos en el binario vía `go:embed`. El fuzzy matching corrige errores de tipeo automáticamente (threshold score ≥ 15 en sahilm/fuzzy).

### Contexto del Intérprete AI

El backend inyecta automáticamente contexto adicional en la conversación para mejorar la precisión del intérprete AI:

#### Inyección de Ubicación

El backend detecta la ubicación del usuario (IP para anónimos, perfil para autenticados) e inyecta un mensaje de sistema con la ciudad/país detectado. Esto evita preguntas innecesarias como "¿Desde dónde?" cuando el usuario no especifica origen.

#### Inyección de Fecha Actual

Para corregir fechas alucinadas por la AI (modelos entrenados con datos de 2024-2025 que devuelven años incorrectos), el backend revisa todas las fechas extraídas por la AI (`outbound_date`, `return_date`, `check_in_date`, `check_out_date`). Si el año es anterior al año actual, se reemplaza automáticamente por el año en curso.

> **Ejemplo:** La AI devuelve `"2025-06-15"` → el backend corrige a `"2026-06-15"`. El día y mes se preservan.

#### Parámetros Soportados

La AI puede extraer los siguientes parámetros del lenguaje natural:

**Vuelos:**
| Parámetro | Descripción | Ejemplo |
|-----------|-------------|---------|
| `departure` | Ciudad/aeropuerto de origen | `"Madrid"`, `"EZE"` |
| `arrival` | Ciudad/aeropuerto de destino | `"Barcelona"`, `"LIM"` |
| `outbound_date` | Fecha de ida (YYYY-MM-DD) | `"2026-06-10"` |
| `return_date` | Fecha de vuelta (YYYY-MM-DD) | `"2026-06-15"` |
| `adults` | Número de adultos (default: 1) | `2` |
| `trip_type` | Tipo de viaje | `"round_trip"`, `"one_way"` |
| `travel_class` | Clase de vuelo | `"economy"`, `"business"` |
| `stops` | Número de escalas | `"nonstop"`, `"max_1"` |
| `max_price` | Precio máximo total | `500.0` |
| `sort_by` | Orden de resultados | `"price"`, `"duration"` |

**Hoteles:**
| Parámetro | Descripción | Ejemplo |
|-----------|-------------|---------|
| `query` | Ciudad/destino del hotel | `"Bali"`, `"París"` |
| `check_in_date` | Fecha de entrada (YYYY-MM-DD) | `"2026-08-01"` |
| `check_out_date` | Fecha de salida (YYYY-MM-DD) | `"2026-08-07"` |
| `adults` | Número de adultos (default: 1) | `2` |
| `children` | Número de niños | `1` |
| `rating` | Puntuación mínima (3.5+) | `8` (4.0+) |
| `max_price` | Precio máximo por noche | `200.0` |
| `free_cancellation` | Solo cancelación gratuita | `true` |

> **Defaults:** Cuando la AI no extrae un parámetro, se usan los defaults del backend: `adults=1`, `trip_type="round_trip"`, `travel_class="economy"`, `stops="any"`, `sort_by="top"`.

### Posibles Errores (AI Search)

| Código | HTTP | Cuándo |
|--------|------|--------|
| `VALIDATION_ERROR` | 400 | `message` vacío o solo espacios en blanco |
| `TURN_LIMIT_EXCEEDED` | 400 | Se alcanzó el límite máximo de turnos en la conversación (5 anónimos, 10 autenticados) |
| `CONVERSATION_NOT_FOUND` | 400 | El `conversation_id` enviado no existe o expiró |
| `RATE_LIMIT_EXCEEDED` | 429 | Demasiadas peticiones al proveedor de IA. Ver [Rate Limiting](#rate-limiting) |
| `AI_PARSE_FAILURE` | 502 | La IA devolvió una respuesta inválida o malformada que no se pudo interpretar |
| `AI_UNAVAILABLE` | 503 | El servicio de IA no está configurado o no responde. Ver [Configuración](#variables-de-entorno-requeridas) |
| `PROVIDER_UNAVAILABLE` | 503 | El proveedor externo (SerpAPI) no está disponible (solo en intent `flights` o `hotels`) |
| `INTERNAL_ERROR` | 500 | Error inesperado del servidor |

> **Diferencia clave entre 502 y 503:** `502 AI_PARSE_FAILURE` significa que la IA respondió pero con un formato inválido (ej: JSON malformado). `503 AI_UNAVAILABLE` significa que la IA no respondió (timeout, conexión rechazada) o no está configurada en el entorno.

---

## Rate Limiting

Rate limiting multi-tier con DragonflyDB y scripts Lua atómicos. Distribuido y seguro en entornos multi-instancia. Todos los límites son configurables vía variables de entorno.

### Tiers Generales

| Tier | Scope | Límite | Aplica a |
|------|-------|--------|----------|
| **Tier 1 — Global** | IP | 100 req/min | Todos los endpoints (DDoS shield) |
| **Tier 2 — Authenticated** | UUID del usuario | 10 req/min | Usuarios autenticados que realizan búsquedas |
| **Tier 3 — Anonymous** | Cookie `__Secure-anon_token` | 5 req/min | Usuarios no autenticados (la mayoría de las búsquedas) |

### Provider-Aware Rate Limiting — AI

| Proveedor | Límite | Descripción |
|-----------|--------|-------------|
| **AI** | 10 req/hora | Límite por IP para llamadas al proveedor de IA. Configurable vía `RATELIMIT_PROVIDER_AI_MAX` y `RATELIMIT_PROVIDER_AI_WINDOW_SEC`. Se aplica un bucket de tokens dedicado antes de cada llamada al intérprete |

> El backend cachea los resultados de interpretación (blake3 hash, 10 min TTL) para reducir el consumo de créditos del proveedor de IA. Un mismo mensaje con el mismo historial no consume llamadas adicionales durante la ventana de caché.

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

Formato **RFC 9457 Problem Details**:

```json
{
  "type": "rate_limit_exceeded",
  "title": "Too Many Requests",
  "status": 429,
  "detail": "Demasiadas peticiones. Esperá 60 segundos antes de reintentar.",
  "instance": "/v1/search/ai",
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

El backend usa una estrategia de caché en dos niveles para el endpoint de AI search:

### Nivel 1: Caché de Interpretación (blake3)

Las respuestas del intérprete de IA se cachean para reducir llamadas al proveedor externo.

| Aspecto | Valor |
|---------|-------|
| TTL de caché | 10 minutos |
| Backend de caché | DragonflyDB (Redis-compatible) |
| Clave de caché | Hash blake3 de `(message + conversation_history)` |
| Qué se cachea | Solo intents completos: `flights`, `hotels`, `both` |
| Qué NO se cachea | Intents `incomplete` y `ambiguous` (necesitan preguntas frescas por contexto de conversación) |
| Invalidación | Por TTL únicamente. No se invalida manualmente |

```
┌─────────────────────────────────────────────────────────┐
│  Mismo mensaje + mismo historial en ventana de 10 min:  │
│  └── Cache HIT → se reutiliza la interpretación        │
│      (sin llamada al proveedor de IA)                   │
│                                                         │
│  Intents incomplete/ambiguous:                          │
│  └── Cache SKIP → siempre se llama a la AI             │
│      (cada conversación necesita preguntas frescas)     │
└─────────────────────────────────────────────────────────┘
```

> El campo `from_cache` en la respuesta indica si la **interpretación** vino de caché. No refleja si los resultados de búsqueda (vuelos/hoteles) estaban cacheados — esos tienen su propia estrategia de caché independiente (ver [Search Flights Cache](search_flights_api.md#cache) y [Search Hotels Cache](search_hotels_api.md#cache)).

### Nivel 2: Caché de Resultados de Búsqueda

Cuando el intent es `flights`, `hotels`, o `both`, los buscadores de vuelos y hoteles aplican su propia estrategia de caché existente (blake3 sobre parámetros, 5 min TTL). Esta caché es idéntica a la de los endpoints directos `POST /v1/search/flights` y `POST /v1/search/hotels`.

---

## Notas de Autenticación

### Usuarios Anónimos vs Autenticados

| Aspecto | Anónimo | Autenticado |
|---------|---------|-------------|
| **Turnos máximos** | 5 | 10 |
| **Persistencia de conversación** | Solo DragonflyDB (TTL 10 min) | DragonflyDB + PostgreSQL (historial) |
| **Defaults de búsqueda** | Configuración del servidor (`DEFAULT_CURRENCY`, `DEFAULT_LANGUAGE`, `DEFAULT_COUNTRY_CODE`) | Perfil del usuario (`gl`, `hl`, `currency`) |
| **Rate limiting de búsqueda** | 5 req/min por cookie anónima | 10 req/min por UUID |
| **Resolución de ubicación** | IP del request → `GET /v1/environment` | Perfil del usuario (con fallback a IP) |

### Resolución de Defaults (GL/HL/Currency)

El backend usa una cadena de prioridad de 4 niveles para resolver los parámetros `gl`, `hl` y `currency` que se pasan a los buscadores de vuelos/hoteles:

```
Tier 1: Parámetros explícitos del cliente  → AI Search no expone estos (no aplica)
Tier 2: Preferencias del perfil (PG)       → Solo si el usuario está autenticado
Tier 3: Caché de entorno (DragonflyDB)     → Detectado por IP en /v1/environment
Tier 4: Configuración por defecto (.env)   → DEFAULT_CURRENCY, DEFAULT_LANGUAGE, DEFAULT_COUNTRY_CODE
```

### Variables de Entorno Requeridas

Para que el endpoint `POST /v1/search/ai` funcione, el servicio de IA debe estar configurado:

| Variable | Descripción | Ejemplo |
|----------|-------------|---------|
| `AI_PROVIDER` | Proveedor de IA: `deepseek`, `ollama`, `openai` | `deepseek` |
| `AI_BASE_URL` | URL base del API de IA (omitir para usar default del proveedor) | `https://api.deepseek.com/v1` |
| `AI_API_KEY` | API key del proveedor (omitir para ollama local) | `sk-xxxxxxxx` |
| `AI_MODEL` | Nombre del modelo a usar | `deepseek-chat` |
| `AI_TIMEOUT` | Timeout para requests de IA (formato Go duration) | `30s` |
| `RATELIMIT_PROVIDER_AI_MAX` | Máximo de requests al proveedor de IA por ventana | `10` |
| `RATELIMIT_PROVIDER_AI_WINDOW_SEC` | Ventana de rate limiting en segundos | `3600` |

> Si `AI_PROVIDER` no está configurado o el usecase es `nil`, el endpoint devuelve **503 AI_UNAVAILABLE** con el mensaje: *"AI search no disponible — el servicio de IA no está configurado en este entorno"*.

### Comportamiento sin AI Configurada

```
POST /v1/search/ai
  → 503 Service Unavailable

{
  "type": "service_unavailable",
  "title": "Service Unavailable",
  "status": 503,
  "detail": "AI search no disponible — el servicio de IA no está configurado en este entorno",
  "instance": "/v1/search/ai",
  "trace_id": "019d5439-cb43-716d-90b5-51dcbe980908"
}
```

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

### Comportamiento de Tokens en AI Search

- El endpoint **no requiere autenticación**. Funciona con o sin cookies.
- Si las cookies están presentes y son válidas, el backend personaliza resultados con las preferencias del usuario y aumenta el límite de turnos a 10.
- Si las cookies expiraron, el backend intenta refrescarlas transparentemente. Si el refresh también falló, el request continúa sin autenticación (5 turnos).
- El `conversation_id` **no** es un token de autenticación. Es un UUID v7 que identifica la conversación multi-turno.

### Prevención de Ataques

| Amenaza | Mitigación |
|---------|------------|
| XSS | `HttpOnly cookies` + `Content-Security-Policy` |
| CSRF | `SameSite=Lax` + cookies automáticas (sin `Authorization` manual) |
| Token Exposure | Cookies HttpOnly — JavaScript no puede leerlas |
| Replay de refresh | Rotación continua + invalidación total ante reúso |
| Third-party cookies | No se usa Partitioned (CHIPS) — SameSite=Lax + Domain=.proactrip.com es suficiente para subdominios |
| Rate limiting abuse | Multi-tier con DragonflyDB + Lua scripts atómicos (IP, usuario autenticado, cookie anónima) + provider bucket para AI |
| Cache poisoning | Clave de caché basada en blake3 hash de parámetros validados |
| Prompt injection | La AI opera en modo restringido con system prompt fijo. Solo extrae parámetros de viaje, nunca ejecuta código ni modifica el sistema |

---

## Flujo Complejo de Ejemplo

### Escenario: Usuario busca "vuelo y hotel en Cancún"

```
┌──────────────────────────────────────────────────────────────────┐
│ TURNO 1 (anónimo)                                                │
│                                                                  │
│ POST /v1/search/ai                                               │
│ { "message": "Quiero vacaciones en Cancún" }                     │
│                                                                  │
│ → Response 200 OK                                                │
│ { intent: "incomplete",                                          │
│   missing_fields: ["departure","outbound_date","return_date"],   │
│   message: "¿Desde dónde salís y en qué fechas?",                │
│   conversation_id: "019ef...aaa" }                               │
├──────────────────────────────────────────────────────────────────┤
│ TURNO 2                                                          │
│                                                                  │
│ POST /v1/search/ai                                               │
│ { "message": "Desde Buenos Aires en junio, una semana",          │
│   "conversation_id": "019ef...aaa" }                             │
│                                                                  │
│ → Response 200 OK                                                │
│ { intent: "ambiguous",                                           │
│   missing_fields: ["outbound_date","return_date"],               │
│   message: "¿Buscás vuelos, hoteles, o ambos? ¿Fechas exactas?", │
│   turn_count: 2 }                                                │
├──────────────────────────────────────────────────────────────────┤
│ TURNO 3                                                          │
│                                                                  │
│ POST /v1/search/ai                                               │
│ { "message": "Vuelo y hotel, del 10 al 17 de junio",             │
│   "conversation_id": "019ef...aaa" }                             │
│                                                                  │
│ → Response 200 OK                                                │
│ { intent: "both",                                                │
│   confidence: 0.91,                                              │
│   message: "Acá tenés los resultados de vuelos y hoteles...",    │
│   flights: { phase:"outbound_selection", other_flights:[...] },  │
│   hotels: { type:"hotels", properties:[...] },                   │
│   turn_count: 3 }                                                │
│                                                                  │
│ → El frontend renderiza ambas secciones                          │
└──────────────────────────────────────────────────────────────────┘
```
