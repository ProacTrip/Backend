// Adapter PostgreSQL para preferencias de viaje.
// Implementa domain.TravelPrefsRepository.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// TravelPrefsRepository — PostgreSQL adapter
// Alineado con migración user_travel_preferences
// =============================================================================

// Compile-time interface check
var _ domain.TravelPrefsRepository = (*TravelPrefsRepository)(nil)

type TravelPrefsRepository struct {
	db *pgxpool.Pool
}

func NewTravelPrefsRepository(db *pgxpool.Pool) *TravelPrefsRepository {
	return &TravelPrefsRepository{db: db}
}

// =============================================================================
// Create — Inserta nuevas preferencias con valores por defecto
// =============================================================================

func (r *TravelPrefsRepository) Create(ctx context.Context, prefs *domain.TravelPreferences) error {
	query := `
		INSERT INTO user_travel_preferences (
			id, user_id, preferred_class, seat_preference, meal_preference,
			special_assistance, preferred_airlines, preferred_hotels,
			avoid_layovers, max_layover_duration, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (user_id) DO NOTHING
	`

	_, err := r.db.Exec(ctx, query,
		prefs.ID,
		prefs.UserID,
		string(prefs.PreferredClass),
		prefs.SeatPreference,
		prefs.MealPreference,
		prefs.SpecialAssistance,
		prefs.PreferredAirlines,
		prefs.PreferredHotels,
		prefs.AvoidLayovers,
		prefs.MaxLayoverDuration,
		prefs.CreatedAt,
		prefs.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create travel preferences: %w", err)
	}
	return nil
}

// =============================================================================
// GetByUserID — Recupera preferencias por user_id (1:1)
// =============================================================================

func (r *TravelPrefsRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.TravelPreferences, error) {
	query := `
		SELECT id, user_id, preferred_class, seat_preference, meal_preference,
		       special_assistance, preferred_airlines, preferred_hotels,
		       avoid_layovers, max_layover_duration, created_at, updated_at
		FROM user_travel_preferences
		WHERE user_id = $1
	`

	var tp domain.TravelPreferences
	var seatPref *domain.SeatPreference
	var mealPref *string
	var maxLayover *int

	err := r.db.QueryRow(ctx, query, userID).Scan(
		&tp.ID,
		&tp.UserID,
		&tp.PreferredClass,
		&seatPref,
		&mealPref,
		&tp.SpecialAssistance,
		&tp.PreferredAirlines,
		&tp.PreferredHotels,
		&tp.AvoidLayovers,
		&maxLayover,
		&tp.CreatedAt,
		&tp.UpdatedAt,
	)

	tp.SeatPreference = seatPref
	tp.MealPreference = mealPref
	tp.MaxLayoverDuration = maxLayover

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrTravelPrefsNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get travel preferences by user_id: %w", err)
	}

	return &tp, nil
}

// =============================================================================
// Update — Merge no sobrescribe: solo los campos no-nil se actualizan
// =============================================================================

func (r *TravelPrefsRepository) Update(ctx context.Context, prefs *domain.TravelPreferences) error {
	query := `
		UPDATE user_travel_preferences SET
			preferred_class      = COALESCE(NULLIF($2, ''), preferred_class),
			seat_preference      = COALESCE($3, seat_preference),
			meal_preference      = COALESCE($4, meal_preference),
			special_assistance   = COALESCE($5, special_assistance),
			preferred_airlines   = COALESCE($6, preferred_airlines),
			preferred_hotels     = COALESCE($7, preferred_hotels),
			avoid_layovers       = COALESCE($8, avoid_layovers),
			max_layover_duration = COALESCE($9, max_layover_duration),
			updated_at           = NOW()
		WHERE user_id = $1
	`

	result, err := r.db.Exec(ctx, query,
		prefs.UserID,
		string(prefs.PreferredClass),
		prefs.SeatPreference,
		prefs.MealPreference,
		prefs.SpecialAssistance,
		prefs.PreferredAirlines,
		prefs.PreferredHotels,
		prefs.AvoidLayovers,
		prefs.MaxLayoverDuration,
	)
	if err != nil {
		return fmt.Errorf("update travel preferences: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrProfileNotFound
	}
	return nil
}
