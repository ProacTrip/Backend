package login

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// Lógica de negocio del login.
// Flujo: validar email → verificar contraseña → registrar intento → generar tokens.

type PasswordService interface {
	Verify(password, encoded string) (bool, error)
}

type TokenService interface {
	GenerateTokenPair(userID uuid.UUID, email string, role string, roleID uuid.UUID) (*token.TokenPair, error)
}

// =============================================================================
// UseCase - Lógica de negocio del login
// =============================================================================

type UseCase struct {
	repo     domain.UserRepository
	hasher   PasswordService
	tokenSvc TokenService
}

type UseCaseDeps struct {
	Repo     domain.UserRepository
	Hasher   PasswordService
	TokenSvc TokenService
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		repo:     deps.Repo,
		hasher:   deps.Hasher,
		tokenSvc: deps.TokenSvc,
	}
}

func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	user, err := uc.repo.GetByEmail(ctx, cmd.Email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}

	if !user.EmailVerified {
		return nil, domain.ErrEmailNotVerified
	}

	// Verificar estado de la cuenta ANTES de validar contraseña.
	// AS-SPEC-006: login rechaza cuentas disabled, suspended, y pending_verification.
	// Esto es siempre enforce (no depende de AUTHZ_ENFORCE_MODE) porque es un login nuevo.
	switch user.Status {
	case domain.StatusDisabled:
		return nil, domain.ErrAccountDisabled
	case domain.StatusSuspended:
		return nil, domain.ErrAccountSuspended
	case domain.StatusPendingVerification:
		return nil, domain.ErrEmailNotVerified
	}

	user.MaybeUnlock()
	if user.IsLocked() {
		return nil, domain.ErrAccountLocked
	}

	valid, err := uc.hasher.Verify(cmd.Password, user.PasswordHash)
	if err != nil || !valid {
		user.RecordFailedLogin(5, 15*time.Minute)
		if updateErr := uc.repo.Update(ctx, user); updateErr != nil {
			slog.ErrorContext(ctx, "failed to record failed login attempt",
				slog.String("email", cmd.Email),
				slog.Any("error", updateErr),
			)
		}
		return nil, domain.ErrInvalidCredentials
	}

	tokenPair, err := uc.tokenSvc.GenerateTokenPair(user.ID, user.Email, user.RoleName, user.RoleID)
	if err != nil {
		return nil, fmt.Errorf("generar tokens de sesión: %w", err)
	}

	user.RecordLogin()
	if err := uc.repo.Update(ctx, user); err != nil {
		// Login recording is informational — don't fail authentication
		slog.ErrorContext(ctx, "failed to update user login record",
			slog.String("user_id", user.ID.String()),
			slog.Any("error", err),
		)
	}

	return &Response{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		User: &UserResponse{
			ID:       user.ID,
			Email:    user.Email,
			RoleName: user.RoleName,
		},
	}, nil
}
