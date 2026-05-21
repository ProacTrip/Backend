// Domain: Entidades y tipos de dominio para perfiles de usuario.
// Define la estructura del perfil y sus estados.
package domain

import (
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
