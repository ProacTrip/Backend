// Lógica de negocio para búsqueda de vuelos.
// Orquesta cache y proveedor externo.
package search_flights

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/shared/pagination"
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

// UseCase orchestrates flight search with caching.
type UseCase struct {
	provider    domain.FlightProvider
	cache       Cache
	rateLimiter *ratelimit.RateLimiter
	searchTTL   time.Duration
	wg          sync.WaitGroup
}

// UseCaseDeps bundles dependencies for the search flights use case.
type UseCaseDeps struct {
	Provider    domain.FlightProvider
	Cache       Cache
	RateLimiter *ratelimit.RateLimiter
	SearchTTL   time.Duration
}

// NewUseCase creates a new search flights use case.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		provider:    deps.Provider,
		cache:       deps.Cache,
		rateLimiter: deps.RateLimiter,
		searchTTL:   deps.SearchTTL,
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

// Execute performs the flight search with caching.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	// 1. Validate
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. Convert to domain request
	domainReq := cmd.ToDomain()

	// 3. Generate cache key
	cacheKey := generateCacheKey(domainReq)

	// 4. Try cache
		if cached, err := uc.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var resp Response
		if err := json.Unmarshal([]byte(cached), &resp); err == nil {
			resp.FromCache = true
			// CachedAt se preserva del valor cacheado — no se recalcula time.Now()
			// (el unmarshal ya pobló resp.CachedAt desde el JSON cacheado)

			// Capture full lengths before slicing (for pagination gating)
			maxLen := max(len(resp.BestFlights), len(resp.OtherFlights))

			// Slice response to return only the requested page
			offset := decodeCursorFromReq(domainReq.Cursor)
			limit := domainReq.Limit
			resp.BestFlights = sliceSlice(resp.BestFlights, offset, limit)
			resp.OtherFlights = sliceSlice(resp.OtherFlights, offset, limit)
			resp.Meta = buildMeta(offset, limit, maxLen)

			return &resp, nil
		}
		slog.WarnContext(ctx, "cache unmarshal failed, falling through to provider",
			slog.String("key", cacheKey),
			slog.Any("err", err),
		)
	}

	// 5. Rate limit check — after cache miss, before provider call
	if uc.rateLimiter != nil {
		if result, err := uc.rateLimiter.ProviderAllow(ctx, "serpapi"); err != nil {
			slog.ErrorContext(ctx, "rate limit check failed", slog.Any("error", err))
			return nil, serrors.ErrInternalError("rate limit service unavailable", err)
		} else if !result.Allowed {
			return nil, domain.ErrRateLimitExceeded
		}
	}

	// 6. Call provider
	resp, err := uc.provider.SearchFlights(ctx, domainReq)
	if err != nil {
		return nil, fmt.Errorf("flight search: %w", err)
	}

	// 7. Cache the full (uncut) response — marshal before slicing to avoid data race
	resp.CachedAt = new(time.Now()) // timestamp del fetch original
	fullData, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		slog.ErrorContext(ctx, "cache marshal failed",
			slog.String("key", cacheKey),
			slog.Any("err", marshalErr),
		)
	} else if cacheKey != "" {
		bgCtx := context.WithoutCancel(ctx)
		uc.wg.Go(func() {
			if err := uc.cache.Set(bgCtx, cacheKey, string(fullData), uc.searchTTL); err != nil {
				slog.ErrorContext(bgCtx, "cache set failed",
					slog.String("key", cacheKey),
					slog.Any("err", err),
				)
			}
		})
	}

	// 8. Slice response and build pagination meta
	maxLen := max(len(resp.BestFlights), len(resp.OtherFlights))
	offset := decodeCursorFromReq(domainReq.Cursor)
	limit := domainReq.Limit
	resp.BestFlights = sliceSlice(resp.BestFlights, offset, limit)
	resp.OtherFlights = sliceSlice(resp.OtherFlights, offset, limit)
	resp.Meta = buildMeta(offset, limit, maxLen)

	return resp, nil
}

// =============================================================================
// Generación de Clave de Cache
// =============================================================================

// generateCacheKey builds a deterministic cache key from the domain request.
// Uses blake3 hash of the JSON-serialized request for fixed-size keys regardless
// of how many filter params are present.
func generateCacheKey(req domain.FlightSearchRequest) string {
	// Cursor is a pagination detail — different pages share the same provider response.
	// Exclude it from cache key so pages hit the same cached result (like hotels excludes page_token).
	req.Cursor = nil
	raw, err := json.Marshal(req)
	if err != nil {
		return fmt.Sprintf("flights:fallback:%s:%s:%s:%s:%s",
			req.TripType, req.Departure, req.Arrival,
			req.OutboundDate, req.ReturnDate)
	}
	return "{search}:flights:" + domain.HashKey(raw)
}

// =============================================================================
// Helpers de Paginación
// =============================================================================

// decodeCursorFromReq decodes the cursor string to an integer offset.
// Returns 0 if cursor is nil or empty (graceful first-page fallback).
func decodeCursorFromReq(cursor *string) int {
	if cursor == nil || *cursor == "" {
		return 0
	}
	offset, _ := pagination.DecodeCursor(*cursor)
	return offset
}

// sliceSlice returns a sub-slice of s starting at offset, up to limit elements.
// Returns nil if offset is beyond the slice bounds. Never panics.
func sliceSlice[T any](s []T, offset, limit int) []T {
	if offset >= len(s) {
		return nil
	}
	end := offset + limit
	if end > len(s) {
		end = len(s)
	}
	return s[offset:end]
}

// buildMeta constructs PaginationMeta given offset, limit, and total count.
// total is the max of BestFlights/OtherFlights lengths (for has_next).
func buildMeta(offset, limit, total int) *domain.PaginationMeta {
	meta := &domain.PaginationMeta{
		HasNext: offset+limit < total,
		Limit:   limit,
	}

	if offset > 0 {
		meta.PrevCursor = new(pagination.EncodeCursor(max(0, offset-limit)))
	}

	if meta.HasNext {
		meta.NextCursor = new(pagination.EncodeCursor(offset + limit))
	}

	return meta
}
