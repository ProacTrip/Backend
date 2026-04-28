// Lógica de negocio para obtener detalles de un vuelo.
// Orkesta cache y proveedor externo.
package flight_details

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Puerto de Cache
// =============================================================================

// Cache is the local port interface for cache operations.
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

// =============================================================================
// UseCase
// =============================================================================

// UseCase orchestrates flight details retrieval with caching.
type UseCase struct {
	provider   domain.FlightProvider
	cache      Cache
	detailsTTL time.Duration
	wg         sync.WaitGroup
}

// UseCaseDeps bundles dependencies for the flight details use case.
type UseCaseDeps struct {
	Provider   domain.FlightProvider
	Cache      Cache
	DetailsTTL time.Duration
}

// NewUseCase creates a new flight details use case.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		provider:   deps.Provider,
		cache:      deps.Cache,
		detailsTTL: deps.DetailsTTL,
	}
}

// Wait blocks until all fire-and-forget goroutines have completed.
// Call during graceful shutdown to avoid losing in-flight operations.
func (uc *UseCase) Wait() {
	uc.wg.Wait()
}

// =============================================================================
// Ejecución Principal
// =============================================================================

// Execute retrieves flight details with caching.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	// 1. Validate
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. Generate cache key — includes ALL params that affect results
	cacheKey := generateCacheKey(cmd)

	// 3. Try cache
	if cached, err := uc.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var resp Response
		if err := json.Unmarshal([]byte(cached), &resp); err == nil {
			resp.FromCache = true
			resp.CachedAt = new(time.Now())
			return &resp, nil
		}
		slog.WarnContext(ctx, "flight details cache unmarshal failed, falling through to provider",
			slog.String("key", cacheKey),
			slog.Any("err", err),
		)
	}

	// 4. Cache miss — call provider
	resp, err := uc.provider.GetDetails(ctx, cmd.BookingToken, cmd.Adults, cmd.Currency, cmd.Departure, cmd.Arrival, cmd.OutboundDate, cmd.ReturnDate)
	if err != nil {
		return nil, fmt.Errorf("flight details: %w", err)
	}

	// 5. Save to cache async — fire-and-forget with WaitGroup tracking
	bgCtx := context.WithoutCancel(ctx)
	uc.wg.Go(func() {
		data, err := json.Marshal(resp)
		if err != nil {
			slog.ErrorContext(bgCtx, "flight details cache marshal failed",
				slog.String("key", cacheKey),
				slog.Any("err", err),
			)
			return
		}
		if err := uc.cache.Set(bgCtx, cacheKey, string(data), uc.detailsTTL); err != nil {
			slog.ErrorContext(bgCtx, "flight details cache set failed",
				slog.String("key", cacheKey),
				slog.Any("err", err),
			)
		}
	})

	return resp, nil
}

// =============================================================================
// Generación de Clave de Cache
// =============================================================================

// generateCacheKey builds a deterministic cache key from ALL params that affect
// the flight details response. Uses blake3 hash for fixed-size keys.
func generateCacheKey(cmd Command) string {
	// Marshal the command to JSON — includes booking_token, adults, currency,
	// departure, arrival, outbound_date, return_date, gl, hl.
	raw, err := json.Marshal(cmd)
	if err != nil {
		// Fallback: limited key (should never happen).
		return fmt.Sprintf("details:fallback:%s:%s", cmd.BookingToken, cmd.Currency)
	}
	return "details:" + domain.HashKey(raw)
}
