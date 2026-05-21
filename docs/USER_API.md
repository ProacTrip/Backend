# User Module API Documentation (Cookie-Based)

> **Arquitectura:** Cookie-based authentication con HttpOnly cookies. El frontend nunca manipula tokens.
> **Perfil auto-creado:** Reactivamente vía Dragonfly Streams al recibir el evento `auth.user.registered`.

---

## Índice

| Sección | Estado |
|---------|--------|
| [Arquitectura](#arquitectura) | ✅ |
| [Seguridad de Cookies](#seguridad-de-cookies) | ✅ |
| [Estrategia de Refresco de Tokens](#estrategia-de-refresco-de-tokens) | ✅ |
| [Base URLs](#base-urls) | ✅ |
| [Errores Estándar](#errores-estándar) | ✅ |
| [Realtime Events (SSE)](#realtime-events-sse) | ✅ Implementado |
| [Get Profile](#get-profile) | ✅ Implementado |
| [Update Profile](#update-profile) | ✅ Implementado |
| [Get Travel Preferences](#get-travel-preferences) | ✅ Implementado |
| [Update Travel Preferences](#update-travel-preferences) | ✅ Implementado |
| [Get Medical Profile](#get-medical-profile) | ✅ Implementado |
| [Update Medical Profile](#update-medical-profile) | ✅ Implementado |
| [List Medical Conflicts](#list-medical-conflicts) | ✅ Implementado |
| [Get Medical Conflict](#get-medical-conflict) | ✅ Implementado |
| [Resolve Medical Conflict](#resolve-medical-conflict) | ✅ Implementado |
| [Upload Avatar (Presigned URL)](#upload-avatar-presigned-url) | ✅ Implementado |
| [Confirm Avatar Upload](#confirm-avatar-upload) | ✅ Implementado |
| [List Document Types](#list-document-types) | ✅ Implementado |
| [Upload Document](#upload-document) | ✅ Implementado |
| [List Documents](#list-documents) | ✅ Implementado |
| [Get Document](#get-document) | ✅ Implementado |
| [Get Document Download URL](#get-document-download-url) | ✅ Implementado |
| [Delete Document](#delete-document) | ✅ Implementado |
| [Features Planificadas](#features-planificadas) | ✅ |
| [Configuración CORS](#configuración-cors) | ✅ |
| [Rate Limiting](#rate-limiting) | ✅ |
| [Cache](#cache) | ✅ |
| [Notas de Seguridad](#notas-de-seguridad) | ✅ |

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

El perfil se inicializa con datos obtenidos del evento `auth.user.registered`:
- `first_name`
- `email`

con el evento `auth.user.verified` se setea también language de Accept-Language header

Si se crea la cuenta con Oauth(ej: Google) se inicializa con datos obtenidos del evento `auth.user.registered`:
- `given_name`
- `family_name`
- `locale(para language)`
- `avatar_url`

El resto de campos son opcionales. se crean en `null` — el usuario los completa después desde la UI.

### Pipeline de Documentos

Procesamiento asíncrono con workers conectados vía Dragonfly Streams:

```
┌──────────────┐
│   Frontend   │  POST /v1/user/profile/documents (multipart/form-data)
└──────┬───────┘
       │
       ▼
┌──────────────────────────────────────────────────────────────────────┐
│ Backend                                                             │
│                                                                      │
│ 1. Auth + size limit                                                 │
│ 2. Quick magic bytes check (primeros 512 bytes, validación mínima)   │
│ 3. Guarda archivo original en R2: raw/{document_id}                  │
│ 4. Publica job en Dragonfly Streams                                  │
│ 5. Responde 202 Accepted                                             │
└──────────────────────────────────────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────────────────────────────┐
│                    Dragonfly Stream: stream:doc:jobs                 │
└──────────────────────────────────────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────────────────────────────┐
│                        DocumentWorker                                │
│                                                                      │
│ 1. Descarga archivo desde R2 raw/                                    │
│ 2. Validación completa (magic bytes, MIME real, tamaño)              │
│ 3. Sanitización (strip EXIF, normalize image, clean PDF)             │
│ 4. Guarda versión procesada en R2 processed/                         │
│ 5. Genera presigned URL temporal (TTL 5 min) del archivo en processed/      │
│    y la envía a DeepSeek para OCR + extracción de datos                       │
│ 6. Guarda resultados en R2 results/                                  │
│ 7. Persiste metadata + extracted_data en PostgreSQL                  │
│ 8. Actualiza estado del documento                                    │
│ 9. Emite evento realtime (SSE)                                       │
└──────────────────────────────────────────────────────────────────────┘
       │
       ├───────────────────────────────┐
       ▼                               ▼
┌──────────────┐               ┌────────────────────┐
│ PostgreSQL   │               │ SSE Event Hub      │
│               │               │                    │
│ documents     │               │ document.updated   │
│ extracted_data│               │ document.completed │
│ status        │               │ document.failed    │
└──────────────┘               └────────────────────┘


R2 Bucket Structure:
proactrip-secure/
├── raw/
├── processed/
└── results/
```

### Resolución de Conflictos Médicos

Cuando OCR detecta datos médicos que entran en conflicto con el perfil existente, el conflicto se persiste en base de datos y se notifica al usuario en tiempo real vía SSE. La resolución SIEMPRE ocurre mediante endpoints estructurados (NO mediante chat AI).

```
┌──────────────┐
│  OCRWorker   │
└──────┬───────┘
       │ Detecta conflicto
       ▼
┌──────────────────────────────┐
│ PostgreSQL                   │
│ medical_profile_conflicts    │
│ status: pending              │
└──────────────┬───────────────┘
               │
               │ Emite SSE
               ▼
┌──────────────────────────────┐
│ SSE Event Hub                │
│ event: medical.conflict.created
└──────────────┬───────────────┘
               ▼
┌──────────────┐
│  Frontend    │
└──────┬───────┘
       │ Usuario revisa diferencias
       ▼
GET /v1/user/profile/medical-conflicts/{conflict_id}

       │ Usuario decide
       ▼
POST /v1/user/profile/medical-conflicts/{conflict_id}/resolve

{
  "action": "accept" | "reject" | "custom",
  "value": "..."
}

       │
       ▼
┌──────────────────────────────┐
│ Backend                      │
│ - Aplica cambio              │
│ - Marca conflicto resuelto   │
│ - Emite SSE update           │
└──────────────────────────────┘
```

Reglas importantes: 

- OCR NUNCA modifica automáticamente datos médicos.
- OCR solo genera sugerencias y detecta conflictos.
- El usuario siempre confirma manualmente los cambios.
- SSE solo notifica eventos; el estado persistente vive en PostgreSQL.
- Los conflictos tienen lifecycle persistente (pending, resolved, rejected).

### Avatar

Un único avatar asignado al perfil del usuario.

- Usuarios OAuth Google usan inicialmente el avatar de Google.
- Avatares personalizados se suben directamente a R2 mediante presigned URL.
- La activación del avatar ocurre asincrónicamente después de validación por worker.
- El frontend recibe actualizaciones en tiempo real vía SSE.

```
┌──────────────┐  POST /v1/user/profile/avatar
│   Frontend   │─────────────────────────────────────┐
└──────────────┘                                     │
                                                     ▼
                                         ┌────────────────────┐
                                         │ Backend            │
                                         │ Genera presigned   │
                                         │ URL + storage_key  │
                                         └─────────┬──────────┘
                                                   │
                                                   ▼
                               { upload_url, storage_key }
                                                   │
┌──────────────┐<──────────────────────────────────┘
│   Frontend   │
└──────┬───────┘
       │
       │ PUT upload_url (direct upload a R2)
       ▼
┌──────────────────────────────┐
│ R2 Bucket: proactrip-assets  │
│ avatars/raw/                 │
└──────────────┬──────────────┘
               │
               │ POST /v1/user/profile/avatar/confirm
               ▼
┌────────────────────┐
│ Backend            │
│ Publica job async  │
│ status=validating  │
└─────────┬──────────┘
          │
          ▼
┌──────────────────────────────┐
│ AvatarWorker                 │
│                              │
│ 1. Descarga archivo desde    │
│    avatars/raw/              │
│ 2. Valida MIME/magic bytes   │
│ 3. Sanitiza imagen (strip EXIF, resize, webp) │
│ 4. Genera variantes (thumbs, webp)            │
│ 5. Mueve a avatars/processed/ y actualiza avatar_url │
│ 6. Emite SSE event: user.avatar.updated      │
└─────────┬────────────────────┘
          │
          ▼
┌──────────────────────────────┐
│ SSE Event Hub                │
│ event: user.avatar.updated   │
└──────────────────────────────┘
```

Reglas importantes: 

- El backend nunca proxya archivos binarios.
- Todas las subidas ocurren directamente a R2 mediante presigned URLs.
- El avatar no se activa inmediatamente después del upload.
- La validación final ocurre asincrónicamente en worker.
- SSE mantiene sincronizada la UI en tiempo real.

---

## Seguridad de Cookies

### Atributos Obligatorios

| Atributo | Valor | Propósito |
|----------|-------|-----------|
| `HttpOnly` | `true` | Inaccesible vía JavaScript (mitiga XSS) |
| `Secure` | `true` | Solo HTTPS en producción |
| `SameSite` | `Lax` | Protección CSRF. Permite navegación top-level. Para OAuth, el Auth module usa `None` durante el callback cross-origin |
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

## Realtime Events (SSE)

Conexión SSE centralizada para todos los módulos. El frontend mantiene **una única conexión** persistente mientras el usuario esté autenticado.

```
GET /v1/realtime/events
```

### Eventos del User Module

| Evento | Cuándo |
|--------|--------|
| `user.avatar.updated` | Avatar procesado y activado por el worker |
| `user.profile.updated` | Perfil modificado (por el usuario o por sistema) |
| `document.processing.completed` | Documento procesado exitosamente por OCR |
| `document.verification.updated` | Admin cambió el estado de verificación |
| `medical.conflict.created` | OCR detectó un conflicto médico |
| `medical.conflict.resolved` | Usuario resolvió un conflicto médico |

### Comportamiento

- **Reconnect automático**: El navegador reconecta automáticamente si la conexión se cae.
- **Late-join**: Si el frontend se conecta después de un evento, consulta el estado actual vía los endpoints REST correspondientes.
- **No storage**: SSE solo notifica. La verdad vive en PostgreSQL.
- **No polling**: El frontend solo escucha y actualiza estado local.

> **Nota:** La documentación completa de eventos SSE (todos los módulos) está en la skill `api-docs` del proyecto.

---

## Perfil

### Get Profile

Retorna el perfil del usuario autenticado.

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
  "location": {
    "currency": "ARS",
    "language": "es"
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
| `location` | object | Datos de ubicación |
| `location.currency` | string | Código ISO 4217 |
| `location.language` | string | Código ISO 639-1 |

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `PROFILE_NOT_FOUND` | 404 | `profile-not-found` | No existe perfil para el usuario autenticado |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

---

### Update Profile

Actualización parcial de información personal. Todos los campos del body son opcionales.

#### Request

```
PATCH /v1/user/profile
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
| `language` | string | No | ISO 639 (2-5 caracteres) | Idioma preferido |
| `currency` | string | No | ISO 4217 (3 caracteres) | Moneda preferida |

**Ejemplo:**

```bash
curl -X PATCH {base_url}/profile \
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
  "message": "Perfil actualizado correctamente."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `PROFILE_NOT_FOUND` | 404 | `profile-not-found` | No existe perfil para el usuario autenticado |
| `INVALID_ENUM` | 400 | `invalid-gender` | Valor de `gender` no válido |
| `VALIDATION_ERROR` | 400 | `validation-error` | Body malformado o campo con formato inválido |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |
| `INVALID_LANGUAGE_CODE` | 400 | `invalid-language-code` | `language_code` no es un código ISO 639 válido |
| `INVALID_CURRENCY_CODE` | 400 | `invalid-currency-code` | `currency_code` no es un código ISO 4217 válido |

---

### Get Travel Preferences

Retorna las preferencias de viaje del usuario autenticado.

#### Request

```
GET /v1/user/profile/travel-preferences
```

> El navegador envía las cookies automáticamente. No requiere body ni headers adicionales.

**Ejemplo:**

```bash
curl -X GET {base_url}/travel-preferences \
  -H "Accept: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
{
  "preferred_class": "economy",
  "seat_preference": "aisle",
  "meal_preference": "vegetarian",
  "special_assistance": ["wheelchair"],
  "preferred_airlines": ["019d5439-cb43-716d-90b5-51dcbe980001"],
  "preferred_hotels": ["Marriott", "Hilton"],
  "avoid_layovers": true,
  "max_layover_duration": 120
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `TRAVEL_PREFS_NOT_FOUND` | 404 | `travel-prefs-not-found` | No existen preferencias para el usuario autenticado |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

---

### Update Travel Preferences

Actualización parcial de preferencias de viaje. Todos los campos son opcionales.

#### Request

```
PATCH /v1/user/profile/travel-preferences
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
curl -X PATCH {base_url}/travel-preferences \
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
  "message": "Preferencias de viaje actualizadas correctamente."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `PROFILE_NOT_FOUND` | 404 | `profile-not-found` | No existe perfil para el usuario autenticado |
| `INVALID_ENUM` | 400 | `invalid-enum` | Valor de `preferred_class` o `seat_preference` no válido |
| `VALIDATION_ERROR` | 400 | `validation-error` | Body malformado o `max_layover_duration` negativo |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

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
curl -X GET {base_url}/medical-profile \
  -H "Accept: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
{
  "data": {
    "blood_type": {
      "value": "A+",
      "source": {
        "type": "ocr",
        "document_id": "019d...",
        "confidence": 0.94
      },
      "updated_at": "2026-03-15T10:30:00Z"
    },
    "allergies": {
      "value": ["Penicilina", "Polen"],
      "source": {
        "type": "ocr",
        "document_id": "019d...",
        "confidence": 0.91
      },
      "updated_at": "2026-04-01T14:22:00Z"
    },
    "medications": {
      "value": [
        {
          "name": "Ibuprofeno",
          "dosage": "600mg",
          "frequency": "Cada 8 horas",
          "duration": "5 días",
          "status": "active"
        },
        {
          "name": "Omeprazol",
          "dosage": "20mg",
          "frequency": "Cada 24 horas (ayunas)",
          "duration": "crónico",
          "status": "active"
        }
      ],
      "source": {
        "type": "manual",
        "document_id": "019d...",
        "confidence": null
      },
      "updated_at": "2026-01-10T08:15:00Z"
    },
    "conditions": {
      "value": ["Asma leve"],
      "source": {
        "type": "manual",
        "document_id": "019d...",
        "confidence": null
      },
      "updated_at": "2025-11-20T16:45:00Z"
    },
    "vaccinations": {
      "value": [
        {
          "name": "COVID-19",
          "doses_received": 3,
          "status": "completed"
        },
        {
          "name": "Fiebre amarilla",
          "doses_received": 1,
          "status": "active"
        }
      ],
      "source": {
        "type": "ocr",
        "document_id": "019d...",
        "confidence": 0.88
      },
      "updated_at": "2026-02-28T09:00:00Z"
    },
    "emergency_contact": {
      "value": {
        "name": "María García",
        "phone": "+5491123456790",
        "relationship": null
      },
      "source": {
        "type": "ocr",
        "document_id": "019d...",
        "confidence": 0.85
      },
      "updated_at": "2026-03-01T12:00:00Z"
    },
    "insurance_info": {
      "value": {
        "company": "ASSA Compañía de Seguros",
        "policy_number": "12345",
        "plan_type": null,
        "expiration_date": null
      },
      "source": {
        "type": "manual",
        "document_id": null,
        "confidence": null
      },
      "updated_at": "2026-03-01T12:05:00Z"
    }
  }
}
```

**Campos de respuesta:**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `data` | object | Datos médicos del perfil |
| `data.blood_type` | object | Grupo sanguíneo con trazabilidad |
| `data.blood_type.value` | string\|null | `A+`, `A-`, `B+`, `B-`, `AB+`, `AB-`, `O+`, `O-` |
| `data.blood_type.source` | object | Objeto que contiene el origen y la trazabilidad del dato. |
| `data.blood_type.source.type` | string | Método de carga: `manual`, `ocr`. |
| `data.blood_type.source.document_id` | string\|null | ID del documento. |
| `data.blood_type.source.confidence` | float\|null | Confianza del OCR (0.0-1.0). `null` si `source.type = manual`. |
| data.blood_type.updated_at | string | Fecha de actualización en formato ISO 8601 UTC. |
| data.allergies | object | Alergias registradas con metadatos de origen. |
| data.allergies.value | array[string] | Lista de sustancias o medicamentos que causan reacción. |
| data.allergies.source | object | Estructura de trazabilidad (type y document_id). |
| data.allergies.updated_at | string | Fecha de actualización en formato ISO 8601 UTC. |
| data.medications | object | Medicamentos actuales con metadatos de origen. |
| data.medications.value | array[object] | Lista de objetos con el detalle de cada tratamiento activo. |
| data.medications.value[].name | string | Nombre comercial o genérico del medicamento. |
| data.medications.value[].dosage | string | Concentración o dosis del medicamento (ej: 600mg). |
| data.medications.value[].frequency | string | Intervalo de administración (ej: Cada 8 horas). |
| data.medications.value[].duration | string | Duración del tratamiento (ej: 5 días, crónico). |
| data.medications.value[].status | string | Estado actual: active, completed, discontinued. |
| data.medications.source | object | Estructura de trazabilidad (type y document_id). |
| data.medications.updated_at | string | Fecha de actualización en formato ISO 8601 UTC. |
| data.conditions | object | Condiciones o enfermedades crónicas con metadatos. |
| data.conditions.value | array[string] | Lista de diagnósticos médicos activos (ej: Asma leve). |
| data.conditions.source | object | Estructura de trazabilidad (type y document_id). |
| data.conditions.updated_at | string | Fecha de actualización en formato ISO 8601 UTC. |
| data.vaccinations | object | Historial de vacunación con metadatos de origen. |
| data.vaccinations.value | array[object] | Lista de vacunas recibidas por el paciente. |
| data.vaccinations.value[].name | string | Nombre de la vacuna o patógeno que combate. |
| data.vaccinations.value[].doses_received | integer | Cantidad de dosis aplicadas de esta vacuna. |
| data.vaccinations.value[].status | string | Estado del esquema: active, completed. |
| data.vaccinations.source | object | Estructura de trazabilidad (type y document_id). |
| data.vaccinations.updated_at | string | Fecha de actualización en formato ISO 8601 UTC. |
| data.emergency_contact | object | Información del contacto en caso de urgencia. |
| data.emergency_contact.value | object | Datos específicos de la persona asignada. |
| data.emergency_contact.value.name | string | Nombre completo del contacto. |
| data.emergency_contact.value.phone | string | Número telefónico con código internacional (E.164). |
| data.emergency_contact.value.relationship | string | null | Parentesco con el usuario (ej: Madre, Esposo). |
| data.emergency_contact.source | object | Estructura de trazabilidad (type y document_id). |
| data.emergency_contact.updated_at | string | Fecha de actualización en formato ISO 8601 UTC. |
| data.insurance_info | object | Información del seguro médico o prepaga. |
| data.insurance_info.value | object | Detalles de la cobertura de salud. |
| data.insurance_info.value.company | string | Nombre de la empresa aseguradora. |
| data.insurance_info.value.policy_number | string | Identificador único de la póliza contratada. |
| data.insurance_info.value.plan_type | string | null | Tipo o nombre del plan de salud específico. |
| data.insurance_info.value.expiration_date | string | null | Fecha de vencimiento de la cobertura (ISO 8601). |
| data.insurance_info.source | object | Estructura de trazabilidad (type y document_id). |
| data.insurance_info.updated_at | string | Fecha de actualización en formato ISO 8601 UTC. |
| is_shared | boolean | Indica si el perfil es accesible de forma pública en emergencias. |
| has_pending_conflicts | boolean | true si el OCR/NLP detectó datos que contradicen lo manual. |
| pending_conflict_count | integer | Número total de inconsistencias que requieren revisión manual. |

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `MEDICAL_PROFILE_NOT_FOUND` | 404 | `medical-profile-not-found` | No existe perfil médico para el usuario |
| `DECRYPTION_ERROR` | 500 | `decryption-error` | Error al desencriptar datos médicos |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

---

### Update Medical Profile

Actualiza campos médicos manualmente (source="manual"). Todos los campos son opcionales.

#### Request

```
PATCH /v1/user/profile/medical
```

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `blood_type` | string\|null | No | `A+`, `A-`, `B+`, `B-`, `AB+`, `AB-`, `O+`, `O-` | Grupo sanguíneo del usuario |
| `allergies` | array[string] | No | — | Lista de alergias (ej: `["Penicilina"]`) |
| `medications` | array[object] | No | Objetos con `name`, `dosage`, `frequency`, `duration`, `status` | Lista de tratamientos activos |
| `conditions` | array[string] | No | — | Condiciones médicas (ej: `["Asma leve"]`) |
| `vaccinations` | array[object] | No | Objetos con `name`, `doses_received`, `status` | Historial de vacunas |
| `emergency_contact` | object | No | Objeto con `name`, `phone`, `relationship` | Datos del contacto de urgencia |
| `insurance_info` | object | No | Objeto con `company`, `policy_number`, `plan_type`, `expiration_date` | Información de la cobertura médica |

**Ejemplo:**

```bash
curl -X PATCH {base_url}/medical-profile \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{
    "blood_type": "O+",
    "allergies": ["Penicilina", "Polen"]
  }'
```

#### Responses

##### 200 OK

```json
{
  "message": "Perfil médico actualizado correctamente.",
  "applied_fields": ["blood_type", "allergies"]
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `MEDICAL_PROFILE_NOT_FOUND` | 404 | `medical-profile-not-found` | No existe perfil médico para el usuario |
| `INVALID_BLOOD_TYPE` | 400 | `invalid-blood-type` | `blood_type` no es un valor válido |
| `ENCRYPTION_ERROR` | 500 | `encryption-error` | Error al encriptar datos médicos |
| `VALIDATION_ERROR` | 400 | `validation-error` | Body malformado |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

---

### List Medical Conflicts

Lista todos los conflictos médicos del usuario, pendientes y resueltos.

#### Request

```
GET /v1/user/profile/medical-conflicts
```

> El navegador envía las cookies automáticamente. No requiere body ni headers adicionales.

**Query Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `status` | string | No | Filtrar por estado: `pending`, `resolved`, `rejected` |

**Ejemplo:**

```bash
curl -X GET "{base_url}/medical-conflicts?status=pending" \
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
      "status": "pending",
      "suggested_at": "2026-04-15T10:30:00Z",
      "expires_at": "2026-05-15T10:30:00Z"
    }
  ]
}
```

**Campos de respuesta:**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `conflicts` | array | Lista de conflictos |
| `conflicts[].id` | string (UUID v7) | ID del conflicto |
| `conflicts[].field` | string | Campo médico en conflicto |
| `conflicts[].current_value` | string | Valor actual en el perfil |
| `conflicts[].proposed_value` | string | Valor propuesto por OCR |
| `conflicts[].source.type` | string | `ocr` |
| `conflicts[].source.document_id` | string (UUID v7) | ID del documento origen |
| `conflicts[].source.file_name` | string | Nombre del archivo origen |
| `conflicts[].status` | string | `pending`, `resolved`, `rejected` |
| `conflicts[].suggested_at` | string (ISO 8601) | Fecha de detección |
| `conflicts[].expires_at` | string (ISO 8601) | Fecha de expiración (30 días) |

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

---

### Get Medical Conflict

Retorna el detalle de un conflicto médico específico.

#### Request

```
GET /v1/user/profile/medical-conflicts/:conflict_id
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `conflict_id` | string (UUID v7) | Sí | ID del conflicto |

**Ejemplo:**

```bash
curl -X GET {base_url}/medical-conflicts/019d5439-cb43-716d-90b5-51dcbe980908 \
  -H "Accept: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
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
  "status": "pending",
  "suggested_at": "2026-04-15T10:30:00Z",
  "expires_at": "2026-05-15T10:30:00Z",
  "resolved_at": null,
  "resolution": null
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `PENDING_UPDATE_NOT_FOUND` | 404 | `pending-update-not-found` | No existe el conflicto o no pertenece al usuario |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

---

### Resolve Medical Conflict

Resuelve un conflicto médico. El usuario revisa los datos sugeridos por OCR y decide si aceptarlos, rechazarlos o ingresar un valor personalizado.

> Se envía notificación SSE en tiempo real cuando se crean o resuelven conflictos.

#### Request

```
POST /v1/user/profile/medical-conflicts/:conflict_id/resolve
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `conflict_id` | string (UUID v7) | Sí | ID del conflicto a resolver |

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `action` | string | Sí | `accept`, `reject`, `custom` | Acción a tomar |
| `value` | string | Solo si `action=custom` | — | Valor personalizado |

**Ejemplo (accept):**

```bash
curl -X POST {base_url}/medical-conflicts/019d5439-cb43-716d-90b5-51dcbe980908/resolve \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{"action": "accept"}'
```

**Ejemplo (custom):**

```bash
curl -X POST {base_url}/medical-conflicts/019d5439-cb43-716d-90b5-51dcbe980908/resolve \
  -H "Content-Type: application/json" \
  -b "__Secure-access_token=v4.local.eyJ..." \
  -d '{"action": "custom", "value": "A-"}'
```

#### Responses

##### 200 OK

```json
{
  "message": "Conflicto médico resuelto correctamente."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `PENDING_UPDATE_NOT_FOUND` | 404 | `pending-update-not-found` | No existe el conflicto con ese ID |
| `PENDING_UPDATE_EXPIRED` | 400 | `pending-update-expired` | El conflicto expiró (más de 30 días) |
| `INVALID_PENDING_ACTION` | 400 | `invalid-pending-action` | `action` no es `accept`, `reject` o `custom`; o falta `value` cuando `action=custom` |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

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
  "message": "Subí el archivo binario a upload_url, luego llamá a /avatar/confirm."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `INVALID_MIME_TYPE` | 400 | `invalid-mime-type` | `mime_type` no es `image/jpeg`, `image/png` o `image/webp` |
| `FILE_TOO_LARGE` | 400 | `file-too-large` | `file_size` > 5242880 bytes |
| `VALIDATION_ERROR` | 400 | `validation-error` | Body malformado o campos requeridos faltantes |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

---

### Confirm Avatar Upload

Verifica que el avatar subido existe en R2 y dispara la validación asíncrona. El avatar **NO** se activa inmediatamente — un worker valida (MIME, tamaño, contenido) y lo activa al finalizar con éxito.
El frontend recibirá evento SSE user.avatar.updated cuando la validación termine.

#### Request

```
POST /v1/user/profile/avatar/confirm
```

**Body:**

| Campo | Tipo | Requerido | Validación | Descripción |
|-------|------|-----------|------------|-------------|
| `storage_key` | string | Sí | — | Clave obtenida en la respuesta de upload |

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
  "message": "Carga de avatar confirmada. Validación en progreso."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `AVATAR_NOT_FOUND` | 404 | `avatar-not-found` | `storage_key` no existe en R2 |
| `VALIDATION_ERROR` | 400 | `validation-error` | Body malformado o falta `storage_key` |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

---

## Documentos

### List Document Types

Retorna el catálogo estático de tipos de documentos soportados. Endpoint público sin autenticación.

#### Request

```
GET /v1/user/profile/documents/types
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

**EL ÚNICO ENDPOINT** que el frontend llama para iniciar el pipeline de documentos. Envía el archivo como `multipart/form-data`. Máximo 20 MB en total(por cada file max 5MB).

El backend ejecuta una verificación de magic bytes en los primeros 512 bytes (rechazo sincrónico si no es un tipo soportado). Si pasa, publica en Dragonfly Streams y responde 202. El pipeline restante es completamente asíncrono.

#### OCR Lifecycle

```
queued → processing (validating → sanitizing → ocr_processing) → completed
                                                                  → rejected
                                                                  → failed
```

| Estado | Significado |
|--------|-------------|
| `queued` | Archivo recibido, magic bytes OK, publicado en pipeline |
| `processing` | En pipeline — sub-estados: `validating`, `sanitizing`, `ocr_processing` |
| `completed` | Pipeline completo, datos extraídos exitosamente |
| `rejected` | No es un documento reconocido (ej: foto de un gato) |
| `failed` | Error técnico durante el procesamiento |

#### Request

```
POST /v1/user/profile/documents
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
  "status": "queued",
  "events_url": "{base_url}/v1/realtime/events",
  "message": "Documento recibido. El procesamiento ha comenzado. Seguí el progreso vía SSE en /v1/realtime/events."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `INVALID_FILE_TYPE` | 400 | `invalid-file-type` | Magic bytes no corresponden a un tipo soportado |
| `FILE_TOO_LARGE` | 400 | `file-too-large` | Archivo > 20 MB |
| `VALIDATION_ERROR` | 400 | `validation-error` | Falta el campo `file` o está malformado |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

---

### List Documents

Lista los documentos del usuario con filtros opcionales.

#### Request

```
GET /v1/user/profile/documents
```

**Query Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `status` | string | No | Filtrar por estado OCR: `queued`, `processing`, `completed`, `rejected`, `failed` |
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
      "verification_status": "verified",
      "created_at": "2026-05-01T10:30:00Z"
    },
    {
      "id": "019d5439-cb43-716d-90b5-51dcbe980909",
      "file_name": "seguro_viaje.pdf",
      "document_type": "travel_insurance",
      "ocr_status": "processing",
      "ocr_confidence": null,
      "verification_status": "unverified",
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
| `documents[].verification_status` | string | Estado de verificación: `verified`, `unverified`, `rejected` |
| `documents[].created_at` | string (ISO 8601) | Fecha de creación |

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `INVALID_ENUM` | 400 | `invalid-enum` | `status` o `document_type` no válidos |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

---

### Get Document

Retorna los metadatos completos y datos extraídos de un documento específico. **NO incluye actualizaciones al perfil médico** — solo datos directamente relacionados con este documento.

#### Request

```
GET /v1/user/profile/documents/:document_id
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
    "issuing_country": "PER",
    "nationality": "ESPANOLA",
    "date_of_birth": "1990-05-15",
    "gender": "M",
    "valid_from": "2020-01-15",
    "valid_until": "2030-01-14"
  },
  "failure_reason": null,
  "verification_status": "verified",
  "verified_at": "2026-05-01T10:35:00Z",
  "verified_by": "019d5439-cb43-716d-90b5-51dcbe980010",
  "created_at": "2026-05-01T10:30:00Z",
  "updated_at": "2026-05-01T10:35:00Z"
}
```

> **Notas sobre los campos:**
> - `mime_type`: el MIME enviado por el frontend.
> - `detected_mime_type`: el MIME detectado por el backend tras validación.
> - `ocr_status`: `queued`, `processing`, `completed`, `rejected`, `failed`.
> - `verification_status`: `pending`, `verified`, `rejected`, `manual_review`, `suspicious`.

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `DOCUMENT_NOT_FOUND` | 404 | `document-not-found` | No existe el documento o no pertenece al usuario |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

---

### Get Document Download URL

Devuelve una URL prefirmada temporal de R2 para que el frontend descargue el archivo directamente. El backend **NO** streamea el contenido.

Solo disponible cuando `ocr_status` es `completed` o `rejected`.

#### Request

```
GET /v1/user/profile/documents/:document_id/download-url
```

**Path Params:**

| Parámetro | Tipo | Requerido | Descripción |
|-----------|------|-----------|-------------|
| `document_id` | string (UUID v7) | Sí | ID del documento |

**Ejemplo:**

```bash
curl -X GET {base_url}/documents/019d5439-cb43-716d-90b5-51dcbe980908/download-url \
  -b "__Secure-access_token=v4.local.eyJ..."
```

#### Responses

##### 200 OK

```json
{
  "download_url": "https://r2.proactrip.com/proactrip-secure/raw/019d...?X-Amz-Algorithm=...",
  "expires_at": "2026-05-06T15:45:00Z",
  "file_name": "pasaporte.pdf"
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `DOCUMENT_NOT_FOUND` | 404 | `document-not-found` | No existe el documento o no pertenece al usuario |
| `DOCUMENT_NOT_READY` | 400 | `document-not-ready` | `ocr_status` no es `completed` ni `rejected` y es `pending`|
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

---

### Delete Document

Elimina el registro del documento y **todos** los archivos asociados en R2 (raw, processed, results).

#### Request

```
DELETE /v1/user/profile/documents/:document_id
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
  "message": "Documento y todos los archivos asociados eliminados correctamente."
}
```

**Response Headers:**

| Header | Valor |
|--------|-------|
| `Cache-Control` | `no-store, private` |

##### Posibles Errores

| Código | HTTP | Problem Type | Cuándo |
|--------|------|-------------|--------|
| `DOCUMENT_NOT_FOUND` | 404 | `document-not-found` | No existe el documento o no pertenece al usuario |
| `TOKEN_INVALID` | 401 | `unauthorized` | Cookie inválida o expirada |
| `INTERNAL_ERROR` | 500 | `internal-error` | Error inesperado |

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
| `POST /v1/user/profile/documents` | 10 req/hora por usuario | Upload de documentos (costoso — pipeline OCR) |
| `POST /v1/user/profile/avatar` | 5 req/hora por usuario | Generación de presigned URLs |
| `POST /v1/user/profile/medical-conflicts/:conflict_id/resolve` | 20 req/hora por usuario | Resolución de conflictos médicos |

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
| `PATCH /v1/user/profile` | `no-store, private` | Mutación de datos |
| `GET /v1/user/profile/travel-preferences` | `no-store, private` | Datos de preferencias del usuario |
| `PATCH /v1/user/profile/travel-preferences` | `no-store, private` | Mutación de datos |
| `GET /v1/user/profile/medical` | `no-store, private` | Datos médicos sensibles |
| `PATCH /v1/user/profile/medical` | `no-store, private` | Mutación de datos médicos |
| `GET /v1/user/profile/medical-conflicts` | `no-store, private` | Conflictos médicos |
| `GET /v1/user/profile/medical-conflicts/:id` | `no-store, private` | Detalle de conflicto médico |
| `POST /v1/user/profile/medical-conflicts/:id/resolve` | `no-store, private` | Mutación de datos médicos |
| `POST /v1/user/profile/avatar` | `no-store, private` | URL prefirmada temporal |
| `POST /v1/user/profile/avatar/confirm` | `no-store, private` | Mutación de avatar |
| `POST /v1/user/profile/documents` | `no-store, private` | Upload de documento |
| `GET /v1/user/profile/documents` | `no-store, private` | Datos de documentos del usuario |
| `GET /v1/user/profile/documents/:document_id` | `no-store, private` | Metadatos de documento |
| `GET /v1/user/profile/documents/:id/download-url` | `no-store, private` | URL prefirmada temporal |
| `DELETE /v1/user/profile/documents/:document_id` | `no-store, private` | Mutación de documento |
| `GET /v1/user/profile/documents/types` | `public, max-age=3600` | Catálogo estático público |

### Agrupación

- **`no-store, private`**: Todos los endpoints que retornan datos del usuario o realizan mutaciones.
- **`public, max-age=3600`**: `GET /v1/user/profile/documents/types` — catálogo estático de tipos de documentos.

---

## Notas de Seguridad

### Cookie-Based Authentication

Toda la autenticación usa cookies HttpOnly (`__Secure-access_token`, `__Secure-refresh_token`) con `SameSite=Lax`. El frontend **nunca** envía headers `Authorization`. Las cookies viajan automáticamente con `credentials: 'include'`. Para OAuth, el Auth module usa temporalmente `SameSite=None` durante el callback cross-origin. Ver [AUTH_API.md](AUTH_API.md) para detalles completos del flujo de autenticación.

### Encriptación de Datos Médicos

Los datos médicos (`blood_type`, `allergies`, `medications`, `conditions`, `vaccinations`, `emergency_contact`, `insurance_info`) se almacenan encriptados en reposo con **ChaCha20-Poly1305** usando una clave de 32 bytes. La encriptación/desencriptación ocurre en el backend — el frontend siempre recibe y envía datos en texto plano. En caso de error de desencriptación, se devuelve `DECRYPTION_ERROR` (500).

### Seguridad del Pipeline de Documentos

1. **Magic bytes sincrónicos**: El backend verifica los primeros 512 bytes antes de aceptar el archivo. Rechazo inmediato si no coincide con tipos soportados.
2. **SanitizerWorker**: Elimina metadatos EXIF de imágenes y limpia PDFs antes del OCR.
3. **R2 Presigned URLs**: URLs temporales con TTL configurable (default 15 min). El backend nunca expone credenciales de R2 al frontend.
4. **Validación post-upload**: El worker de avatar valida MIME, tamaño y contenido antes de activar. Protege contra archivos maliciosos renombrados.

### Expiración de Conflictos Médicos

Los conflictos médicos pendientes (`pending_updates`) expiran automáticamente a los **30 días**. Pasado ese tiempo, el endpoint `POST /v1/user/profile/medical-conflicts/:id/resolve` devuelve `PENDING_UPDATE_EXPIRED` (400). Esto previene acumulación infinita de conflictos no resueltos.

### Verificación de Documentos (Admin)

La verificación de documentos es un flujo administrativo. Los endpoints correspondientes están documentados en [DASHBOARD_API.md](DASHBOARD_API.md). El User module solo expone endpoints de consulta para el usuario autenticado.

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
