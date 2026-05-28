// Handler para GET /v1/auth/me.
// Devuelve la identidad del usuario autenticado sin restricción de rol.
// Usa los claims del token PASETO inyectados por AuthMiddleware.
package me

import (
	"net/http"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/labstack/echo/v5"
)

// Handler maneja GET /v1/auth/me.
type Handler struct{}

// NewHandler crea un nuevo handler para /v1/auth/me.
func NewHandler() *Handler {
	return &Handler{}
}

// Handle procesa GET /v1/auth/me.
func (h *Handler) Handle(c *echo.Context) error {
	claims, err := sharedauth.GetAccessClaims(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error": "not authenticated",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":          claims.UserID.String(),
			"email":       claims.Email,
			"role_name":   claims.Role,
			"permissions": claims.Permissions,
		},
	})
}
