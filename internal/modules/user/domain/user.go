// Domain: Entidades y tipos de dominio para perfiles de usuario.
// Define la estructura del perfil y sus estados.
package domain

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Constantes: valores por defecto del perfil
// =============================================================================

// Valores por defecto usados como fallback cuando no hay EnvPrefs del evento
// de registro ni llamada a UpsertProfile. Son sobreescritos por:
//   - Evento UserRegistered con EnvPrefs (detección de entorno)
//   - Caso de uso UpsertProfile
const (
	DefaultLanguage = "es"
	DefaultCurrency = "EUR"
	DefaultRole     = "client"
)

// =============================================================================
// Tipos de perfil de usuario
// =============================================================================

// UserProfileStatus representa el estado del perfil
type UserProfileStatus string

const (
	ProfileStatusActive    UserProfileStatus = "active"
	ProfileStatusInactive  UserProfileStatus = "inactive"
	ProfileStatusSuspended UserProfileStatus = "suspended"
	ProfileStatusDeleted   UserProfileStatus = "deleted"
)

// Gender representa el género del usuario
type Gender string

const (
	GenderMale           Gender = "male"
	GenderFemale         Gender = "female"
	GenderNonBinary      Gender = "non_binary"
	GenderPreferNotToSay Gender = "prefer_not_to_say"
)

// UserProfile representa el perfil de usuario (alineado con migración)
type UserProfile struct {
	ID              uuid.UUID  `json:"id"`      // PK - diferente de user_id
	UserID          uuid.UUID  `json:"user_id"` // FK al dominio Auth
	Email           string     `json:"email"` // denormalized from registration event
	FirstName       *string    `json:"first_name,omitzero"`
	LastName        *string    `json:"last_name,omitzero"`
	DateOfBirth     *time.Time `json:"date_of_birth,omitzero"`
	Gender          *Gender    `json:"gender,omitzero"`
	Nationality     *string    `json:"nationality,omitzero"`
	Phone        *string `json:"phone,omitzero"`
	AvatarURL    *string `json:"avatar_url,omitzero"`
	Bio          *string `json:"bio,omitzero"`
	LanguageCode string  `json:"language_code"` // NOT NULL DEFAULT 'es'
	CurrencyCode string  `json:"currency_code"` // NOT NULL DEFAULT 'EUR'
	Role         string           `json:"role,omitzero"`  // "client" or "admin", default "client"
	Status       UserProfileStatus `json:"status,omitzero"` // default "active"
	OcrPopulated bool              `json:"ocr_populated"`   // true cuando OCR ya pobló datos personales
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

// =============================================================================
// Métodos de dominio
// =============================================================================

// NewUserProfile crea un nuevo perfil de usuario.
// email: viene del evento UserRegistered (denormalizado para evitar cross-DB joins).
// IMPORTANTE: Usa user_id (FK al auth) diferente del id (PK auto-generado)
func NewUserProfile(userID uuid.UUID, email string) *UserProfile {
	now := time.Now()
	return &UserProfile{
		ID:           uuid.Must(uuid.NewV7()),
		UserID:       userID,
		Email:        email,
		LanguageCode: DefaultLanguage,
		CurrencyCode: DefaultCurrency,
		Role:         DefaultRole,
		Status:       ProfileStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// SetName establece nombre y apellido
func (p *UserProfile) SetName(firstName, lastName *string) {
	p.FirstName = firstName
	p.LastName = lastName
	p.UpdatedAt = time.Now()
}

// SetPersonalInfo establece información personal
func (p *UserProfile) SetPersonalInfo(dateOfBirth *time.Time, gender *Gender, nationality *string, bio *string) {
	p.DateOfBirth = dateOfBirth
	p.Gender = gender
	p.Nationality = nationality
	p.Bio = bio
	p.UpdatedAt = time.Now()
}

// SetPreferences establece preferencias de idioma y moneda.
func (p *UserProfile) SetPreferences(language, currency *string) {
	if language != nil {
		p.LanguageCode = *language
	}
	if currency != nil {
		p.CurrencyCode = *currency
	}
	p.UpdatedAt = time.Now()
}

// SetAvatar establece la URL del avatar
func (p *UserProfile) SetAvatar(avatarURL *string) {
	p.AvatarURL = avatarURL
	p.UpdatedAt = time.Now()
}

// PopulateFromOCR rellena datos personales desde un documento de identidad (pasaporte, DNI, etc.).
// Solo aplica si OcrPopulated == false — one-time population.
// El nombre se divide asumiendo formato español: APELLIDOS Nombres (surnames first).
// Con 3+ palabras: primeras N-2 = apellidos, últimas 2 = nombres.
// Con 2 palabras: asume formato inglés (first = nombre, second = apellido).
func (p *UserProfile) PopulateFromOCR(extracted *ExtractedData) bool {
	if p.OcrPopulated {
		return false
	}
	changed := false
	if extracted.FullName != nil && *extracted.FullName != "" {
		parts := strings.Fields(*extracted.FullName)
		var firstName, lastName string
		switch {
		case len(parts) >= 4:
			// Formato español con 2+ apellidos y 2 nombres: SURNAME1 SURNAME2 NAME1 NAME2
			firstName = strings.Join(parts[len(parts)-2:], " ")
			lastName = strings.Join(parts[:len(parts)-2], " ")
		case len(parts) == 3:
			// SURNAME1 SURNAME2 NAME o SURNAME NAME1 NAME2
			// Asumir 2 apellidos + 1 nombre (más común en docs españoles)
			firstName = parts[len(parts)-1]
			lastName = strings.Join(parts[:len(parts)-1], " ")
		case len(parts) == 2:
			// Formato inglés: NAME SURNAME
			firstName = parts[0]
			lastName = parts[1]
		default:
			firstName = parts[0]
		}
		p.FirstName = &firstName
		if lastName != "" {
			p.LastName = &lastName
		}
		changed = true
	}
	if extracted.DateOfBirth != nil && *extracted.DateOfBirth != "" {
		if dob, err := parseDate(*extracted.DateOfBirth); err == nil {
			p.DateOfBirth = &dob
			changed = true
		} else {
			slog.Warn("ocr: unparseable date_of_birth, field not applied to profile",
				"raw", *extracted.DateOfBirth,
				"error", err,
			)
		}
	}
	if extracted.Nationality != nil && *extracted.Nationality != "" {
		p.Nationality = extracted.Nationality
		changed = true
	}
	if extracted.Gender != nil && *extracted.Gender != "" {
		g := mapGender(*extracted.Gender)
		p.Gender = &g
		changed = true
	}
	if changed {
		p.OcrPopulated = true
		p.UpdatedAt = time.Now()
	}
	return changed
}

// FullName retorna el nombre completo
func (p *UserProfile) FullName() string {
	if p.FirstName == nil && p.LastName == nil {
		return ""
	}
	var full string
	if p.FirstName != nil {
		full = *p.FirstName
	}
	if p.LastName != nil {
		if full != "" {
			full += " "
		}
		full += *p.LastName
	}
	return full
}

// =============================================================================
// Environment prefs extracted from registration event / geoip
// =============================================================================

// EnvPrefs holds environment-based preferences extracted from the user
// registration event (language_code, currency_code, country_code, timezone_name).
// All fields are optional — empty means "not provided, use defaults".
// TimezoneName and CountryCode are used for cache only, not persisted to profile.
type EnvPrefs struct {
	LanguageCode string
	CurrencyCode string
	CountryCode  string // for cache only, not persisted to profile column
	TimezoneName string // for cache only, not persisted to profile column
}

// HasAny returns true if at least one preference is non-empty.
func (e EnvPrefs) HasAny() bool {
	return e.LanguageCode != "" || e.CurrencyCode != "" || e.CountryCode != "" || e.TimezoneName != ""
}

// =============================================================================
// Helpers para PopulateFromOCR
// =============================================================================

// parseDate intenta parsear una fecha en formatos comunes de documentos,
// incluyendo meses textuales en inglés y español (OCR y AI a veces los mezclan).
func parseDate(raw string) (time.Time, error) {
	// Normalizar: AI/OCR a veces concatena abreviaturas bilingües (ej: "AGOIAUG")
	raw = normalizeBilingualMonth(raw)

	formats := []string{
		"2006-01-02",          // ISO
		"02/01/2006",          // DD/MM/YYYY
		"02 01 2006",          // DD MM YYYY
		"2006/01/02",          // YYYY/MM/DD
		"02-01-2006",          // DD-MM-YYYY
		"20060102",            // YYYYMMDD
		"02 Jan 2006",         // English 3-letter month
		"2 Jan 2006",          // English 3-letter month (1-2 digit day)
		"02 January 2006",     // English full month
		"2 January 2006",      // English full month (1-2 digit day)
	}
	for _, f := range formats {
		if t, err := time.Parse(f, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable date: %s", raw)
}

// normalizeBilingualMonth corrige corrupciones donde la AI concatena
// abreviaturas de mes en dos idiomas (ej: "14 AGOIAUG 2004" → "14 Aug 2004").
// También traduce abreviaturas en español al inglés (ej: "ENE" → "Jan").
func normalizeBilingualMonth(raw string) string {
	// Spanish → English month abbreviation mapping
	esToEn := map[string]string{
		"ENE": "Jan", "FEBR": "Feb", "MAR": "Mar", "ABR": "Apr",
		"MAY": "May", "JUN": "Jun", "JUL": "Jul", "AGO": "Aug",
		"SEPT": "Sep", "SEP": "Sep", "OCT": "Oct", "NOV": "Nov",
		"NOVI": "Nov", "DIC": "Dec",
	}
	monthAbbrs := map[string]bool{
		"JAN": true, "FEB": true, "MAR": true, "APR": true,
		"MAY": true, "JUN": true, "JUL": true, "AUG": true,
		"SEP": true, "OCT": true, "NOV": true, "DEC": true,
		"ENE": true, "FEBR": true, "ABR": true, "AGO": true,
		"SEPT": true, "NOVI": true, "DIC": true,
	}
	parts := strings.Fields(raw)
	for i, part := range parts {
		upper := strings.ToUpper(part)
		// Encontrar la abreviatura de mes más larga que aparece al inicio o final
		lastMatch := ""
		for abbr := range monthAbbrs {
			if (strings.HasPrefix(upper, abbr) || strings.HasSuffix(upper, abbr)) && len(abbr) > len(lastMatch) {
				lastMatch = abbr
			}
		}
		if lastMatch != "" {
			// Traducir español → inglés para que time.Parse lo entienda
			if en, ok := esToEn[lastMatch]; ok {
				parts[i] = en
			} else {
				parts[i] = lastMatch
			}
		}
	}
	return strings.Join(parts, " ")
}

// mapGender mapea representaciones comunes de género a los valores del dominio.
func mapGender(raw string) Gender {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "M", "MALE", "MASCULINO", "HOMBRE", "VARON", "VARÓN":
		return GenderMale
	case "F", "FEMALE", "FEMENINO", "MUJER":
		return GenderFemale
	case "X", "NB", "NON_BINARY", "NO_BINARIO", "NO_BINARIE":
		return GenderNonBinary
	default:
		return GenderPreferNotToSay
	}
}
