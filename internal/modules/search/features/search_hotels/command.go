// DTO de entrada para búsqueda de hoteles y vacation rentals.
// Valida parámetros y mapea a parámetros del adaptador SerpAPI.
package search_hotels

import (
	"fmt"
	"time"

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
	GL              *string  `json:"gl,omitzero"`
	HL              *string  `json:"hl,omitzero"`
	Currency        *string  `json:"currency,omitzero"`
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

// ToDomain converts the input command to a domain hotel search request.
func (cmd Command) ToDomain() domain.HotelSearchRequest {
	return domain.HotelSearchRequest{
		Query:            cmd.Query,
		CheckInDate:      cmd.CheckInDate,
		CheckOutDate:     cmd.CheckOutDate,
		Adults:           cmd.Adults,
		Children:         cmd.Children,
		ChildrenAges:     cmd.ChildrenAges,
		GL:               cmd.GL,
		HL:               cmd.HL,
		Currency:         cmd.Currency,
		MinPrice:         cmd.MinPrice,
		MaxPrice:         cmd.MaxPrice,
		SortBy:           cmd.SortBy,
		Rating:           cmd.Rating,
		PropertyTypes:    cmd.PropertyTypes,
		Amenities:        cmd.Amenities,
		VacationRentals:  cmd.VacationRentals,
		HotelClasses:     cmd.HotelClasses,
		Brands:           cmd.Brands,
		FreeCancellation: cmd.FreeCancellation,
		SpecialOffers:    cmd.SpecialOffers,
		EcoCertified:     cmd.EcoCertified,
		Bedrooms:         cmd.Bedrooms,
		Bathrooms:        cmd.Bathrooms,
		PageToken:        cmd.PageToken,
	}
}

// Validate checks required fields and parameter constraints.
func (cmd *Command) Validate() error {
	// Trim whitespace before checking empty — whitespace-only query is invalid
	if trimSpaces(cmd.Query) == "" {
		return fmt.Errorf("%w: query", domain.ErrMissingRequiredField)
	}
	if cmd.CheckInDate == "" {
		return fmt.Errorf("%w: check_in_date", domain.ErrMissingRequiredField)
	}
	if cmd.CheckOutDate == "" {
		return fmt.Errorf("%w: check_out_date", domain.ErrMissingRequiredField)
	}

	// Validate YYYY-MM-DD date format
	if _, err := time.Parse("2006-01-02", cmd.CheckInDate); err != nil {
		return fmt.Errorf("%w: check_in_date debe ser YYYY-MM-DD", domain.ErrInvalidParameterRange)
	}
	if _, err := time.Parse("2006-01-02", cmd.CheckOutDate); err != nil {
		return fmt.Errorf("%w: check_out_date debe ser YYYY-MM-DD", domain.ErrInvalidParameterRange)
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

// trimSpaces removes leading and trailing whitespace.
func trimSpaces(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
