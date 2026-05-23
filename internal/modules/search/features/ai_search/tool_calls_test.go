// Tests para definiciones de tools search_hotels y search_flights.
// Validación de JSON Schema completo según API spec.
package ai_search

import (
	"encoding/json"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/search/features/search_flights"
)

// =============================================================================
// RED: Test that tool definitions exist and have correct structure
// =============================================================================

func TestSearchHotelsToolDefinition_Exists(t *testing.T) {
	// This will fail because SearchHotelsToolDef does NOT exist yet — RED.
	schema := SearchHotelsToolDef()

	if schema.Type != "function" {
		t.Errorf("Type = %q, want 'function'", schema.Type)
	}
	if schema.Function.Name != "search_hotels" {
		t.Errorf("Name = %q, want 'search_hotels'", schema.Function.Name)
	}
	if schema.Function.Description == "" {
		t.Error("Description should not be empty")
	}

	// Unmarshal parameters as JSON Schema
	var params map[string]interface{}
	if err := json.Unmarshal(schema.Function.Parameters, &params); err != nil {
		t.Fatalf("Parameters is not valid JSON: %v", err)
	}

	// Must have type "object"
	if params["type"] != "object" {
		t.Errorf("Parameters type = %v, want 'object'", params["type"])
	}

	// Required fields must be present
	required, ok := params["required"].([]interface{})
	if !ok {
		t.Fatal("Parameters.required is missing or not an array")
	}

	requiredFields := make(map[string]bool)
	for _, r := range required {
		requiredFields[r.(string)] = true
	}

	// query, check_in_date, check_out_date are required per API spec
	for _, field := range []string{"query", "check_in_date", "check_out_date"} {
		if !requiredFields[field] {
			t.Errorf("required field %q is missing", field)
		}
	}

	// Properties must exist
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Parameters.properties is missing or not an object")
	}

	// All hotel search fields must be present
	hotelFields := []string{
		"query", "check_in_date", "check_out_date", "adults", "children",
		"children_ages", "gl", "hl", "currency", "min_price", "max_price",
		"sort_by", "rating", "property_types", "amenities", "vacation_rentals",
		"hotel_classes", "brands", "free_cancellation", "special_offers",
		"eco_certified", "bedrooms", "bathrooms", "page_token",
	}
	for _, field := range hotelFields {
		if _, exists := props[field]; !exists {
			t.Errorf("hotel property %q is missing from parameters", field)
		}
	}

	// Verify types for key fields
	typeChecks := []struct {
		field    string
		wantType string
	}{
		{"query", "string"},
		{"check_in_date", "string"},
		{"check_out_date", "string"},
		{"adults", "integer"},
		{"children", "integer"},
		{"children_ages", "array"},
		{"min_price", "number"},
		{"max_price", "number"},
		{"sort_by", "integer"},
		{"rating", "integer"},
		{"property_types", "array"},
		{"amenities", "array"},
		{"vacation_rentals", "boolean"},
		{"free_cancellation", "boolean"},
		{"special_offers", "boolean"},
		{"eco_certified", "boolean"},
		{"page_token", "string"},
	}
	for _, tc := range typeChecks {
		fieldProps, ok := props[tc.field].(map[string]interface{})
		if !ok {
			t.Errorf("property %q is not an object", tc.field)
			continue
		}
		if fieldProps["type"] != tc.wantType {
			t.Errorf("property %q type = %v, want %s", tc.field, fieldProps["type"], tc.wantType)
		}
	}

	// Verify defaults
	if adultProps, ok := props["adults"].(map[string]interface{}); ok {
		if adultProps["default"] != float64(2) {
			t.Errorf("adults default = %v, want 2", adultProps["default"])
		}
	}
	if childProps, ok := props["children"].(map[string]interface{}); ok {
		if childProps["default"] != float64(0) {
			t.Errorf("children default = %v, want 0", childProps["default"])
		}
	}
	if vrProps, ok := props["vacation_rentals"].(map[string]interface{}); ok {
		if vrProps["default"] != false {
			t.Errorf("vacation_rentals default = %v, want false", vrProps["default"])
		}
	}
}

func TestSearchFlightsToolDefinition_Exists(t *testing.T) {
	// This will fail because SearchFlightsToolDef does NOT exist yet — RED.
	schema := SearchFlightsToolDef()

	if schema.Type != "function" {
		t.Errorf("Type = %q, want 'function'", schema.Type)
	}
	if schema.Function.Name != "search_flights" {
		t.Errorf("Name = %q, want 'search_flights'", schema.Function.Name)
	}
	if schema.Function.Description == "" {
		t.Error("Description should not be empty")
	}

	var params map[string]interface{}
	if err := json.Unmarshal(schema.Function.Parameters, &params); err != nil {
		t.Fatalf("Parameters is not valid JSON: %v", err)
	}

	if params["type"] != "object" {
		t.Errorf("Parameters type = %v, want 'object'", params["type"])
	}

	required, ok := params["required"].([]interface{})
	if !ok {
		t.Fatal("Parameters.required is missing or not an array")
	}

	requiredFields := make(map[string]bool)
	for _, r := range required {
		requiredFields[r.(string)] = true
	}

	// trip_type, departure, arrival, outbound_date are required per API spec
	for _, field := range []string{"trip_type", "departure", "arrival", "outbound_date"} {
		if !requiredFields[field] {
			t.Errorf("required field %q is missing", field)
		}
	}

	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Parameters.properties is missing or not an object")
	}

	// All flight search fields must be present
	flightFields := []string{
		"trip_type", "departure", "arrival", "outbound_date", "return_date",
		"adults", "children", "infants_in_seat", "infants_on_lap",
		"travel_class", "gl", "hl", "currency", "bags", "max_price",
		"sort_by", "stops", "include_airlines", "exclude_airlines",
		"outbound_times", "return_times", "emissions_filter",
		"layover_duration", "exclude_connections", "max_duration_minutes",
		"cursor", "limit",
	}
	for _, field := range flightFields {
		if _, exists := props[field]; !exists {
			t.Errorf("flight property %q is missing from parameters", field)
		}
	}

	// Verify types for key fields
	typeChecks := []struct {
		field    string
		wantType string
	}{
		{"trip_type", "string"},
		{"departure", "string"},
		{"arrival", "string"},
		{"outbound_date", "string"},
		{"return_date", "string"},
		{"adults", "integer"},
		{"children", "integer"},
		{"infants_in_seat", "integer"},
		{"infants_on_lap", "integer"},
		{"travel_class", "string"},
		{"bags", "integer"},
		{"max_price", "number"},
		{"sort_by", "string"},
		{"stops", "string"},
		{"include_airlines", "array"},
		{"exclude_airlines", "array"},
		{"outbound_times", "object"},
		{"return_times", "object"},
		{"emissions_filter", "boolean"},
		{"layover_duration", "object"},
		{"exclude_connections", "array"},
		{"max_duration_minutes", "integer"},
		{"cursor", "string"},
		{"limit", "integer"},
	}
	for _, tc := range typeChecks {
		fieldProps, ok := props[tc.field].(map[string]interface{})
		if !ok {
			t.Errorf("property %q is not an object", tc.field)
			continue
		}
		if fieldProps["type"] != tc.wantType {
			t.Errorf("property %q type = %v, want %s", tc.field, fieldProps["type"], tc.wantType)
		}
	}

	// Verify defaults
	if adultProps, ok := props["adults"].(map[string]interface{}); ok {
		if adultProps["default"] != float64(1) {
			t.Errorf("adults default = %v, want 1", adultProps["default"])
		}
	}
	if childProps, ok := props["children"].(map[string]interface{}); ok {
		if childProps["default"] != float64(0) {
			t.Errorf("children default = %v, want 0", childProps["default"])
		}
	}
}

// =============================================================================
// TRIANGULATE: Enum validation for constrained fields
// =============================================================================

func TestSearchFlightsToolDefinition_Enums(t *testing.T) {
	schema := SearchFlightsToolDef()
	var params map[string]interface{}
	json.Unmarshal(schema.Function.Parameters, &params)
	props := params["properties"].(map[string]interface{})

	// trip_type must have enum values
	tripTypeProps := props["trip_type"].(map[string]interface{})
	if enum, ok := tripTypeProps["enum"]; !ok {
		t.Error("trip_type is missing enum constraint")
	} else {
		enumSlice := enum.([]interface{})
		enumStrs := make(map[string]bool)
		for _, e := range enumSlice {
			enumStrs[e.(string)] = true
		}
		for _, val := range []string{"round_trip", "one_way"} {
			if !enumStrs[val] {
				t.Errorf("trip_type enum missing value %q", val)
			}
		}
	}

	// travel_class enums
	travelProps := props["travel_class"].(map[string]interface{})
	if enum, ok := travelProps["enum"]; ok {
		enumSlice := enum.([]interface{})
		enumStrs := make(map[string]bool)
		for _, e := range enumSlice {
			enumStrs[e.(string)] = true
		}
		for _, val := range []string{"economy", "premium_economy", "business", "first"} {
			if !enumStrs[val] {
				t.Errorf("travel_class enum missing value %q", val)
			}
		}
	}

	// stops enums
	stopsProps := props["stops"].(map[string]interface{})
	if enum, ok := stopsProps["enum"]; ok {
		enumSlice := enum.([]interface{})
		enumStrs := make(map[string]bool)
		for _, e := range enumSlice {
			enumStrs[e.(string)] = true
		}
		for _, val := range []string{"any", "nonstop", "max_1", "max_2"} {
			if !enumStrs[val] {
				t.Errorf("stops enum missing value %q", val)
			}
		}
	}

	// sort_by enums
	sortProps := props["sort_by"].(map[string]interface{})
	if enum, ok := sortProps["enum"]; ok {
		enumSlice := enum.([]interface{})
		enumStrs := make(map[string]bool)
		for _, e := range enumSlice {
			enumStrs[e.(string)] = true
		}
		for _, val := range []string{"top", "price", "departure_time", "arrival_time", "duration", "emissions"} {
			if !enumStrs[val] {
				t.Errorf("sort_by enum missing value %q", val)
			}
		}
	}
}

func TestSearchHotelsToolDefinition_Enums(t *testing.T) {
	schema := SearchHotelsToolDef()
	var params map[string]interface{}
	json.Unmarshal(schema.Function.Parameters, &params)
	props := params["properties"].(map[string]interface{})

	// sort_by enum (integer values per SerpAPI)
	sortProps := props["sort_by"].(map[string]interface{})
	if desc, ok := sortProps["description"]; !ok {
		t.Error("sort_by is missing description")
	} else if desc.(string) == "" {
		t.Error("sort_by description is empty")
	}

	// rating has min/max constraints
	ratingProps := props["rating"].(map[string]interface{})
	if minVal, ok := ratingProps["minimum"]; !ok {
		t.Error("rating is missing minimum constraint")
	} else {
		if minVal.(float64) < 1 {
			t.Errorf("rating minimum = %v, want >= 1", minVal)
		}
	}
	if maxVal, ok := ratingProps["maximum"]; !ok {
		t.Error("rating is missing maximum constraint")
	} else {
		if maxVal.(float64) < 7 {
			t.Errorf("rating maximum = %v, want >= 7", maxVal)
		}
	}
}

// =============================================================================
// TRIANGULATE: Verify required fields work correctly (REQ-003 — AI fills only relevant params)
// =============================================================================

func TestSearchHotelsToolDefinition_RequiredOnly(t *testing.T) {
	schema := SearchHotelsToolDef()
	var params map[string]interface{}
	json.Unmarshal(schema.Function.Parameters, &params)

	required := params["required"].([]interface{})
	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r.(string)] = true
	}

	// Only 3 fields are required: query, check_in_date, check_out_date
	if len(requiredSet) != 3 {
		t.Errorf("required field count = %d, want 3 (query, check_in_date, check_out_date)", len(requiredSet))
	}

	// Verify adults, children, vacation_rentals are NOT required (REQ-003: defaults)
	for _, field := range []string{"adults", "children", "vacation_rentals", "min_price", "free_cancellation"} {
		if requiredSet[field] {
			t.Errorf("field %q should NOT be required (has defaults per REQ-003)", field)
		}
	}
}

func TestSearchFlightsToolDefinition_RequiredOnly(t *testing.T) {
	schema := SearchFlightsToolDef()
	var params map[string]interface{}
	json.Unmarshal(schema.Function.Parameters, &params)

	required := params["required"].([]interface{})
	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r.(string)] = true
	}

	// Only 4 fields are required: trip_type, departure, arrival, outbound_date
	if len(requiredSet) != 4 {
		t.Errorf("required field count = %d, want 4 (trip_type, departure, arrival, outbound_date)", len(requiredSet))
	}

	// Verify optional fields are NOT required
	for _, field := range []string{"return_date", "infants_in_seat", "infants_on_lap", "bags", "max_price", "include_airlines", "exclude_airlines", "cursor", "emissions_filter"} {
		if requiredSet[field] {
			t.Errorf("field %q should NOT be required (optional per API spec)", field)
		}
	}
}

// =============================================================================
// Task 2.1 — ParseHotelToolCall / ParseFlightToolCall
// =============================================================================

func TestParseHotelToolCall_HappyPath(t *testing.T) {
	// RED: ParseHotelToolCall does not exist yet
	args := map[string]interface{}{
		"query":          "Barcelona, España",
		"check_in_date":  "2026-07-01",
		"check_out_date": "2026-07-05",
		"adults":         float64(2),
		"rating":         float64(8),
		"free_cancellation": true,
	}

	cmd, err := ParseHotelToolCall(args)
	if err != nil {
		t.Fatalf("ParseHotelToolCall failed: %v", err)
	}

	if cmd.Query != "Barcelona, España" {
		t.Errorf("Query = %q, want 'Barcelona, España'", cmd.Query)
	}
	if cmd.CheckInDate != "2026-07-01" {
		t.Errorf("CheckInDate = %q, want '2026-07-01'", cmd.CheckInDate)
	}
	if cmd.CheckOutDate != "2026-07-05" {
		t.Errorf("CheckOutDate = %q, want '2026-07-05'", cmd.CheckOutDate)
	}
	if cmd.Adults != 2 {
		t.Errorf("Adults = %d, want 2", cmd.Adults)
	}
	if cmd.Rating == nil || *cmd.Rating != 8 {
		t.Errorf("Rating = %v, want 8", cmd.Rating)
	}
	if !cmd.FreeCancellation {
		t.Error("FreeCancellation should be true")
	}
}

func TestParseHotelToolCall_MissingRequired(t *testing.T) {
	// RED: missing query should return error
	args := map[string]interface{}{
		"check_in_date":  "2026-07-01",
		"check_out_date": "2026-07-05",
	}

	_, err := ParseHotelToolCall(args)
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestParseHotelToolCall_Defaults(t *testing.T) {
	// When optional fields are omitted, defaults should be applied
	args := map[string]interface{}{
		"query":          "Madrid",
		"check_in_date":  "2026-08-01",
		"check_out_date": "2026-08-05",
	}

	cmd, err := ParseHotelToolCall(args)
	if err != nil {
		t.Fatalf("ParseHotelToolCall failed: %v", err)
	}

	// adults defaults to 2
	if cmd.Adults != 2 {
		t.Errorf("Adults default = %d, want 2", cmd.Adults)
	}
	// children defaults to 0
	if cmd.Children != 0 {
		t.Errorf("Children default = %d, want 0", cmd.Children)
	}
	// free_cancellation defaults to false
	if cmd.FreeCancellation {
		t.Error("FreeCancellation default should be false")
	}
}

func TestParseHotelToolCall_ArrayFields(t *testing.T) {
	args := map[string]interface{}{
		"query":          "Barcelona",
		"check_in_date":  "2026-07-01",
		"check_out_date": "2026-07-05",
		"amenities":      []interface{}{float64(35), float64(4), float64(5)},
		"hotel_classes":  []interface{}{float64(4), float64(5)},
		"property_types": []interface{}{float64(1), float64(2)},
	}

	cmd, err := ParseHotelToolCall(args)
	if err != nil {
		t.Fatalf("ParseHotelToolCall failed: %v", err)
	}

	if len(cmd.Amenities) != 3 {
		t.Errorf("Amenities len = %d, want 3", len(cmd.Amenities))
	}
	if cmd.Amenities[0] != 35 || cmd.Amenities[1] != 4 || cmd.Amenities[2] != 5 {
		t.Errorf("Amenities = %v, want [35, 4, 5]", cmd.Amenities)
	}
	if len(cmd.HotelClasses) != 2 {
		t.Errorf("HotelClasses len = %d, want 2", len(cmd.HotelClasses))
	}
}

func TestParseFlightToolCall_HappyPath(t *testing.T) {
	// RED: ParseFlightToolCall does not exist yet
	args := map[string]interface{}{
		"trip_type":     "round_trip",
		"departure":     "MAD",
		"arrival":       "BCN",
		"outbound_date": "2026-07-15",
		"return_date":   "2026-07-20",
		"adults":        float64(2),
		"travel_class":  "business",
		"stops":         "nonstop",
		"sort_by":       "price",
	}

	cmd, err := ParseFlightToolCall(args)
	if err != nil {
		t.Fatalf("ParseFlightToolCall failed: %v", err)
	}

	if cmd.TripType != search_flights.TripTypeRoundTrip {
		t.Errorf("TripType = %q, want %q", cmd.TripType, search_flights.TripTypeRoundTrip)
	}
	if cmd.Departure != "MAD" {
		t.Errorf("Departure = %q, want 'MAD'", cmd.Departure)
	}
	if cmd.Arrival != "BCN" {
		t.Errorf("Arrival = %q, want 'BCN'", cmd.Arrival)
	}
	if cmd.OutboundDate != "2026-07-15" {
		t.Errorf("OutboundDate = %q, want '2026-07-15'", cmd.OutboundDate)
	}
	if cmd.ReturnDate != "2026-07-20" {
		t.Errorf("ReturnDate = %q, want '2026-07-20'", cmd.ReturnDate)
	}
	if cmd.Adults != 2 {
		t.Errorf("Adults = %d, want 2", cmd.Adults)
	}
	if cmd.TravelClass != search_flights.TravelClassBusiness {
		t.Errorf("TravelClass = %q, want %q", cmd.TravelClass, search_flights.TravelClassBusiness)
	}
	if cmd.Stops != search_flights.StopsNonstop {
		t.Errorf("Stops = %q, want %q", cmd.Stops, search_flights.StopsNonstop)
	}
	if cmd.SortBy != search_flights.SortByPrice {
		t.Errorf("SortBy = %q, want %q", cmd.SortBy, search_flights.SortByPrice)
	}
}

func TestParseFlightToolCall_MissingRequired(t *testing.T) {
	args := map[string]interface{}{
		"departure":     "MAD",
		"arrival":       "BCN",
		"outbound_date": "2026-07-15",
		// trip_type missing
	}

	_, err := ParseFlightToolCall(args)
	if err == nil {
		t.Fatal("expected error for missing trip_type")
	}
}

func TestParseFlightToolCall_Defaults(t *testing.T) {
	args := map[string]interface{}{
		"trip_type":     "round_trip",
		"departure":     "MAD",
		"arrival":       "BCN",
		"outbound_date": "2026-07-15",
		"return_date":   "2026-07-20",
	}

	cmd, err := ParseFlightToolCall(args)
	if err != nil {
		t.Fatalf("ParseFlightToolCall failed: %v", err)
	}

	// adults defaults to 1
	if cmd.Adults != 1 {
		t.Errorf("Adults default = %d, want 1", cmd.Adults)
	}
	// travel_class defaults to economy
	if cmd.TravelClass != search_flights.TravelClassEconomy {
		t.Errorf("TravelClass default = %q, want %q", cmd.TravelClass, search_flights.TravelClassEconomy)
	}
	// stops defaults to any
	if cmd.Stops != search_flights.StopsAny {
		t.Errorf("Stops default = %q, want %q", cmd.Stops, search_flights.StopsAny)
	}
	// sort_by defaults to top
	if cmd.SortBy != search_flights.SortByTop {
		t.Errorf("SortBy default = %q, want %q", cmd.SortBy, search_flights.SortByTop)
	}
}

func TestParseFlightToolCall_OneWay(t *testing.T) {
	args := map[string]interface{}{
		"trip_type":     "one_way",
		"departure":     "EZE",
		"arrival":       "MAD",
		"outbound_date": "2026-08-01",
	}

	cmd, err := ParseFlightToolCall(args)
	if err != nil {
		t.Fatalf("ParseFlightToolCall failed: %v", err)
	}

	if cmd.TripType != search_flights.TripTypeOneWay {
		t.Errorf("TripType = %q, want 'one_way'", cmd.TripType)
	}
	if cmd.ReturnDate != "" {
		t.Error("ReturnDate should be empty for one_way")
	}
}

func TestParseFlightToolCall_Arrays(t *testing.T) {
	args := map[string]interface{}{
		"trip_type":     "round_trip",
		"departure":     "MAD",
		"arrival":       "CDG",
		"outbound_date": "2026-07-01",
		"return_date":   "2026-07-10",
		"include_airlines": []interface{}{"IB", "UX"},
		"exclude_connections": []interface{}{"LHR", "FRA"},
	}

	cmd, err := ParseFlightToolCall(args)
	if err != nil {
		t.Fatalf("ParseFlightToolCall failed: %v", err)
	}

	if len(cmd.IncludeAirlines) != 2 {
		t.Errorf("IncludeAirlines len = %d, want 2", len(cmd.IncludeAirlines))
	}
	if cmd.IncludeAirlines[0] != "IB" || cmd.IncludeAirlines[1] != "UX" {
		t.Errorf("IncludeAirlines = %v, want [IB, UX]", cmd.IncludeAirlines)
	}
	if len(cmd.ExcludeConnections) != 2 {
		t.Errorf("ExcludeConnections len = %d, want 2", len(cmd.ExcludeConnections))
	}
}

// =============================================================================
// RED: Tasks 2.1-2.2 — EmitMedicalAlertsToolDef + ParseMedicalAlertsToolCall
// =============================================================================

func TestEmitMedicalAlertsToolDef_Exists(t *testing.T) {
	schema := EmitMedicalAlertsToolDef()

	if schema.Type != "function" {
		t.Errorf("Type = %q, want 'function'", schema.Type)
	}
	if schema.Function.Name != "emit_medical_alerts" {
		t.Errorf("Name = %q, want 'emit_medical_alerts'", schema.Function.Name)
	}
	if schema.Function.Description == "" {
		t.Error("Description should not be empty")
	}

	// Unmarshal parameters as JSON Schema
	var params map[string]interface{}
	if err := json.Unmarshal(schema.Function.Parameters, &params); err != nil {
		t.Fatalf("Parameters is not valid JSON: %v", err)
	}

	if params["type"] != "object" {
		t.Errorf("Parameters type = %v, want 'object'", params["type"])
	}

	// Required fields must be present
	required, ok := params["required"].([]interface{})
	if !ok {
		t.Fatal("Parameters.required is missing or not an array")
	}

	requiredFields := make(map[string]bool)
	for _, r := range required {
		requiredFields[r.(string)] = true
	}

	if !requiredFields["alerts"] {
		t.Errorf("required field 'alerts' is missing")
	}

	// Properties must exist
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Parameters.properties is missing or not an object")
	}

	// alerts property must exist and be an array
	alerts, ok := props["alerts"].(map[string]interface{})
	if !ok {
		t.Fatal("alerts property is missing or not an object")
	}
	if alerts["type"] != "array" {
		t.Errorf("alerts type = %v, want 'array'", alerts["type"])
	}

	// alerts items must have level, type, message
	items, ok := alerts["items"].(map[string]interface{})
	if !ok {
		t.Fatal("alerts items is missing")
	}
	itemProps, ok := items["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("alerts items.properties is missing")
	}

	// Check level enum
	level, ok := itemProps["level"].(map[string]interface{})
	if !ok {
		t.Error("level property missing from alert item")
	} else {
		levelEnum := level["enum"]
		if levelEnum == nil {
			t.Error("level enum is missing")
		}
	}

	// Check type enum
	alertType, ok := itemProps["type"].(map[string]interface{})
	if !ok {
		t.Error("type property missing from alert item")
	} else {
		typeEnum := alertType["enum"]
		if typeEnum == nil {
			t.Error("type enum is missing")
		}
	}

	// Check message
	if _, ok := itemProps["message"]; !ok {
		t.Error("message property missing from alert item")
	}
}

func TestParseMedicalAlertsToolCall_Valid(t *testing.T) {
	args := map[string]interface{}{
		"alerts": []interface{}{
			map[string]interface{}{
				"level":   "warning",
				"type":    "allergy",
				"message": "Alergia detectada: Maní",
			},
			map[string]interface{}{
				"level":   "info",
				"type":    "document",
				"message": "Pasaporte vence en 30 días",
			},
		},
	}

	alerts, err := ParseMedicalAlertsToolCall(args)
	if err != nil {
		t.Fatalf("ParseMedicalAlertsToolCall failed: %v", err)
	}

	if len(alerts) != 2 {
		t.Fatalf("len(alerts) = %d, want 2", len(alerts))
	}

	if alerts[0].Level != "warning" {
		t.Errorf("alerts[0].Level = %q, want 'warning'", alerts[0].Level)
	}
	if alerts[0].Type != "allergy" {
		t.Errorf("alerts[0].Type = %q, want 'allergy'", alerts[0].Type)
	}
	if alerts[0].Message != "Alergia detectada: Maní" {
		t.Errorf("alerts[0].Message = %q", alerts[0].Message)
	}

	if alerts[1].Level != "info" {
		t.Errorf("alerts[1].Level = %q, want 'info'", alerts[1].Level)
	}
}

func TestParseMedicalAlertsToolCall_MissingAlerts(t *testing.T) {
	args := map[string]interface{}{}

	_, err := ParseMedicalAlertsToolCall(args)
	if err == nil {
		t.Fatal("expected error for missing alerts field")
	}
}

func TestParseMedicalAlertsToolCall_InvalidAlerts(t *testing.T) {
	args := map[string]interface{}{
		"alerts": "not an array",
	}

	_, err := ParseMedicalAlertsToolCall(args)
	if err == nil {
		t.Fatal("expected error for non-array alerts")
	}
}

func TestParseMedicalAlertsToolCall_MissingFields(t *testing.T) {
	args := map[string]interface{}{
		"alerts": []interface{}{
			map[string]interface{}{
				"level": "warning",
				// missing type and message
			},
		},
	}

	alerts, err := ParseMedicalAlertsToolCall(args)
	if err != nil {
		t.Fatalf("ParseMedicalAlertsToolCall: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1", len(alerts))
	}
	// Missing fields should result in zero values
	if alerts[0].Message != "" {
		t.Errorf("expected empty message for missing field, got %q", alerts[0].Message)
	}
}

// =============================================================================
// get_destination_weather — tool definition + parser tests
// =============================================================================

func TestGetDestinationWeatherToolDefinition_Exists(t *testing.T) {
	schema := GetDestinationWeatherToolDef()

	if schema.Type != "function" {
		t.Errorf("Type = %q, want 'function'", schema.Type)
	}
	if schema.Function.Name != "get_destination_weather" {
		t.Errorf("Name = %q, want 'get_destination_weather'", schema.Function.Name)
	}
	if schema.Function.Description == "" {
		t.Error("Description should not be empty")
	}

	var params map[string]interface{}
	if err := json.Unmarshal(schema.Function.Parameters, &params); err != nil {
		t.Fatalf("Parameters is not valid JSON: %v", err)
	}

	if params["type"] != "object" {
		t.Errorf("Parameters type = %v, want 'object'", params["type"])
	}

	required, ok := params["required"].([]interface{})
	if !ok {
		t.Fatal("Parameters.required is missing or not an array")
	}

	requiredFields := make(map[string]bool)
	for _, r := range required {
		requiredFields[r.(string)] = true
	}

	for _, field := range []string{"lat", "lng", "date"} {
		if !requiredFields[field] {
			t.Errorf("required field %q is missing", field)
		}
	}

	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Parameters.properties is missing or not an object")
	}

	for _, field := range []string{"lat", "lng", "date"} {
		if _, exists := props[field]; !exists {
			t.Errorf("property %q is missing from parameters", field)
		}
	}

	// Verify types
	if lat, ok := props["lat"].(map[string]interface{}); ok {
		if lat["type"] != "number" {
			t.Errorf("lat.type = %v, want 'number'", lat["type"])
		}
	}
	if lng, ok := props["lng"].(map[string]interface{}); ok {
		if lng["type"] != "number" {
			t.Errorf("lng.type = %v, want 'number'", lng["type"])
		}
	}
	if date, ok := props["date"].(map[string]interface{}); ok {
		if date["type"] != "string" {
			t.Errorf("date.type = %v, want 'string'", date["type"])
		}
	}
}

func TestParseDestinationWeatherToolCall_ValidArgs(t *testing.T) {
	args := map[string]interface{}{
		"lat":  41.38,
		"lng":  2.17,
		"date": "2026-08-15",
	}

	cmd, err := ParseDestinationWeatherToolCall(args)
	if err != nil {
		t.Fatalf("ParseDestinationWeatherToolCall: %v", err)
	}
	if cmd.Lat != 41.38 {
		t.Errorf("Lat = %f, want 41.38", cmd.Lat)
	}
	if cmd.Lng != 2.17 {
		t.Errorf("Lng = %f, want 2.17", cmd.Lng)
	}
	if cmd.Date != "2026-08-15" {
		t.Errorf("Date = %q, want '2026-08-15'", cmd.Date)
	}
}

func TestParseDestinationWeatherToolCall_MissingLat(t *testing.T) {
	args := map[string]interface{}{
		"lng":  2.17,
		"date": "2026-08-15",
	}

	_, err := ParseDestinationWeatherToolCall(args)
	if err == nil {
		t.Fatal("expected error for missing lat")
	}
}

func TestParseDestinationWeatherToolCall_MissingLng(t *testing.T) {
	args := map[string]interface{}{
		"lat":  41.38,
		"date": "2026-08-15",
	}

	_, err := ParseDestinationWeatherToolCall(args)
	if err == nil {
		t.Fatal("expected error for missing lng")
	}
}

func TestParseDestinationWeatherToolCall_MissingDate(t *testing.T) {
	args := map[string]interface{}{
		"lat": 41.38,
		"lng": 2.17,
	}

	_, err := ParseDestinationWeatherToolCall(args)
	if err == nil {
		t.Fatal("expected error for missing date")
	}
}

func TestParseDestinationWeatherToolCall_EmptyDate(t *testing.T) {
	args := map[string]interface{}{
		"lat":  41.38,
		"lng":  2.17,
		"date": "",
	}

	_, err := ParseDestinationWeatherToolCall(args)
	if err == nil {
		t.Fatal("expected error for empty date")
	}
}

func TestParseDestinationWeatherToolCall_FloatArgsFromJSON(t *testing.T) {
	// JSON numbers are decoded as float64 — verify the parser handles this
	input := `{"lat": 41.38, "lng": 2.17, "date": "2026-08-15"}`
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	cmd, err := ParseDestinationWeatherToolCall(args)
	if err != nil {
		t.Fatalf("ParseDestinationWeatherToolCall: %v", err)
	}
	if cmd.Lat != 41.38 {
		t.Errorf("Lat = %f, want 41.38", cmd.Lat)
	}
}

func TestGetDestinationWeatherTool_InBuildDefaultTools(t *testing.T) {
	tools := buildDefaultTools(false) // no medical alerts

	found := false
	for _, tool := range tools {
		if fn, ok := tool["function"].(map[string]interface{}); ok {
			if fn["name"] == "get_destination_weather" {
				found = true
				break
			}
		}
	}

	if !found {
		t.Error("get_destination_weather tool should be present in buildDefaultTools()")
	}
}
