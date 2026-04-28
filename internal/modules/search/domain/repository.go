// Puerto para persistir historial de búsquedas.
// Permite guardar búsquedas para analítica y features proactivas.
package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// SearchHistory Repository
// =============================================================================

// SearchHistoryRepository records search intent for analytics and proactive features.
type SearchHistoryRepository interface {
	Save(ctx context.Context, entry SearchHistoryEntry) error
}

// SearchHistoryEntry represents a recorded search attempt.
type SearchHistoryEntry struct {
	ID              uuid.UUID
	UserID          *uuid.UUID
	SessionID       string
	QueryType       string
	RawQuery        string
	ParsedParams    json.RawMessage
	ResultCount     int
	ExecutionTimeMs *int
	CacheHit        bool
	IPAddress       string
	UserAgent       string
	CreatedAt       time.Time
}
