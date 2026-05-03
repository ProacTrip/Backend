# Hotel Search API Documentation (Cookie-Based)

> **Arquitectura:** Cookie-based authentication con HttpOnly cookies.
> El frontend nunca manipula tokens.

---

## Índice

- [Arquitectura](#arquitectura)
- [Seguridad de Cookies](#seguridad-de-cookies)
- [Base URLs](#base-urls)
- [Errores Estándar](#errores-estándar)
- [Estrategia de Refresco de Tokens](#estrategia-de-refresco-de-tokens)
- [Search Hotels](#search-hotels)
  - [Flujo de Búsqueda](#flujo-de-búsqueda)
  - [Request](#request)
  - [Responses](#responses)
    - [Hotels (matching)](#hotels-matching)
    - [Vacation Rentals (matching)](#vacation-rentals-matching)
    - [Empty / non_matching_only](#empty--non_matching_only)
  - [Response Fields Explained](#response-fields-explained)
  - [Paginación](#paginación)
  - [Posibles Errores](#posibles-errores-search-hotels)
- [Hotel Details](#hotel-details)
  - [Request](#request-hotel-details)
  - [Responses](#responses-hotel-details)
    - [Hotel Detail](#hotel-detail)
    - [Vacation Rental Detail](#vacation-rental-detail)
  - [Response Fields Explained](#response-fields-explained-hotel-details)
  - [Diferencias Clave Hotel vs VR](#diferencias-clave-entre-hotel-y-vacation-rental)
  - [Posibles Errores](#posibles-errores-hotel-details)
- [Configuración CORS](#configuración-cors)
- [Rate Limiting](#rate-limiting)
- [Cache](#cache)
- [Notas de Seguridad](#notas-de-seguridad)

---

## Arquitectura

### Flujo de Búsqueda de Hoteles

```
┌─────────────┐   POST /v1/search/hotels       ┌─────────────┐    ┌─────────────┐
│   Browser   │ ─────────────────────────────> │   Backend   │───>│   SerpAPI   │
│  (Frontend) │  {query, check_in_date, ...}   │             │    │  (Google    │
└─────────────┘                                └─────────────┘    │   Hotels)   │
^                                                      │          └─────────────┘
│                               Set-Cookie: __Secure-access_token=...           │
│                              (si el usuario está autenticado)                 │
│                              Response: { type, properties[], brands[], ... }  │
└───────────────────────────────────────────────────────────────────────────────┘

Las cookies de autenticación se envían AUTOMÁTICAMENTE en cada request.
El frontend NO almacena ni lee tokens. No se requiere header Authorization.
```

### Un Solo Endpoint, Dos Modos

El endpoint `POST /v1/search/hotels` maneja tanto hoteles como alquileres vacacionales:

```
vacation_rentals: false (default)
  └── Busca hoteles tradicionales
      └── Response: type: "hotels", con brands[] disponible

vacation_rentals: true
  └── Busca casas, villas, apartamentos
      └── Response: type: "vacation_rentals", con capacity y excluded_amenities
```

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

Formato **RFC 7807 Problem Details**:

```json
{
  "type": "validation_error",
  "title": "Validation Error",
  "status": 400,
  "detail": "El campo 'query' es requerido",
  "instance": "/v1/search/hotels",
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

## Search Hotels

Busca hoteles y alquileres vacacionales estructuradamente usando SerpAPI (Google Hotels).

### Flujo de Búsqueda

```
┌────────────────────────────────────────────────────────────────────┐
│                     HOTELS (vacation_rentals: false)               │
│                                                                    │
│  POST /v1/search/hotels                                            │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Request:  { query:"Bali", check_in_date:"2026-03-16",        │  │
│  │            check_out_date:"2026-03-20", adults:2, ... }      │  │
│  │ Response: { type:"hotels", results_state:"matching",         │  │
│  │            properties[{id, hotel_class, price, ...}],        │  │
│  │            brands[{id, name, chains[]}],                     │  │
│  │            pagination{next_token, has_more} }                │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                            ↓                                       │
│        Usuario elige un hotel → su id (property_token)             │
│                            ↓                                       │
│  POST /v1/search/hotel-details { id, check_in/out_date, ... }      │
│  → Detalle completo con external_reviews, nearby_places, etc.      │
└────────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────────┐
│               VACATION RENTALS (vacation_rentals: true)            │
│                                                                    │
│  POST /v1/search/hotels                                            │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Request:  { query:"Bali", check_in_date:"2026-03-16",        │  │
│  │            check_out_date:"2026-03-20",                      │  │
│  │            vacation_rentals:true, property_types:[10], ... } │  │
│  │ Response: { type:"vacation_rentals",                         │  │
│  │            results_state:"matching",                         │  │
│  │            properties[{id, capacity, excluded_amenities...}],│  │
│  │            brands:null }                                     │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                            ↓                                       │
│        Usuario elige una propiedad → su id (property_token)        │
│                            ↓                                       │
│  POST /v1/search/hotel-details                                     │
│    { id, check_in/out_date, vacation_rentals:true, ... }           │
│  → Detalle completo con capacity, price, nearby_places             │
└────────────────────────────────────────────────────────────────────┘
```

### Request

```
POST /v1/search/hotels
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
  "query": "Bali Resorts",
  "check_in_date": "2026-03-16",
  "check_out_date": "2026-03-20",
  "adults": 2,
  "children": 0,
  "children_ages": [],
  "gl": "ES",
  "hl": "es",
  "currency": "EUR",
  "min_price": null,
  "max_price": null,
  "sort_by": null,
  "rating": null,
  "property_types": [],
  "amenities": [],
  "vacation_rentals": false,
  "hotel_classes": [],
  "brands": [],
  "free_cancellation": false,
  "special_offers": false,
  "eco_certified": false,
  "bedrooms": null,
  "bathrooms": null,
  "page_token": null
}
```

**Campos Requeridos:**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `query` | string | Destino de búsqueda o nombre de hotel/VR. Ej: `"Bali"`, `"Paris France"`, `"cerca de 28938, Móstoles"` |
| `check_in_date` | string | Fecha de entrada. Formato `YYYY-MM-DD` |
| `check_out_date` | string | Fecha de salida. Formato `YYYY-MM-DD` |

**Campos Opcionales:**

| Campo | Tipo | Default | Descripción |
|-------|------|---------|-------------|
| `adults` | integer | `2` | Número de adultos. Mínimo 1 |
| `children` | integer | `0` | Número de niños |
| `children_ages` | integer[] | `[]` | Edades de los niños (1-17). Debe coincidir con `children` |
| `gl` | string\|null | `null` | Código ISO 3166-1 alpha-2. Ej: `"ES"`, `"PE"`. Personaliza resultados al país. Debe tener relación con `query` |
| `hl` | string\|null | `null` | Código de idioma ISO 639-1. Ej: `"es"`, `"en"`, `"fr"` |
| `currency` | string | `"USD"` | Código ISO 4217. Ej: `"EUR"`, `"GBP"` |
| `min_price` | number\|null | `null` | Precio mínimo por noche |
| `max_price` | number\|null | `null` | Precio máximo por noche |
| `sort_by` | integer\|null | `null` | Orden de resultados. Ver tabla de ordenamiento |
| `rating` | integer\|null | `null` | Filtro de rating mínimo. Ver tabla de valores |
| `property_types` | integer[] | `[]` | Tipos de propiedad. Ver tabla de valores |
| `amenities` | integer[] | `[]` | Servicios requeridos. Ver tabla de valores |
| `vacation_rentals` | boolean | `false` | `true` para buscar alquileres vacacionales |
| `hotel_classes` | integer[] | `[]` | Categorías de hotel (2-5). Solo para Hotels |
| `brands` | integer[] | `[]` | IDs de cadenas hoteleras. Solo para Hotels |
| `free_cancellation` | boolean | `false` | Solo hoteles con cancelación gratuita |
| `special_offers` | boolean | `false` | Solo hoteles con ofertas especiales |
| `eco_certified` | boolean | `false` | Solo hoteles con certificación ecológica |
| `bedrooms` | integer\|null | `null` | Mínimo de dormitorios. Solo para VR |
| `bathrooms` | integer\|null | `null` | Mínimo de baños. Solo para VR |
| `page_token` | string\|null | `null` | Token para paginación |

### Valores Codificados

**`hotel_classes` — clase del hotel:**

| Valor | Significado |
|-------|-------------|
| `2` | 2 estrellas |
| `3` | 3 estrellas |
| `4` | 4 estrellas |
| `5` | 5 estrellas |

**`property_types` — tipo de alojamiento:**

*Vacation Rentals:*

| Valor | Tipo |
|-------|------|
| `1` | Apartments |
| `2` | Bungalows |
| `3` | Cabins |
| `4` | Chalets |
| `5` | Cottages |
| `6` | Gîtes |
| `7` | Holiday villages |
| `8` | Houses |
| `9` | Houseboats |
| `10` | Villas |
| `11` | Other |
| `21` | Apartment hotels |

*Hotels:*

| Valor | Tipo |
|-------|------|
| `12` | Beach hotels |
| `13` | Boutique hotels |
| `14` | Hostels |
| `15` | Inns |
| `16` | Motels |
| `17` | Resorts |
| `18` | Spa hotels |
| `19` | Bed & breakfasts |
| `20` | Other |
| `21` | Apartment hotels |
| `22` | Minshuku |
| `23` | Japanese-style business hotels |
| `24` | Ryokan |

**`amenities` — servicios disponibles:**

*Hotels:*

| Valor | Servicio |
|-------|----------|
| `1` | Free parking |
| `3` | Parking |
| `4` | Indoor pool |
| `5` | Outdoor pool |
| `6` | Pool |
| `7` | Fitness center |
| `8` | Restaurant |
| `9` | Free breakfast |
| `10` | Spa |
| `11` | Beach access |
| `12` | Child-friendly |
| `15` | Bar |
| `19` | Pet-friendly |
| `22` | Room service |
| `35` | Free Wi-Fi |
| `40` | Air-conditioned |
| `52` | All-inclusive available |
| `53` | Wheelchair accessible |
| `61` | EV charger |

*Vacation Rentals:*

| Valor | Servicio |
|-------|----------|
| `2` | Hot tub |
| `4` | Air-conditioned |
| `6` | Outdoor grill |
| `10` | Fireplace |
| `12` | Patio or deck |
| `15` | Kitchen |
| `16` | Fitness centre |
| `18` | Cot |
| `20` | Beach access |
| `21` | Child-friendly |
| `24` | Pet-friendly |
| `29` | Free Wi-Fi |
| `32` | Pool |

**`rating` — puntuación mínima:**

| Valor | Significado |
|-------|-------------|
| `7` | 3.5+ |
| `8` | 4.0+ |
| `9` | 4.5+ |

**`sort_by` — orden de resultados:**

| Valor | Orden |
|-------|-------|
| `3` | Precio más bajo |
| `8` | Mayor rating |
| `13` | Más reseñas |

> Omitir `sort_by` = relevancia por defecto del proveedor.

### Notas sobre Parámetros de Localización

La ubicación del usuario la resuelve **siempre el backend** a partir de la IP de la request. El frontend nunca llama a APIs externas de geolocalización directamente.

**Usuarios no autenticados:** Al cargar la página por primera vez, el frontend llama a `GET /v1/environment` para obtener la ubicación y el clima detectados desde la IP. Con esa respuesta, el frontend muestra un mensaje al usuario indicando la ubicación y el clima detectados y pone por defecto `gl`, `hl` y `currency` en las búsquedas posteriores pero el usuario puede cambiarlos.

```
Frontend (primera carga en cualquier página)
  │
  ├── GET /v1/environment  →  { location: {...}, weather: {...} }
  │
  └── POST /v1/search/hotels  { query:"...", gl:"ES", hl:"es", currency:"EUR", ... }
```

El endpoint `/v1/environment` usa ipquery.io internamente para resolver la IP y OpenWeather para el clima.

Si se necesita buscar hoteles cerca de la ubicación del usuario, se pasa `gl` y en `query` se pasa código postal y municipio/distrito. Ej: `"cerca de 28938, Móstoles"`. `query` y `gl` deben estar relacionados — no se puede buscar hoteles en Miami con `gl: "ES"`.

**Usuarios autenticados:** El backend usa los datos guardados en el perfil (país, idioma, moneda) y el `environment` recibido cacheados o no. Los parámetros `gl`, `hl`, `currency` pueden omitirse o usarse para sobrescribir temporalmente las preferencias.

### Ejemplos curl

#### Búsqueda básica de hoteles

```bash
curl -X POST {base_url}/hotels \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ...; __Secure-refresh_token=v4.local.eyJ..." \
  -d '{
    "query": "Bali",
    "check_in_date": "2026-03-16",
    "check_out_date": "2026-03-20",
    "adults": 2,
    "currency": "EUR"
  }'
```

> **Nota:** Las cookies se envían con `-b` solo si el usuario está autenticado. Para búsquedas anónimas, omitir el flag `-b`.

#### Búsqueda con localización (usuario no autenticado)

```bash
curl -X POST {base_url}/hotels \
  -H "Content-Type: application/json" \
  -d '{
    "query": "cerca de 28938, Móstoles",
    "check_in_date": "2026-03-16",
    "check_out_date": "2026-03-20",
    "adults": 1,
    "gl": "ES",
    "hl": "es",
    "currency": "EUR"
  }'
```

#### Búsqueda con filtros (Hotels)

```bash
curl -X POST {base_url}/hotels \
  -H "Content-Type: application/json" \
  -d '{
    "query": "Bali Resorts",
    "check_in_date": "2026-03-16",
    "check_out_date": "2026-03-20",
    "adults": 2,
    "children": 2,
    "children_ages": [5, 8],
    "currency": "EUR",
    "min_price": 100,
    "max_price": 500,
    "sort_by": 3,
    "rating": 8,
    "hotel_classes": [4, 5],
    "property_types": [17, 18],
    "amenities": [35, 9, 10],
    "free_cancellation": true
  }'
```

#### Búsqueda de Vacation Rentals

```bash
curl -X POST {base_url}/hotels \
  -H "Content-Type: application/json" \
  -d '{
    "query": "Bali",
    "check_in_date": "2026-03-16",
    "check_out_date": "2026-03-20",
    "adults": 4,
    "currency": "EUR",
    "vacation_rentals": true,
    "property_types": [10],
    "bedrooms": 2,
    "bathrooms": 2,
    "amenities": [15, 4, 32]
  }'
```

#### Segunda página de resultados

```bash
curl -X POST {base_url}/hotels \
  -H "Content-Type: application/json" \
  -d '{
    "query": "Bali",
    "check_in_date": "2026-03-16",
    "check_out_date": "2026-03-20",
    "adults": 2,
    "currency": "EUR",
    "page_token": "CBI="
  }'
```

### Responses

#### Hotels (matching)

```json
{
  "type": "hotels",
  "results_state": "matching",
  "properties": [
    {
      "id": "ChcIquf58YLhjLVOGgsvZy8xdzB6cWNmbRAB",
      "type": "hotel",
      "name": "Pullman Bali Legian Beach",
      "description": "Hotel de alta gama con 2 restaurantes, bar, spa y una piscina infinita en la azotea con vistas al mar.",
      "booking_url": "https://proactrip.com/book/hotel/ChcIquf58YLhjLVOGgsvZy8xdzB6cWNmbRAB",
      "gps": {
        "lat": -8.7097252,
        "lng": 115.1672141
      },
      "hotel_class": 5,
      "check_in": "15:00",
      "check_out": "12:00",
      "rating": {
        "overall": 4.6,
        "location": 4.4
      },
      "total_reviews": 9434,
      "price": {
        "currency": "EUR",
        "per_night": {
          "amount": 205.0,
          "before_taxes": 169.0
        },
        "total": {
          "amount": 820.0,
          "before_taxes": 677.0
        }
      },
      "images": [
        {
          "thumbnail": "https://lh3.googleusercontent.com/gps-cs-s/AHVAweo3_SYKnNk2xbz5j8HoEpu6Pacic6Ao9V7mg3Rjg-EjBL5qjcARREq9BTpOs7ApcH_T8FKE_e8-JL_rFbTLP8QNH_tEi8_qwleaGFN6vyMoSKvaqY4Zhndsc_p_Ce0yBhsqGa96u2KFLazk=s287-w287-h192-n-k-no-v1",
          "original": "https://lh3.googleusercontent.com/gps-cs-s/AHVAweo3_SYKnNk2xbz5j8HoEpu6Pacic6Ao9V7mg3Rjg-EjBL5qjcARREq9BTpOs7ApcH_T8FKE_e8-JL_rFbTLP8QNH_tEi8_qwleaGFN6vyMoSKvaqY4Zhndsc_p_Ce0yBhsqGa96u2KFLazk=s10000"
        }
      ],
      "amenities": [
        "Free Wi-Fi",
        "Outdoor pool",
        "Air conditioning",
        "Fitness center",
        "Spa",
        "Bar",
        "Restaurant",
        "Room service",
        "Kid-friendly",
        "Accessible"
      ],
      "nearby_places": [
        {
          "name": "Kuta Beach",
          "transport": [
            { "type": "Walking", "duration": "1 min" }
          ]
        }
      ],
      "free_cancellation": false,
      "special_offer": true,
      "eco_certified": true,
      "ratings": [
        { "stars": 5, "count": 4321 },
        { "stars": 4, "count": 3890 },
        { "stars": 3, "count": 987 },
        { "stars": 2, "count": 156 },
        { "stars": 1, "count": 80 }
      ],
      "reviews_breakdown": [
        {
          "name": "Service",
          "description": "Guest satisfaction with hotel staff and service",
          "total_mentioned": 5200,
          "positive": 4100,
          "negative": 600,
          "neutral": 500
        },
        {
          "name": "Cleanliness",
          "description": "Guest satisfaction with room and property cleanliness",
          "total_mentioned": 4800,
          "positive": 4300,
          "negative": 250,
          "neutral": 250
        },
        {
          "name": "Location",
          "description": "Guest satisfaction with the property's location",
          "total_mentioned": 3900,
          "positive": 3500,
          "negative": 200,
          "neutral": 200
        }
      ]
    }
  ],
  "brands": [
    {
      "id": 33,
      "name": "Accor Live Limitless",
      "chains": [
        { "id": 67, "name": "Banyan Tree" },
        { "id": 101, "name": "Grand Mercure" },
        { "id": 47, "name": "Novotel" },
        { "id": 90, "name": "Pullman Hotels and Resorts" }
      ]
    }
  ],
  "pagination": {
    "next_token": "CBI=",
    "has_more": true
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

#### Vacation Rentals (matching)

```json
{
  "type": "vacation_rentals",
  "results_state": "matching",
  "properties": [
    {
      "id": "ChkQxJznqd6lrK1EGg0vZy8xMXZoeTVjcTdiEAI",
      "type": "vacation_rental",
      "name": "Villa Mahayoni. Exquisite, jungle view villa.",
      "booking_url": "https://proactrip.com/book/rental/ChkQxJznqd6lrK1EGg0vZy8xMXZoeTVjcTdiEAI",
      "gps": {
        "lat": -8.5096197,
        "lng": 115.2966079
      },
      "check_in": null,
      "check_out": "12:00",
      "rating": {
        "overall": 4.4,
        "location": 2.9
      },
      "total_reviews": 123,
      "price": {
        "currency": "EUR",
        "per_night": {
          "amount": 49.0,
          "before_taxes": 44.0
        },
        "total": {
          "amount": 195.0,
          "before_taxes": 178.0
        }
      },
      "images": [
        {
          "thumbnail": "https://lh6.googleusercontent.com/proxy/nGBNISjXWBKVIqfTSP7erqG5HpQqd9JTAr0IIp6PrJOHfVPp6t8WyWOgquKsgpEFwkSx3Wk7HSf5LuiCuvdzJ6QAuh7otWkMApDvjvTBi833YlNQ4pRtPZALsgrKe9d9FzrV9p2i5QPQSX2qYDD-HE4n47xHRw=s287-w287-h192-n-k-no-v1",
          "original": "https://pix8.agoda.net/hotelImages/34168113/0/09a7c808fbd0501049f8b346ac68cdd0.jpg?ce=0"
        }
      ],
      "amenities": [
        "Balcón",
        "Jacuzzi",
        "Piscina interior",
        "Wi‑Fi gratis"
      ],
      "excluded_amenities": [
        "No hay servicio de traslado al aeropuerto",
        "No hay acceso a la playa"
      ],
      "nearby_places": [
        {
          "name": "Aeropuerto Internacional Ngurah Rai",
          "transport": [
            { "type": "Taxi", "duration": "1 h 15 min" }
          ]
        }
      ],
      "capacity": {
        "unit_type": "Villa completa",
        "guests": 2,
        "bedrooms": 1,
        "bathrooms": 1,
        "beds": null,
        "area": "45 ft²"
      },
      "ratings": [
        { "stars": 5, "count": 45 },
        { "stars": 4, "count": 52 },
        { "stars": 3, "count": 18 },
        { "stars": 2, "count": 5 },
        { "stars": 1, "count": 3 }
      ],
      "reviews_breakdown": [
        {
          "name": "Cleanliness",
          "description": "Guest satisfaction with room and property cleanliness",
          "total_mentioned": 110,
          "positive": 95,
          "negative": 8,
          "neutral": 7
        },
        {
          "name": "Value for money",
          "description": "Guest satisfaction with the value received",
          "total_mentioned": 98,
          "positive": 80,
          "negative": 10,
          "neutral": 8
        }
      ],
      "prices": [
        {
          "source": "Booking.com",
          "logo": "https://www.gstatic.com/travel-hotels/branding/icon_booking.png",
          "num_guests": 2,
          "rate_per_night": { "amount": 49.0, "before_taxes": 44.0 }
        },
        {
          "source": "Agoda",
          "logo": "https://www.gstatic.com/travel-hotels/branding/icon_agoda.png",
          "num_guests": 2,
          "rate_per_night": { "amount": 51.0, "before_taxes": 46.0 }
        }
      ]
    }
  ],
  "brands": null,
  "pagination": {
    "next_token": "CBM=",
    "has_more": true
  },
  "from_cache": false,
  "cached_at": null
}
```

#### Empty / non_matching_only

Cuando los filtros son demasiado restrictivos, el proveedor devuelve resultados no exactos marcados como `non_matching_only`.

```json
{
  "type": "hotels",
  "results_state": "non_matching_only",
  "properties": [
    {
      "id": "ChcIquf58YLhjLVOGgsvZy8xdzB6cWNmbRAB",
      "type": "hotel",
      "name": "Pullman Bali Legian Beach",
      "description": "Hotel de alta gama con 2 restaurantes, bar, spa y una piscina infinita en la azotea con vistas al mar.",
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
      "amenities": [],
      "nearby_places": [],
      "free_cancellation": false,
      "special_offer": true,
      "eco_certified": true,
      "ratings": [],
      "reviews_breakdown": []
    }
  ],
  "brands": null,
  "pagination": {
    "next_token": null,
    "has_more": false
  },
  "from_cache": false,
  "cached_at": null
}
```

> **Nota para el frontend:** Cuando `results_state` es `"non_matching_only"`, mostrar al usuario: *"No encontramos resultados exactos con tus filtros. Mostrando los alojamientos más cercanos."*

### Response Fields Explained

#### Nivel Raíz

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `type` | string | `"hotels"` o `"vacation_rentals"` según el modo de búsqueda |
| `results_state` | string | `"matching"` = resultados exactos; `"non_matching_only"` = filtros restrictivos, aproximaciones mostradas |
| `properties` | array | Lista de propiedades encontradas |
| `brands` | array\|null | Cadenas hoteleras disponibles para filtro. Solo presente en Hotels; `null` en VR |
| `pagination.next_token` | string\|null | Token para siguiente página. `null` si no hay más |
| `pagination.has_more` | boolean | `true` si hay más páginas disponibles |
| `from_cache` | boolean | `true` si la respuesta vino de caché |
| `cached_at` | string\|null | Timestamp ISO 8601 del momento en que se cacheó. `null` si no es de caché |

#### Property Object (Hotel)

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `id` | string | **Importante:** Identificador opaco (property_token). Usar en `POST /v1/search/hotel-details` y para reservas |
| `type` | string | `"hotel"` |
| `name` | string | Nombre del hotel |
| `description` | string\|null | Descripción corta |
| `booking_url` | string\|null | Enlace interno para proceder con la reserva |
| `gps.lat` | number | Latitud |
| `gps.lng` | number | Longitud |
| `hotel_class` | integer\|null | Estrellas (2-5) |
| `check_in` | string\|null | Hora de entrada. Ej: `"15:00"` |
| `check_out` | string\|null | Hora de salida. Ej: `"12:00"` |
| `rating.overall` | number\|null | Puntuación media (0-5) |
| `rating.location` | number\|null | Puntuación de ubicación (0-5) |
| `total_reviews` | integer\|null | Total de reseñas |
| `price.currency` | string | Código ISO 4217 |
| `price.per_night.amount` | number | Precio por noche con impuestos |
| `price.per_night.before_taxes` | number\|null | Precio por noche sin impuestos |
| `price.total.amount` | number | Precio total con impuestos |
| `price.total.before_taxes` | number\|null | Precio total sin impuestos |
| `images` | array | Imágenes: `thumbnail` (listado) y `original` (detalle) |
| `amenities` | string[] | Servicios disponibles (texto legible) |
| `nearby_places` | array | Lugares cercanos con transporte |
| `free_cancellation` | boolean | Cancelación gratuita disponible |
| `special_offer` | boolean | Oferta especial activa |
| `eco_certified` | boolean | Certificación ecológica |
| `ratings` | array | Distribución de puntuaciones por estrellas (histograma de ratings) |
| `ratings[].stars` | integer | Cantidad de estrellas (1–5) |
| `ratings[].count` | integer | Número de reseñas con esa puntuación |
| `reviews_breakdown` | array | Desglose de reseñas por categoría con análisis de sentimiento |
| `reviews_breakdown[].name` | string | Nombre de la categoría. Ej: `"Service"`, `"Cleanliness"`, `"Location"` |
| `reviews_breakdown[].description` | string | Descripción de la categoría |
| `reviews_breakdown[].total_mentioned` | integer | Total de reseñas que mencionan esta categoría |
| `reviews_breakdown[].positive` | integer | Reseñas con sentimiento positivo en esta categoría |
| `reviews_breakdown[].negative` | integer | Reseñas con sentimiento negativo en esta categoría |
| `reviews_breakdown[].neutral` | integer | Reseñas con sentimiento neutral en esta categoría |

#### Property Object (Vacation Rental)

Todos los campos de Hotel más (incluye `ratings` y `reviews_breakdown`, heredados de Hotel):

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `excluded_amenities` | string[] | Servicios que NO tiene la propiedad. Lista de amenities ausentes en formato legible (ej: `"No hay servicio de traslado al aeropuerto"`, `"No outdoor pool"`). Los amenities presentes van en el array `amenities` |
| `capacity.unit_type` | string | Tipo de unidad. Ej: `"Villa completa"`, `"Entire house"` |
| `capacity.guests` | integer\|null | Capacidad máxima de huéspedes |
| `capacity.bedrooms` | integer\|null | Número de dormitorios |
| `capacity.bathrooms` | integer\|null | Número de baños |
| `capacity.beds` | integer\|null | Número de camas |
| `capacity.area` | string\|null | Superficie. Ej: `"45 ft²"` |
| `prices` | array | Precios por fuente de reserva (comparación de OTAs). Permite mostrar tarifas por plataforma |
| `prices[].source` | string | Nombre de la plataforma. Ej: `"Booking.com"`, `"Agoda"`, `"Expedia"` |
| `prices[].logo` | string\|null | URL del logo de la plataforma |
| `prices[].num_guests` | integer\|null | Número de huéspedes para esa tarifa |
| `prices[].rate_per_night.amount` | number | Precio por noche con impuestos en esa plataforma |
| `prices[].rate_per_night.before_taxes` | number\|null | Precio por noche sin impuestos en esa plataforma |

Y los siguientes campos están **ausentes** en VR:
- `hotel_class` — no aplica (no hay estrellas)
- `free_cancellation`, `special_offer`, `eco_certified` — no incluidos en VR

#### Nearby Place (en properties[])

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `name` | string | Nombre del lugar cercano |
| `transport` | array | Medios de transporte disponibles |
| `transport[].type` | string | Tipo: `"Walking"`, `"Taxi"`, `"Public transport"` |
| `transport[].duration` | string | Duración estimada. Ej: `"1 min"`, `"20 min"` |

#### Brands Object (solo Hotels)

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `id` | integer | ID de la cadena principal. Usar en filtro `brands` |
| `name` | string | Nombre de la cadena principal |
| `chains` | array | Sub-cadenas disponibles |
| `chains[].id` | integer | ID de la sub-cadena para filtro `brands` |
| `chains[].name` | string | Nombre de la sub-cadena |

### Paginación

Cuando una búsqueda devuelve más resultados de los que caben en una página, la respuesta incluye metadatos de paginación en `pagination`.

```
1. Primera página: POST /v1/search/hotels { ..., "page_token": null }
2. Siguiente página: POST /v1/search/hotels { ..., "page_token": "<next_token>" }
```

> **Nota:** El `page_token` es opaco y no debe ser interpretado ni construido por el cliente. Su formato puede cambiar sin previo aviso.

### Posibles Errores (Search Hotels)

| Código | HTTP | Cuándo |
|--------|------|--------|
| `VALIDATION_ERROR` | 400 | Body inválido, falta `query`, fechas mal formateadas, `children_ages` no coincide con `children` |
| `INVALID_PARAM_RANGE` | 422 | Parámetros fuera de rango (`bedrooms`, `bathrooms` negativos, `adults` < 1) |
| `PROVIDER_UNAVAILABLE` | 503 | El proveedor externo (SerpAPI) no está disponible |
| `TOKEN_INVALID` | 401 | Cookie de sesión inválida o expirada (solo si el usuario estaba autenticado) |
| `RATE_LIMIT_EXCEEDED` | 429 | Demasiadas peticiones (RFC 7807 Problem JSON). Ver [Rate Limiting](#rate-limiting) |
| `INTERNAL_ERROR` | 500 | Error inesperado del servidor |

---

## Hotel Details

Devuelve los detalles completos de una propiedad específica — hotel o vacation rental — usando el `id` obtenido de `POST /v1/search/hotels`.

### Request (Hotel Details)

```
POST /v1/search/hotel-details
```

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Content-Type` | string | Sí | `application/json` |

> Las cookies se envían automáticamente si existen. No se requiere header `Authorization`.

**Body:**

```json
{
  "id": "ChcI9NCGtf-jlI0BGgsvZy8xdGpoMW50cxAB",
  "check_in_date": "2026-03-16",
  "check_out_date": "2026-03-20",
  "adults": 2,
  "children": 0,
  "children_ages": [],
  "gl": "ES",
  "hl": "es",
  "currency": "EUR",
  "vacation_rentals": false
}
```

**Campos:**

| Campo | Tipo | Requerido | Default | Descripción |
|-------|------|-----------|---------|-------------|
| `id` | string | Sí | — | Identificador opaco de la propiedad. Es el campo `id` (`property_token`) del resultado de búsqueda |
| `check_in_date` | string | Sí | — | Fecha de entrada. Formato `YYYY-MM-DD` |
| `check_out_date` | string | Sí | — | Fecha de salida. Formato `YYYY-MM-DD` |
| `adults` | integer | No | `2` | Número de adultos |
| `children` | integer | No | `0` | Número de niños |
| `children_ages` | integer[] | No | `[]` | Edades de los niños (1-17) |
| `gl` | string\|null | No | `null` | Código ISO 3166-1 alpha-2. Ej: `"ES"`, `"PE"` |
| `hl` | string\|null | No | `null` | Código de idioma ISO 639-1. Ej: `"es"`, `"en"` |
| `currency` | string | No | `"USD"` | Código ISO 4217 |
| `vacation_rentals` | boolean | No | `false` | `true` si el `id` pertenece a una vacation rental |

> **Importante sobre `id`:** Es el `property_token` de SerpAPI — el campo `id` que el backend normaliza en la respuesta de búsqueda. Es opaco e inmutable. El frontend debe conservarlo tal cual y reenviarlo.

### Ejemplos curl

#### Detalle de Hotel

```bash
curl -X POST {base_url}/hotel-details \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ...; __Secure-refresh_token=v4.local.eyJ..." \
  -d '{
    "id": "ChcI9NCGtf-jlI0BGgsvZy8xdGpoMW50cxAB",
    "check_in_date": "2026-03-16",
    "check_out_date": "2026-03-20",
    "adults": 2,
    "children": 2,
    "children_ages": [5, 8],
    "currency": "EUR",
    "vacation_rentals": false
  }'
```

#### Detalle de Vacation Rental

```bash
curl -X POST {base_url}/hotel-details \
  -H "Content-Type: application/json" \
  -d '{
    "id": "ChoQnPq_qJbm2PL6ARoNL2cvMTF0eHFsenpzNhAC",
    "check_in_date": "2026-03-16",
    "check_out_date": "2026-03-20",
    "adults": 2,
    "children": 2,
    "children_ages": [5, 8],
    "currency": "EUR",
    "vacation_rentals": true
  }'
```

### Responses (Hotel Details)

#### Hotel Detail

```json
{
  "id": "ChcI9NCGtf-jlI0BGgsvZy8xdGpoMW50cxAB",
  "type": "hotel",
  "name": "Novotel Bali Benoa",
  "description": "Este lujoso hotel, que ocupa un conjunto de edificios con techo de paja rodeados de palmeras, se encuentra en la playa de Tanjung Benoa, a 13 km del animado parque Garuda Wisnu Kencana y de la pintoresca península de Bukit. Las habitaciones son elegantes y cuentan con televisión por satélite, Wi-Fi gratis, cafetera, tetera y, en algunos casos, terraza y vistas al mar. El hotel dispone de 3 restaurantes al aire libre, 3 bares, 3 piscinas exteriores, pistas de tenis, spa, gimnasio y aparcamiento gratuito.",
  "booking_url": "https://proactrip.com/book/hotel/ChcI9NCGtf-jlI0BGgsvZy8xdGpoMW50cxAB",
  "address": "Jl. Pratama No.70, Tj. Benoa, Kec. Kuta Sel., Kabupaten Badung, Bali 80361, Indonesia",
  "directions_url": "https://maps.google.com/maps?hl=es&gl=US&daddr=Novotel+Bali+Benoa...",
  "gps": {
    "lat": -8.76436,
    "lng": 115.222681
  },
  "hotel_class": 5,
  "check_in": "14:00",
  "check_out": "12:00",
  "price_range": {
    "currency": "EUR",
    "min": 168.0,
    "max": 252.0
  },
  "rating": {
    "overall": 4.7,
    "location": 4.2
  },
  "total_reviews": 3977,
  "external_reviews": [
    {
      "source": "all.accor.com",
      "logo_url": "//www.gstatic.com/travel-hotels/branding/icon_default.png",
      "score": 4.2,
      "max_score": 5.0,
      "total_reviews": 749,
      "featured_review": {
        "author": "Lucy",
        "date": "2025-04-15T10:30:00Z",
        "score": 5.0,
        "comment": "Todo espectacular! Excelente desayuno, atención muy amable, espacios muy relajantes, habitaciones espaciosas y completas.",
        "url": null
      }
    },
    {
      "source": "Tripadvisor",
      "logo_url": "https://www.gstatic.com/travel-hotels/branding/icon_100532569.png",
      "score": 4.3,
      "max_score": 5.0,
      "total_reviews": 4004,
      "featured_review": {
        "author": "Companion24533989353",
        "date": "2025-03-20T14:22:00Z",
        "score": 5.0,
        "comment": "La estancia fue extraordinaria, el hotel muy cómodo y limpio. El personal muy amable y atento.",
        "url": "https://www.tripadvisor.es/Hotel_Review-..."
      }
    }
  ],
  "images": [
    {
      "thumbnail": "https://lh3.googleusercontent.com/gps-cs-s/AHVAweqMXMuz9NVKjY6emuge2EtNyObODhU60YdyBbaLg3oqGLD980yB6p9XWvA-y7AjfEH_ZeQywiW1zraSqs6fzAYEYIWnCYZE42oG4wo_9GYEZ_AzGLPns3_M-QtLbPJpkhcv_lOQCqhAurU=s287-w287-h192-n-k-no-v1",
      "original": "https://lh3.googleusercontent.com/gps-cs-s/AHVAweqMXMuz9NVKjY6emuge2EtNyObODhU60YdyBbaLg3oqGLD980yB6p9XWvA-y7AjfEH_ZeQywiW1zraSqs6fzAYEYIWnCYZE42oG4wo_9GYEZ_AzGLPns3_M-QtLbPJpkhcv_lOQCqhAurU=s10000"
    }
  ],
  "amenities": [
    "Breakfast ($)",
    "Free Wi-Fi",
    "Parking ($)",
    "Outdoor pool",
    "Air conditioning",
    "Fitness center",
    "Spa",
    "Bar",
    "Restaurant",
    "Room service",
    "Airport shuttle",
    "Full-service laundry",
    "Accessible",
    "Business center",
    "Kid-friendly"
  ],
  "health_and_safety": [
    {
      "category": "Enhanced cleaning",
      "items": [
        { "name": "Disinfectant used to clean the property", "available": true },
        { "name": "High-touch surfaces disinfected", "available": true }
      ]
    },
    {
      "category": "Minimized contact",
      "items": [
        { "name": "No-contact check-in and check-out", "available": true }
      ]
    }
  ],
  "sustainability": [
    {
      "category": "Energy efficiency",
      "items": [
        { "name": "Energy use audited by an independent organization", "available": true },
        { "name": "LED lighting throughout the property", "available": true },
        { "name": "Renewable energy used", "available": true }
      ]
    },
    {
      "category": "Water use",
      "items": [
        { "name": "Water use audited by an independent organization", "available": true },
        { "name": "Water-efficient faucets, toilets, and showers", "available": true },
        { "name": "Towel and linen reuse program", "available": true }
      ]
    },
    {
      "category": "Waste reduction",
      "items": [
        { "name": "Recycling program", "available": true },
        { "name": "Food waste reduction program", "available": true },
        { "name": "No single-use plastic water bottles or straws", "available": true }
      ]
    },
    {
      "category": "Sustainable sourcing",
      "items": [
        { "name": "Organic food and beverages", "available": true },
        { "name": "Locally sourced food and beverages", "available": true },
        { "name": "Vegetarian and vegan meals", "available": true }
      ]
    }
  ],
  "eco_certified": true,
  "nearby_places": [
    {
      "category": "Point of interest",
      "name": "Seminyak",
      "description": "Región de playa orientada al oeste famosa por el surf y la variedad de complejos turísticos vacacionales.",
      "rating": null,
      "total_reviews": null,
      "thumbnail_url": "https://encrypted-tbn1.gstatic.com/images?q=tbn:ANd9GcRUPA5nye2McEkUIaREEUp0WKc-HctPeH6GVeGAPC_g-wznXQUW",
      "maps_url": "https://www.google.com/search?q=Seminyak...",
      "gps": {
        "lat": -8.6909879,
        "lng": 115.1677598
      },
      "transport": [
        { "type": "Taxi", "duration": "37 min" }
      ]
    },
    {
      "category": "Airport",
      "name": "Aeropuerto Internacional Ngurah Rai",
      "description": null,
      "rating": null,
      "total_reviews": null,
      "thumbnail_url": null,
      "maps_url": null,
      "gps": {
        "lat": -8.747499999999999,
        "lng": 115.169167
      },
      "transport": [
        { "type": "Taxi", "duration": "23 min" }
      ]
    },
    {
      "category": "Bus stop",
      "name": "Badung Jl. Segara Geni",
      "description": null,
      "rating": null,
      "total_reviews": null,
      "thumbnail_url": null,
      "maps_url": null,
      "gps": {
        "lat": -8.755488699999999,
        "lng": 115.21924299999999
      },
      "transport": [
        { "type": "Walking", "duration": "16 min" }
      ]
    },
    {
      "category": "Seafood restaurant",
      "name": "Depot Bas",
      "description": null,
      "rating": 4.6,
      "total_reviews": 307,
      "thumbnail_url": "https://lh3.googleusercontent.com/gps-cs-s/...",
      "maps_url": null,
      "gps": {
        "lat": -8.761994399999999,
        "lng": 115.2219082
      },
      "transport": [
        { "type": "Taxi", "duration": "1 min" },
        { "type": "Walking", "duration": "4 min" }
      ]
    }
  ],
  "ratings": [
    { "stars": 5, "count": 2100 },
    { "stars": 4, "count": 1200 },
    { "stars": 3, "count": 450 },
    { "stars": 2, "count": 150 },
    { "stars": 1, "count": 77 }
  ],
  "reviews_breakdown": [
    {
      "name": "Service",
      "description": "Guest satisfaction with hotel staff and service",
      "total_mentioned": 2800,
      "positive": 2100,
      "negative": 450,
      "neutral": 250
    },
    {
      "name": "Cleanliness",
      "description": "Guest satisfaction with room and property cleanliness",
      "total_mentioned": 2500,
      "positive": 2200,
      "negative": 150,
      "neutral": 150
    },
    {
      "name": "Location",
      "description": "Guest satisfaction with the property's location",
      "total_mentioned": 2100,
      "positive": 1800,
      "negative": 150,
      "neutral": 150
    }
  ],
  "from_cache": false,
  "cached_at": null
}
```

#### Vacation Rental Detail

```json
{
  "id": "ChoQnPq_qJbm2PL6ARoNL2cvMTF0eHFsenpzNhAC",
  "type": "vacation_rental",
  "name": "Caroline Guest House - Superior Double Room",
  "description": "Caroline Guest House se encuentra en Canggu, a 6 min a pie de Echo Beach, y ofrece alojamiento con salón de uso común, parking privado gratis y bar. This air-conditioned double room is consisted of a flat-screen TV with cable channels and a private bathroom.",
  "booking_url": "https://proactrip.com/book/rental/ChoQnPq_qJbm2PL6ARoNL2cvMTF0eHFsenpzNhAC",
  "address": null,
  "directions_url": null,
  "gps": {
    "lat": -8.651740074157715,
    "lng": 115.12867736816406
  },
  "hotel_class": null,
  "check_in": "14:00",
  "check_out": "12:00",
  "price_range": null,
  "price": {
    "currency": "EUR",
    "per_night": {
      "amount": 16.0,
      "before_taxes": 16.0
    },
    "total": {
      "amount": 64.0,
      "before_taxes": 64.0
    }
  },
  "rating": {
    "overall": 4.55,
    "location": 4.2
  },
  "total_reviews": 7,
  "external_reviews": null,
  "images": [
    {
      "thumbnail": "https://lh3.googleusercontent.com/proxy/ARNitNtG81XAoOLImYhTEjhGPW6JalM0Etjfb4olR5gzna18MFovKkWsclxw7AEpMZLWKS-cOkdBodm-AmIYzD7DwgJuNgwNv4KrzOXK68n0jXxGAg3_rZBKavYk4zf5O7eTdCfSAnBNe6wPcDEpwycpDEx0vA=s287-w287-h192-n-k-no-v1",
      "original": "https://q-xx.bstatic.com/xdata/images/hotel/max1024x683/199430268.jpg?k=8b917107837857240d40ab1a1e8f44c4e77c29966d25b0e89b21e72554856e42&o=&a=2344045"
    }
  ],
  "amenities": [
    "Air conditioning",
    "Airport transfer",
    "Kid-friendly",
    "Crib",
    "Elevator",
    "Indoor pool",
    "Kitchen",
    "Non-smoking",
    "Cable TV",
    "Washer",
    "Wheelchair accessible",
    "Free parking",
    "Free Wi-Fi"
  ],
  "excluded_amenities": [
    "No outdoor pool"
  ],
  "capacity": {
    "unit_type": "Entire house",
    "guests": 2,
    "bedrooms": 1,
    "bathrooms": 1,
    "beds": 1,
    "area": null
  },
  "health_and_safety": null,
  "sustainability": null,
  "eco_certified": null,
  "nearby_places": [
    {
      "category": "Bus station",
      "name": "Canggu Central Parkir",
      "description": null,
      "rating": null,
      "total_reviews": null,
      "thumbnail_url": null,
      "maps_url": null,
      "gps": {
        "lat": -8.651484199999999,
        "lng": 115.13352689999999
      },
      "transport": [
        { "type": "Walking", "duration": "9 min" }
      ]
    },
    {
      "category": "Airport",
      "name": "Aeropuerto Internacional Ngurah Rai",
      "description": null,
      "rating": null,
      "total_reviews": null,
      "thumbnail_url": null,
      "maps_url": null,
      "gps": {
        "lat": -8.747499999999999,
        "lng": 115.169167
      },
      "transport": [
        { "type": "Taxi", "duration": "51 min" }
      ]
    },
    {
      "category": "Family restaurant",
      "name": "Inklusiv Warung",
      "description": null,
      "rating": 4.8,
      "total_reviews": 2524,
      "thumbnail_url": "https://lh3.googleusercontent.com/gps-cs-s/...",
      "maps_url": null,
      "gps": {
        "lat": -8.653039999999999,
        "lng": 115.1308062
      },
      "transport": [
        { "type": "Taxi", "duration": "1 min" },
        { "type": "Walking", "duration": "5 min" }
      ]
    }
  ],
  "ratings": [
    { "stars": 5, "count": 3 },
    { "stars": 4, "count": 3 },
    { "stars": 3, "count": 1 }
  ],
  "reviews_breakdown": null,
  "from_cache": false,
  "cached_at": null
}
```

### Response Fields Explained (Hotel Details)

> **Leyenda:** **H** = exclusivo de Hotels, **VR** = exclusivo de Vacation Rentals, sin marca = común a ambos.

#### Campos Base

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `id` | string | Identificador opaco de la propiedad (property_token) |
| `type` | string | `"hotel"` o `"vacation_rental"` |
| `name` | string | Nombre del alojamiento |
| `description` | string | Descripción completa (más extensa que en búsqueda) |
| `booking_url` | string\|null | Enlace interno para proceder con la reserva |
| `address` **H** | string\|null | Dirección física completa. `null` en VR |
| `directions_url` **H** | string\|null | URL de Google Maps con ruta. `null` en VR |
| `gps.lat` | number | Latitud |
| `gps.lng` | number | Longitud |
| `hotel_class` **H** | integer\|null | Número de estrellas (2–5). `null` en VR |
| `check_in` | string\|null | Hora de entrada. Ej: `"14:00"` |
| `check_out` | string\|null | Hora de salida. Ej: `"12:00"` |

#### Precios

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `price_range` **H** | object\|null | Rango de precios típico del hotel. Sin fechas específicas |
| `price_range.currency` | string | Código ISO 4217 de la moneda |
| `price_range.min` | number | Precio mínimo típico por noche |
| `price_range.max` | number | Precio máximo típico por noche |
| `price` **VR** | object\|null | Precio específico para las fechas solicitadas |
| `price.currency` | string | Código ISO 4217 |
| `price.per_night.amount` | number | Precio por noche con impuestos |
| `price.per_night.before_taxes` | number | Precio por noche antes de impuestos |
| `price.total.amount` | number | Precio total con impuestos |
| `price.total.before_taxes` | number | Precio total antes de impuestos |

#### Ratings y Reseñas

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `rating.overall` | number\|null | Puntuación media (0–5) |
| `rating.location` | number\|null | Puntuación de ubicación (0–5) |
| `total_reviews` | integer\|null | Total de reseñas |
| `external_reviews` **H** | array\|null | Reseñas de plataformas externas. `null` en VR |
| `external_reviews[].source` | string | Nombre de la fuente |
| `external_reviews[].logo_url` | string | URL del logo |
| `external_reviews[].score` | number | Puntuación en la plataforma |
| `external_reviews[].max_score` | number | Puntuación máxima posible |
| `external_reviews[].total_reviews` | integer | Total de reseñas en esa fuente |
| `external_reviews[].featured_review` | object | Reseña destacada |
| `external_reviews[].featured_review.author` | string | Autor |
| `external_reviews[].featured_review.date` | string | Fecha ISO 8601 |
| `external_reviews[].featured_review.score` | number | Puntuación de la reseña |
| `external_reviews[].featured_review.comment` | string | Texto de la reseña |
| `external_reviews[].featured_review.url` | string\|null | URL de la reseña |
| `ratings` | array | Distribución de puntuaciones por estrellas (histograma de ratings) |
| `ratings[].stars` | integer | Cantidad de estrellas (1–5) |
| `ratings[].count` | integer | Número de reseñas con esa puntuación |
| `reviews_breakdown` | array\|null | Desglose de reseñas por categoría con análisis de sentimiento. `null` en VR sin suficientes datos |
| `reviews_breakdown[].name` | string | Nombre de la categoría. Ej: `"Service"`, `"Cleanliness"`, `"Location"` |
| `reviews_breakdown[].description` | string | Descripción de la categoría |
| `reviews_breakdown[].total_mentioned` | integer | Total de reseñas que mencionan esta categoría |
| `reviews_breakdown[].positive` | integer | Reseñas con sentimiento positivo en esta categoría |
| `reviews_breakdown[].negative` | integer | Reseñas con sentimiento negativo en esta categoría |
| `reviews_breakdown[].neutral` | integer | Reseñas con sentimiento neutral en esta categoría |

#### Imágenes y Amenities

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `images` | array | Lista de imágenes |
| `images[].thumbnail` | string | URL de miniatura (para listados) |
| `images[].original` | string | URL de imagen original (para detalle/galería) |
| `amenities` | string[] | Servicios disponibles (texto legible en el idioma de búsqueda) |
| `excluded_amenities` **VR** | string[] | Servicios que la propiedad NO tiene |

#### Capacidad (solo Vacation Rentals)

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `capacity` **VR** | object\|null | Información de capacidad |
| `capacity.unit_type` | string | Tipo de unidad. Ej: `"Entire house"`, `"Entire villa"` |
| `capacity.guests` | integer\|null | Número máximo de huéspedes |
| `capacity.bedrooms` | integer\|null | Dormitorios |
| `capacity.bathrooms` | integer\|null | Baños |
| `capacity.beds` | integer\|null | Camas |
| `capacity.area` | string\|null | Superficie. Ej: `"45 ft²"`, `"151 ft²"` |

#### Salud y Sostenibilidad (solo Hotels)

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `health_and_safety` **H** | array\|null | Medidas de higiene y seguridad. `null` en VR |
| `health_and_safety[].category` | string | Categoría. Ej: `"Enhanced cleaning"`, `"Minimized contact"` |
| `health_and_safety[].items[].name` | string | Nombre de la medida |
| `health_and_safety[].items[].available` | boolean | Si está disponible |
| `sustainability` **H** | array\|null | Prácticas sostenibles. `null` en VR |
| `sustainability[].category` | string | Categoría. Ej: `"Energy efficiency"`, `"Water use"` |
| `sustainability[].items[].name` | string | Nombre de la práctica |
| `sustainability[].items[].available` | boolean | Si está disponible |
| `eco_certified` **H** | boolean\|null | Certificación ecológica independiente. `null` en VR |

#### Lugares Cercanos

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `nearby_places` | array | Lugares cercanos (más detallado que en búsqueda) |
| `nearby_places[].category` | string | Tipo de lugar: `"Point of interest"`, `"Airport"`, `"Bus station"`, `"Restaurant"`, etc. |
| `nearby_places[].name` | string | Nombre del lugar |
| `nearby_places[].description` | string\|null | Descripción corta |
| `nearby_places[].rating` | number\|null | Puntuación de Google Maps |
| `nearby_places[].total_reviews` | integer\|null | Número de reseñas en Google Maps |
| `nearby_places[].thumbnail_url` | string\|null | Imagen en miniatura |
| `nearby_places[].maps_url` | string\|null | URL de búsqueda en Google Maps |
| `nearby_places[].gps` | object\|null | Coordenadas `lat` y `lng` |
| `nearby_places[].transport` | array | Medios de transporte |
| `nearby_places[].transport[].type` | string | Tipo: `"Taxi"`, `"Walking"`, `"Public transport"` |
| `nearby_places[].transport[].duration` | string | Duración estimada |

#### Cache

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `from_cache` | boolean | `true` si la respuesta vino de caché |
| `cached_at` | string\|null | Timestamp ISO 8601 del momento de caché |

### Diferencias Clave entre Hotel y Vacation Rental

| Aspecto | Hotel | Vacation Rental |
|---------|-------|-----------------|
| **Precios** | `price_range` (rango típico, sin fechas) | `price` (precio exacto para fechas solicitadas) |
| **Dirección** | Siempre presente (`address`, `directions_url`) | Puede ser `null` |
| **Estrellas** | `hotel_class` presente (2-5) | Siempre `null` |
| **Capacidad** | No aplica | `capacity` con detalles de habitaciones/baños/camas/área |
| **Amenities negativos** | No aplica | `excluded_amenities` con lo que NO tiene |
| **Reseñas externas** | `external_reviews` con fuentes múltiples + featured review | `null` |
| **Sostenibilidad** | `sustainability`, `eco_certified`, `health_and_safety` | `null` |
| **Brands en búsqueda** | `brands` disponible | `null` |
| **Filtros específicos** | `hotel_classes`, `brands`, `free_cancellation`, `special_offers`, `eco_certified` | `bedrooms`, `bathrooms` |

### Posibles Errores (Hotel Details)

| Código | HTTP | Cuándo |
|--------|------|--------|
| `VALIDATION_ERROR` | 400 | `id` vacío o ausente, fechas inválidas, falta `check_in_date` o `check_out_date` |
| `PROPERTY_NOT_FOUND` | 404 | La propiedad con ese `id` no fue encontrada por el proveedor |
| `INVALID_PARAM_RANGE` | 422 | Parámetros fuera de rango (`adults` < 1, `children_ages` no coincide con `children`) |
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
  "instance": "/v1/search/hotels",
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
| Proveedor externo | Siempre datos frescos (`from_cache: false`) |

- Si un usuario busca sin autenticarse, se registra, y vuelve a buscar con los mismos parámetros dentro de la ventana de caché → se reutilizan los resultados cacheados (sin nueva llamada a SerpAPI)
- `from_cache` es **siempre `false`** en todas las respuestas. La caché no se expone al frontend
- `cached_at` es **siempre `null`** en todas las respuestas

> **Motivo:** Los resultados de vuelos y hoteles cambian constantemente. 5 minutos es el máximo razonable antes de que los datos queden obsoletos. El manejo interno de caché evita llamadas redundantes al proveedor sin exponer detalles al frontend.

### Campos que Forman la Clave de Caché

La clave de caché se genera haciendo hash de los siguientes campos del request:

| Campo | Incluido en clave |
|-------|-------------------|
| `query` | Sí |
| `check_in_date` | Sí |
| `check_out_date` | Sí |
| `adults` | Sí |
| `children` | Sí |
| `children_ages` | Sí |
| `gl` | Sí |
| `hl` | Sí |
| `currency` | Sí |
| `min_price` | Sí |
| `max_price` | Sí |
| `sort_by` | Sí |
| `rating` | Sí |
| `property_types` | Sí |
| `amenities` | Sí |
| `vacation_rentals` | Sí |
| `hotel_classes` | Sí |
| `brands` | Sí |
| `free_cancellation` | Sí |
| `special_offers` | Sí |
| `eco_certified` | Sí |
| `bedrooms` | Sí |
| `bathrooms` | Sí |
| `page_token` | **No** — la paginación ocurre post-caché en el servidor |

> Dos requests con exactamente los mismos parámetros (excepto `page_token`) se benefician de la caché interna durante la ventana de TTL, evitando llamadas redundantes a SerpAPI.

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
- El `id` de las propiedades **no** es un token de autenticación. Es el `property_token` opaco de SerpAPI.

### Prevención de Ataques

| Amenaza | Mitigación |
|---------|------------|
| XSS | `HttpOnly cookies` + `Content-Security-Policy` |
| CSRF | `SameSite=Lax` + cookies automáticas (sin `Authorization` manual) |
| Token Exposure | Cookies HttpOnly — JavaScript no puede leerlas |
| Replay de refresh | Rotación continua + invalidación total ante reúso |
| Third-party cookies | No se usa Partitioned (CHIPS) — SameSite=Lax + Domain=.proactrip.com es suficiente para subdominios |
| Rate limiting abuse | Multi-tier con DragonflyDB + Lua scripts atómicos (IP, usuario autenticado, cookie anónima) |
| Cache poisoning | Clave de caché basada en hash de parámetros validados |

### Normalización del Backend (Nota para desarrolladores)

El backend transforma la respuesta cruda de SerpAPI antes de devolverla al frontend:

1. **Imágenes:** `original_image` → `original` (nombre consistente en búsqueda y detalles)
2. **GPS:** `gps_coordinates.latitude/longitude` → `gps.lat/lng`
3. **Horarios:** `check_in_time/check_out_time` → `check_in/check_out`
4. **Precios:** `rate_per_night` y `total_rate` con `extracted_lowest/extracted_before_taxes_fees` → `price.per_night` y `price.total` con `amount/before_taxes`
5. **Moneda en price_range:** Agregar `currency` desde el request (el proveedor no lo incluye)
6. **Nearby places:** `transportations` → `transport`; extraer `category`, `description`, `rating`, `total_reviews`, `thumbnail_url`, `maps_url`, `gps`
7. **External reviews:** Fechas relativas a ISO 8601; normalizar estructura de `featured_review`
8. **Health/safety y sustainability:** `groups[].title`/`list` → `category`/`items` plano
9. **Capacity (VR):** Parsear `essential_info[]` strings → objeto `capacity` con `unit_type`, `guests`, `bedrooms`, `bathrooms`, `beds`, `area`
10. **Hotel class:** `extracted_hotel_class` numérico → `hotel_class`
11. **Amenities y excluded_amenities:** Siempre como strings legibles (nunca IDs numéricos)
