// Adaptador SerpAPI — implementa domain.FlightProvider.
// Mapea respuestas crudas de SerpAPI a entidades de dominio.
package serpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/shared/airports"
)

// =============================================================================
// Adaptador
// =============================================================================

var _ domain.FlightProvider = (*Adapter)(nil)
var _ domain.HotelProvider = (*Adapter)(nil)

// Adapter maps SerpAPI SDK responses (map[string]interface{}) to domain entities.
type Adapter struct {
	client *Client
}

// NewAdapter creates a new SerpAPI adapter.
func NewAdapter(client *Client) *Adapter {
	return &Adapter{client: client}
}

// =============================================================================
// Búsqueda de Vuelos
// =============================================================================

// SearchFlights performs a flight search via SerpAPI and maps results to domain entities.
func (a *Adapter) SearchFlights(ctx context.Context, req domain.FlightSearchRequest) (*domain.FlightSearchResponse, error) {
	params := buildSerpapiParams(req)

	raw, err := a.client.Search(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("serpapi search: %w", err)
	}

	// Convert SDK's map[string]interface{} → typed DTO
	dto, err := convertToSearchResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("serpapi parse response: %w", err)
	}

	return mapSearchResponse(dto, req.Currency, req.TripType, req.OutboundSelectionToken != ""), nil
}

// =============================================================================
// Detalles de Reserva
// =============================================================================

// GetFlightDetails retrieves booking details for a booking token.
func (a *Adapter) GetFlightDetails(ctx context.Context, req domain.FlightDetailsRequest) (*domain.FlightDetailsResponse, error) {
	raw, err := a.client.GetBookingDetails(ctx, req.BookingToken, req.Adults, req.Currency, req.DepartureID, req.ArrivalID, req.OutboundDate, req.ReturnDate)
	if err != nil {
		return nil, fmt.Errorf("serpapi booking details: %w", err)
	}

	dto, err := convertToBookingResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("serpapi parse booking response: %w", err)
	}

	return mapBookingResponse(dto, req.Adults, req.Currency), nil
}

// =============================================================================
// Conversión SDK → DTO
// =============================================================================

func convertToSearchResponse(raw map[string]interface{}) (*serpapiSearchResponse, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal serpapi response: %w", err)
	}
	var dto serpapiSearchResponse
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, fmt.Errorf("unmarshal serpapi response: %w", err)
	}
	return &dto, nil
}

func convertToBookingResponse(raw map[string]interface{}) (*serpapiBookingResponse, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal serpapi booking response: %w", err)
	}
	var dto serpapiBookingResponse
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, fmt.Errorf("unmarshal serpapi booking response: %w", err)
	}
	return &dto, nil
}

// =============================================================================
// Construcción de Parámetros
// =============================================================================

func buildSerpapiParams(req domain.FlightSearchRequest) map[string]string {
	params := make(map[string]string)

	// Resolve country names to IATA codes (e.g., "Perú" → "LIM", "México" → "MEX").
	// Only resolves when the input is a single identifier (no commas — those are
	// already explicit IATA/kgmid lists). Already-valid IATA codes like "MAD"
	// and kgmids like "/m/04jpl" pass through unchanged.
	departureID := resolveIdentifier(req.Departure)
	arrivalID := resolveIdentifier(req.Arrival)

	// Required fields
	params["departure_id"] = departureID
	params["arrival_id"] = arrivalID
	params["outbound_date"] = req.OutboundDate
	params["currency"] = req.Currency

	// Trip type
	tripType := mapTripType(req.TripType)
	if tripType != "" {
		params["type"] = tripType
	}

	// Return date
	if req.ReturnDate != "" {
		params["return_date"] = req.ReturnDate
	}

	// Travel class
	travelClass := mapTravelClass(req.TravelClass)
	if travelClass != "" {
		params["travel_class"] = travelClass
	}

	// Sort by
	sortBy := mapSortBy(req.SortBy)
	if sortBy != "" {
		params["sort_by"] = sortBy
	}

	// Stops
	stops := mapStops(req.Stops)
	if stops != "" {
		params["stops"] = stops
	}

	// Language / market
	if req.HL != "" {
		params["hl"] = req.HL
	}
	if req.GL != "" {
		params["gl"] = req.GL
	}

	// Passengers
	if req.Adults > 0 {
		params["adults"] = itoa(req.Adults)
	}
	if req.Children > 0 {
		params["children"] = itoa(req.Children)
	}
	if req.InfantsInSeat > 0 {
		params["infants_in_seat"] = itoa(req.InfantsInSeat)
	}
	if req.InfantsOnLap > 0 {
		params["infants_on_lap"] = itoa(req.InfantsOnLap)
	}

	// Bags
	if req.Bags > 0 {
		params["bags"] = itoa(req.Bags)
	}

	// Max price
	if req.MaxPrice != nil {
		params["max_price"] = ftoa(*req.MaxPrice)
	}

	// Airlines
	if len(req.IncludeAirlines) > 0 {
		params["include_airlines"] = strings.Join(req.IncludeAirlines, ",")
	}
	if len(req.ExcludeAirlines) > 0 {
		params["exclude_airlines"] = strings.Join(req.ExcludeAirlines, ",")
	}

	// Time ranges
	if req.OutboundTimes != nil {
		params["outbound_times"] = formatTimes(req.OutboundTimes)
	}
	if req.ReturnTimes != nil {
		params["return_times"] = formatTimes(req.ReturnTimes)
	}

	// Emissions filter
	if req.EmissionsFilter {
		params["emissions"] = "1"
	}

	// Layover duration
	if req.LayoverDuration != nil {
		params["layover_duration"] = fmt.Sprintf("%d,%d",
			req.LayoverDuration.MinMinutes,
			req.LayoverDuration.MaxMinutes,
		)
	}

	// Exclude connections
	if len(req.ExcludeConnections) > 0 {
		params["exclude_conns"] = strings.Join(req.ExcludeConnections, ",")
	}

	// Max duration
	if req.MaxDurationMinutes != nil {
		params["max_duration"] = itoa(*req.MaxDurationMinutes)
	}

	// Outbound selection token → SerpAPI departure_token
	if req.OutboundSelectionToken != "" {
		params["departure_token"] = req.OutboundSelectionToken
	}

	// Multi-city legs
	if len(req.Legs) > 0 {
		params["multi_city_json"] = marshalMultiCityLegs(req.Legs)
	}

	// Internal SerpAPI params (always set, not exposed to clients)
	params["show_hidden"] = "1"   // Show ALL results (equivalent to "View more flights" on Google Flights)
	params["deep_search"] = "1"   // Enable deep search for better results (matches browser experience)
	params["exclude_basic"] = "0" // Don't exclude basic economy results
	params["no_cache"] = "false"  // Use SerpAPI's own cache

	return params
}

// resolveIdentifier translates a user-provided location identifier into a
// SerpAPI-compatible format. It handles three cases:
//  1. Country name → IATA code (via countryToMainAirport map)
//  2. Already-valid IATA code (e.g., "MAD") → passes through unchanged
//  3. kgmid (e.g., "/m/04jpl") → passes through unchanged
//  4. Comma-separated list (e.g., "CDG,ORY") → passes through unchanged
//     (assumes user provided explicit airport identifiers)
func resolveIdentifier(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return trimmed
	}

	// Comma-separated lists are explicit airport identifiers — don't resolve
	if strings.Contains(trimmed, ",") {
		return trimmed
	}

	// kgmids start with /m/ — pass through
	if strings.HasPrefix(trimmed, "/m/") {
		return trimmed
	}

	// Check if it looks like a valid IATA code (3 uppercase letters)
	// If not, try country name resolution
	if len(trimmed) == 3 && isAllUpperAlpha(trimmed) {
		// Already looks like IATA — pass through
		return trimmed
	}

	// Try country name resolution
	if iata, ok := airports.ResolveCountryToIATA(trimmed); ok {
		return iata
	}

	// No match — pass through as-is (it might be a city name that SerpAPI accepts)
	return trimmed
}

// isAllUpperAlpha checks if a string consists only of uppercase ASCII letters.
func isAllUpperAlpha(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// =============================================================================
// Helpers de Mapeo de Valores
// =============================================================================

func mapTripType(t string) string {
	switch t {
	case "round_trip":
		return "1"
	case "one_way":
		return "2"
	case "multi_city":
		return "3"
	default:
		return ""
	}
}

func mapTravelClass(c string) string {
	switch c {
	case "economy":
		return "1"
	case "premium_economy":
		return "2"
	case "business":
		return "3"
	case "first":
		return "4"
	default:
		return ""
	}
}

func mapSortBy(s string) string {
	switch s {
	case "top":
		return "1"
	case "price":
		return "2"
	case "departure_time":
		return "3"
	case "arrival_time":
		return "4"
	case "duration":
		return "5"
	case "emissions":
		return "6"
	default:
		return ""
	}
}

func mapStops(s string) string {
	switch s {
	case "any":
		return "0"
	case "nonstop":
		return "1"
	case "max_1":
		return "2"
	case "max_2":
		return "3"
	default:
		return ""
	}
}

func formatTimes(tr *domain.TimeRange) string {
	if tr.ArrivalFrom != nil && tr.ArrivalTo != nil {
		return fmt.Sprintf("%d,%d,%d,%d",
			tr.DepartureFrom, tr.DepartureTo, *tr.ArrivalFrom, *tr.ArrivalTo,
		)
	}
	return fmt.Sprintf("%d,%d", tr.DepartureFrom, tr.DepartureTo)
}

// =============================================================================
// Multi-City
// =============================================================================

type multiCityLegJSON struct {
	DepartureID string  `json:"departure_id"`
	ArrivalID   string  `json:"arrival_id"`
	Date        string  `json:"date"`
	Times       *string `json:"times,omitempty"`
}

func marshalMultiCityLegs(legs []domain.MultiCityLeg) string {
	jsonLegs := make([]multiCityLegJSON, len(legs))
	for i, leg := range legs {
		mc := multiCityLegJSON{
			DepartureID: leg.Departure,
			ArrivalID:   leg.Arrival,
			Date:        leg.Date,
		}
		if leg.Times != nil {
			timesStr := formatTimes(leg.Times)
			mc.Times = new(timesStr)
		}
		jsonLegs[i] = mc
	}
	data, _ := json.Marshal(jsonLegs)
	return string(data)
}

// =============================================================================
// Mapeo de Response
// =============================================================================

func mapSearchResponse(raw *serpapiSearchResponse, currency string, tripType string, hasOutboundToken bool) *domain.FlightSearchResponse {
	airportLookup := buildAirportLookup(raw.Airports)

	bestFlights := mapFlightGroups(raw.BestFlights, currency, airportLookup)
	otherFlights := mapFlightGroups(raw.OtherFlights, currency, airportLookup)

	// D1: For round_trip, best_flights should be empty (both outbound_selection and return_selection phases)
	if tripType == "round_trip" {
		bestFlights = nil
	}

	priceInsights := mapPriceInsights(raw.PriceInsights, currency)
	// D2: For return_selection phase, price_insights should be null
	if hasOutboundToken {
		priceInsights = nil
	}

	response := &domain.FlightSearchResponse{
		TripType:      tripType,
		Phase:         determinePhase(tripType, hasOutboundToken),
		BestFlights:   bestFlights,
		OtherFlights:  otherFlights,
		Airports:      mapAirports(raw.Airports),
		PriceInsights: priceInsights,
		ResultsState:  "empty",
	}

	if len(response.BestFlights) > 0 || len(response.OtherFlights) > 0 {
		response.ResultsState = "matching"
	}

	return response
}

func determinePhase(tripType string, hasOutboundToken bool) string {
	if tripType != "round_trip" {
		return "complete"
	}
	if hasOutboundToken {
		return "return_selection"
	}
	return "outbound_selection"
}

// =============================================================================
// Flight Group → Flight
// =============================================================================

func mapFlightGroups(groups []serpapiFlightGroup, currency string, lookup map[string]serpapiAirportDetail) []domain.Flight {
	if groups == nil {
		return nil
	}
	flights := make([]domain.Flight, 0, len(groups))
	for _, g := range groups {
		flights = append(flights, mapFlightGroup(g, currency, lookup))
	}
	return flights
}

func mapFlightGroup(g serpapiFlightGroup, currency string, lookup map[string]serpapiAirportDetail) domain.Flight {
	f := domain.Flight{
		DepartureToken:       g.DepartureToken,
		BookingToken:         g.BookingToken,
		Legs:                 mapFlights(g.Flights, lookup),
		Layovers:             mapLayovers(g.Layovers),
		TotalDurationMinutes: g.TotalDuration,
		Price: domain.PriceInfo{
			Amount:   float64(g.Price),
			Currency: currency,
		},
		CarbonEmissions: mapCarbonEmissions(g.CarbonEmissions),
		Type:            g.Type,
		AirlineLogoURL:  g.AirlineLogo,
	}
	return f
}

func mapFlights(raw []serpapiFlight, lookup map[string]serpapiAirportDetail) []domain.Leg {
	if raw == nil {
		return nil
	}
	legs := make([]domain.Leg, 0, len(raw))
	for _, rf := range raw {
		legs = append(legs, mapFlight(rf, lookup))
	}
	return legs
}

func mapFlight(rf serpapiFlight, lookup map[string]serpapiAirportDetail) domain.Leg {
	features := parseFeatures(rf.Extensions)
	features.Raw = rf.Extensions

	return domain.Leg{
		Departure:      mapAirportTime(rf.DepartureAirport, lookup),
		Arrival:        mapAirportTime(rf.ArrivalAirport, lookup),
		DurationMinutes: rf.Duration,
		Aircraft:       rf.Airplane,
		Airline:        rf.Airline,
		AirlineCode:    extractAirlineCode(rf.FlightNumber),
		AirlineLogoURL: rf.AirlineLogo,
		FlightNumber:   rf.FlightNumber,
		TravelClass:    rf.TravelClass,
		Legroom:        rf.Legroom,
		LegroomQuality: parseLegroomQuality(rf.Legroom),
		AlsoSoldBy:     rf.TicketAlsoSoldBy,
		Features:       features,
		Overnight:      rf.Overnight,
		OftenDelayed:   rf.OftenDelayedByOver30Min,
		OperatedBy:     rf.PlaneAndCrewBy,
	}
}

// parseLegroomQuality determines legroom quality from a legroom string like "31 in" or "79 cm".
// Returns "below_average", "average", "above_average", or "" for unparseable strings.
func parseLegroomQuality(legroom string) string {
	if legroom == "" {
		return ""
	}

	// Extract numeric value and unit
	var value float64
	var unit string
	n, _ := fmt.Sscanf(legroom, "%f %s", &value, &unit)
	if n < 2 {
		return ""
	}

	unit = strings.ToLower(unit)

	switch {
	case unit == "in" || unit == "in.":
		switch {
		case value < 29:
			return "below_average"
		case value <= 31:
			return "average"
		default:
			return "above_average"
		}
	case unit == "cm":
		switch {
		case value < 74:
			return "below_average"
		case value <= 79:
			return "average"
		default:
			return "above_average"
		}
	}
	return ""
}

// extractAirlineCode extracts the IATA airline code from a flight number (e.g., "IB 125" → "IB").
func extractAirlineCode(flightNumber string) string {
	if len(flightNumber) < 2 {
		return flightNumber
	}
	// Try 2-char IATA code first, then 3-char ICAO
	for i := 2; i <= 3; i++ {
		if i > len(flightNumber) {
			break
		}
		prefix := flightNumber[:i]
		if isAlpha(prefix) {
			return prefix
		}
	}
	return flightNumber[:2]
}

func isAlpha(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return len(s) > 0
}

func mapAirportTime(at serpapiAirportTime, lookup map[string]serpapiAirportDetail) domain.AirportTime {
	result := domain.AirportTime{
		AirportCode: at.ID,
		AirportName: at.Name,
		Datetime:    at.Time,
	}
	if detail, ok := lookup[at.ID]; ok {
		result.City = detail.City
		result.Country = detail.Country
		result.CountryCode = detail.CountryCode
	}
	return result
}

func mapLayovers(raw []serpapiLayover) []domain.Layover {
	if raw == nil {
		return nil
	}
	layovers := make([]domain.Layover, 0, len(raw))
	for _, rl := range raw {
		layovers = append(layovers, domain.Layover{
			AirportCode:     rl.ID,
			AirportName:     rl.Name,
			DurationMinutes: rl.Duration,
			Overnight:       rl.Overnight,
		})
	}
	return layovers
}

func mapCarbonEmissions(ce serpapiCarbonEmissions) domain.CarbonEmissions {
	return domain.CarbonEmissions{
		ThisFlightGrams:   ce.ThisFlight,
		TypicalRouteGrams: ce.TypicalForThisRoute,
		DifferencePercent: ce.DifferencePercent,
	}
}

// =============================================================================
// Sección de Aeropuertos
// =============================================================================

// buildAirportLookup creates a lookup map from airport code to airport detail
// using the airports section data (city, country, country_code).
func buildAirportLookup(airportGroups []serpapiAirportGroup) map[string]serpapiAirportDetail {
	lookup := make(map[string]serpapiAirportDetail)
	for _, group := range airportGroups {
		for _, dep := range group.Departure {
			lookup[dep.Airport.ID] = dep
		}
		for _, arr := range group.Arrival {
			lookup[arr.Airport.ID] = arr
		}
	}
	return lookup
}

func mapAirports(raw []serpapiAirportGroup) []domain.Airport {
	if raw == nil {
		return nil
	}
	var airports []domain.Airport
	for _, group := range raw {
		for _, dep := range group.Departure {
			airports = append(airports, domain.Airport{
				Role:         "departure",
				AirportCode:  dep.Airport.ID,
				AirportName:  dep.Airport.Name,
				City:         dep.City,
				Country:      dep.Country,
				CountryCode:  dep.CountryCode,
				ImageURL:     dep.Image,
				ThumbnailURL: dep.Thumbnail,
			})
		}
		for _, arr := range group.Arrival {
			airports = append(airports, domain.Airport{
				Role:         "arrival",
				AirportCode:  arr.Airport.ID,
				AirportName:  arr.Airport.Name,
				City:         arr.City,
				Country:      arr.Country,
				CountryCode:  arr.CountryCode,
				ImageURL:     arr.Image,
				ThumbnailURL: arr.Thumbnail,
			})
		}
	}
	return airports
}

// =============================================================================
// Price Insights
// =============================================================================

func mapPriceInsights(pi *serpapiPriceInsights, currency string) *domain.PriceInsights {
	if pi == nil {
		return nil
	}
	insights := &domain.PriceInsights{
		LowestPrice: domain.PriceInfo{
			Amount:   float64(pi.LowestPrice),
			Currency: currency,
		},
		PriceLevel: pi.PriceLevel,
		TypicalRange: domain.PriceRange{
			Min:      float64(pi.TypicalPriceRange[0]),
			Max:      float64(pi.TypicalPriceRange[1]),
			Currency: currency,
		},
	}
	if len(pi.PriceHistory) > 0 {
		insights.PriceHistory = make([]domain.PricePoint, len(pi.PriceHistory))
		for i, ph := range pi.PriceHistory {
			insights.PriceHistory[i] = domain.PricePoint{
				Timestamp: int64(ph[0]),
				Price:     float64(ph[1]),
			}
		}
	}
	return insights
}

// =============================================================================
// Parsing de Features
// =============================================================================

func parseFeatures(extensions []string) domain.FlightFeatures {
	f := domain.FlightFeatures{}

	for _, ext := range extensions {
		lower := strings.ToLower(ext)

		// WiFi detection
		if strings.Contains(lower, "wi-fi") || strings.Contains(lower, "wifi") {
			switch {
			case strings.Contains(lower, "fee") || strings.Contains(lower, "paid"):
				f.WiFi = "paid"
			case strings.Contains(lower, "complimentary") || strings.Contains(lower, "free"):
				f.WiFi = "free"
			default:
				f.WiFi = "available"
			}
		}

		// Power outlets
		if strings.Contains(lower, "power") || strings.Contains(lower, "outlet") {
			f.PowerOutlets = true
		}

		// USB
		if strings.Contains(lower, "usb") {
			f.USB = true
		}

		// Entertainment
		if f.Entertainment == "" {
			switch {
			case strings.Contains(lower, "on-demand") || strings.Contains(lower, "video on demand"):
				f.Entertainment = "on_demand"
			case strings.Contains(lower, "stream"):
				f.Entertainment = "stream"
			case strings.Contains(lower, "live tv"):
				f.Entertainment = "live_tv"
			}
		}
	}

	return f
}

// =============================================================================
// Mapeo de Detalles de Reserva
// =============================================================================

func mapBookingResponse(raw *serpapiBookingResponse, adults int, currency string) *domain.FlightDetailsResponse {
	tripType := "round_trip"
	if len(raw.SelectedFlights) == 1 {
		tripType = "one_way"
	}

	itinerary := domain.FlightItinerary{
		TripType: tripType,
	}

	if len(raw.SelectedFlights) >= 1 {
		itinerary.Outbound = mapFlightDetail(raw.SelectedFlights[0], currency)
	}
	if len(raw.SelectedFlights) >= 2 {
		itinerary.Return = new(mapFlightDetail(raw.SelectedFlights[1], currency))
	}

	options := make([]domain.BookingOption, 0, len(raw.BookingOptions))
	for _, bo := range raw.BookingOptions {
		options = append(options, mapBookingOption(bo, currency, tripType))
	}

	return &domain.FlightDetailsResponse{
		Itinerary:      itinerary,
		BookingOptions: options,
	}
}

func mapFlightDetail(g serpapiFlightGroup, currency string) domain.FlightDetail {
	return domain.FlightDetail{
		Legs:                 mapFlights(g.Flights, nil),
		Layovers:             mapLayovers(g.Layovers),
		TotalDurationMinutes: g.TotalDuration,
		CarbonEmissions:      mapCarbonEmissions(g.CarbonEmissions),
	}
}

func mapBookingOption(bo serpapiBookingOption, currency string, tripType string) domain.BookingOption {
	opt := domain.BookingOption{
		TripType:        tripType,
		SeparateTickets: bo.SeparateTickets,
		Together:        mapBookingDetail(bo.Together, currency),
	}
	if bo.Departing != nil {
		opt.Departing = new(mapBookingDetail(*bo.Departing, currency))
	}
	if bo.Returning != nil {
		opt.Returning = new(mapBookingDetail(*bo.Returning, currency))
	}
	return opt
}

func mapBookingDetail(d serpapiBookingDetail, currency string) domain.BookingDetail {
	detail := domain.BookingDetail{
		BookWith:          d.BookWith,
		Airline:           d.Airline,
		AirlineLogos:      d.AirlineLogos,
		MarketedAs:        d.MarketedAs,
		Price:             d.Price,
		OptionTitle:       d.OptionTitle,
		BaggagePrices:     d.BaggagePrices,
		BookingPhone:      d.BookingPhone,
	}

	if d.EstimatedPhoneServiceFee != 0 {
		detail.EstimatedServiceFee = new(d.EstimatedPhoneServiceFee)
	}

	if len(d.LocalPrices) > 0 {
		detail.LocalPrices = make([]domain.LocalPrice, len(d.LocalPrices))
		for i, lp := range d.LocalPrices {
			detail.LocalPrices[i] = domain.LocalPrice{
				Currency: lp.Currency,
				Price:    lp.Price,
			}
		}
	}

	if d.BookingRequest != nil {
		detail.BookingRequest = &domain.BookingRequest{
			URL:      d.BookingRequest.URL,
			PostData: d.BookingRequest.PostData,
		}
	}

	return detail
}

// =============================================================================
// Utilidades de Formato
// =============================================================================

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func ftoa(f float64) string {
	return fmt.Sprintf("%.0f", f)
}
