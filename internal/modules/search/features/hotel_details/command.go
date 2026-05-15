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
	Query           string `json:"query,omitzero"`
	CheckInDate     string `json:"check_in_date"`
	CheckOutDate    string `json:"check_out_date"`
	Adults          int    `json:"adults"`
	Children        int    `json:"children"`
	ChildrenAges    []int  `json:"children_ages"`
	GL              *string `json:"gl,omitzero"`
	HL              *string `json:"hl,omitzero"`
	Currency        *string `json:"currency,omitzero"`
	VacationRentals bool   `json:"vacation_rentals"`
}

// =============================================================================
// Validate
// =============================================================================

// ToDomain converts the input command to a domain hotel details request.
func (cmd Command) ToDomain() domain.HotelDetailsRequest {
	return domain.HotelDetailsRequest{
		ID:              cmd.ID,
		Query:           cmd.Query,
		CheckInDate:     cmd.CheckInDate,
		CheckOutDate:    cmd.CheckOutDate,
		Adults:          cmd.Adults,
		Children:        cmd.Children,
		ChildrenAges:    cmd.ChildrenAges,
		GL:              cmd.GL,
		HL:              cmd.HL,
		Currency:        cmd.Currency,
		VacationRentals: cmd.VacationRentals,
	}
}

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
