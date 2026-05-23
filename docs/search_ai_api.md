# Documentación de AI Search API (Tool Calling)

> **Arquitectura:** Endpoint unificado de búsqueda con interpretación de lenguaje natural y Tool Calling.
> El usuario envía un mensaje conversacional. El backend interpreta la intención vía AI,
> resuelve parámetros automáticamente (ubicación por IP, moneda/idioma del perfil),
> y ejecuta búsquedas de vuelos y/o hoteles.

---

## Índice

| Sección | Estado |
|---------|--------|
| [Arquitectura](#arquitectura) | ✅ |
| [Tool Calling](#tool-calling) | ✅ |
| [Seguridad de Cookies](#seguridad-de-cookies) | ✅ |
| [Base URLs](#base-urls) | ✅ |
| [Errores Estándar](#errores-estándar) | ✅ |
| [Estrategia de Refresco de Tokens](#estrategia-de-refresco-de-tokens) | ✅ |
| [AI Search — POST /ai](#ai-search) | ✅ |
| [Flujo de Conversación](#flujo-de-conversación) | ✅ |
| [Modelo Multi-Turno](#modelo-multi-turno) | ✅ |
| [Request](#request) | ✅ |
| [SSE Streaming](#sse-streaming) | ✅ |
| [Responses](#responses) | ✅ |
| [Response Fields](#response-fields-explained) | ✅ |
| [Tipos de Intento](#tipos-de-intento) | ✅ |
| [Discovery Mode](#discovery-mode) | ✅ |
| [Conversation CRUD](#conversation-crud) | ✅ |
| [Realtime Events](#realtime-events) | ✅ |
| [Resolución IATA](#resolución-iata) | ✅ |
| [Contexto Médico y de Viaje](#contexto-médico-y-de-viaje) | ✅ |
| [Posibles Errores](#posibles-errores-ai-search) | ✅ |
| [Rate Limiting](#rate-limiting) | ✅ |
| [Cache](#cache) | ✅ |
| [Notas de Autenticación](#notas-de-autenticación) | ✅ |
| [Configuración CORS](#configuración-cors) | ✅ |
| [Notas de Seguridad](#notas-de-seguridad) | ✅ |

---

## Arquitectura

### Flujo de AI Search (Tool Calling)

```
┌─────────────┐   POST /v1/search/ai            ┌─────────────┐    ┌─────────────┐
│   Browser   │ ────────────────────────────────>│   Backend   │───>│  AI Provider│
│  (Frontend) │  {"message":"Busco vuelos a..."} │             │    │ (DeepSeek/  │
└─────────────┘                                  └─────────────┘    │  Ollama/    │
       ^                                               │            │  OpenAI)    │
       │                                               │            └─────────────┘
       │                        ┌──────────────────────┘                  │
       │                        │   1. AI decide si necesita datos        │
       │                        │      → tool call: search_flights        │
       │                        │      → tool call: search_hotels         │
       │                        │                                         │
       │                        │   2. Backend ejecuta tools en paralelo   │
       │                        │      (prefill GL/HL/Currency)           │
       │                        │                                         │
       │                        │   3. AI genera respuesta final           │
       │                        │      con resultados en lenguaje natural  │
       │                        │                                         │
       │                        │   4. Backend guarda conversación         │
       │                        │      en DragonflyDB (TTL 5 min)         │
       │                        │                                         │
       │                        ▼                                         │
       │   Response JSON: { intent, message, flights?, hotels?,           │
       │                    conversation_id, from_cache, cached_at }      │
       │                                                                  │
       │   Set-Cookie: __Secure-access_token=..                           │
       │   (si el usuario está autenticado)                               │
       └──────────────────────────────────────────────────────────────────┘
```

> **El AI decide el modo de búsqueda automáticamente.** Ya no existe el campo `search_mode` en el request. El AI interpreta la consulta y decide si necesita ejecutar `search_flights`, `search_hotels`, ambos, o ninguno (pregunta de seguimiento).

### Tool Calling

El endpoint `POST /v1/search/ai` usa **Tool Calling** — el modelo de AI decide cuándo y con qué parámetros ejecutar búsquedas. El backend define dos tools:

| Tool | Propósito | Campos requeridos |
|------|-----------|-------------------|
| `search_flights` | Busca vuelos entre dos aeropuertos | `trip_type`, `departure`, `arrival`, `outbound_date` |
| `search_hotels` | Busca hoteles en un destino | `query`, `check_in_date`, `check_out_date` |
| `get_destination_weather` | Obtiene el clima en un destino para una fecha específica | `lat`, `lng`, `date` |

**Flujo:**
1. El frontend envía `{"message": "..."}` (y opcionalmente `conversation_id`, `stream`)
2. El backend resuelve ubicación vía IP (`env:{ip}` en DragonflyDB), preferencias de moneda/idioma del perfil del usuario
3. El backend inyecta contexto de ubicación como system message
4. La AI recibe el mensaje + herramientas disponibles y decide:
   - **No necesita tools** → devuelve respuesta directa o pregunta de seguimiento
   - **Necesita una o ambas tools** → el backend ejecuta las búsquedas, inyecta resultados, la AI genera respuesta final
5. El backend guarda el estado de la conversación en DragonflyDB (TTL 5 min)
6. El backend devuelve la respuesta unificada

**Prefilling automático de GL/HL/Currency:** Cuando la AI omite `gl`, `hl` o `currency` en una tool call, el backend los rellena automáticamente desde el contexto de la conversación:
- `gl` → código de país del usuario (lowercase, 2 letras)
- `hl` → idioma detectado
- `currency` → moneda del perfil o default del servidor

**min_price=1 default para hoteles:** En `search_hotels`, el backend aplica `min_price=1` por defecto para filtrar hoteles sin precios (placeholders de Google Maps que no se pueden reservar).

---

## Seguridad de Cookies

### Atributos Obligatorios

| Atributo | Valor | Propósito |
|----------|-------|-----------|
| `HttpOnly` | `true` | Inaccesible vía JavaScript (mitiga XSS) |
| `Secure` | `true` | Solo HTTPS en producción |
| `SameSite` | `Lax` | Protección CSRF. Permite navegación top-level |
| `Path` | `/` | Disponible en todas las rutas |
| `Domain` | `.proactrip.com` | Compartido entre subdominios |

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
  "type": "https://api.proactrip.com/errors/validation-error",
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
| `X-Trace-Id` | UUID v7 para trazabilidad. Asignado globalmente por middleware |
| `traceparent` | W3C Trace Context |
| `X-Request-Id` | ID de request no-W3C para correlación de logs |

---

## Estrategia de Refresco de Tokens

El backend maneja el refresco de tokens transparentemente vía middleware.

- Si `access_token` es válido → la petición continúa
- Si `access_token` está expirado pero `refresh_token` es válido → nuevos tokens emitidos automáticamente
- Si ambos están expirados → el request continúa sin autenticación (búsqueda pública)

El frontend nunca llama manualmente a `/refresh-token`. Las cookies se gestionan solas.

---

## AI Search

Busca vuelos y/o hoteles mediante interpretación de lenguaje natural con Tool Calling. Un solo endpoint que reemplaza formularios de búsqueda tradicionales con una conversación inteligente.

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
│  │            flights:{ ... },                                   │  │
│  │            turn_count:2, max_turns:10 }                       │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                            ↓                                        │
│  El frontend muestra los resultados de vuelos normalmente           │
│  (mismos componentes que POST /v1/search/flights)                   │
└────────────────────────────────────────────────────────────────────┘
```

### Modelo Multi-Turno

El endpoint mantiene conversaciones con estado persistido en DragonflyDB. Cada turno agrega contexto que la AI usa para refinar la interpretación.

| Aspecto | Valor |
|---------|-------|
| **Duración de conversación (TTL)** | 5 minutos desde la última actividad POST. Se reinicia en cada POST |
| **Turnos anónimos** | Máximo 5 |
| **Turnos autenticados** | Máximo 10 |
| **Persistencia** | DragonflyDB para todos (clave `{conv}:{id}`, índice `user:convs:{userID}`). PostgreSQL adicional para autenticados |
| **Nuevo conversation_id** | Se genera al primer mensaje (`POST` sin `conversation_id`) |
| **Reanudar conversación** | Se envía `conversation_id` en requests subsiguientes |
| **Recuperación F5** | `GET /v1/search/ai/conversations/{id}` reconstruye el estado completo de la conversación. El frontend puede restaurar el chat tras un refresh de página |

> **Importante:** Si la conversación expira (5 min sin actividad), el `conversation_id` deja de ser válido. El frontend debe manejar el error 400 `CONVERSATION_NOT_FOUND` y crear una nueva conversación. El evento SSE `search.conversation.expired` notifica la expiración en tiempo real.

---

### Request

```
POST /v1/search/ai
```

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Content-Type` | string | Sí | `application/json` |
| `X-Trace-Id` | string | No | UUID v7 opcional. El middleware asigna uno automáticamente si no se envía |

> Las cookies `__Secure-access_token` y `__Secure-refresh_token` se envían automáticamente si existen. No se requiere header `Authorization`.

**Body:**

```json
{
  "message": "Quiero viajar a Madrid desde Buenos Aires del 15 al 22 de marzo, 2 adultos",
  "conversation_id": "019ef5439-cb43-716d-90b5-51dcbe980908",
  "stream": false
}
```

**Campos:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `message` | string | **Sí** | Mensaje en lenguaje natural. No puede estar vacío ni ser solo espacios. Ej: `"Busco vuelos baratos a Lima en marzo"`, `"hoteles en Bali con pileta"`, `"vuelo y hotel a Cancún para 2"` |
| `conversation_id` | string | No | UUID v7 de una conversación existente. Omitir en el primer mensaje — el backend genera uno nuevo. Usar el mismo ID en turnos subsiguientes |
| `stream` | boolean | No | `true` para recibir la respuesta como SSE (`Content-Type: text/event-stream`). `false` o ausente → respuesta JSON estándar. Default: `false` |

> **Ya no se envían:** `search_mode` (la AI decide el modo vía Tool Calling), `lat`, `lng`, `timezone`, `country_code` (el backend los resuelve automáticamente por IP vía `env:{ip}` en DragonflyDB).

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

### SSE Streaming

Cuando `stream: true`, el backend responde con `Content-Type: text/event-stream`. El flujo de eventos es:

```
event: status
data: {"status":"thinking"}

event: chunk
data: {"content":"Voy a buscar vuelos..."}

event: weather
data: {"destination":"Bali, Indonesia","weather":{"temp":28.5,"feels_like":31.2,"description":"cielo claro","icon":"01d","icon_url":"https://openweathermap.org/img/wn/01d@4x.png","humidity":65,"wind_speed":4.2}}

event: search
data: {"destination":"MAD→BCN","type":"flights","data":{...}}

event: chunk
data: {"content":"Encontré 5 vuelos. También busco hoteles..."}

event: search
data: {"destination":"Barcelona, España","type":"hotels","data":{...}}

event: chunk
data: {"content":"Acá están todos los resultados."}

event: result
data: {"conversation_id":"...","intent":"both","message":"...","flights":{...},"hotels":{...}}

event: error
data: {"error":"mensaje de error"}
```

**Tipos de eventos SSE:**

| Evento | Formato | Cuándo |
|--------|--------|--------|
| `status` | `{"status":"thinking"}` | Inmediatamente al recibir el request. Indica que el procesamiento comenzó |
| `chunk` | `{"content":"..."}` | Fragmento de texto generado por la AI. Se envía en tiempo real |
| `search` | `{"destination":"...","type":"hotels\|flights","data":{...}}` | Resultados de una búsqueda ejecutada (tool call completado) |
| `weather` | `{"destination":"Bali, Indonesia","weather":{"temp":28.5,"feels_like":31.2,"description":"cielo claro","icon":"01d","icon_url":"https://openweathermap.org/img/wn/01d@4x.png","humidity":65,"wind_speed":4.2}}` | Datos climáticos del destino para la fecha solicitada. La AI decide si llamar a esta tool según la consulta del usuario. Cacheado 10 min en DragonflyDB |
| `result` | `{...respuesta JSON completa...}` | Respuesta final unificada. Contiene todos los campos de la respuesta JSON estándar |
| `error` | `{"error":"mensaje"}` | Error durante el procesamiento (AI no disponible, rate limit, etc.) |
| `alert` | `{"alerts":[{"level":"warning"\|"info","type":"allergy"\|"medication_restricted"\|"vaccination"\|"condition"\|"travel"\|"document","message":"..."}]}` | Alertas médicas o de viaje detectadas por la AI. Solo para usuarios autenticados |

**Formato wire:**
```
event: {tipo}\ndata: {json}\n\n
```

> **Importante para el frontend:** En modo streaming, usar `EventSource` o `fetch` con `ReadableStream`. El header `Content-Type` de la respuesta será `text/event-stream`. En modo no-streaming, la respuesta es JSON estándar con `Content-Type: application/json`.

---

### Responses

#### Intento Incompleto (incomplete)

Cuando el mensaje no contiene suficiente información para ejecutar una búsqueda. El backend devuelve una pregunta de seguimiento generada por la AI.

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

> **Nota para el frontend:** Cuando `intent` es `"incomplete"`, mostrar el `message` como pregunta de seguimiento. Los campos `missing_fields` indican qué datos faltan. No hay `flights` ni `hotels` en la respuesta.

#### Intento Ambiguo (ambiguous)

Cuando la AI entiende parcialmente la consulta pero necesita una aclaración.

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

Cuando la AI interpreta que el usuario busca vuelos y tiene todos los parámetros necesarios. La respuesta incluye la estructura completa de `search_flights`.

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
        "country_code": "AR"
      },
      {
        "role": "arrival",
        "airport_code": "MAD",
        "airport_name": "Aeropuerto Adolfo Suárez Madrid-Barajas",
        "city": "Madrid",
        "country": "España",
        "country_code": "ES"
      }
    ],
    "price_insights": {
      "lowest_price": { "amount": 1245000, "currency": "ARS" },
      "price_level": "typical",
      "typical_range": { "min": 900000, "max": 1500000, "currency": "ARS" },
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

> **Nota:** La estructura del campo `flights` es **idéntica** a la respuesta de `POST /v1/search/flights`. Ver [Search Flights API](search_flights_api.md#responses) para la documentación completa.

#### Hoteles (hotels)

Cuando la AI interpreta que el usuario solo busca hoteles.

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

Cuando la AI interpreta que el usuario quiere vuelos Y hoteles. Ambos buscadores se ejecutan **en paralelo** con `errgroup`.

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
    "other_flights": [],
    "airports": [],
    "from_cache": false
  },
  "hotels": {
    "type": "hotels",
    "results_state": "matching",
    "properties": [],
    "brands": [],
    "pagination": { "next_token": null, "has_more": false },
    "from_cache": false
  },
  "from_cache": false
}
```

#### Partial Failure en Both

Cuando un buscador falla y el otro tiene éxito, el backend devuelve resultados parciales con `flights_error` o `hotels_error`.

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
    "properties": [],
    "brands": [],
    "pagination": { "next_token": null, "has_more": false },
    "from_cache": false
  },
  "flights_error": "flight search: provider unavailable",
  "from_cache": false
}
```

> **Nota para el frontend:** Cuando `flights_error` o `hotels_error` están presentes, significa que esa búsqueda falló. Mostrar un mensaje informativo. La otra búsqueda contiene resultados válidos. Si AMBOS fallan, la respuesta es 502 Bad Gateway.

---

### Campos de la Respuesta

#### Nivel Raíz

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `conversation_id` | string | UUID v7 de la conversación. Usar en requests subsiguientes |
| `turn_count` | integer | Número de turno actual (1-indexado, incrementa en cada request) |
| `max_turns` | integer | Límite máximo de turnos: 5 para anónimos, 10 para autenticados |
| `intent` | string | Tipo de intención interpretada. Ver [Tipos de Intento](#tipos-de-intento) |
| `confidence` | float | Nivel de confianza de la AI en la interpretación (0.0 a 1.0). 0.0 para `incomplete` |
| `message` | string | Texto de respuesta en lenguaje natural o pregunta de seguimiento |
| `missing_fields` | string[] | Campos que faltan. Vacío para intents completos. Omitido si vacío (`omitzero`) |
| `flights` | object\|null | Resultados de vuelos (formato `search_flights`). Omitido si no hay búsqueda de vuelos (`omitzero`) |
| `hotels` | object\|null | Resultados de hoteles (formato `search_hotels`). Omitido si no hay búsqueda de hoteles (`omitzero`) |
| `flights_error` | string | Mensaje de error del buscador de vuelos. Solo en partial failure de `"both"`. Omitido si vacío (`omitzero`) |
| `hotels_error` | string | Mensaje de error del buscador de hoteles. Solo en partial failure de `"both"`. Omitido si vacío (`omitzero`) |
| `from_cache` | boolean | `true` si la **interpretación** de la AI vino de caché (blake3 hash). `false` si fue fresca. **No** indica si los resultados de búsqueda son cacheados |
| `cached_at` | string\|null | Timestamp ISO 8601 del momento en que se cacheó la interpretación. Omitido si `from_cache` es `false` (`omitzero`) |
| `mode` | string | `"discovery"` en respuestas del pipeline de discovery. Omitido en búsqueda exacta (`omitzero`) |
| `candidates` | object[] | Destinos rankeados del pipeline de discovery (top 3-5). Omitido si no aplica (`omitzero`) |
| `total_candidates` | integer | Total de candidatos antes del truncado. Omitido si es 0 (`omitzero`) |
| `needs_clarification` | boolean | `true` si se necesita más información del usuario. Omitido si `false` (`omitzero`) |
| `clarification_question` | string | Pregunta de aclaración generada. Omitido si vacío (`omitzero`) |

### Tipos de Intento

| Tipo | Significado | Resultados incluidos | Acción del frontend |
|------|-------------|---------------------|---------------------|
| `"incomplete"` | Faltan datos esenciales | `flights: null`, `hotels: null` | Mostrar `message` como pregunta. Usar `missing_fields` para guiar al usuario |
| `"ambiguous"` | Hay parámetros pero la intención no es clara | `flights: null`, `hotels: null` | Mostrar `message` como pregunta de aclaración |
| `"flights"` | Búsqueda de vuelos completa | `flights: {...}`, `hotels: null` | Renderizar resultados con los mismos componentes de `POST /v1/search/flights` |
| `"hotels"` | Búsqueda de hoteles completa | `flights: null`, `hotels: {...}` | Renderizar resultados con los mismos componentes de `POST /v1/search/hotels` |
| `"both"` | Búsqueda combinada | `flights: {...}`, `hotels: {...}` | Renderizar ambos. Si hay `flights_error` o `hotels_error`, mostrar partial failure |
| `"discovery"` | Recomendación de destinos | `message: "..."`, `candidates: [...]` | Renderizar recomendaciones en lenguaje natural y/o candidatos estructurados |

### Discovery Mode

> **AI-Powered (DeepSeek v4 Flash).** El modo discovery usa DeepSeek v4 Flash para interpretar consultas abiertas del tipo "recomiéndame playa", "destinos baratos para verano", o "a dónde viajar en diciembre". El intérprete AI recibe contexto de ubicación (resuelto por IP), preferencias (`currency`, `language`) y la fecha actual, y genera recomendaciones de destinos en lenguaje natural.

La ruta al pipeline de discovery es interna — el AI decide si una consulta es discovery basándose en el contenido del mensaje. El frontend no necesita enviar `search_mode`.

#### Response (JSON, non-streaming)

```json
{
  "conversation_id": "019ef5439-cb43-716d-90b5-51dcbe980908",
  "turn_count": 1,
  "max_turns": 5,
  "mode": "discovery",
  "intent": "discovery",
  "confidence": 1.0,
  "message": "¡Claro! Para un verano de playa te recomiendo...\n\n**1. Punta Cana, República Dominicana**\n- Playas caribeñas de arena blanca\n- Todo incluido disponible\n\n**2. Cancún, México**\n- Aguas turquesas y vida nocturna\n\n**3. Bali, Indonesia**\n- Paraíso tropical\n- Mejor época: abril a octubre",
  "from_cache": false
}
```

#### Discovery Response Fields

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `mode` | string | `"discovery"` cuando el pipeline de discovery está activo. Omitido en búsqueda exacta |
| `intent` | string | `"discovery"` — indica que la respuesta es del pipeline de descubrimiento |
| `confidence` | float64 | Confianza (siempre 1.0 en discovery AI-powered) |
| `message` | string | Respuesta en lenguaje natural generada por DeepSeek v4 Flash con recomendaciones. Formateada en Markdown |
| `candidates` | object[] | Destinos rankeados (top 3-5). Omitido si no aplica |
| `from_cache` | boolean | `true` si la interpretación vino de caché, `false` si fue fresh |

---

### Conversation CRUD

Endpoints para gestionar el estado de las conversaciones. Útiles para recuperación F5, lista de conversaciones activas, y eliminación.

#### GET /conversations — Listar conversaciones activas

```
GET /v1/search/ai/conversations
```

Devuelve las conversaciones activas del usuario (autenticado o anónimo vía `__Secure-anon_token`).

**Response 200:**
```json
[
  {
    "id": "019ef5439-cb43-716d-90b5-51dcbe980908",
    "preview": "Quiero viajar a Barcelona desde Buenos Aires",
    "turn_count": 3,
    "updated_at": "2026-05-23T15:30:00Z"
  },
  {
    "id": "019ef6789-ab12-345c-90d5-12ab34cd5678",
    "preview": "Hoteles en Bali con pileta",
    "turn_count": 1,
    "updated_at": "2026-05-23T14:00:00Z"
  }
]
```

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `id` | string | UUID v7 de la conversación |
| `preview` | string | Primer mensaje del usuario |
| `turn_count` | integer | Número de turnos |
| `updated_at` | string | Timestamp ISO 8601 de la última actualización |

> Si no hay conversaciones o el usuario no tiene identificador (sin auth y sin cookie anónima), devuelve `[]`.

#### GET /conversations/{id} — Obtener estado completo (F5 recovery)

```
GET /v1/search/ai/conversations/{id}
```

Devuelve el estado completo de una conversación. Permite al frontend reconstruir el chat tras un refresh de página (F5).

**Response 200:**
```json
{
  "id": "019ef5439-cb43-716d-90b5-51dcbe980908",
  "user_id": "",
  "messages": [
    {
      "role": "user",
      "content": "Quiero viajar a Barcelona",
      "timestamp": "2026-05-23T15:25:00Z"
    },
    {
      "role": "assistant",
      "content": "Encontré 15 vuelos...",
      "timestamp": "2026-05-23T15:25:02Z"
    }
  ],
  "search_cache": {
    "call_abc": {
      "response": {...},
      "destination": "Barcelona, España",
      "type": "hotels"
    }
  },
  "filters": {},
  "context": {
    "location": "Buenos Aires, Argentina",
    "country_code": "AR",
    "currency": "ARS",
    "language": "es"
  },
  "turn_count": 2,
  "max_turns": 5,
  "created_at": "2026-05-23T15:25:00Z",
  "updated_at": "2026-05-23T15:25:02Z"
}
```

| Código | HTTP | Significado |
|--------|------|-------------|
| 200 | 200 OK | Conversación encontrada y devuelta |
| 400 | 400 Bad Request | `id` vacío o inválido |
| 403 | 403 Forbidden | La conversación pertenece a otro usuario |
| 404 | 404 Not Found | Conversación no encontrada o expirada |

#### DELETE /conversations/{id} — Eliminar conversación

```
DELETE /v1/search/ai/conversations/{id}
```

Elimina una conversación y la remueve del índice del usuario.

| Código | HTTP | Significado |
|--------|------|-------------|
| 204 | 204 No Content | Conversación eliminada exitosamente |
| 400 | 400 Bad Request | `id` vacío o inválido |
| 403 | 403 Forbidden | La conversación pertenece a otro usuario |
| 404 | 404 Not Found | Conversación no encontrada |

---

### Realtime Events (SSE)

El backend emite eventos en tiempo real vía Server-Sent Events (SSE) para mantener la UI sincronizada.

#### Endpoint

```
GET /v1/realtime/events
```

> **Requerido:** Autenticación vía cookie `__Secure-access_token`. No disponible para usuarios anónimos.

#### Eventos

| Evento | Payload | Cuándo |
|--------|---------|--------|
| `search.conversation.expired` | `{"conversation_id": "019ef..."}` | Una conversación expiró automáticamente (TTL de DragonflyDB agotado, 5 min sin actividad) |
| `search.medical.alerts` | `{"alerts":[{"level":"warning"\|"info","type":"allergy"\|"medication_restricted"\|"vaccination"\|"condition"\|"travel"\|"document","message":"..."}]}` | Alertas médicas o de viaje detectadas por la AI durante una búsqueda. Solo usuarios autenticados |

#### Comportamiento Esperado del Frontend

Cuando el frontend recibe `search.conversation.expired`:

1. **Eliminar** la conversación expirada de la lista de conversaciones activas **inmediatamente**, sin esperar refresco de página.
2. **NO mostrar** ningún mensaje, notificación, toast, ni alerta. La eliminación debe ser **silenciosa**.
3. Si el usuario tiene abierta la conversación expirada, mostrar un estado de "conversación expirada" (mismo comportamiento que al recibir 404 en `GET /conversations/{id}`).

#### Comportamiento Esperado — Alertas Médicas

Cuando el frontend recibe `search.medical.alerts`:

1. **Mostrar** las alertas en una ventana emergente única con formato de lista (bullets).
2. **NO mostrar** las alertas en el texto del chat — la AI no repite el contenido de las alertas en su respuesta.
3. Las alertas se emiten solo para usuarios autenticados. Usuarios anónimos nunca reciben este evento.

#### Formato Wire — Alertas Médicas

```
event: search.medical.alerts
data: {"alerts":[{"level":"warning","type":"vaccination","message":"Se recomienda vacuna contra la fiebre amarilla para viajar a Brasil"},{"level":"info","type":"allergy","message":"Alta prevalencia de maní en la gastronomía tailandesa"}]}

```

#### Formato Wire — Conversación Expirada

```
event: search.conversation.expired
data: {"conversation_id":"019ef..."}

```

El header `Content-Type` de la respuesta es `text/event-stream`. Usar `EventSource` o `fetch` con `ReadableStream`.

#### Eventos Futuros

Este endpoint está diseñado para extenderse:

- `search.conversation.updated` — otra pestaña/dispositivo modificó la conversación
- `search.provider_unavailable` — SerpAPI no está disponible temporalmente

---

### Resolución IATA

Cuando la AI interpreta un mensaje, puede devolver nombres de ciudad en lugar de códigos IATA. El backend resuelve estos nombres a códigos de aeropuerto usando 3 niveles:

```
Nivel 1: Coincidencia exacta (embedded JSON ~300 aeropuertos)
  ├── IATA code:           "EZE" → Aeropuerto de Ezeiza
  ├── Ciudad:              "Buenos Aires" → EZE
  └── Alias:               "Madrid-Barajas" → MAD

Nivel 2: Country fallback
  ├── "Perú"               → "LIM"
  ├── "España"             → "MAD"
  └── "Argentina"          → "EZE"

Nivel 3: Fuzzy matching (sahilm/fuzzy)
  ├── "bueno aires"        → EZE (score alto)
  └── "barselona"          → BCN (score alto)
```

> **Dataset:** ~300 aeropuertos principales del mundo embebidos en el binario vía `go:embed`.

### Contexto del Intérprete AI

El backend inyecta automáticamente contexto adicional para mejorar la precisión:

#### Inyección de Ubicación

El backend detecta la ubicación del usuario por IP (`env:{ip}` en DragonflyDB) e inyecta un mensaje de sistema con la ciudad/país detectado. Esto evita preguntas innecesarias como "¿Desde dónde?".

#### Corrección de Fechas

Para corregir fechas alucinadas por la AI (modelos entrenados con datos históricos), el backend revisa todas las fechas extraídas. Si el año es anterior al año actual, se reemplaza automáticamente por el año en curso.

> **Ejemplo:** La AI devuelve `"2025-06-15"` → el backend corrige a `"2026-06-15"`. El día y mes se preservan.

---

### Contexto Médico y de Viaje

Cuando el usuario está autenticado, el backend inyecta automáticamente en el system prompt de la AI:

- **Perfil médico**: alergias, condiciones, medicamentos activos, vacunas
- **Preferencias de viaje**: clase preferida, asiento, comida, asistencias, evitar escalas
- **Documentos**: pasaporte y visados (número, país emisor, nacionalidad, vigencia)
- **Nacionalidad**: del perfil del usuario

La AI usa este contexto para:
- Alertar sobre alergias alimentarias prevalentes en el destino
- Verificar si medicamentos están restringidos/prohibidos en el país destino
- Recomendar vacunas faltantes
- Detectar si el pasaporte vence durante el viaje
- Evaluar requisitos de visa según nacionalidad

Las alertas se emiten vía la herramienta `emit_medical_alerts` como eventos SSE `alert`, 
y NO se repiten en el texto del chat. El frontend las muestra en una ventana emergente única 
con formato de lista.

Usuarios anónimos: este contexto se omite completamente.

---

### Posibles Errores (AI Search)

| Código Go | HTTP | Problem Type | Cuándo |
|-----------|------|-------------|--------|
| `domain.ErrMissingRequiredField` | 400 | Validation Error | `message` vacío o solo espacios |
| `domain.ErrConversationNotFound` | 400 | Bad Request | El `conversation_id` enviado no existe o expiró |
| `domain.ErrTurnLimitExceeded` | 400 | Bad Request | Límite máximo de turnos alcanzado (5 anónimos, 10 autenticados) |
| `domain.ErrInvalidTripType` | 400 | Bad Request | `trip_type` no es válido |
| `domain.ErrTokenInvalid` | 401 | Unauthorized | Token de reserva inválido o expirado |
| `domain.ErrTokenRequired` | 400 | Bad Request | Token de reserva requerido pero ausente |
| `domain.ErrBookingTokenExpired` | 404 | Not Found | El token de reserva ha expirado |
| `domain.ErrPropertyNotFound` | 404 | Not Found | La propiedad no fue encontrada |
| `domain.ErrRateLimitExceeded` | 429 | Too Many Requests | Demasiadas peticiones. Ver [Rate Limiting](#rate-limiting) |
| `domain.ErrAIParseFailure` | 502 | Bad Gateway | La IA devolvió una respuesta inválida o malformada |
| `domain.ErrProviderBadRequest` | 502 | Bad Gateway | El proveedor externo (SerpAPI) rechazó la solicitud |
| `domain.ErrAIUnavailable` | 503 | Service Unavailable | El servicio de IA no está configurado o no responde |
| `domain.ErrProviderUnavailable` | 503 | Service Unavailable | El proveedor externo (SerpAPI) no está disponible |
| `domain.ErrProviderError` | 503 | Service Unavailable | Error interno del proveedor externo |
| `domain.ErrConversationStoreFailed` | 503 | Service Unavailable | El almacenamiento de conversaciones (DragonflyDB) no está disponible |
| `domain.ErrDiscoveryDisabled` | 503 | Service Unavailable | El modo discovery no está habilitado |
| `domain.ErrMedicalContextFailure` | 500 | Internal Server Error | Error al leer el perfil médico, preferencias o documentos del usuario |
| (genérico) | 500 | Internal Server Error | Error inesperado no capturado por ningún mapper |

> **Diferencia clave entre 502 y 503:** `502 AI_PARSE_FAILURE` significa que la IA respondió pero con formato inválido. `503 AI_UNAVAILABLE` significa que la IA no respondió (timeout, conexión rechazada) o no está configurada.

#### Errores de Conversation CRUD

| Código Go | HTTP | Problem Type | Cuándo |
|-----------|------|-------------|--------|
| Validación en handler | 400 | Bad Request | `id` vacío en `GET/DELETE /conversations/{id}` |
| Validación en handler | 403 | Forbidden | La conversación pertenece a otro usuario |
| Validación en handler | 404 | Not Found | Conversación no encontrada o expirada |
| `domain.ErrConversationStoreFailed` | 503 | Service Unavailable | Fallo en DragonflyDB al leer/escribir |

---

## Rate Limiting

Rate limiting multi-tier con DragonflyDB y scripts Lua atómicos. Todos los límites son configurables vía variables de entorno.

### Tiers Generales

| Tier | Scope | Límite | Aplica a |
|------|-------|--------|----------|
| **Tier 1 — Global** | IP | 100 req/min | Todos los endpoints (DDoS shield) |
| **Tier 2 — Authenticated** | UUID del usuario | 10 req/min | Usuarios autenticados |
| **Tier 3 — Anonymous** | Cookie `__Secure-anon_token` | 5 req/min | Usuarios no autenticados |

### Provider-Aware Rate Limiting — AI

| Proveedor | Límite | Descripción |
|-----------|--------|-------------|
| **AI** | 10 req/hora | Límite por IP para llamadas al proveedor de IA. Configurable vía `RATELIMIT_PROVIDER_AI_MAX` y `RATELIMIT_PROVIDER_AI_WINDOW_SEC` |

> El backend cachea resultados de interpretación (blake3 hash, 10 min TTL) para reducir el consumo de créditos del proveedor de IA.

### Cookie Anónima (`__Secure-anon_token`)

Para búsquedas sin autenticación, el backend establece una cookie anónima para rate limiting y tracking de conversaciones:

```
Set-Cookie: __Secure-anon_token=019d5439-cb43-716d-90b5-51dcbe980908; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=315360000
```

| Atributo | Valor | Propósito |
|----------|-------|-----------|
| Nombre | `__Secure-anon_token` | Identificador anónimo (UUID v7) |
| TTL | 10 años (Max-Age=315360000) | Persiste entre sesiones |
| `HttpOnly` | `true` | Inaccesible vía JavaScript |
| `Secure` | `true` | Solo HTTPS en producción |
| `SameSite` | `Lax` | Se envía en navegación top-level |

> El frontend no necesita hacer nada con esta cookie. El navegador la envía automáticamente. El backend la usa para identificar conversaciones anónimas en `GET /conversations` y `DELETE /conversations/{id}`.

### Response on 429 (Rate Limit Exceeded)

Formato **RFC 9457 Problem Details**:

```json
{
  "type": "https://api.proactrip.com/errors/rate-limit-exceeded",
  "title": "Too Many Requests",
  "status": 429,
  "detail": "Demasiadas peticiones. Esperá 60 segundos antes de reintentar.",
  "instance": "/v1/search/ai",
  "trace_id": "019d5439-cb43-716d-90b5-51dcbe980908"
}
```

### Rate Limit Headers

| Header | Descripción |
|--------|-------------|
| `RateLimit-Limit` | Máximo permitido en la ventana actual |
| `RateLimit-Remaining` | Peticiones restantes |
| `RateLimit-Reset` | Segundos hasta que se reinicia la ventana |
| `Retry-After` | Segundos a esperar (solo en respuestas 429) |

---

## Cache

El backend usa una estrategia de caché en dos niveles:

### Nivel 1: Caché de Interpretación (blake3)

| Aspecto | Valor |
|---------|-------|
| TTL de caché | 10 minutos (configurable vía `AI_INTERPRETATION_CACHE_TTL`) |
| Backend de caché | DragonflyDB |
| Clave de caché | Hash blake3 de `(message + conversation_history)` |
| Qué se cachea | Solo intents completos: `flights`, `hotels`, `both` |
| Qué NO se cachea | Intents `incomplete` y `ambiguous` (necesitan preguntas frescas por contexto) |
| Invalidación | Por TTL únicamente |

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

> El campo `from_cache` en la respuesta indica si la **interpretación** vino de caché. `cached_at` indica el timestamp de cacheo. No reflejan si los resultados de búsqueda (vuelos/hoteles) estaban cacheados — esos tienen su propia estrategia independiente.

### Nivel 2: Caché de Resultados de Búsqueda

Cuando el intent es `flights`, `hotels`, o `both`, los buscadores aplican su propia estrategia de caché (blake3 sobre parámetros, 5 min TTL). Idéntica a la de los endpoints directos `POST /v1/search/flights` y `POST /v1/search/hotels`.

---

## Notas de Autenticación

### Usuarios Anónimos vs Autenticados

| Aspecto | Anónimo | Autenticado |
|---------|---------|-------------|
| **Turnos máximos** | 5 | 10 |
| **Persistencia de conversación** | DragonflyDB (TTL 5 min) | DragonflyDB + PostgreSQL (historial) |
| **Defaults de búsqueda** | Configuración del servidor (`DEFAULT_CURRENCY`, `DEFAULT_LANGUAGE`, `DEFAULT_COUNTRY_CODE`) | Perfil del usuario (`gl`, `hl`, `currency`) |
| **Rate limiting de búsqueda** | 5 req/min por cookie `__Secure-anon_token` | 10 req/min por UUID |
| **Resolución de ubicación** | IP del request → caché `env:{ip}` en DragonflyDB | Misma resolución por IP (ya no se lee `country_code` del perfil) |
| **Contexto médico** | No disponible | Inyectado en system prompt de la AI |

### Resolución de Defaults (GL/HL/Currency)

Cadena de prioridad para resolver `gl`, `hl` y `currency`:

```
Tier 1: Preferencias del perfil (DragonflyDB) → Solo si el usuario está autenticado
Tier 2: Configuración por defecto (.env)      → DEFAULT_CURRENCY, DEFAULT_LANGUAGE
```

> La ubicación (`lat`, `lng`, `timezone`, `country_code`) se resuelve automáticamente desde la IP del cliente vía `env:{ip}` en DragonflyDB. Ya no se leen del perfil del usuario ni del body del request.

### Variables de Entorno Requeridas

| Variable | Descripción | Ejemplo |
|----------|-------------|---------|
| `AI_SEARCH_PROVIDER` | Proveedor de IA: `deepseek`, `ollama`, `openai` | `deepseek` |
| `AI_SEARCH_BASE_URL` | URL base del API de IA | `https://api.deepseek.com/chat/completions` |
| `AI_SEARCH_MODEL` | Nombre del modelo | `deepseek-v4-flash` |
| `AI_SEARCH_API_KEY` | API key del proveedor (omitir para ollama local) | `sk-xxxxxxxx` |
| `AI_SEARCH_TIMEOUT` | Timeout para requests de IA | `30s` |
| `RATELIMIT_PROVIDER_AI_MAX` | Máximo de requests al proveedor de IA por ventana | `10` |
| `RATELIMIT_PROVIDER_AI_WINDOW_SEC` | Ventana de rate limiting en segundos | `3600` |

> Si el proveedor de IA no está configurado, el endpoint devuelve **503 AI_UNAVAILABLE**: *"AI search no disponible — el servicio de IA no está configurado en este entorno"*.

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

| Token | TTL | Propósito |
|-------|-----|-----------|
| `access_token` (cookie `__Secure-access_token`) | 15 min | Autenticar requests |
| `refresh_token` (cookie `__Secure-refresh_token`) | 7 días | Rotación de sesión |

### Rotación de Refresh Tokens

Cada vez que el backend refresca un `__Secure-access_token`, rota también el `__Secure-refresh_token`. Si un refresh token revocado es reutilizado, **todas las sesiones del usuario se invalidan** (detección de robo).

### Comportamiento de Tokens en AI Search

- El endpoint **no requiere autenticación**. Funciona con o sin cookies.
- Si las cookies están presentes, el backend personaliza resultados con preferencias del usuario y aumenta el límite de turnos a 10.
- Si las cookies expiraron, el backend intenta refrescarlas transparentemente. Si el refresh falla, el request continúa sin autenticación (5 turnos).
- El `conversation_id` **no** es un token de autenticación. Es un UUID v7 que identifica la conversación multi-turno.

### Prevención de Ataques

| Amenaza | Mitigación |
|---------|------------|
| XSS | `HttpOnly cookies` + `Content-Security-Policy` |
| CSRF | `SameSite=Lax` + cookies automáticas (sin `Authorization` manual) |
| Token Exposure | Cookies HttpOnly — JavaScript no puede leerlas |
| Replay de refresh | Rotación continua + invalidación total ante reúso |
| Third-party cookies | No se usa Partitioned (CHIPS) — SameSite=Lax + Domain es suficiente |
| Rate limiting abuse | Multi-tier con DragonflyDB + Lua scripts atómicos |
| Cache poisoning | Clave de caché basada en blake3 hash de parámetros validados |
| Prompt injection | La AI opera en modo restringido con system prompt fijo. Solo extrae parámetros de viaje |

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
