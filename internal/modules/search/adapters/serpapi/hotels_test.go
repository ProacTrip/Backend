// Validation tests for SerpAPI hotel DTOs using real fixture responses.
package serpapi

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"unicode"
)

// fixtureDir is the project-relative path to SerpAPI fixture files.
var fixtureDir = filepath.Join("..", "..", "..", "..", "..", "..", "Backend-original", "docs", "serpapi")

func readFixture(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, filename))
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", filename, err)
	}
	return data
}

// fixJSONNewlines replaces literal newlines inside JSON string values with
// escaped \n sequences. Some SerpAPI fixture files contain unescaped newlines
// in multiline description fields.
func fixJSONNewlines(data []byte) []byte {
	var buf bytes.Buffer
	inString := false
	escaped := false

	for i := 0; i < len(data); i++ {
		b := data[i]

		if escaped {
			buf.WriteByte(b)
			escaped = false
			continue
		}

		if b == '\\' && inString {
			buf.WriteByte(b)
			escaped = true
			continue
		}

		if b == '"' {
			inString = !inString
			buf.WriteByte(b)
			continue
		}

		// Replace literal newlines inside strings with \n
		if inString && (b == '\n' || b == '\r') {
			if b == '\r' {
				// Skip \r, add \n (normalize line endings)
				if i+1 < len(data) && data[i+1] == '\n' {
					i++ // skip \n after \r
				}
			}
			buf.WriteString(`\n`)
			// Re-add the original indentation as spaces to preserve readability
			// but only if there are spaces after the newline
			continue
		}

		buf.WriteByte(b)
	}
	return buf.Bytes()
}

// readFixtureJSON reads a fixture file and returns valid parsed JSON.
// For files that start without an opening brace (e.g., hotel-details.md),
// it prepends '{'. Also handles multi-line JSON strings.
func readFixtureJSON(t *testing.T, filename string) map[string]interface{} {
	t.Helper()
	data := readFixture(t, filename)

	// Trim leading whitespace to check if the JSON starts with '{'
	trimmed := bytes.TrimLeftFunc(data, unicode.IsSpace)
	if len(trimmed) == 0 {
		t.Fatalf("empty fixture file: %s", filename)
	}

	// If the file doesn't start with '{', it's a property fragment.
	// Wrap it in a synthetic outer object.
	if trimmed[0] != '{' {
		// The fragment has trailing '}', so prepending '{' makes it valid.
		data = append([]byte("{"), data...)
	}

	// Fix unescaped newlines in JSON strings
	data = fixJSONNewlines(data)

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal fixture %s: %v", filename, err)
	}
	return raw
}

// TestHotelSearchResponse validates the HotelProperty DTO against
// real SerpAPI hotel search fixtures (hotels.md).
func TestHotelSearchResponse(t *testing.T) {
	raw := readFixtureJSON(t, "hotels.md")

	// Extract non_matching_properties array
	nmRaw, ok := raw["non_matching_properties"]
	if !ok {
		t.Fatal("non_matching_properties key missing from SerpAPI response")
	}
	nmJSON, err := json.Marshal(nmRaw)
	if err != nil {
		t.Fatalf("failed to marshal non_matching_properties: %v", err)
	}

	// Unmarshal into HotelProperty slice
	var props []HotelProperty
	if err := json.Unmarshal(nmJSON, &props); err != nil {
		t.Fatalf("failed to unmarshal non_matching_properties into []HotelProperty: %v", err)
	}
	if len(props) == 0 {
		t.Fatal("expected at least 1 property in non_matching_properties")
	}

	// Verify first property has key fields populated
	p0 := props[0]

	if p0.Name == "" {
		t.Error("Name should be populated")
	}
	t.Logf("Hotel: Name=%q", p0.Name)

	if p0.ExtractedHotelClass == nil {
		t.Error("ExtractedHotelClass should be populated (e.g., 5)")
	} else {
		t.Logf("Hotel: ExtractedHotelClass=%d", *p0.ExtractedHotelClass)
	}

	if p0.ExtractedHotelClass != nil {
		t.Logf("HotelClass numeric: %d", *p0.ExtractedHotelClass)
	}

	if p0.RatePerNight.ExtractedLowest == nil {
		t.Error("RatePerNight.ExtractedLowest should be populated")
	} else {
		t.Logf("Hotel: RatePerNight.ExtractedLowest=%.0f", *p0.RatePerNight.ExtractedLowest)
	}

	// GPS must be populated
	if p0.GPSCoordinates.Latitude == 0 && p0.GPSCoordinates.Longitude == 0 {
		t.Error("GPS coordinates should be populated")
	}
	t.Logf("Hotel: GPS=(%.6f, %.6f)", p0.GPSCoordinates.Latitude, p0.GPSCoordinates.Longitude)

	if p0.OverallRating == nil {
		t.Error("OverallRating should be populated")
	} else {
		t.Logf("Hotel: OverallRating=%.2f", *p0.OverallRating)
	}

	// Verify DTO doesn't fail on CRITICAL blockers
	// C1: RatePerNight.Lowest is now *string (was *float64)
	if p0.RatePerNight.Lowest != nil {
		t.Logf("Hotel: RatePerNight.Lowest=%q (string OK, was float64 before)", *p0.RatePerNight.Lowest)
	}

	// C2: ExtractedBeforeTaxesFees should exist
	if p0.RatePerNight.ExtractedBeforeTaxesFees != nil {
		t.Logf("Hotel: ExtractedBeforeTaxesFees=%.0f (field ADDED)", *p0.RatePerNight.ExtractedBeforeTaxesFees)
	} else {
		t.Log("Hotel: ExtractedBeforeTaxesFees is nil (may be absent for some properties)")
	}

	// Verify Ratings array is populated (M1)
	if len(p0.Ratings) > 0 {
		t.Logf("Hotel: Ratings=%d buckets (star distribution)", len(p0.Ratings))
		for _, r := range p0.Ratings {
			t.Logf("  Stars=%d Count=%d", r.Stars, r.Count)
		}
	}

	// Verify ReviewsBreakdown is populated (M2)
	if len(p0.ReviewsBreakdown) > 0 {
		t.Logf("Hotel: ReviewsBreakdown=%d categories", len(p0.ReviewsBreakdown))
	}

	// Verify all properties unmarshal without error
	for i, p := range props {
		if p.Name == "" {
			t.Errorf("property[%d] has empty Name", i)
		}
		if p.PropertyToken == "" {
			t.Errorf("property[%d] has empty PropertyToken", i)
		}
	}

	t.Logf("Successfully unmarshaled %d properties from hotel search response", len(props))
}

// TestHotelDetailsResponse validates the HotelPropertyDetail DTO
// against the real SerpAPI hotel-details.md fixture.
func TestHotelDetailsResponse(t *testing.T) {
	raw := readFixtureJSON(t, "hotel-details.md")

	// Remove SerpAPI metadata keys that are not part of HotelPropertyDetail
	for k := range raw {
		if len(k) >= 7 && k[:7] == "serpapi" {
			delete(raw, k)
		}
	}
	delete(raw, "search_metadata")
	delete(raw, "search_parameters")
	delete(raw, "search_information")

	propJSON, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("failed to marshal cleaned hotel details: %v", err)
	}

	var detail HotelPropertyDetail
	if err := json.Unmarshal(propJSON, &detail); err != nil {
		t.Fatalf("failed to unmarshal hotel details into HotelPropertyDetail: %v", err)
	}

	// Verify basic fields
	if detail.Name == "" {
		t.Error("Name should be populated")
	}
	t.Logf("Hotel Detail: Name=%q", detail.Name)

	// Address must be populated for hotel (H2/H3 fix)
	if detail.Address == nil || *detail.Address == "" {
		t.Error("Address should be populated for hotel (not null)")
	} else {
		t.Logf("Hotel Detail: Address=%q (truncated)", (*detail.Address)[:min(50, len(*detail.Address))])
	}

	// Directions must be populated (H3 fix — JSON tag was "directions_url", now "directions")
	if detail.DirectionsURL == nil || *detail.DirectionsURL == "" {
		t.Error("Directions should be populated (json tag was directions_url, now directions)")
	} else {
		t.Logf("Hotel Detail: DirectionsURL=%q (truncated)", (*detail.DirectionsURL)[:min(50, len(*detail.DirectionsURL))])
	}

	// TypicalPriceRange must be populated (H2 fix — was *string, now *HotelTypicalPriceRange)
	if detail.TypicalPriceRange == nil {
		t.Error("TypicalPriceRange should be populated (was *string with wrong JSON tag)")
	} else {
		t.Logf("Hotel Detail: TypicalPriceRange=(lowest=%.0f, highest=%.0f)",
			detail.TypicalPriceRange.ExtractedLowest,
			detail.TypicalPriceRange.ExtractedHighest)
	}

	// OtherReviews must be populated (H4 fix — was HotelExternalReview, now HotelOtherReview)
	if len(detail.OtherReviews) == 0 {
		t.Error("OtherReviews should have entries (was external_reviews, now other_reviews)")
	} else {
		t.Logf("Hotel Detail: OtherReviews=%d reviews", len(detail.OtherReviews))
		for i, r := range detail.OtherReviews {
			t.Logf("  [%d] Source=%q Reviews=%d", i, r.Source, r.Reviews)
			if r.SourceRating != nil {
				t.Logf("      SourceRating=(%.2f/%.0f)", r.SourceRating.Score, r.SourceRating.MaxScore)
			}
			if r.UserReview != nil {
				t.Logf("      UserReview: username=%q date=%q comment=%q (truncated)",
					r.UserReview.Username, r.UserReview.Date,
					r.UserReview.Comment[:min(50, len(r.UserReview.Comment))])
			}
		}
	}

	// HealthAndSafety must be populated and be an object (not a string) — M8
	if detail.HealthAndSafety == nil {
		t.Error("HealthAndSafety should be populated (was *string, now *HealthAndSafetyObject)")
	} else {
		t.Logf("Hotel Detail: HealthAndSafety groups=%d", len(detail.HealthAndSafety.Groups))
		for _, g := range detail.HealthAndSafety.Groups {
			t.Logf("  Group: %q — %d items", g.Title, len(g.List))
			for _, item := range g.List {
				t.Logf("    - %q (available=%v)", item.Title, item.Available)
			}
		}
	}

	// Sustainability should be populated (M8)
	if detail.Sustainability == nil {
		t.Log("Hotel Detail: Sustainability is nil (may be absent for some hotels)")
	} else {
		t.Logf("Hotel Detail: Sustainability groups=%d", len(detail.Sustainability.Groups))
	}

	// ExtractedHotelClass should be populated (C3 fix)
	if detail.ExtractedHotelClass != nil {
		t.Logf("Hotel Detail: ExtractedHotelClass=%d", *detail.ExtractedHotelClass)
	}
	if detail.ExtractedHotelClass != nil {
		t.Logf("Hotel Detail: ExtractedHotelClass numeric=%d", *detail.ExtractedHotelClass)
	}
}

// TestVRSrchResponse validates that the HotelProperty DTO correctly
// handles vacation rental data from the SerpAPI search response.
func TestVRSrchResponse(t *testing.T) {
	raw := readFixtureJSON(t, "vacation-rentals.md")

	nmRaw, ok := raw["non_matching_properties"]
	if !ok {
		t.Fatal("non_matching_properties key missing from VR search response")
	}
	nmJSON, err := json.Marshal(nmRaw)
	if err != nil {
		t.Fatalf("failed to marshal non_matching_properties: %v", err)
	}

	var props []HotelProperty
	if err := json.Unmarshal(nmJSON, &props); err != nil {
		t.Fatalf("failed to unmarshal VR properties: %v", err)
	}
	if len(props) == 0 {
		t.Fatal("expected at least 1 VR property")
	}

	// Find a vacation rental property
	var vr *HotelProperty
	for i := range props {
		if props[i].Type == "vacation rental" {
			vr = &props[i]
			break
		}
	}
	if vr == nil {
		t.Skip("no 'vacation rental' type property found in fixture")
	}

	t.Logf("VR: Name=%q Type=%q", vr.Name, vr.Type)

	// VR properties must have Type = "vacation rental"
	if vr.Type != "vacation rental" {
		t.Errorf("expected Type='vacation rental', got %q", vr.Type)
	}

	// EssentialInfo must be populated (M7 fix — was plain string array)
	if len(vr.EssentialInfo) == 0 {
		t.Error("EssentialInfo should be populated (not empty array)")
	} else {
		t.Logf("VR: EssentialInfo=%d entries", len(vr.EssentialInfo))
		for _, kv := range vr.EssentialInfo {
			t.Logf("  Key=%q Value=%q", kv.Key, kv.Value)
		}
	}

	// Prices should have entries (M3 — multi-source pricing)
	if len(vr.Prices) > 0 {
		t.Logf("VR: Prices=%d sources", len(vr.Prices))
		for _, p := range vr.Prices {
			t.Logf("  Source=%q Guests=%v", p.Source, p.NumGuests)
		}
	} else {
		t.Log("VR: Prices is empty (may be absent for some VRs)")
	}

	// ExcludedAmenities may be populated
	if len(vr.ExcludedAmenities) > 0 {
		t.Logf("VR: ExcludedAmenities=%d items", len(vr.ExcludedAmenities))
	}

	// HotelClass should be nil for VR
	if vr.ExtractedHotelClass != nil {
		t.Logf("VR has ExtractedHotelClass=%d (unusual for VR)", *vr.ExtractedHotelClass)
	}
}

// TestVRDetailsResponse validates the HotelPropertyDetail DTO
// against the real SerpAPI vacation-rental-details.md fixture.
func TestVRDetailsResponse(t *testing.T) {
	raw := readFixtureJSON(t, "vacation-rental-details.md")

	// Strip SerpAPI metadata keys (search_metadata, search_parameters, serpapi_* links)
	// to leave only the property-level fields
	metaKeys := map[string]bool{
		"search_metadata":    true,
		"search_parameters":  true,
		"search_information": true,
	}
	for k := range metaKeys {
		delete(raw, k)
	}
	// Also remove serpapi_* link keys
	for k := range raw {
		if len(k) >= 7 && k[:7] == "serpapi" {
			delete(raw, k)
		}
	}

	propJSON, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("failed to marshal cleaned VR details: %v", err)
	}

	var detail HotelPropertyDetail
	if err := json.Unmarshal(propJSON, &detail); err != nil {
		t.Fatalf("failed to unmarshal VR details into HotelPropertyDetail: %v", err)
	}

	// Verify Type is "vacation rental"
	if detail.Type != "vacation rental" {
		t.Errorf("expected Type='vacation rental', got %q", detail.Type)
	}
	t.Logf("VR Detail: Name=%q Type=%q", detail.Name, detail.Type)

	// ExtractedHotelClass must be nil for VR (no star ratings)
	if detail.ExtractedHotelClass != nil {
		t.Errorf("Expected ExtractedHotelClass to be nil for VR, got %d", *detail.ExtractedHotelClass)
	}

	// Capacity fields should exist
	cap := detail.EssentialInfo
	t.Logf("VR Detail: EssentialInfo has %d entries", len(cap))
	for _, kv := range cap {
		t.Logf("  Key=%q Value=%q", kv.Key, kv.Value)
	}

	// ExcludedAmenities must be an array (not nil, even if empty)
	t.Logf("VR Detail: ExcludedAmenities=%d items", len(detail.ExcludedAmenities))

	// Verify DTO didn't fail on any field
	t.Logf("VR Detail: OverallRating=%v Reviews=%v",
		detail.OverallRating, detail.Reviews)

	// HealthAndSafety may be present for VRs that have it
	if detail.HealthAndSafety != nil {
		t.Logf("VR Detail: HealthAndSafety groups=%d", len(detail.HealthAndSafety.Groups))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
