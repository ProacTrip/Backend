// Handler HTTP para GET /v1/user/favorites.
package list_favorites

import (
	"net/http"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler procesa GET /v1/user/favorites?entity_type=hotel.
type Handler struct {
	usecase *UseCase
}

func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle lista favoritos del usuario con filtro opcional por entity_type.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	claims, err := echo.ContextGet[*token.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	// Filtro opcional por query param
	entityTypeStr := c.QueryParam("entity_type")

	var entityTypeFilter *string
	if entityTypeStr != "" {
		entityTypeFilter = &entityTypeStr
	}

	resp, err := h.usecase.Execute(c.Request().Context(), claims.UserID.String(), entityTypeFilter)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, resp)
}
