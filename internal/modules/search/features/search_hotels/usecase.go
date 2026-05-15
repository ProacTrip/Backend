// Lógica de negocio para búsqueda de hoteles y vacation rentals.
// Orquesta cache y proveedor externo via HotelProvider interface.
package search_hotels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	searchshared "github.com/ProacTrip/Backend/internal/modules/search/shared"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
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

// UseCase orchestrates hotel search with caching.
type UseCase struct {
	hotelProvider domain.HotelProvider
	cache         Cache
	rateLimiter   *ratelimit.RateLimiter
	searchTTL     time.Duration
	wg            sync.WaitGroup
}

// UseCaseDeps bundles dependencies for the search hotels use case.
type UseCaseDeps struct {
	Provider    domain.HotelProvider
	Cache       Cache
	RateLimiter *ratelimit.RateLimiter
	SearchTTL   time.Duration
}

// NewUseCase creates a new search hotels use case.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		hotelProvider: deps.Provider,
		cache:         deps.Cache,
		rateLimiter:   deps.RateLimiter,
		searchTTL:     deps.SearchTTL,
	}
}

// Wait blocks until all fire-and-forget goroutines have completed.
func (uc *UseCase) Wait() {
	uc.wg.Wait()
}

// =============================================================================
// Ejecución Principal
// =============================================================================

// Execute performs the hotel search with caching.
// Validate() ya se ejecutó en el handler — no se repite acá.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	// 2. Convert to domain request
	domainReq := cmd.ToDomain()

	// Filtrar hoteles sin precios (placeholders de Google Maps)
	// estableciendo min_price=1 internamente solo si el usuario no especificó uno.
	if domainReq.MinPrice == nil || *domainReq.MinPrice == 0 {
		minPrice := 1.0
		domainReq.MinPrice = &minPrice
	}

	// 3. Generate cache key — includes page_token for independent page caching
	cacheKey := generateCacheKey(domainReq)

	// 4. Try cache
	slog.DebugContext(ctx, "checking cache for hotel search",
		slog.String("key", cacheKey),
		slog.String("query", cmd.Query),
	)

	if cached, err := uc.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var resp Response
		if err := json.Unmarshal([]byte(cached), &resp); err == nil {
			slog.InfoContext(ctx, "hotel search cache hit",
				slog.String("key", cacheKey),
				slog.String("query", cmd.Query),
				slog.Int("property_count", len(resp.Properties)),
			)
			resp.FromCache = true
			// CachedAt is already stored in the cache entry — don't recompute
			return &resp, nil
		}
		slog.WarnContext(ctx, "hotel search cache unmarshal failed, falling through to provider",
			slog.String("key", cacheKey),
			slog.Any("err", err),
		)
	}

	slog.InfoContext(ctx, "hotel search cache miss, calling provider",
		slog.String("key", cacheKey),
		slog.String("query", cmd.Query),
		slog.String("check_in", cmd.CheckInDate),
		slog.String("check_out", cmd.CheckOutDate),
		slog.Int("adults", cmd.Adults),
	)

	// 5. Rate limit check — after cache miss, before provider call
	if uc.rateLimiter != nil {
		if result, err := uc.rateLimiter.ProviderAllow(ctx, "serpapi"); err != nil {
			slog.ErrorContext(ctx, "rate limit check failed", slog.Any("error", err))
			return nil, serrors.ErrInternalError("rate limit service unavailable", err)
		} else if !result.Allowed {
			return nil, domain.ErrRateLimitExceeded
		}
	}

	// 6. Cache miss — call provider via HotelProvider interface
	slog.InfoContext(ctx, "hotel search calling provider",
		slog.String("query", cmd.Query),
		slog.String("check_in", cmd.CheckInDate),
		slog.String("check_out", cmd.CheckOutDate),
	)
	resp, err := uc.hotelProvider.SearchHotels(ctx, domainReq)
	if err != nil {
		slog.ErrorContext(ctx, "hotel search provider call failed",
			slog.String("query", cmd.Query),
			slog.Any("error", err),
		)
		return nil, fmt.Errorf("hotel search: %w", err)
	}

	slog.InfoContext(ctx, "hotel search provider response received",
		slog.String("query", cmd.Query),
		slog.Int("properties_count", len(resp.Properties)),
		slog.String("results_state", resp.ResultsState),
	)

	// 7. Set cached_at timestamp and save to cache async — fire-and-forget with WaitGroup tracking.
	resp.CachedAt = new(time.Now())
	fullData, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		slog.ErrorContext(ctx, "hotel search cache marshal failed",
			slog.String("key", cacheKey),
			slog.Any("err", marshalErr),
		)
	} else {
		bgCtx := context.WithoutCancel(ctx)
		uc.wg.Go(func() {
			if err := uc.cache.Set(bgCtx, cacheKey, string(fullData), uc.searchTTL); err != nil {
				slog.ErrorContext(bgCtx, "hotel search cache set failed",
					slog.String("key", cacheKey),
					slog.Any("err", err),
				)
			}
		})
	}

	return resp, nil
}

// =============================================================================
// Generación de Clave de Cache
// =============================================================================

// generateCacheKey builds a deterministic cache key from the domain request.
// PageToken is included so each page gets its own independent cache entry.
func generateCacheKey(req domain.HotelSearchRequest) string {
	raw, err := json.Marshal(req)
	if err != nil {
		// Fallback: limited key
		return fmt.Sprintf("hotels:v2:fallback:%s:%s:%s:%s",
			req.Query, req.CheckInDate, req.CheckOutDate, searchshared.PtrOrEmpty(req.Currency))
	}
	return "{search}:hotels:v2:" + domain.HashKey(raw)
}
