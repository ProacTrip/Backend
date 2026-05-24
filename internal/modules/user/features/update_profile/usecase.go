// Caso de uso: Actualizar perfil de usuario (PUT /v1/user/profile).
// Valida género y nacionalidad antes de actualizar.
package update_profile

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Ports
// =============================================================================

type ProfileRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error)
	Update(ctx context.Context, profile *domain.UserProfile) error
}

type EventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

// =============================================================================
// UseCase
// =============================================================================

type UseCaseDeps struct {
	ProfileRepo    ProfileRepo
	EventPublisher EventPublisher // nil-safe: si es nil, no se publican eventos
}

type UseCase struct {
	profileRepo    ProfileRepo
	eventPublisher EventPublisher
	wg             sync.WaitGroup
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		profileRepo:    deps.ProfileRepo,
		eventPublisher: deps.EventPublisher,
	}
}

// Wait espera a que todos los eventos publicados asíncronamente terminen.
func (uc *UseCase) Wait() { uc.wg.Wait() }

// Execute valida el comando y actualiza el perfil con un update parcial.
// Solo los campos no-nil en el comando se aplican.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) error {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	// 1. Verificar que el perfil existe
	existing, err := uc.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get profile by user_id: %w", err)
	}
	if existing == nil {
		return domain.ErrProfileNotFound // sentinel directo — el error mapper hace errors.Is
	}

	// 2. Validar género si se envió
	if cmd.Gender != nil {
		if !isValidGender(*cmd.Gender) {
			return domain.ErrInvalidGender
		}
	}

	// 3. Validar nacionalidad (ISO 3166-1 alpha-2, 2 letras).
	// La validación completa contra una lista de países se difiere.
	if cmd.Nationality != nil {
		if len(*cmd.Nationality) != 2 {
			return domain.ErrInvalidCountryCode
		}
	}

	// 4. Validar language code (ISO 639, 2-5 caracteres)
	if cmd.Language != nil {
		if len(*cmd.Language) < 2 || len(*cmd.Language) > 5 {
			return domain.ErrInvalidLanguageCode
		}
	}

	// 5. Validar currency code (ISO 4217, 3 caracteres)
	if cmd.Currency != nil {
		if len(*cmd.Currency) != 3 {
			return domain.ErrInvalidCurrencyCode
		}
	}

	// 6. Validar teléfono E.164 si se envió
	if cmd.Phone != nil && !IsValidPhone(cmd.Phone) {
		return domain.ErrInvalidPhone
	}

	// 7. Merge command fields into existing profile
	if cmd.FirstName != nil { existing.FirstName = cmd.FirstName }
	if cmd.LastName != nil { existing.LastName = cmd.LastName }
	if cmd.Gender != nil {
		g := domain.Gender(*cmd.Gender)
		existing.Gender = &g
	}
	if cmd.DateOfBirth != nil { existing.DateOfBirth = cmd.DateOfBirth }
	if cmd.Nationality != nil { existing.Nationality = cmd.Nationality }
	if cmd.Phone != nil { existing.Phone = cmd.Phone }
	if cmd.Bio != nil { existing.Bio = cmd.Bio }
	if cmd.Language != nil { existing.LanguageCode = *cmd.Language }
	if cmd.Currency != nil { existing.CurrencyCode = *cmd.Currency }

	// 8. Update en DB
	if err := uc.profileRepo.Update(ctx, existing); err != nil {
		return fmt.Errorf("update profile: %w", err)
	}

	// 8. Emitir evento (best-effort)
	if uc.eventPublisher != nil {
		uc.wg.Go(func() {
			bgCtx := context.WithoutCancel(ctx)
			_, err := uc.eventPublisher.Publish(bgCtx,
				eventbus.StreamName("user.profile.updated"),
				map[string]interface{}{
					"user_id": userID.String(),
				},
			)
			if err != nil {
				slog.WarnContext(bgCtx, "publish profile updated event failed",
					slog.String("user_id", userID.String()),
					slog.String("error", err.Error()),
				)
			}
		})
	}

	return nil
}

// isValidGender verifica que el valor esté en el enum de Gender.
func isValidGender(g string) bool {
	switch domain.Gender(g) {
	case domain.GenderMale, domain.GenderFemale, domain.GenderNonBinary, domain.GenderPreferNotToSay:
		return true
	default:
		return false
	}
}
