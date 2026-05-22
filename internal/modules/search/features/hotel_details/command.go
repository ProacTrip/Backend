// DTO de entrada para detalles de hotel.
// Valida el property_token requerido.
package hotel_details

import (
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	searchshared "github.com/ProacTrip/Backend/internal/modules/search/shared"
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

// Validate checks required fields and parameter constraints.
func (cmd *Command) Validate() error {
	// Trim whitespace before checking empty — whitespace-only query is invalid
	if searchshared.TrimSpaces(cmd.Query) == "" {
		return fmt.Errorf("%w: query", domain.ErrMissingRequiredField)
	}
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

	// children_ages must match children count
	if len(cmd.ChildrenAges) > 0 && len(cmd.ChildrenAges) != cmd.Children {
		return fmt.Errorf("%w: children_ages length must equal children count (%d != %d)",
			domain.ErrInvalidParameterRange, len(cmd.ChildrenAges), cmd.Children)
	}

	// children_ages must be in range 1-17
	for i, age := range cmd.ChildrenAges {
		if age < 1 || age > 17 {
			return fmt.Errorf("%w: children_ages[%d]=%d debe estar entre 1 y 17",
				domain.ErrInvalidParameterRange, i, age)
		}
	}

	return nil
}
