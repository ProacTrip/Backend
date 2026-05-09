package login

import (
	"context"
	"log/slog"
	"strings"
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
	GenerateTokenPair(userID uuid.UUID, email string, role string, roleID, sessionID uuid.UUID) (*token.TokenPair, error)
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
	if !strings.Contains(cmd.Email, "@") {
		return nil, domain.ErrInvalidEmail
	}

	if len(cmd.Password) < 8 {
		return nil, domain.ErrPasswordTooShort
	}

	user, err := uc.repo.GetByEmail(ctx, cmd.Email)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	if !user.EmailVerified {
		return nil, domain.ErrEmailNotVerified
	}

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

	sessionID := uuid.Must(uuid.NewV7())
	tokenPair, err := uc.tokenSvc.GenerateTokenPair(user.ID, user.Email, user.RoleName, user.RoleID, sessionID)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	user.RecordLogin()
	if updateErr := uc.repo.Update(ctx, user); updateErr != nil {
		slog.ErrorContext(ctx, "failed to record successful login",
			slog.String("email", cmd.Email),
			slog.Any("error", updateErr),
		)
	}

	return &Response{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		User: &UserResponse{
			ID:            user.ID,
			Email:         user.Email,
			EmailVerified: true,
			RoleName:      user.RoleName,
		},
	}, nil
}
