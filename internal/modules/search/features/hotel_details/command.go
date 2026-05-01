// DTO de entrada para detalles de hotel.
// Valida el property_token requerido.
package hotel_details

import (
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Command — DTO de entrada para hotel details
// =============================================================================

// Command is the input DTO for hotel details.
type Command struct {
	ID              string `json:"id"`
	CheckInDate     string `json:"check_in_date"`
	CheckOutDate    string `json:"check_out_date"`
	Adults          int    `json:"adults"`
	Children        int    `json:"children"`
	ChildrenAges    []int  `json:"children_ages"`
	GL              string `json:"gl"`
	HL              string `json:"hl"`
	Currency        string `json:"currency"`
	VacationRentals bool   `json:"vacation_rentals"`
}

// =============================================================================
// Validate
// =============================================================================

// Validate checks required fields.
func (cmd *Command) Validate() error {
	if cmd.ID == "" {
		return fmt.Errorf("%w: id (property_token)", domain.ErrTokenRequired)
	}
	if cmd.CheckInDate == "" {
		return fmt.Errorf("%w: check_in_date", domain.ErrMissingRequiredField)
	}
	if cmd.CheckOutDate == "" {
		return fmt.Errorf("%w: check_out_date", domain.ErrMissingRequiredField)
	}

	if cmd.Adults < 1 {
		cmd.Adults = 2
	}

	return nil
}
