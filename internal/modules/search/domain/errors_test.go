package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrRateLimitExceededSentinel(t *testing.T) {
	// Verify sentinel is defined and has expected message
	if ErrRateLimitExceeded.Error() != "rate limit exceeded" {
		t.Errorf("ErrRateLimitExceeded.Error() = %q, want %q",
			ErrRateLimitExceeded.Error(), "rate limit exceeded")
	}
}

func TestErrRateLimitExceededIsMatchable(t *testing.T) {
	// Verify errors.Is works with wrapped errors
	err := fmt.Errorf("serpapi rate limit exceeded: 50/50: %w", ErrRateLimitExceeded)
	if !errors.Is(err, ErrRateLimitExceeded) {
		t.Error("errors.Is should match wrapped ErrRateLimitExceeded")
	}

	// Verify it does NOT match with a different sentinel
	if errors.Is(ErrRateLimitExceeded, ErrProviderUnavailable) {
		t.Error("ErrRateLimitExceeded should NOT match ErrProviderUnavailable")
	}
}
