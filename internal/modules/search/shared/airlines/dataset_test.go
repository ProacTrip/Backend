package airlines_test

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/search/shared/airlines"
)

func TestResolveAirline_ExactIATA(t *testing.T) {
	entry, err := airlines.ResolveAirline("IB")
	if err != nil {
		t.Fatalf("expected IB to resolve, got error: %v", err)
	}
	if entry.IATA != "IB" {
		t.Errorf("expected IATA IB, got %s", entry.IATA)
	}
	if entry.Name != "Iberia" {
		t.Errorf("expected name Iberia, got %s", entry.Name)
	}
}

func TestResolveAirline_ExactName(t *testing.T) {
	entry, err := airlines.ResolveAirline("Iberia")
	if err != nil {
		t.Fatalf("expected Iberia to resolve, got error: %v", err)
	}
	if entry.IATA != "IB" {
		t.Errorf("expected IATA IB, got %s", entry.IATA)
	}
}

func TestResolveAirline_CaseInsensitive(t *testing.T) {
	entry, err := airlines.ResolveAirline("iberia")
	if err != nil {
		t.Fatalf("expected iberia to resolve, got error: %v", err)
	}
	if entry.IATA != "IB" {
		t.Errorf("expected IATA IB, got %s", entry.IATA)
	}
}

func TestResolveAirline_Alias(t *testing.T) {
	entry, err := airlines.ResolveAirline("Iberia Airlines")
	if err != nil {
		t.Fatalf("expected alias to resolve, got error: %v", err)
	}
	if entry.IATA != "IB" {
		t.Errorf("expected IATA IB, got %s", entry.IATA)
	}
}

func TestResolveAirline_FuzzyTypo(t *testing.T) {
	entry, err := airlines.ResolveAirline("Iberai")
	if err != nil {
		t.Fatalf("expected fuzzy match for Iberai, got error: %v", err)
	}
	if entry.IATA != "IB" {
		t.Errorf("expected IATA IB, got %s", entry.IATA)
	}
}

func TestResolveAirline_Unknown(t *testing.T) {
	_, err := airlines.ResolveAirline("zzzzz_unknown_airline")
	if err == nil {
		t.Fatal("expected error for unknown airline, got nil")
	}
}

func TestResolveAirline_EmptyQuery(t *testing.T) {
	_, err := airlines.ResolveAirline("")
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}
}

func TestResolveAirline_WhitespaceOnly(t *testing.T) {
	_, err := airlines.ResolveAirline("   ")
	if err == nil {
		t.Fatal("expected error for whitespace query, got nil")
	}
}

func TestResolveAirlineToIATA(t *testing.T) {
	iata, err := airlines.ResolveAirlineToIATA("Iberia")
	if err != nil {
		t.Fatalf("expected IATA, got error: %v", err)
	}
	if iata != "IB" {
		t.Errorf("expected IB, got %s", iata)
	}
}

func TestResolveAirlineToIATA_Unknown(t *testing.T) {
	_, err := airlines.ResolveAirlineToIATA("zzzzz_unknown")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveAirline_MultipleAirlines(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantIATA string
	}{
		{name: "Iberia", query: "IB", wantIATA: "IB"},
		{name: "LATAM", query: "LA", wantIATA: "LA"},
		{name: "American", query: "American Airlines", wantIATA: "AA"},
		{name: "United", query: "United", wantIATA: "UA"},
		{name: "Delta", query: "Delta", wantIATA: "DL"},
		{name: "British Airways", query: "BA", wantIATA: "BA"},
		{name: "Lufthansa", query: "Lufthansa", wantIATA: "LH"},
		{name: "Air France", query: "Air France", wantIATA: "AF"},
		{name: "KLM", query: "KLM", wantIATA: "KL"},
		{name: "Emirates", query: "Emirates", wantIATA: "EK"},
		{name: "Ryanair", query: "FR", wantIATA: "FR"},
		{name: "Vueling", query: "VY", wantIATA: "VY"},
		{name: "Aerolíneas Argentinas", query: "Aerolíneas Argentinas", wantIATA: "AR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := airlines.ResolveAirline(tt.query)
			if err != nil {
				t.Fatalf("ResolveAirline(%q) error: %v", tt.query, err)
			}
			if entry.IATA != tt.wantIATA {
				t.Errorf("ResolveAirline(%q) = %s, want %s", tt.query, entry.IATA, tt.wantIATA)
			}
		})
	}
}

func TestAll(t *testing.T) {
	all, err := airlines.All()
	if err != nil {
		t.Fatalf("expected airlines, got error: %v", err)
	}
	if len(all) < 60 {
		t.Errorf("expected at least 60 airlines, got %d", len(all))
	}
}
