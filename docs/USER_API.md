# User Module API Documentation (Cookie-Based)

> **Arquitectura:** Cookie-based authentication con HttpOnly cookies. El frontend nunca manipula tokens.
> **Perfil auto-creado:** Reactivamente vía Dragonfly Streams al recibir el evento `auth.user.registered`.

---

## Índice

- [Arquitectura](#arquitectura)
- [Seguridad de Cookies](#seguridad-de-cookies)
- [Estrategia de Refresco de Tokens](#estrategia-de-refresco-de-tokens)
- [Base URLs](#base-urls)
- [Errores Estándar](#errores-estándar)
- [Perfil](#perfil)
  - [Get Profile](#get-profile)
  - [Update Profile](#update-profile)
  - [Update Locale](#update-locale)
  - [Update Travel Preferences](#update-travel-preferences)
  - [Update Notification Preferences](#update-notification-preferences)
- [Perfil Médico](#perfil-médico)
  - [Get Medical Profile](#get-medical-profile)
  - [Update Medical Profile](#update-medical-profile)
  - [List Pending Medical Conflicts](#list-pending-medical-conflicts)
  - [Resolve Medical Conflict](#resolve-medical-conflict)
- [Avatares](#avatares)
  - [Upload Avatar (Presigned URL)](#upload-avatar-presigned-url)
  - [Confirm Avatar Upload](#confirm-avatar-upload)
- [Documentos](#documentos)
  - [List Document Types](#list-document-types)
  - [Upload Document](#upload-document)
  - [List Documents](#list-documents)
  - [Get Document](#get-document)
  - [Download Document](#download-document)
  - [Delete Document](#delete-document)
  - [Verify Document](#verify-document)
  - [Document Events (SSE)](#document-events-sse)
- [Búsquedas Guardadas](#búsquedas-guardadas)
  - [Create Saved Search](#create-saved-search)
  - [List Saved Searches](#list-saved-searches)
  - [Update Saved Search](#update-saved-search)
  - [Delete Saved Search](#delete-saved-search)
  - [Toggle Price Alert](#toggle-price-alert)
- [Favoritos](#favoritos)
  - [Add Favorite](#add-favorite)
  - [List Favorites](#list-favorites)
  - [Delete Favorite](#delete-favorite)
- [Configuración CORS](#configuración-cors)
- [Rate Limiting](#rate-limiting)
- [Cache](#cache)
- [Notas de Seguridad](#notas-de-seguridad)

---

## Arquitectura

### Creación Reactiva del Perfil

El perfil de usuario **NO se crea sincrónicamente** durante el registro. El Auth module publica un evento `UserRegistered` a Dragonfly Streams y el User module lo consume asincrónicamente para crear el perfil.

```
┌──────────────┐  UserRegistered  ┌─────────────────────────┐
│ Auth Module  │ ───────────────> │ Dragonfly Streams       │
│ (register/   │                  │ {events}:auth.user.     │
│  oauth)      │                  │ registered              │
└──────────────┘                  └─────────────────────────┘
                                             │
                                    ┌────────┴────────┐
                                    ▼                 ▼
                            ┌──────────────┐  ┌──────────────┐
                            │ User Module  │  │ Notification │
                            │ Consumer     │  │ Consumer     │
                            │ (crea        │  │ (envía       │
                            │  perfil)     │  │  email)      │
                            └──────────────┘  └──────────────┘
```

El perfil se inicializa con datos cacheados de `/v1/environment`:
- `timezone` = `location.timezone`
- `language` = `location.language`
- `currency` = `location.currency`

Campos opcionales (`first_name`, `last_name`, `phone`, `bio`, etc.) se crean en `null` — el usuario los completa después desde la UI.

### Pipeline de Documentos

Procesamiento asíncrono con workers conectados vía Dragonfly Streams:

```
┌──────────────┐
│   Frontend   │  POST /v1/user/documents (multipart/form-data)
└──────┬───────┘
       │
       ▼
┌──────────────────────────────────────────────────────────────────┐
│  Backend: Magic bytes check (primeros 512 bytes) — sincrónico    │
│  Si pasa → publica en Dragonfly Streams → 202 Accepted           │
└──────────────────────────────────────────────────────────────────┘
       │
       ▼
┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐
│ ValidatorWorker  │────>│ SanitizerWorker  │────>│   OCRWorker      │
│ (magic bytes,    │     │ (strip EXIF,     │     │ (GroqCloud,      │
│  MIME, size)     │     │  clean PDFs)     │     │  extraer datos,  │
│ stream:doc:      │     │ stream:doc:      │     │  detectar        │
│ validate         │     │ sanitize         │     │  conflictos)     │
└──────────────────┘     └──────────────────┘     │ stream:doc:ocr   │
                                                   └────────┬─────────┘
                                                            │
                                            ┌───────────────┴───────────────┐
                                            ▼                               ▼
                                    ┌──────────────┐                ┌──────────────┐
                                    │ PostgreSQL   │                │ Redis Pub/Sub│
                                    │ (metadata,   │                │ doc:events:  │
                                    │  extracted_  │                │ {doc_id}     │
                                    │  data)       │                │ (SSE)        │
                                    └──────────────┘                └──────────────┘
```

### Resolución de Conflictos Médicos

Cuando OCR detecta datos médicos que entran en conflicto con el perfil existente, se notifica al usuario en tiempo real vía SSE y este resuelve mediante un endpoint estructurado (NO mediante chat AI).

```
┌──────────────┐                              ┌──────────────┐
│  OCRWorker   │  Detecta conflicto           │  Frontend    │
│              │ ──────────────────────────>  │  (SSE:       │
└──────────────┘  user:events:{user_id}       │   user:events│
                                               │   :{user_id})│
                                               └──────┬───────┘
                                                      │
                                      El usuario revisa y decide:
                                      POST /v1/user/profile/medical/pending/resolve
                                      { action: "accept" | "reject" | "custom" }
```

### Avatar

Un único avatar asignado al registro. Usuarios OAuth Google usan el avatar de su perfil de Google. El avatar personalizado se sube vía presigned URL de R2 y se activa asincrónicamente después de validación por worker.

```
┌──────────────┐  POST /avatar         ┌──────────────┐
│   Frontend   │ ────────────────────> │   Backend    │
└──────────────┘                       └──────────────┘
       │                                       │
       │  { upload_url, storage_key }          │
       │<──────────────────────────────────────│
       │                                       │
       │  PUT upload_url (directo a R2)        │
       │──────────────────────────────────────>│  ┌──────┐
       │                                       │  │  R2  │
       │                                       │  └──────┘
       │                                       │
       │  POST /avatar/confirm                 │
       │──────────────────────────────────────>│  ┌──────────────┐
       │                                       │──>│Worker valida │
       │  202 { status: "validating" }         │  │y activa      │
       │<──────────────────────────────────────│  └──────────────┘
```

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

---

## Estrategia de Refresco de Tokens

El backend maneja el refresco de tokens transparentemente vía middleware.

- Si `access_token` es válido → la petición continúa
- Si `access_token` está expirado pero `refresh_token` es válido → nuevos tokens emitidos
- Si ambos están expirados → 401 Unauthorized

El frontend nunca llama manualmente a `/refresh-token`.

---

## Base URLs

| Entorno | Base URL |
|---------|----------|
| **Production** | `https://api.proactrip.com/v1/user` |
| **Development** | `http://localhost:8080/v1/user` |

Todos los ejemplos usan `{base_url}` como placeholder.

---

## Errores Estándar

Formato **RFC 9457 Problem Details**. Todas las respuestas de error usan `Content-Type: application/problem+json`.

```json
{
  "type": "https://api.proactrip.com/errors/profile-not-found",
  "title": "Profile Not Found",
  "status": 404,
  "detail": "No se encontró el perfil para el usuario autenticado.",
  "instance": "/v1/user/profile",
  "trace_id": "019d5439-cb43-716d-90b5-51dcbe980908"
}
```

**Headers de respuesta en TODOS los endpoints:**

| Header        | Descripción                                             |
| ------------- | ------------------------------------------------------- |
| `X-Trace-Id`  | UUID v7 para trazabilidad. Asignado globalmente por middleware, nunca por handlers individuales |
| `traceparent` | W3C Trace Context                                       |

---

## Perfil

### Get Profile

Retorna el perfil completo del usuario autenticado incluyendo preferencias de viaje y notificaciones. La sección de ubicación/timezone/moneda/idioma sigue el mismo formato que `GET /v1/environment`.

> Las cookies `__Secure-access_token` y `__Secure-refresh_token` se envían automáticamente. No requiere header `Authorization`.

#### Request

```
GET /v1/user/profile
```

> El navegador envía las cookies automáticamente. No requiere body ni headers adicionales.

**Ejemplo:**

```bash
curl -X GET {base_url}/profile \
  -H "Accept: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
{
  "id": "019d5439-cb43-716d-90b5-51dcbe980908",
  "user_id": "019d5439-cb43-716d-90b5-51dcbe980909",
  "first_name": "Aurelio",
  "last_name": "García",
  "email": "aurelio@example.com",
  "avatar_url": "https://r2.proactrip.com/avatars/default.webp",
  "date_of_birth": "1990-05-15",
  "gender": "male",
  "nationality": "AR",
  "phone": "+5491123456789",
  "bio": "Viajero frecuente",
  "is_public": true,
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
  "travel_preferences": {
    "preferred_class": "economy",
    "seat_preference": "aisle",
    "meal_preference": "vegetarian",
    "special_assistance": ["wheelchair"],
    "preferred_airlines": ["019d5439-cb43-716d-90b5-51dcbe980001"],
    "preferred_hotels": ["Marriott", "Hilton"],
    "avoid_layovers": true,
    "max_layover_duration": 120
  },
  "notification_preferences": {
    "booking_confirmation": { "email": true, "sms": false, "websocket": true },
    "flight_reminder": { "email": true, "sms": false, "websocket": true },
    "promotional": { "email": false, "sms": false, "websocket": false }
  }
}
```

**Campos de respuesta:**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `id` | string (UUID v7) | ID del perfil (diferente de `user_id`) |
| `user_id` | string (UUID v7) | ID del usuario en la tabla `users` |
| `first_name` | string\|null | Nombre del usuario |
| `last_name` | string\|null | Apellido del usuario |
| `email` | string | Email del usuario |
| `avatar_url` | string | URL del avatar actual |
| `date_of_birth` | string\|null | Fecha de nacimiento (YYYY-MM-DD) |
| `gender` | string\|null | `male`, `female`, `non_binary`, `prefer_not_to_say` |
| `nationality` | string\|null | Código ISO 3166-1 alpha-2 (ej: `AR`) |
| `phone` | string\|null | Número en formato E.164 |
| `bio` | string\|null | Biografía del usuario |
| `is_public` | boolean | Perfil visible públicamente |
| `location` | object | Datos de ubicación (mismo formato que `/v1/environment`) |
| `location.country` | string | Nombre completo del país |
| `location.country_code` | string | Código ISO 3166-1 alpha-2 |
| `location.city` | string | Ciudad |
| `location.state` | string | Estado/provincia |
| `location.zipcode` | string | Código postal |
| `location.timezone` | string | Timezone IANA |
| `location.currency` | string | Código ISO 4217 |
| `location.language` | string | Código ISO 639-1 |
| `location.latitude` | float64 | Latitud geográfica |
| `location.longitude` | float64 | Longitud geográfica |
| `travel_preferences` | object | Preferencias de viaje |
| `travel_preferences.preferred_class` | string\|null | `economy`, `premium_economy`, `business`, `first` |
| `travel_preferences.seat_preference` | string\|null | `window`, `aisle`, `middle`, `no_preference` |
| `travel_preferences.meal_preference` | string\|null | Preferencia de comida |
| `travel_preferences.special_assistance` | string[] | Asistencias especiales requeridas |
| `travel_preferences.preferred_airlines` | UUID[] | Aerolíneas preferidas |
| `travel_preferences.preferred_hotels` | string[] | Cadenas hoteleras preferidas |
| `travel_preferences.avoid_layovers` | boolean | Evitar escalas |
| `travel_preferences.max_layover_duration` | int | Duración máxima de escala en minutos |
| `notification_preferences` | object | Preferencias de notificación por tipo |
| `notification_preferences.{type}` | object | Canales activos: `email`, `sms`, `websocket` |

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `PROFILE_NOT_FOUND` | 404 | No existe perfil para el usuario autenticado |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Update Profile

Actualización parcial de información personal. Todos los campos del body son opcionales.

> **Nota:** `current_location` **NO** se actualiza aquí — usar `PUT /v1/user/profile/locale`.

#### Request

```
PUT /v1/user/profile
```

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `first_name` | string | No | — | Nombre del usuario |
| `last_name` | string | No | — | Apellido del usuario |
| `date_of_birth` | string | No | ISO 8601 (YYYY-MM-DD) | Fecha de nacimiento |
| `gender` | string | No | `male`, `female`, `non_binary`, `prefer_not_to_say` | Género |
| `nationality` | string | No | ISO 3166-1 alpha-2 (2 letras) | Nacionalidad |
| `phone` | string | No | E.164 (ej: `+5491123456789`) | Teléfono |
| `bio` | string | No | — | Biografía |
| `is_public` | boolean | No | — | Visibilidad pública del perfil |

**Ejemplo:**

```bash
curl -X PUT {base_url}/profile \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "first_name": "Aurelio",
    "last_name": "García",
    "gender": "male",
    "nationality": "AR",
    "phone": "+5491123456789"
  }'
```

#### Responses

##### 200 OK

```json
{
  "message": "Profile updated successfully."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `PROFILE_NOT_FOUND` | 404 | No existe perfil para el usuario autenticado |
| `INVALID_ENUM` | 400 | Valor de `gender` no válido |
| `INVALID_COUNTRY_CODE` | 400 | `nationality` no es un código ISO 3166-1 alpha-2 válido |
| `VALIDATION_ERROR` | 400 | Body malformado o campo con formato inválido |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Update Locale

Actualiza configuración regional incluyendo `current_location`. Todos los campos son opcionales.

#### Request

```
PUT /v1/user/profile/locale
```

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `timezone_name` | string | No | IANA timezone (ej: `America/Argentina/Buenos_Aires`) | Zona horaria |
| `language_code` | string | No | ISO 639 (2-5 caracteres) | Idioma preferido |
| `currency_code` | string | No | ISO 4217 (3 caracteres) | Moneda preferida |
| `current_location` | string | No | — | Nombre de la ciudad actual |

**Ejemplo:**

```bash
curl -X PUT {base_url}/profile/locale \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "timezone_name": "Europe/Madrid",
    "language_code": "es",
    "currency_code": "EUR",
    "current_location": "Madrid"
  }'
```

#### Responses

##### 200 OK

```json
{
  "message": "Locale updated successfully."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `PROFILE_NOT_FOUND` | 404 | No existe perfil para el usuario autenticado |
| `INVALID_TIMEZONE` | 400 | `timezone_name` no es un timezone IANA válido |
| `INVALID_LANGUAGE_CODE` | 400 | `language_code` no es un código ISO 639 válido |
| `INVALID_CURRENCY_CODE` | 400 | `currency_code` no es un código ISO 4217 válido |
| `VALIDATION_ERROR` | 400 | Body malformado |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Update Travel Preferences

Actualización parcial de preferencias de viaje. Todos los campos son opcionales.

#### Request

```
PUT /v1/user/profile/travel-preferences
```

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `preferred_class` | string | No | `economy`, `premium_economy`, `business`, `first` | Clase preferida |
| `seat_preference` | string | No | `window`, `aisle`, `middle`, `no_preference` | Preferencia de asiento |
| `meal_preference` | string | No | — | Preferencia de comida |
| `special_assistance` | string[] | No | — | Asistencias especiales requeridas |
| `preferred_airlines` | UUID[] | No | — | IDs de aerolíneas preferidas |
| `preferred_hotels` | string[] | No | — | Cadenas hoteleras preferidas |
| `avoid_layovers` | boolean | No | — | Evitar escalas |
| `max_layover_duration` | int | No | ≥ 0 | Duración máxima de escala en minutos |

**Ejemplo:**

```bash
curl -X PUT {base_url}/profile/travel-preferences \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "preferred_class": "business",
    "seat_preference": "window",
    "meal_preference": "vegetarian",
    "avoid_layovers": true,
    "max_layover_duration": 90
  }'
```

#### Responses

##### 200 OK

```json
{
  "message": "Travel preferences updated successfully."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `PROFILE_NOT_FOUND` | 404 | No existe perfil para el usuario autenticado |
| `INVALID_ENUM` | 400 | Valor de `preferred_class` o `seat_preference` no válido |
| `VALIDATION_ERROR` | 400 | Body malformado o `max_layover_duration` negativo |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Update Notification Preferences

Upsert de una preferencia de notificación individual. Solo se requiere especificar un canal a la vez.

> ⚠️ **SMS:** El canal `sms` está planificado pero **NO implementado**. Solo `email` y `websocket` son funcionales. Las preferencias se guardan pero SMS no enviará hasta que se implemente.

#### Request

```
PUT /v1/user/profile/notifications
```

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `channel` | string | Sí | `email`, `sms`, `websocket` | Canal de notificación |
| `notification_type` | string | Sí | — | Tipo de notificación (ej: `booking_confirmation`, `flight_reminder`, `promotional`) |
| `enabled` | boolean | Sí | — | Activar o desactivar |

**Ejemplo:**

```bash
curl -X PUT {base_url}/profile/notifications \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "channel": "email",
    "notification_type": "booking_confirmation",
    "enabled": true
  }'
```

#### Responses

##### 200 OK

```json
{
  "message": "Notification preference updated successfully."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `INVALID_ENUM` | 400 | Valor de `channel` no válido |
| `VALIDATION_ERROR` | 400 | Body malformado o campos requeridos faltantes |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## Perfil Médico

### Get Medical Profile

Retorna el perfil médico del usuario con trazabilidad de procedencia por campo (quién ingresó cada dato y cuándo). Los datos médicos se almacenan encriptados con ChaCha20-Poly1305 en reposo y se devuelven como texto plano vía API.

#### Request

```
GET /v1/user/profile/medical
```

> El navegador envía las cookies automáticamente. No requiere body ni headers adicionales.

**Ejemplo:**

```bash
curl -X GET {base_url}/profile/medical \
  -H "Accept: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
{
  "blood_type": {
    "value": "A+",
    "source": "manual",
    "updated_at": "2026-03-15T10:30:00Z"
  },
  "allergies": {
    "value": "Penicilina, Polen",
    "source": "ocr:019d5439-cb43-716d-90b5-51dcbe980908",
    "updated_at": "2026-04-01T14:22:00Z"
  },
  "medications": {
    "value": "Loratadina 10mg",
    "source": "manual",
    "updated_at": "2026-01-10T08:15:00Z"
  },
  "conditions": {
    "value": "Asma leve",
    "source": "manual",
    "updated_at": "2025-11-20T16:45:00Z"
  },
  "vaccinations": {
    "value": "COVID-19 (3 dosis), Fiebre amarilla",
    "source": "ocr:019d5439-cb43-716d-90b5-51dcbe980909",
    "updated_at": "2026-02-28T09:00:00Z"
  },
  "emergency_contact": {
    "value": "María García, +5491123456790",
    "source": "manual",
    "updated_at": "2026-03-01T12:00:00Z"
  },
  "insurance_info": {
    "value": "ASSA Compañía de Seguros, Póliza #12345",
    "source": "manual",
    "updated_at": "2026-03-01T12:05:00Z"
  },
  "is_shared": true,
  "has_pending_conflicts": true,
  "pending_conflict_count": 2
}
```

**Campos de respuesta:**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `blood_type` | object | Grupo sanguíneo con trazabilidad |
| `blood_type.value` | string\|null | `A+`, `A-`, `B+`, `B-`, `AB+`, `AB-`, `O+`, `O-` |
| `blood_type.source` | string | `manual`, `ocr:{document_id}`, `nlp:{conversation_id}` |
| `blood_type.updated_at` | string (ISO 8601) | Última actualización |
| `allergies` | object | Alergias con trazabilidad |
| `medications` | object | Medicamentos con trazabilidad |
| `conditions` | object | Condiciones médicas con trazabilidad |
| `vaccinations` | object | Vacunas con trazabilidad |
| `emergency_contact` | object | Contacto de emergencia con trazabilidad |
| `insurance_info` | object | Información de seguro con trazabilidad |
| `is_shared` | boolean | Perfil médico compartido (para emergencias) |
| `has_pending_conflicts` | boolean | Hay conflictos médicos sin resolver |
| `pending_conflict_count` | int | Cantidad de conflictos pendientes |

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `MEDICAL_PROFILE_NOT_FOUND` | 404 | No existe perfil médico para el usuario |
| `DECRYPTION_ERROR` | 500 | Error al desencriptar datos médicos |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Update Medical Profile

Actualiza campos médicos manualmente (source="manual"). Todos los campos son opcionales.

#### Request

```
PUT /v1/user/profile/medical
```

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `blood_type` | string | No | `A+`, `A-`, `B+`, `B-`, `AB+`, `AB-`, `O+`, `O-` | Grupo sanguíneo |
| `allergies` | string | No | — | Alergias |
| `medications` | string | No | — | Medicamentos |
| `conditions` | string | No | — | Condiciones médicas |
| `vaccinations` | string | No | — | Vacunas |
| `emergency_contact` | string | No | — | Contacto de emergencia |
| `insurance_info` | string | No | — | Información de seguro |
| `is_shared` | boolean | No | — | Compartir perfil médico |

**Ejemplo:**

```bash
curl -X PUT {base_url}/profile/medical \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "blood_type": "O+",
    "allergies": "Ibuprofeno",
    "is_shared": true
  }'
```

#### Responses

##### 200 OK

```json
{
  "message": "Medical profile updated successfully.",
  "applied_fields": ["blood_type", "allergies", "is_shared"]
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `MEDICAL_PROFILE_NOT_FOUND` | 404 | No existe perfil médico para el usuario |
| `INVALID_BLOOD_TYPE` | 400 | `blood_type` no es un valor válido |
| `ENCRYPTION_ERROR` | 500 | Error al encriptar datos médicos |
| `VALIDATION_ERROR` | 400 | Body malformado |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### List Pending Medical Conflicts

Lista todos los conflictos médicos pendientes detectados por OCR (y futuros por NLP). Los conflictos expiran a los 30 días.

#### Request

```
GET /v1/user/profile/medical/pending
```

> El navegador envía las cookies automáticamente. No requiere body ni headers adicionales.

**Ejemplo:**

```bash
curl -X GET {base_url}/profile/medical/pending \
  -H "Accept: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
{
  "conflicts": [
    {
      "id": "019d5439-cb43-716d-90b5-51dcbe980908",
      "field": "blood_type",
      "current_value": "A+",
      "proposed_value": "O+",
      "source": {
        "type": "ocr",
        "document_id": "019d5439-cb43-716d-90b5-51dcbe980800",
        "file_name": "carnet_vacunacion.pdf"
      },
      "suggested_at": "2026-04-15T10:30:00Z",
      "expires_at": "2026-05-15T10:30:00Z"
    },
    {
      "id": "019d5439-cb43-716d-90b5-51dcbe980909",
      "field": "allergies",
      "current_value": "Penicilina",
      "proposed_value": "Penicilina, Sulfa",
      "source": {
        "type": "ocr",
        "document_id": "019d5439-cb43-716d-90b5-51dcbe980801",
        "file_name": "receta_medica.pdf"
      },
      "suggested_at": "2026-04-16T14:22:00Z",
      "expires_at": "2026-05-16T14:22:00Z"
    }
  ]
}
```

**Campos de respuesta:**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `conflicts` | array | Lista de conflictos pendientes |
| `conflicts[].id` | string (UUID v7) | ID del conflicto |
| `conflicts[].field` | string | Campo médico en conflicto |
| `conflicts[].current_value` | string | Valor actual en el perfil |
| `conflicts[].proposed_value` | string | Valor propuesto por OCR/NLP |
| `conflicts[].source.type` | string | `ocr` o `nlp` |
| `conflicts[].source.document_id` | string (UUID v7) | ID del documento origen |
| `conflicts[].source.file_name` | string | Nombre del archivo origen |
| `conflicts[].suggested_at` | string (ISO 8601) | Fecha de detección |
| `conflicts[].expires_at` | string (ISO 8601) | Fecha de expiración (30 días) |

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Resolve Medical Conflict

Resuelve un conflicto médico pendiente. El usuario revisa los datos sugeridos por OCR y decide si aceptarlos, rechazarlos o ingresar un valor personalizado.

> Se envía notificación SSE en tiempo real cuando se crean conflictos, vía Redis pub/sub en el canal `user:events:{user_id}`.

#### Request

```
POST /v1/user/profile/medical/pending/resolve
```

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `pending_update_id` | string (UUID v7) | Sí | — | ID del conflicto a resolver |
| `action` | string | Sí | `accept`, `reject`, `custom` | Acción a tomar |
| `custom_value` | string | Solo si `action=custom` | — | Valor personalizado |

**Ejemplo (accept):**

```bash
curl -X POST {base_url}/profile/medical/pending/resolve \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "pending_update_id": "019d5439-cb43-716d-90b5-51dcbe980908",
    "action": "accept"
  }'
```

**Ejemplo (custom):**

```bash
curl -X POST {base_url}/profile/medical/pending/resolve \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "pending_update_id": "019d5439-cb43-716d-90b5-51dcbe980908",
    "action": "custom",
    "custom_value": "A-"
  }'
```

#### Responses

##### 200 OK

```json
{
  "message": "Medical profile updated successfully."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `PENDING_UPDATE_NOT_FOUND` | 404 | No existe el conflicto con ese ID |
| `PENDING_UPDATE_EXPIRED` | 400 | El conflicto expiró (más de 30 días) |
| `INVALID_PENDING_ACTION` | 400 | `action` no es `accept`, `reject` o `custom`; o falta `custom_value` cuando `action=custom` |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## Avatares

### Upload Avatar (Presigned URL)

Genera una URL prefirmada de R2 para que el frontend suba el archivo binario directamente. El backend **nunca** recibe el contenido del archivo.

#### Request

```
POST /v1/user/profile/avatar
```

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `file_name` | string | Sí | — | Nombre del archivo |
| `mime_type` | string | Sí | `image/jpeg`, `image/png`, `image/webp` | Tipo MIME |
| `file_size` | int | No | ≤ 5242880 (5 MB) | Tamaño en bytes |
| `ttl_minutes` | int | No | — | TTL de la URL prefirmada (default: 15) |

**Ejemplo:**

```bash
curl -X POST {base_url}/profile/avatar \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "file_name": "avatar.webp",
    "mime_type": "image/webp",
    "file_size": 204800,
    "ttl_minutes": 10
  }'
```

#### Responses

##### 201 Created

```json
{
  "upload_url": "https://r2.proactrip.com/avatars/019d5439-cb43-716d-90b5-51dcbe980908?X-Amz-Algorithm=...",
  "storage_key": "avatars/019d5439-cb43-716d-90b5-51dcbe980908",
  "expires_at": "2026-05-06T15:45:00Z",
  "message": "PUT the file binary to upload_url, then call /avatar/confirm."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `INVALID_MIME_TYPE` | 400 | `mime_type` no es `image/jpeg`, `image/png` o `image/webp` |
| `FILE_TOO_LARGE` | 400 | `file_size` > 5242880 bytes |
| `VALIDATION_ERROR` | 400 | Body malformado o campos requeridos faltantes |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Confirm Avatar Upload

Verifica que el avatar subido existe en R2 y dispara la validación asíncrona. El avatar **NO** se activa inmediatamente — un worker valida (MIME, tamaño, contenido) y lo activa al finalizar con éxito.

#### Request

```
POST /v1/user/profile/avatar/confirm
```

**Body:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `storage_key` | string | Sí | Clave obtenida en la respuesta de upload |

**Ejemplo:**

```bash
curl -X POST {base_url}/profile/avatar/confirm \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "storage_key": "avatars/019d5439-cb43-716d-90b5-51dcbe980908"
  }'
```

#### Responses

##### 202 Accepted

```json
{
  "status": "validating",
  "message": "Avatar upload confirmed. Validation in progress."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `FILE_NOT_FOUND` | 404 | `storage_key` no existe en R2 |
| `VALIDATION_ERROR` | 400 | Body malformado o falta `storage_key` |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## Documentos

### List Document Types

Retorna el catálogo estático de tipos de documentos soportados. Endpoint público sin autenticación.

#### Request

```
GET /v1/user/documents/types
```

**Ejemplo:**

```bash
curl -X GET {base_url}/documents/types \
  -H "Accept: application/json"
```

#### Responses

##### 200 OK

```json
{
  "document_types": [
    { "code": "passport", "name": "Pasaporte", "description": "Documento de identidad internacional", "is_identity": true, "requires_ocr": true },
    { "code": "national_id", "name": "DNI / Identificación Nacional", "description": "Documento nacional de identidad", "is_identity": true, "requires_ocr": true },
    { "code": "drivers_license", "name": "Licencia de Conducir", "description": "Licencia de conducción válida", "is_identity": true, "requires_ocr": true },
    { "code": "visa", "name": "Visa", "description": "Visa de viaje o residencia", "is_identity": false, "requires_ocr": true },
    { "code": "travel_insurance", "name": "Seguro de Viaje", "description": "Póliza de seguro de viaje", "is_identity": false, "requires_ocr": true },
    { "code": "vaccination_cert", "name": "Certificado de Vacunación", "description": "Certificado de vacunación internacional", "is_identity": false, "requires_ocr": true },
    { "code": "boarding_pass", "name": "Tarjeta de Embarque", "description": "Tarjeta de embarque de vuelo", "is_identity": false, "requires_ocr": false },
    { "code": "receipt", "name": "Recibo / Factura", "description": "Comprobante de pago o factura", "is_identity": false, "requires_ocr": false }
  ]
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `public, max-age=3600` |

No requiere autenticación.

---

### Upload Document

**EL ÚNICO ENDPOINT** que el frontend llama para iniciar el pipeline de documentos. Envía el archivo como `multipart/form-data`. Máximo 20 MB.

El backend ejecuta una verificación de magic bytes en los primeros 512 bytes (rechazo sincrónico si no es un tipo soportado). Si pasa, publica en Dragonfly Streams y responde 202. El pipeline restante es completamente asíncrono.

#### OCR Lifecycle

```
uploaded → processing (validating → sanitizing → ocr_processing) → completed
                                                                  → rejected
                                                                  → failed
```

| Estado | Significado |
|--------|-------------|
| `uploaded` | Archivo recibido, magic bytes OK, publicado en pipeline |
| `processing` | En pipeline — sub-estados: `validating`, `sanitizing`, `ocr_processing` |
| `completed` | Pipeline completo, datos extraídos exitosamente |
| `rejected` | No es un documento reconocido (ej: foto de un gato) |
| `failed` | Error técnico durante el procesamiento |

#### Request

```
POST /v1/user/documents
```

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Content-Type` | string | Sí | `multipart/form-data` |

**Form fields:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `file` | binary | Sí | Archivo a procesar (máx 20 MB) |
| `file_name` | string | No | Nombre del archivo (si no se envía, se extrae del header) |

**Ejemplo:**

```bash
curl -X POST {base_url}/documents \
  -H "Content-Type: multipart/form-data" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -F "file=@pasaporte.pdf" \
  -F "file_name=pasaporte.pdf"
```

#### Responses

##### 202 Accepted

```json
{
  "document_id": "019d5439-cb43-716d-90b5-51dcbe980908",
  "status": "uploaded",
  "events_url": "/v1/user/documents/019d5439-cb43-716d-90b5-51dcbe980908/events",
  "message": "Document received. Processing has started. Track progress via events_url."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `INVALID_FILE_TYPE` | 400 | Magic bytes no corresponden a un tipo soportado |
| `FILE_TOO_LARGE` | 400 | Archivo > 20 MB |
| `VALIDATION_ERROR` | 400 | Falta el campo `file` o está malformado |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### List Documents

Lista los documentos del usuario con filtros opcionales.

#### Request

```
GET /v1/user/documents
```

**Query Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `status` | string | No | Filtrar por estado OCR: `uploaded`, `processing`, `completed`, `rejected`, `failed` |
| `document_type` | string | No | Filtrar por tipo (code): `passport`, `national_id`, `drivers_license`, `visa`, etc. |

**Ejemplo:**

```bash
curl -X GET "{base_url}/documents?status=completed&document_type=passport" \
  -H "Accept: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
{
  "documents": [
    {
      "id": "019d5439-cb43-716d-90b5-51dcbe980908",
      "file_name": "pasaporte.pdf",
      "document_type": "passport",
      "ocr_status": "completed",
      "ocr_confidence": 0.97,
      "is_verified": true,
      "created_at": "2026-05-01T10:30:00Z"
    },
    {
      "id": "019d5439-cb43-716d-90b5-51dcbe980909",
      "file_name": "seguro_viaje.pdf",
      "document_type": "travel_insurance",
      "ocr_status": "processing",
      "ocr_confidence": null,
      "is_verified": false,
      "created_at": "2026-05-02T14:22:00Z"
    }
  ]
}
```

**Campos de respuesta:**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `documents` | array | Lista de documentos |
| `documents[].id` | string (UUID v7) | ID del documento |
| `documents[].file_name` | string | Nombre del archivo |
| `documents[].document_type` | string\|null | Tipo detectado (`passport`, `visa`, etc.) |
| `documents[].ocr_status` | string | Estado del pipeline OCR |
| `documents[].ocr_confidence` | float\|null | Confianza del OCR (0.0-1.0) |
| `documents[].is_verified` | boolean | Verificado por admin |
| `documents[].created_at` | string (ISO 8601) | Fecha de creación |

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `INVALID_ENUM` | 400 | `status` o `document_type` no válidos |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Get Document

Retorna los metadatos completos y datos extraídos de un documento específico. **NO incluye actualizaciones al perfil médico** — solo datos directamente relacionados con este documento.

#### Request

```
GET /v1/user/documents/:document_id
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `document_id` | string (UUID v7) | Sí | ID del documento |

**Ejemplo:**

```bash
curl -X GET {base_url}/documents/019d5439-cb43-716d-90b5-51dcbe980908 \
  -H "Accept: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
{
  "id": "019d5439-cb43-716d-90b5-51dcbe980908",
  "user_id": "019d5439-cb43-716d-90b5-51dcbe980909",
  "file_name": "pasaporte.pdf",
  "file_size": 204800,
  "mime_type": "application/pdf",
  "detected_mime_type": "application/pdf",
  "detected_size_bytes": 204800,
  "document_type": "passport",
  "storage_key": "documents/019d5439-cb43-716d-90b5-51dcbe980908/raw",
  "ocr_status": "completed",
  "ocr_confidence": 0.97,
  "extracted_data": {
    "first_name": "Aurelio",
    "last_name": "García",
    "document_number": "A12345678",
    "issuing_country": "AR",
    "nationality": "AR",
    "date_of_birth": "1990-05-15",
    "gender": "M",
    "valid_from": "2020-01-15",
    "valid_until": "2030-01-14"
  },
  "failure_reason": null,
  "is_verified": true,
  "verified_at": "2026-05-01T10:35:00Z",
  "verified_by": "019d5439-cb43-716d-90b5-51dcbe980010",
  "valid_from": "2020-01-15",
  "valid_until": "2030-01-14",
  "document_number": "A12345678",
  "issuing_country": "AR",
  "created_at": "2026-05-01T10:30:00Z",
  "updated_at": "2026-05-01T10:35:00Z"
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `DOCUMENT_NOT_FOUND` | 404 | No existe el documento o no pertenece al usuario |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Download Document

Redirección interna transparente. El frontend llama a este endpoint y recibe el archivo directamente (streaming). El backend genera la URL prefirmada de R2 internamente y streamea el archivo. El frontend **NO** hace una segunda llamada a R2.

Solo disponible cuando `ocr_status` es `completed` o `rejected`.

#### Request

```
GET /v1/user/documents/:document_id/download
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `document_id` | string (UUID v7) | Sí | ID del documento |

**Ejemplo:**

```bash
curl -X GET {base_url}/documents/019d5439-cb43-716d-90b5-51dcbe980908/download \
  -b "__Secure-access_token=v4.local.eyJ..." \
  --output pasaporte.pdf
```

#### Responses

##### 200 OK

El archivo binario se devuelve con:

```
Content-Type: application/pdf
Content-Disposition: attachment; filename="pasaporte.pdf"
Cache-Control: private, max-age=300
```

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `DOCUMENT_NOT_FOUND` | 404 | No existe el documento o no pertenece al usuario |
| `DOCUMENT_NOT_READY` | 400 | `ocr_status` no es `completed` ni `rejected` |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Delete Document

Elimina el registro del documento y **todos** los archivos asociados en R2 (raw, processed, results).

#### Request

```
DELETE /v1/user/documents/:document_id
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `document_id` | string (UUID v7) | Sí | ID del documento |

**Ejemplo:**

```bash
curl -X DELETE {base_url}/documents/019d5439-cb43-716d-90b5-51dcbe980908 \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
{
  "message": "Document and all associated files deleted successfully."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `DOCUMENT_NOT_FOUND` | 404 | No existe el documento o no pertenece al usuario |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Verify Document

**ADMIN ONLY.** Verificación manual de autenticidad de documentos. Requiere rol `admin`.

Lógica de verificación:
- **Pasaporte con MRZ válido** → auto-verificado (`is_verified=true`) durante OCR; admin puede sobrescribir
- **DNI / documentos médicos / otros** → requieren verificación manual (`is_verified=false` por defecto)
- Admin puede establecer `is_verified=true` para cualquier documento
- Si admin setea `is_verified=true` en un pasaporte que no fue auto-verificado → dispara reprocesamiento OCR

#### Request

```
PUT /v1/user/documents/:document_id/verify
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `document_id` | string (UUID v7) | Sí | ID del documento |

**Body:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `verified_by` | string (UUID v7) | Sí | ID del admin que verifica |
| `is_verified` | boolean | Sí | `true` para verificar, `false` para marcar como no verificado/falsificado |

**Ejemplo:**

```bash
curl -X PUT {base_url}/documents/019d5439-cb43-716d-90b5-51dcbe980908/verify \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "verified_by": "019d5439-cb43-716d-90b5-51dcbe980010",
    "is_verified": true
  }'
```

#### Responses

##### 200 OK

```json
{
  "message": "Document verification updated.",
  "is_verified": true
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `DOCUMENT_NOT_FOUND` | 404 | No existe el documento |
| `VALIDATION_ERROR` | 400 | Body malformado o campos requeridos faltantes |
| `PERMISSION_DENIED` | 403 | Usuario no tiene rol `admin` |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Document Events (SSE)

Server-Sent Events para seguimiento en tiempo real del procesamiento de documentos. Conexión HTTP persistente.

> **Late-connection:** Si el frontend se conecta después de que el procesamiento comenzó, se emite un evento sintético con el estado actual desde `doc:status:{id}` en Redis (TTL 1h).

#### Request

```
GET /v1/user/documents/:document_id/events
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `document_id` | string (UUID v7) | Sí | ID del documento |

**Headers:**

| Header | Tipo | Requerido | Descripción |
|--------|------|-----------|-------------|
| `Accept` | string | Sí | `text/event-stream` |

**Ejemplo:**

```bash
curl -X GET {base_url}/documents/019d5439-cb43-716d-90b5-51dcbe980908/events \
  -H "Accept: text/event-stream" \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

Response Headers permanentes:

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

**Eventos SSE:**

##### `processing`

Emitido cuando el documento avanza por el pipeline. Incluye sub-estado.

```
event: processing
data: {"status":"processing","sub_state":"ocr_processing","message":"Extrayendo datos del documento..."}
```

Sub-estados posibles: `validating`, `sanitizing`, `ocr_processing`.

##### `completed`

Emitido cuando el procesamiento finaliza exitosamente.

```
event: completed
data: {"status":"completed","document_type":"passport","ocr_confidence":0.97,"message":"Documento procesado exitosamente."}
```

##### `rejected`

Emitido cuando el documento no es reconocido.

```
event: rejected
data: {"status":"rejected","failure_reason":"not_a_document","detail":"El archivo no contiene un documento reconocible."}
```

##### `failed`

Emitido cuando ocurre un error técnico.

```
event: failed
data: {"status":"failed","failure_reason":"ocr_timeout","detail":"El servicio OCR excedió el tiempo de espera."}
```

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `DOCUMENT_NOT_FOUND` | 404 | No existe el documento o no pertenece al usuario |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## Búsquedas Guardadas

### Create Saved Search

Guarda una búsqueda con deduplicación por hash de parámetros. Las búsquedas son compatibles con los endpoints del módulo search (`search_flights`, `search_hotels`, `search_ai`). Solo se soportan búsquedas de hoteles, vuelos o ambos.

#### Request

```
POST /v1/user/saved-searches
```

**Body:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `name` | string | No | Nombre descriptivo de la búsqueda |
| `parameters` | JSON object | Sí | Parámetros de búsqueda (origen, destino, fechas, pasajeros, etc.) |
| `filters` | JSON object | No | Filtros adicionales (max_price, currency, etc.) |
| `alert_enabled` | boolean | No | Activar alerta de precio (default: false) |

**Ejemplo:**

```bash
curl -X POST {base_url}/saved-searches \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "name": "Madrid en junio",
    "parameters": {
      "origin": "EZE",
      "destination": "MAD",
      "outbound_date": "2026-06-15",
      "return_date": "2026-06-22",
      "adults": 2,
      "travel_class": "economy"
    },
    "filters": {
      "max_price": 1500,
      "currency": "EUR"
    },
    "alert_enabled": true
  }'
```

#### Responses

##### 201 Created

```json
{
  "search_id": "019d5439-cb43-716d-90b5-51dcbe980908",
  "message": "Saved search created successfully."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `DUPLICATE_SEARCH` | 409 | Ya existe una búsqueda idéntica para este usuario (mismo hash de parámetros) |
| `VALIDATION_ERROR` | 400 | `parameters` vacío o malformado |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### List Saved Searches

Lista todas las búsquedas guardadas del usuario.

#### Request

```
GET /v1/user/saved-searches
```

> El navegador envía las cookies automáticamente. No requiere body ni headers adicionales.

**Ejemplo:**

```bash
curl -X GET {base_url}/saved-searches \
  -H "Accept: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
{
  "searches": [
    {
      "id": "019d5439-cb43-716d-90b5-51dcbe980908",
      "name": "Madrid en junio",
      "parameters": {
        "origin": "EZE",
        "destination": "MAD",
        "outbound_date": "2026-06-15",
        "return_date": "2026-06-22",
        "adults": 2,
        "travel_class": "economy"
      },
      "filters": {
        "max_price": 1500,
        "currency": "EUR"
      },
      "alert_enabled": true,
      "last_executed_at": "2026-05-05T10:30:00Z",
      "result_count": 15,
      "created_at": "2026-05-01T10:30:00Z",
      "updated_at": "2026-05-01T10:30:00Z"
    }
  ]
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

---

### Update Saved Search

Edita una búsqueda guardada existente. Actualización parcial — solo los campos enviados se modifican.

El hash de parámetros se recalcula si `parameters` cambia. Retorna 409 si el nuevo hash colisiona con una búsqueda existente del mismo usuario.

#### Request

```
PUT /v1/user/saved-searches/:search_id
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `search_id` | string (UUID v7) | Sí | ID de la búsqueda guardada |

**Body:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `name` | string | No | Nuevo nombre |
| `parameters` | JSON object | No | Nuevos parámetros de búsqueda |
| `filters` | JSON object | No | Nuevos filtros |

**Ejemplo:**

```bash
curl -X PUT {base_url}/saved-searches/019d5439-cb43-716d-90b5-51dcbe980908 \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "name": "Madrid en julio",
    "parameters": {
      "origin": "EZE",
      "destination": "MAD",
      "outbound_date": "2026-07-10",
      "return_date": "2026-07-20",
      "adults": 2,
      "travel_class": "economy"
    }
  }'
```

#### Responses

##### 200 OK

```json
{
  "message": "Saved search updated successfully."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `SEARCH_NOT_FOUND` | 404 | No existe la búsqueda o no pertenece al usuario |
| `DUPLICATE_SEARCH` | 409 | El nuevo hash de parámetros colisiona con una búsqueda existente |
| `VALIDATION_ERROR` | 400 | Body malformado |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Delete Saved Search

Elimina una búsqueda guardada y su configuración de alerta asociada.

#### Request

```
DELETE /v1/user/saved-searches/:search_id
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `search_id` | string (UUID v7) | Sí | ID de la búsqueda guardada |

**Ejemplo:**

```bash
curl -X DELETE {base_url}/saved-searches/019d5439-cb43-716d-90b5-51dcbe980908 \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
{
  "message": "Saved search deleted successfully."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `SEARCH_NOT_FOUND` | 404 | No existe la búsqueda o no pertenece al usuario |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Toggle Price Alert

Activa o desactiva la alerta de precio para una búsqueda guardada.

#### Request

```
PUT /v1/user/saved-searches/:search_id/alert
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `search_id` | string (UUID v7) | Sí | ID de la búsqueda guardada |

**Body:**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `enabled` | boolean | Sí | Activar (`true`) o desactivar (`false`) |

**Ejemplo:**

```bash
curl -X PUT {base_url}/saved-searches/019d5439-cb43-716d-90b5-51dcbe980908/alert \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{"enabled": true}'
```

#### Responses

##### 200 OK

```json
{
  "search_id": "019d5439-cb43-716d-90b5-51dcbe980908",
  "alert_enabled": true,
  "message": "Price alert enabled successfully."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `SEARCH_NOT_FOUND` | 404 | No existe la búsqueda o no pertenece al usuario |
| `VALIDATION_ERROR` | 400 | Falta `enabled` o no es booleano |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

## Favoritos

### Add Favorite

Guarda un lugar/item como favorito.

#### Request

```
POST /v1/user/favorites
```

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `entity_id` | string (UUID v7) | Sí | — | ID de la entidad (hotel, vuelo, destino) |
| `entity_type` | string | Sí | `hotel`, `flight`, `destination` | Tipo de entidad |
| `title` | string | Sí | — | Título descriptivo |
| `notes` | string | No | — | Notas personales |

**Ejemplo:**

```bash
curl -X POST {base_url}/favorites \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "entity_id": "019d5439-cb43-716d-90b5-51dcbe980500",
    "entity_type": "hotel",
    "title": "Pullman Bali Legian Beach",
    "notes": "Opción con spa y piscina infinita"
  }'
```

#### Responses

##### 201 Created

```json
{
  "favorite_id": "019d5439-cb43-716d-90b5-51dcbe980908",
  "message": "Added to favorites."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `VALIDATION_ERROR` | 400 | Campos requeridos faltantes o `entity_type` inválido |
| `DUPLICATE_FAVORITE` | 409 | Ya existe un favorito para este `entity_id` + `entity_type` |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### List Favorites

Lista todos los favoritos del usuario. Opcionalmente filtra por tipo de entidad.

#### Request

```
GET /v1/user/favorites
```

**Query Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `entity_type` | string | No | Filtrar por tipo: `hotel`, `flight`, `destination` |

**Ejemplo:**

```bash
curl -X GET "{base_url}/favorites?entity_type=hotel" \
  -H "Accept: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
{
  "favorites": [
    {
      "id": "019d5439-cb43-716d-90b5-51dcbe980908",
      "entity_id": "019d5439-cb43-716d-90b5-51dcbe980500",
      "entity_type": "hotel",
      "title": "Pullman Bali Legian Beach",
      "notes": "Opción con spa y piscina infinita",
      "created_at": "2026-05-01T10:30:00Z"
    },
    {
      "id": "019d5439-cb43-716d-90b5-51dcbe980909",
      "entity_id": "019d5439-cb43-716d-90b5-51dcbe980600",
      "entity_type": "destination",
      "title": "Tokio",
      "notes": "Viaje soñado",
      "created_at": "2026-05-02T14:22:00Z"
    }
  ]
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `INVALID_ENUM` | 400 | `entity_type` no es `hotel`, `flight` o `destination` |
| `TOKEN_INVALID` | 401 | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | Error inesperado |

---

### Delete Favorite

Elimina un favorito.

#### Request

```
DELETE /v1/user/favorites/:favorite_id
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `favorite_id` | string (UUID v7) | Sí | ID del favorito |

**Ejemplo:**

```bash
curl -X DELETE {base_url}/favorites/019d5439-cb43-716d-90b5-51dcbe980908 \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
{
  "message": "Favorite removed successfully."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Cuándo |
|--------|------|--------|
| `FAVORITE_NOT_FOUND` | 404 | No existe el favorito o no pertenece al usuario |
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

> **Crítico:** NUNCA usar `Access-Control-Allow-Origin: *` cuando se envían cookies. Debe ser origen explícito.

---

## Rate Limiting

Rate limiting multi-tier con DragonflyDB y scripts Lua atómicos. Distribuido y seguro en entornos multi-instancia. Todos los límites son configurables vía variables de entorno.

### Tiers

| Tier | Scope | Límite | Aplica a |
|------|-------|--------|----------|
| **Tier 1 — Global** | IP | 100 req/min | Todos los endpoints (DDoS shield) |
| **Tier 2 — Authenticated** | UUID del usuario | 10 req/min | Endpoints protegidos con auth |

### Rate Limiting por Tipo de Endpoint

| Endpoints | Límite adicional | Descripción |
|-----------|-------------------|-------------|
| `POST /v1/user/documents` | 10 req/hora por usuario | Upload de documentos (costoso — pipeline OCR) |
| `POST /v1/user/profile/avatar` | 5 req/hora por usuario | Generación de presigned URLs |
| `POST /v1/user/profile/medical/pending/resolve` | 20 req/hora por usuario | Resolución de conflictos médicos |

### Response on 429 (Rate Limit Exceeded)

Formato **RFC 9457 Problem Details**:

```json
{
  "type": "https://api.proactrip.com/errors/rate-limit-exceeded",
  "title": "Too Many Requests",
  "status": 429,
  "detail": "Demasiadas peticiones. Esperá 60 segundos antes de reintentar.",
  "instance": "/v1/user/profile",
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

### Estrategia General

| Endpoint | Cache-Control | Motivo |
|----------|---------------|--------|
| `GET /v1/user/profile` | `no-store, private` | Datos personales sensibles |
| `PUT /v1/user/profile` | `no-store, private` | Mutación de datos |
| `PUT /v1/user/profile/locale` | `no-store, private` | Mutación de datos |
| `PUT /v1/user/profile/travel-preferences` | `no-store, private` | Mutación de datos |
| `PUT /v1/user/profile/notifications` | `no-store, private` | Mutación de datos |
| `GET /v1/user/profile/medical` | `no-store, private` | Datos médicos sensibles |
| `PUT /v1/user/profile/medical` | `no-store, private` | Mutación de datos médicos |
| `GET /v1/user/profile/medical/pending` | `no-store, private` | Conflictos médicos sensibles |
| `POST /v1/user/profile/medical/pending/resolve` | `no-store, private` | Mutación de datos médicos |
| `POST /v1/user/profile/avatar` | `no-store, private` | URL prefirmada temporal |
| `POST /v1/user/profile/avatar/confirm` | `no-store, private` | Mutación de avatar |
| `POST /v1/user/documents` | `no-store, private` | Upload de documento |
| `GET /v1/user/documents` | `no-store, private` | Datos de documentos del usuario |
| `GET /v1/user/documents/:document_id` | `no-store, private` | Metadatos de documento |
| `GET /v1/user/documents/:document_id/download` | `private, max-age=300` | Descarga de archivo (cache breve) |
| `DELETE /v1/user/documents/:document_id` | `no-store, private` | Mutación de documento |
| `PUT /v1/user/documents/:document_id/verify` | `no-store, private` | Verificación admin |
| `GET /v1/user/documents/:document_id/events` | `no-cache` (SSE) | Streaming en tiempo real |
| `GET /v1/user/documents/types` | `public, max-age=3600` | Catálogo estático público |
| `POST /v1/user/saved-searches` | `no-store, private` | Creación de búsqueda |
| `GET /v1/user/saved-searches` | `no-store, private` | Datos de búsquedas del usuario |
| `PUT /v1/user/saved-searches/:search_id` | `no-store, private` | Mutación de búsqueda |
| `DELETE /v1/user/saved-searches/:search_id` | `no-store, private` | Eliminación de búsqueda |
| `PUT /v1/user/saved-searches/:search_id/alert` | `no-store, private` | Mutación de alerta |
| `POST /v1/user/favorites` | `no-store, private` | Creación de favorito |
| `GET /v1/user/favorites` | `no-store, private` | Datos de favoritos del usuario |
| `DELETE /v1/user/favorites/:favorite_id` | `no-store, private` | Eliminación de favorito |

### Agrupación

- **`no-store, private`**: Todos los endpoints que retornan datos del usuario o realizan mutaciones.
- **`public, max-age=3600`**: `GET /v1/user/documents/types` — catálogo estático de tipos de documentos.
- **`private, max-age=300`**: `GET /v1/user/documents/:document_id/download` — cache breve de descarga de archivos.
- **`no-cache`**: `GET /v1/user/documents/:document_id/events` — SSE streaming.

---

## Notas de Seguridad

### Cookie-Based Authentication

Toda la autenticación usa cookies HttpOnly (`__Secure-access_token`, `__Secure-refresh_token`) con `SameSite=Lax`. El frontend **nunca** envía headers `Authorization`. Las cookies viajan automáticamente con `credentials: 'include'`. Ver [AUTH_API.md](AUTH_API.md) para detalles completos del flujo de autenticación.

### Encriptación de Datos Médicos

Los datos médicos (`blood_type`, `allergies`, `medications`, `conditions`, `vaccinations`, `emergency_contact`, `insurance_info`) se almacenan encriptados en reposo con **ChaCha20-Poly1305** usando una clave de 32 bytes. La encriptación/desencriptación ocurre en el backend — el frontend siempre recibe y envía datos en texto plano. En caso de error de desencriptación, se devuelve `DECRYPTION_ERROR` (500).

### Seguridad del Pipeline de Documentos

1. **Magic bytes sincrónicos**: El backend verifica los primeros 512 bytes antes de aceptar el archivo. Rechazo inmediato si no coincide con tipos soportados.
2. **SanitizerWorker**: Elimina metadatos EXIF de imágenes y limpia PDFs antes del OCR.
3. **R2 Presigned URLs**: URLs temporales con TTL configurable (default 15 min). El backend nunca expone credenciales de R2 al frontend.
4. **Validación post-upload**: El worker de avatar valida MIME, tamaño y contenido antes de activar. Protege contra archivos maliciosos renombrados.

### Expiración de Conflictos Médicos

Los conflictos médicos pendientes (`pending_updates`) expiran automáticamente a los **30 días**. Pasado ese tiempo, el endpoint `POST /v1/user/profile/medical/pending/resolve` devuelve `PENDING_UPDATE_EXPIRED` (400). Esto previene acumulación infinita de conflictos no resueltos.

### SMS — No Implementado

El canal de notificación `sms` está planificado pero **NO** implementado. Las preferencias se guardan correctamente en base de datos, pero el backend no enviará SMS hasta que se complete la integración con un proveedor (ej: Twilio). Solo `email` y `websocket` son funcionales.

### Verificación Admin

El endpoint `PUT /v1/user/documents/:document_id/verify` requiere rol `admin`. Usuarios con rol `client` reciben `PERMISSION_DENIED` (403). La verificación incluye:
- Pasaportes con MRZ válido → auto-verificados durante OCR
- Otros documentos → requieren verificación manual
- Admin puede sobrescribir cualquier estado de verificación

### Avatares — R2 Direct Upload

El backend **nunca** recibe el contenido del archivo de avatar. El flujo es:
1. Backend genera presigned PUT URL de R2 (TTL configurable)
2. Frontend sube el binario directamente a R2
3. Frontend confirma vía `POST /avatar/confirm`
4. Worker valida asincrónicamente y activa

Esto evita que el backend sea un cuello de botella para uploads y reduce la superficie de ataque.

### Separación de Módulos

Los endpoints de ubicación y clima **NO** están en el User module. Pertenecen al Environment module (`/v1/environment`). El perfil de usuario incluye `location` con el mismo formato que `/v1/environment` para consistencia, pero la resolución de IP y weather se hace en el módulo separado. Ver [ENVIRONMENT_API.md](ENVIRONMENT_API.md).

### Headers de Seguridad

Todas las respuestas incluyen:

```
Content-Security-Policy: default-src 'self'
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
Strict-Transport-Security: max-age=31536000
```

### Prevención de Ataques

| Amenaza | Mitigación |
|---------|------------|
| XSS | `HttpOnly cookies` + `Content-Security-Policy` |
| CSRF | `SameSite=Lax` + cookies automáticas (sin `Authorization` manual) |
| Token Exposure | Cookies HttpOnly — JavaScript no puede leerlas |
| Replay de refresh | Rotación continua + invalidación total ante reúso |
| Third-party cookies | No se usa Partitioned (CHIPS) — SameSite=Lax + Domain=.proactrip.com es suficiente para subdominios |
| Rate limiting abuse | Multi-tier con DragonflyDB + Lua scripts atómicos (IP, usuario autenticado) + límites por endpoint |
| Malicious file upload | Magic bytes check (512 bytes) + SanitizerWorker + R2 presigned URLs con TTL |
| Medical data exposure | ChaCha20-Poly1305 at rest + TLS in transit + field-level provenance tracking |
| IDOR (Insecure Direct Object Reference) | Todos los endpoints validan que el recurso pertenece al usuario autenticado |
