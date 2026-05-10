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
// Agrega JTI a blacklist en Dragonfly. Soporta logout-all para invalidar todas las sesiones.

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

	if cmd.LogoutAll {
		sessionSetKey := fmt.Sprintf("{auth}:sessions:%s", claims.UserID.String())
		if err := uc.dragonflyDB.Del(ctx, sessionSetKey).Err(); err != nil {
			slog.WarnContext(ctx, "logout_all: failed to clear session set",
				slog.String("user_id", claims.UserID.String()),
				slog.String("error", err.Error()),
			)
		}

		revokeKey := fmt.Sprintf("{auth}:revoked_at:%s", claims.UserID.String())
		uc.dragonflyDB.Set(ctx, revokeKey, time.Now().Unix(), 7*24*time.Hour)
	}

	return &Response{Message: "Logged out successfully."}, nil
}


