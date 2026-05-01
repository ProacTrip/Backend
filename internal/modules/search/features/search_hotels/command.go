// DTO de entrada para búsqueda de hoteles y vacation rentals.
// Valida parámetros y mapea a parámetros del adaptador SerpAPI.
package search_hotels

import (
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Command — DTO de entrada para search hotels
// =============================================================================

// Command is the input DTO for hotel search following the API spec.
type Command struct {
	Query           string   `json:"query"`
	CheckInDate     string   `json:"check_in_date"`
	CheckOutDate    string   `json:"check_out_date"`
	Adults          int      `json:"adults"`
	Children        int      `json:"children"`
	ChildrenAges    []int    `json:"children_ages"`
	GL              string   `json:"gl"`
	HL              string   `json:"hl"`
	Currency        string   `json:"currency"`
	MinPrice        *float64 `json:"min_price"`
	MaxPrice        *float64 `json:"max_price"`
	SortBy          *int     `json:"sort_by"`
	Rating          *int     `json:"rating"`
	PropertyTypes   []int    `json:"property_types"`
	Amenities       []int    `json:"amenities"`
	VacationRentals bool     `json:"vacation_rentals"`
	HotelClasses    []int    `json:"hotel_classes"`
	Brands          []int    `json:"brands"`
	FreeCancellation bool   `json:"free_cancellation"`
	SpecialOffers   bool     `json:"special_offers"`
	EcoCertified    bool     `json:"eco_certified"`
	Bedrooms        *int     `json:"bedrooms"`
	Bathrooms       *int     `json:"bathrooms"`
	PageToken       string   `json:"page_token"`
}

// =============================================================================
// Validate
// =============================================================================

// Validate checks required fields and parameter constraints.
func (cmd *Command) Validate() error {
	if cmd.Query == "" {
		return fmt.Errorf("%w: query", domain.ErrMissingRequiredField)
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

	return nil
}
