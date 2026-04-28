// DTO de entrada para detalles de vuelo.
// Valida el booking_token requerido.
package flight_details

import (
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Command — DTO de entrada para flight details
// =============================================================================

// Command is the input DTO for flight details.
type Command struct {
	BookingToken string `json:"booking_token"`
	Adults       int    `json:"adults"`
	Departure    string `json:"departure"`
	Arrival      string `json:"arrival"`
	OutboundDate string `json:"outbound_date"`
	ReturnDate   string `json:"return_date"`
	GL           string `json:"gl"`
	HL           string `json:"hl"`
	Currency     string `json:"currency"`
}

// =============================================================================
// Validate — valida el comando
// =============================================================================

func (cmd *Command) Validate() error {
	if cmd.BookingToken == "" {
		return fmt.Errorf("%w: booking_token", domain.ErrTokenRequired)
	}

	if cmd.Departure == "" {
		return fmt.Errorf("%w: departure", domain.ErrMissingRequiredField)
	}
	if cmd.Arrival == "" {
		return fmt.Errorf("%w: arrival", domain.ErrMissingRequiredField)
	}
	if cmd.OutboundDate == "" {
		return fmt.Errorf("%w: outbound_date", domain.ErrMissingRequiredField)
	}

	if cmd.Adults < 1 {
		cmd.Adults = 1
	}

	return nil
}
