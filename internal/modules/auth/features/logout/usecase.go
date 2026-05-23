package logout

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// Lógica de negocio del logout.
// Agrega JTI a blacklist en Dragonfly. Single-session: invalida únicamente el access token actual.

type TokenService interface {
	ValidateAccessToken(ctx context.Context, tokenString string) (*token.AccessClaims, error)
}

type UseCase struct {
	tokenSvc    TokenService
	dragonflyDB *redis.Client
}

type UseCaseDeps struct {
	TokenSvc    TokenService
	DragonflyDB *redis.Client
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		tokenSvc:    deps.TokenSvc,
		dragonflyDB: deps.DragonflyDB,
	}
}

func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	claims, err := uc.tokenSvc.ValidateAccessToken(ctx, cmd.Token)
	if err != nil {
		slog.DebugContext(ctx, "logout: token validation failed, clearing cookies anyway",
			slog.String("error", err.Error()),
		)
		return &Response{Message: "Logged out successfully."}, nil
	}

	blacklistKey := fmt.Sprintf("{auth}:blacklist:jti:%s", claims.JTI.String())
	blacklistTTL := 15 * time.Minute

	if err := uc.dragonflyDB.Set(ctx, blacklistKey, "1", blacklistTTL).Err(); err != nil {
		slog.ErrorContext(ctx, "logout: failed to blacklist JTI",
			slog.String("jti", claims.JTI.String()),
			slog.String("error", err.Error()),
		)
		return nil, domain.ErrSessionNotFound
	}

	return &Response{Message: "Logged out successfully."}, nil
}


