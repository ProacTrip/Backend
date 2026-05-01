package verify_email

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/verification"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// Lógica de negocio de verificación de email.
// Flujo: validar token → buscar usuario → marcar verificado → generar tokens.

type VerificationService interface {
	VerifyToken(ctx context.Context, tokenString string) (*verification.TokenClaims, error)
}

type TokenService interface {
	GenerateTokenPair(userID uuid.UUID, email string, roleID, sessionID uuid.UUID) (*token.TokenPair, error)
}

type UseCase struct {
	verifySvc VerificationService
	repo      domain.UserRepository
	tokenSvc  TokenService
}

type UseCaseDeps struct {
	VerifySvc VerificationService
	Repo      domain.UserRepository
	TokenSvc  TokenService
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		verifySvc: deps.VerifySvc,
		repo:      deps.Repo,
		tokenSvc:  deps.TokenSvc,
	}
}

func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	claims, err := uc.verifySvc.VerifyToken(ctx, cmd.Token)
	if err != nil {
		return nil, fmt.Errorf("verify email: %w", domain.ErrTokenInvalid)
	}

	user, err := uc.repo.GetByEmail(ctx, claims.Email)
	if err != nil {
		return nil, fmt.Errorf("verify email: %w", err)
	}

	user.VerifyEmail()
	if err := uc.repo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("verify email: update user: %w", err)
	}

	sessionID := uuid.Must(uuid.NewV7())
	tokenPair, err := uc.tokenSvc.GenerateTokenPair(user.ID, user.Email, user.RoleID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("verify email: generate tokens: %w", err)
	}

	return &Response{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		User: UserResponse{
			Email:         user.Email,
			EmailVerified: true,
			RoleName:      user.RoleName,
		},
	}, nil
}
