// DTO de entrada para búsqueda de vuelos.
// Valida parámetros y mapea a dominio.
package search_flights

import (
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Constantes
// =============================================================================

const (
	TripTypeRoundTrip = "round_trip"
	TripTypeOneWay    = "one_way"
	TripTypeMultiCity = "multi_city"

	TravelClassEconomy       = "economy"
	TravelClassPremiumEconomy = "premium_economy"
	TravelClassBusiness      = "business"
	TravelClassFirst         = "first"

	SortByTop           = "top"
	SortByPrice         = "price"
	SortByDepartureTime = "departure_time"
	SortByArrivalTime   = "arrival_time"
	SortByDuration      = "duration"
	SortByEmissions     = "emissions"

	StopsAny     = "any"
	StopsNonstop = "nonstop"
	StopsMax1    = "max_1"
	StopsMax2    = "max_2"

	PhaseOutboundSelection = "outbound_selection"
	PhaseReturnSelection   = "return_selection"
	PhaseComplete          = "complete"
)

var validTripTypes = map[string]bool{
	TripTypeRoundTrip: true,
	TripTypeOneWay:    true,
	TripTypeMultiCity: true,
}

var validTravelClasses = map[string]bool{
	TravelClassEconomy:       true,
	TravelClassPremiumEconomy: true,
	TravelClassBusiness:      true,
	TravelClassFirst:         true,
}

var validSortBy = map[string]bool{
	SortByTop:           true,
	SortByPrice:         true,
	SortByDepartureTime: true,
	SortByArrivalTime:   true,
	SortByDuration:      true,
	SortByEmissions:     true,
}

var validStops = map[string]bool{
	StopsAny:     true,
	StopsNonstop: true,
	StopsMax1:    true,
	StopsMax2:    true,
}

// =============================================================================
// Command
// =============================================================================

// Command is the input DTO for search flights following the API spec.
type Command struct {
	TripType               string               `json:"trip_type"`
	Departure              string               `json:"departure"`
	Arrival                string               `json:"arrival"`
	OutboundDate           string               `json:"outbound_date"`
	ReturnDate             string               `json:"return_date"`
	Legs                   []MultiCityLegCmd    `json:"legs"`
	Adults                 int                  `json:"adults"`
	Children               int                  `json:"children"`
	InfantsInSeat          int                  `json:"infants_in_seat"`
	InfantsOnLap           int                  `json:"infants_on_lap"`
	TravelClass            string               `json:"travel_class"`
	GL                     *string              `json:"gl,omitzero"`
	HL                     *string              `json:"hl,omitzero"`
	Currency               *string              `json:"currency,omitzero"`
	Bags                   int                  `json:"bags"`
	MaxPrice               *float64             `json:"max_price,omitzero"`
	SortBy                 string               `json:"sort_by"`
	Stops                  string               `json:"stops"`
	IncludeAirlines        []string             `json:"include_airlines"`
	ExcludeAirlines        []string             `json:"exclude_airlines"`
	OutboundTimes          *domain.TimeRange    `json:"outbound_times,omitzero"`
	ReturnTimes            *domain.TimeRange    `json:"return_times,omitzero"`
	EmissionsFilter        bool                 `json:"emissions_filter"`
	LayoverDuration        *domain.LayoverRange `json:"layover_duration,omitzero"`
	ExcludeConnections     []string             `json:"exclude_connections"`
	MaxDurationMinutes     *int                 `json:"max_duration_minutes,omitzero"`
	OutboundSelectionToken string               `json:"outbound_selection_token"`
	Cursor                 *string              `json:"cursor,omitzero"`
	Limit                  int                  `json:"limit"`

	// Request metadata (populated by the handler, not via JSON binding).
	IPAddress string `json:"-"`
	UserAgent string `json:"-"`
}

const (
	DefaultLimit    = domain.DefaultLimit
	MaxResultsLimit = domain.MaxResultsLimit
)



// MultiCityLegCmd is the input DTO for multi-city legs.
type MultiCityLegCmd struct {
	Departure string            `json:"departure"`
	Arrival   string            `json:"arrival"`
	Date      string            `json:"date"`
	Times     *domain.TimeRange `json:"times,omitzero"`
}

// =============================================================================
// Validación
// =============================================================================

func (cmd *Command) Validate() error {
	if cmd.TripType == "" {
		cmd.TripType = TripTypeRoundTrip // default
	}

	if !validTripTypes[cmd.TripType] {
		return fmt.Errorf("%w: %s", domain.ErrInvalidTripType, cmd.TripType)
	}

	switch cmd.TripType {
	case TripTypeRoundTrip:
		if cmd.Departure == "" {
			return fmt.Errorf("%w: departure", domain.ErrMissingRequiredField)
		}
		if cmd.Arrival == "" {
			return fmt.Errorf("%w: arrival", domain.ErrMissingRequiredField)
		}
		if cmd.OutboundDate == "" {
			return fmt.Errorf("%w: outbound_date", domain.ErrMissingRequiredField)
		}
		if cmd.ReturnDate == "" {
			return fmt.Errorf("%w: return_date", domain.ErrMissingRequiredField)
		}
		if len(cmd.Legs) > 0 {
			return fmt.Errorf("%w: legs no permitidos en round_trip", domain.ErrInvalidParameterRange)
		}

	case TripTypeOneWay:
		if cmd.Departure == "" {
			return fmt.Errorf("%w: departure", domain.ErrMissingRequiredField)
		}
		if cmd.Arrival == "" {
			return fmt.Errorf("%w: arrival", domain.ErrMissingRequiredField)
		}
		if cmd.OutboundDate == "" {
			return fmt.Errorf("%w: outbound_date", domain.ErrMissingRequiredField)
		}
		if cmd.ReturnDate != "" {
			return fmt.Errorf("%w: return_date no permitido en one_way", domain.ErrInvalidParameterRange)
		}
		if len(cmd.Legs) > 0 {
			return fmt.Errorf("%w: legs no permitidos en one_way", domain.ErrInvalidParameterRange)
		}

	case TripTypeMultiCity:
		if len(cmd.Legs) == 0 {
			return fmt.Errorf("%w: legs", domain.ErrMissingRequiredField)
		}
		if cmd.Departure != "" || cmd.Arrival != "" || cmd.OutboundDate != "" || cmd.ReturnDate != "" {
			return fmt.Errorf("%w: departure/arrival/outbound_date/return_date no permitidos en multi_city", domain.ErrInvalidParameterRange)
		}
	}

	if cmd.Adults < 1 {
		cmd.Adults = 1 // enforce minimum
	}

	// Validate defaults: TravelClass, SortBy, Stops
	if cmd.TravelClass == "" {
		cmd.TravelClass = TravelClassEconomy
	}
	if !validTravelClasses[cmd.TravelClass] {
		return fmt.Errorf("%w: travel_class debe ser economy, premium_economy, business o first", domain.ErrInvalidParameterRange)
	}
	if cmd.SortBy == "" {
		cmd.SortBy = SortByTop
	}
	if !validSortBy[cmd.SortBy] {
		return fmt.Errorf("%w: sort_by debe ser top, price, departure_time, arrival_time, duration o emissions", domain.ErrInvalidParameterRange)
	}
	if cmd.Stops == "" {
		cmd.Stops = StopsAny
	}
	if !validStops[cmd.Stops] {
		return fmt.Errorf("%w: stops debe ser any, nonstop, max_1 o max_2", domain.ErrInvalidParameterRange)
	}

	// Bags cannot exceed total passengers (adults + children + infants_in_seat)
	totalPassengers := cmd.Adults + cmd.Children + cmd.InfantsInSeat
	if cmd.Bags > totalPassengers {
		return fmt.Errorf("%w: bags no puede superar el número de pasajeros (%d)", domain.ErrInvalidParameterRange, totalPassengers)
	}

	// Validate time ranges (0-23)
	if cmd.OutboundTimes != nil {
		if err := validateTimeRange(cmd.OutboundTimes); err != nil {
			return fmt.Errorf("%w: outbound_times: %w", domain.ErrInvalidParameterRange, err)
		}
	}
	if cmd.ReturnTimes != nil {
		if err := validateTimeRange(cmd.ReturnTimes); err != nil {
			return fmt.Errorf("%w: return_times: %w", domain.ErrInvalidParameterRange, err)
		}
	}

	// Layover duration must be non-negative and min <= max
	if cmd.LayoverDuration != nil {
		if cmd.LayoverDuration.MinMinutes < 0 || cmd.LayoverDuration.MaxMinutes < 0 {
			return fmt.Errorf("%w: layover_duration no puede ser negativo", domain.ErrInvalidParameterRange)
		}
		if cmd.LayoverDuration.MinMinutes > cmd.LayoverDuration.MaxMinutes {
			return fmt.Errorf("%w: layover_duration min_minutes no puede ser mayor que max_minutes", domain.ErrInvalidParameterRange)
		}
	}

	// Validate Limit: default to 10 if zero, reject if out of range [1,100]
	if cmd.Limit == 0 {
		cmd.Limit = DefaultLimit
	}
	if cmd.Limit < 1 || cmd.Limit > MaxResultsLimit {
		return fmt.Errorf("%w: limit debe estar entre 1 y %d", domain.ErrInvalidParameterRange, MaxResultsLimit)
	}

	if len(cmd.IncludeAirlines) > 0 && len(cmd.ExcludeAirlines) > 0 {
		return fmt.Errorf("%w: include_airlines y exclude_airlines no pueden usarse juntos", domain.ErrInvalidParameterRange)
	}

	if cmd.OutboundSelectionToken != "" && cmd.TripType != TripTypeRoundTrip {
		return fmt.Errorf("%w: outbound_selection_token solo permitido en round_trip", domain.ErrInvalidParameterRange)
	}

	return nil
}

// validateTimeRange checks that departure_from/to and optional arrival_from/to are in 0-23.
func validateTimeRange(tr *domain.TimeRange) error {
	if tr.DepartureFrom < 0 || tr.DepartureFrom > 23 || tr.DepartureTo < 0 || tr.DepartureTo > 23 {
		return fmt.Errorf("departure_from y departure_to deben estar entre 0 y 23")
	}
	if tr.ArrivalFrom != nil && (*tr.ArrivalFrom < 0 || *tr.ArrivalFrom > 23) {
		return fmt.Errorf("arrival_from debe estar entre 0 y 23")
	}
	if tr.ArrivalTo != nil && (*tr.ArrivalTo < 0 || *tr.ArrivalTo > 23) {
		return fmt.Errorf("arrival_to debe estar entre 0 y 23")
	}
	return nil
}

// =============================================================================
// Mapeo a Dominio
// =============================================================================

func (cmd *Command) ToDomain() domain.FlightSearchRequest {
	req := domain.FlightSearchRequest{
		TripType:               cmd.TripType,
		Departure:              cmd.Departure,
		Arrival:                cmd.Arrival,
		OutboundDate:           cmd.OutboundDate,
		ReturnDate:             cmd.ReturnDate,
		Adults:                 cmd.Adults,
		Children:               cmd.Children,
		InfantsInSeat:          cmd.InfantsInSeat,
		InfantsOnLap:           cmd.InfantsOnLap,
		TravelClass:            cmd.TravelClass,
		GL:                     ptrStr(cmd.GL),
		HL:                     ptrStr(cmd.HL),
		Currency:               ptrStr(cmd.Currency),
		Bags:                   cmd.Bags,
		MaxPrice:               cmd.MaxPrice,
		SortBy:                 cmd.SortBy,
		Stops:                  cmd.Stops,
		IncludeAirlines:        cmd.IncludeAirlines,
		ExcludeAirlines:        cmd.ExcludeAirlines,
		EmissionsFilter:        cmd.EmissionsFilter,
		ExcludeConnections:     cmd.ExcludeConnections,
		MaxDurationMinutes:     cmd.MaxDurationMinutes,
		OutboundSelectionToken: cmd.OutboundSelectionToken,
		Cursor:                 cmd.Cursor,
		Limit:                  cmd.Limit,
	}

	// Map OutboundTimes
	req.OutboundTimes = cmd.OutboundTimes

	// Map ReturnTimes
	req.ReturnTimes = cmd.ReturnTimes

	// Map LayoverDuration (already domain.LayoverRange)
	req.LayoverDuration = cmd.LayoverDuration

	// Map multi-city Legs
	if len(cmd.Legs) > 0 {
		req.Legs = make([]domain.MultiCityLeg, len(cmd.Legs))
		for i, leg := range cmd.Legs {
			dl := domain.MultiCityLeg{
				Departure: leg.Departure,
				Arrival:   leg.Arrival,
				Date:      leg.Date,
				Times:     leg.Times,
			}
			req.Legs[i] = dl
		}
	}

	return req
}

// ptrStr returns the dereferenced string, or "" if nil.
// Used to convert *string command fields to plain string domain fields.
func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
