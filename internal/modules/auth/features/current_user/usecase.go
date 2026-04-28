package current_user

import (
	"context"
	"log/slog"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// Lógica de negocio para obtener el usuario actual.
// Flujo: validar token → buscar usuario → retornar datos.

type TokenService interface {
	ValidateAccessToken(ctx context.Context, tokenString string) (*token.AccessClaims, error)
}

type Response struct {
	User    *UserResponse `json:"user"`
	Context *ContextData  `json:"context"`
}

type UseCase struct {
	repo     domain.UserRepository
	tokenSvc TokenService
}

type UseCaseDeps struct {
	Repo     domain.UserRepository
	TokenSvc TokenService
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		repo:     deps.Repo,
		tokenSvc: deps.TokenSvc,
	}
}

func (uc *UseCase) Execute(ctx context.Context, tokenString string) (*Response, error) {
	if tokenString == "" {
		return nil, nil
	}

	claims, err := uc.tokenSvc.ValidateAccessToken(ctx, tokenString)
	if err != nil {
		slog.Warn("current_user: token validation failed", "error", err)
		return nil, nil
	}

	user, err := uc.repo.GetByID(ctx, claims.UserID)
	if err != nil {
		slog.Warn("current_user: user not found", "error", err, "user_id", claims.UserID)
		return nil, nil
	}

	slog.Debug("current_user: authenticated", "email", user.Email)

	return &Response{
		User: &UserResponse{
			Email:         user.Email,
			EmailVerified: user.EmailVerified,
			RoleName:      user.RoleName,
		},
		Context: defaultContext(),
	}, nil
}

func defaultContext() *ContextData {
	return &ContextData{
		Location: LocationData{
			Country:     "Unknown",
			CountryCode: "XX",
			CountryName: "Unknown",
			City:        "Unknown",
			Timezone:    "UTC",
			Currency:    "USD",
			Language:    "en",
		},
		Weather: WeatherData{
			Temp:      20.0,
			Condition: "clear",
			Humidity:  50,
		},
	}
}
