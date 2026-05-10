package me

import (
	"context"
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
// busca el usuario y retorna sus datos públicos.
type Handler struct {
	tokenSvc TokenService
	userRepo UserRepository
}

// NewHandler crea un nuevo Handler.
func NewHandler(tokenSvc TokenService, userRepo UserRepository) *Handler {
	return &Handler{tokenSvc: tokenSvc, userRepo: userRepo}
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

	return c.JSON(http.StatusOK, Response{
		User: UserResponse{
			ID:            user.ID,
			Email:         user.Email,
			EmailVerified: user.EmailVerified,
			RoleName:      user.RoleName,
		},
	})
}
