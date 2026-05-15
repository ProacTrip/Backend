// DTOs de respuesta para feature limits del dashboard.
package feature_limits

import "time"

// =============================================================================
// FeatureLimitResponse — item individual
// =============================================================================

// FeatureLimitResponse es el DTO de respuesta para un límite de feature.
// FL-SPEC-001: cada límite mapea (user_id, feature_key) → limit_value.
type FeatureLimitResponse struct {
	FeatureKey string `json:"feature_key"`
	// LimitValue: nil = ilimitado, 0 = bloqueado, >0 = cuota.
	LimitValue *int   `json:"limit_value,omitzero"`
	Window     string `json:"window,omitzero"`
}

// =============================================================================
// FeatureLimitsListResponse — lista de límites
// =============================================================================

// FeatureLimitsListResponse es la respuesta para listados de límites (usuario o rol).
type FeatureLimitsListResponse struct {
	Limits []FeatureLimitResponse `json:"limits"`
}

// =============================================================================
// FeatureLimitRow — forma de scan de DB (privado al usecase)
// =============================================================================

// FeatureLimitRow es el resultado de scan de DB para límites de feature.
type FeatureLimitRow struct {
	FeatureKey string
	LimitValue *int
	Window     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
