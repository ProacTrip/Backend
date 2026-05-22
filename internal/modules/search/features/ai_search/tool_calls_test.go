// Tests para definiciones de tools search_hotels y search_flights.
// Validación de JSON Schema completo según API spec.
package ai_search

import (
	"encoding/json"
	"testing"
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
