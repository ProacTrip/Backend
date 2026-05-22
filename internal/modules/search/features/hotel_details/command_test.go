package hotel_details

import (
	"errors"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Validate — children_ages validation (C-3)
// =============================================================================

func TestCommandValidate_ChildrenAges_MismatchedCount(t *testing.T) {
	cmd := &Command{
		ID:          "prop_abc123",
		Query:       "Grand Hotel",
		CheckInDate: "2026-06-15",
		CheckOutDate: "2026-06-20",
		Adults:      2,
		Children:    2,
		ChildrenAges: []int{5}, // solo 1 edad, pero children=2
	}

	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for mismatched children_ages count")
	}
	if !errors.Is(err, domain.ErrInvalidParameterRange) {
		t.Errorf("expected ErrInvalidParameterRange, got: %v", err)
	}
}

func TestCommandValidate_ChildrenAges_OutOfRange(t *testing.T) {
	tests := []struct {
		name string
		age  int
	}{
		{"age zero", 0},
		{"age eighteen", 18},
		{"negative age", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &Command{
				ID:          "prop_abc123",
				Query:       "Grand Hotel",
				CheckInDate: "2026-06-15",
				CheckOutDate: "2026-06-20",
				Adults:      2,
				Children:    1,
				ChildrenAges: []int{tt.age},
			}

			err := cmd.Validate()
			if err == nil {
				t.Fatalf("expected error for age=%d", tt.age)
			}
			if !errors.Is(err, domain.ErrInvalidParameterRange) {
				t.Errorf("expected ErrInvalidParameterRange, got: %v", err)
			}
		})
	}
}

func TestCommandValidate_ChildrenAges_Valid(t *testing.T) {
	cmd := &Command{
		ID:          "prop_abc123",
		Query:       "Grand Hotel",
		CheckInDate: "2026-06-15",
		CheckOutDate: "2026-06-20",
		Adults:      2,
		Children:    2,
		ChildrenAges: []int{5, 12},
	}

	err := cmd.Validate()
	if err != nil {
		t.Errorf("expected no error for valid children_ages, got: %v", err)
	}
}

// =============================================================================
// Validate — query non-empty validation (C-4)
// =============================================================================

func TestCommandValidate_Query_Empty(t *testing.T) {
	cmd := &Command{
		ID:          "prop_abc123",
		Query:       "",
		CheckInDate: "2026-06-15",
		CheckOutDate: "2026-06-20",
		Adults:      2,
	}

	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	if !errors.Is(err, domain.ErrMissingRequiredField) {
		t.Errorf("expected ErrMissingRequiredField, got: %v", err)
	}
}

func TestCommandValidate_Query_WhitespaceOnly(t *testing.T) {
	cmd := &Command{
		ID:          "prop_abc123",
		Query:       "   ",
		CheckInDate: "2026-06-15",
		CheckOutDate: "2026-06-20",
		Adults:      2,
	}

	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for whitespace-only query")
	}
	if !errors.Is(err, domain.ErrMissingRequiredField) {
		t.Errorf("expected ErrMissingRequiredField, got: %v", err)
	}
}

func TestCommandValidate_Query_Valid(t *testing.T) {
	cmd := &Command{
		ID:          "prop_abc123",
		Query:       "Grand Hotel",
		CheckInDate: "2026-06-15",
		CheckOutDate: "2026-06-20",
		Adults:      2,
	}

	err := cmd.Validate()
	if err != nil {
		t.Errorf("expected no error for valid query, got: %v", err)
	}
}

// =============================================================================
// Validate — ID validation (pre-existing, regression)
// =============================================================================

func TestCommandValidate_ID_Empty(t *testing.T) {
	cmd := &Command{
		Query:       "Grand Hotel",
		CheckInDate: "2026-06-15",
		CheckOutDate: "2026-06-20",
	}

	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
	if !errors.Is(err, domain.ErrTokenRequired) {
		t.Errorf("expected ErrTokenRequired, got: %v", err)
	}
}
