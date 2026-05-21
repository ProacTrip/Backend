// Caso de uso: Obtener perfil de usuario.
// Consulta profile desde el repositorio.
package get_profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports (interfaces requeridas por el usecase)
// =============================================================================

type ProfileRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error)
}

// =============================================================================
// UseCase — Consulta el perfil del usuario
// =============================================================================

type UseCaseDeps struct {
	ProfileRepo ProfileRepo
}

type UseCase struct {
	profileRepo ProfileRepo
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		profileRepo: deps.ProfileRepo,
	}
}

// Execute consulta el perfil del usuario.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*GetProfileResponse, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	profile, err := uc.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrProfileNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}
	if profile == nil {
		return nil, domain.ErrProfileNotFound
	}

	// Construir respuesta flat
	resp := &GetProfileResponse{
		ID:          profile.ID,
		UserID:      profile.UserID,
		Email:       profile.Email,
		FirstName:   profile.FirstName,
		LastName:    profile.LastName,
		Gender:      genderToString(profile.Gender),
		Nationality: profile.Nationality,
		Phone:       profile.Phone,
		Bio:       profile.Bio,
		AvatarURL: profile.AvatarURL,
		Location: LocationResponse{
			Currency: profile.CurrencyCode,
			Language: profile.LanguageCode,
		},
	}

	if profile.DateOfBirth != nil {
		dobStr := profile.DateOfBirth.Format("2006-01-02")
		resp.DateOfBirth = &dobStr
	}

	return resp, nil
}

// =============================================================================
// Helpers
// =============================================================================

func genderToString(g *domain.Gender) *string {
	if g == nil {
		return nil
	}
	s := string(*g)
	return &s
}
