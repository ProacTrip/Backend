package me

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// TokenService valida un access token y retorna los claims.
type TokenService interface {
	ValidateAccessToken(ctx context.Context, tokenStr string) (*token.AccessClaims, error)
}

// UserRepository obtiene usuarios por ID.
type UserRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

// Handler HTTP para GET /v1/auth/me.
// Extrae el access_token de la cookie, valida el token,
// busca el usuario, enriquece con datos del perfil y retorna sus datos públicos.
type Handler struct {
	tokenSvc        TokenService
	userRepo        UserRepository
	profileProvider UserProfileProvider
}

// NewHandler crea un nuevo Handler.
// profileProvider puede ser nil — en ese caso avatar_url siempre será null.
func NewHandler(tokenSvc TokenService, userRepo UserRepository, profileProvider UserProfileProvider) *Handler {
	return &Handler{tokenSvc: tokenSvc, userRepo: userRepo, profileProvider: profileProvider}
}

// Handle procesa GET /v1/auth/me.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	// Extract token from cookie — try secure prefix first, then dev fallback
	cookie, err := c.Cookie("__Secure-access_token")
	if err != nil {
		cookie, err = c.Cookie("access_token")
	}
	if err != nil {
		return httperr.MapError(c, domain.ErrNotAuthenticated)
	}

	// Validate token
	claims, err := h.tokenSvc.ValidateAccessToken(c.Request().Context(), cookie.Value)
	if err != nil {
		return httperr.MapError(c, domain.ErrTokenInvalid)
	}

	// Get user
	user, err := h.userRepo.GetByID(c.Request().Context(), claims.UserID)
	if err != nil {
		return httperr.MapError(c, err)
	}

	// Enriquecer con avatar_url del perfil de usuario (módulo user).
	// El perfil se crea asíncronamente por el consumer de eventos → puede no existir aún.
	// nil y errores se tratan como "sin avatar" (null) — no fallan el request.
	var avatarURL *string
	if h.profileProvider != nil {
		profile, profileErr := h.profileProvider.GetByUserID(c.Request().Context(), user.ID)
		if profileErr != nil {
			slog.Warn("me: failed to fetch user profile, avatar_url will be null",
				"user_id", user.ID.String(),
				"error", profileErr)
		} else if profile != nil {
			avatarURL = profile.AvatarURL
		}
	}

	return c.JSON(http.StatusOK, Response{
		User: UserResponse{
			ID:            user.ID,
			Email:         user.Email,
			EmailVerified: user.EmailVerified,
			RoleName:      user.RoleName,
			AvatarURL:     avatarURL,
		},
	})
}
