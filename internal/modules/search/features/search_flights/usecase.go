// Lógica de negocio para búsqueda de vuelos.
// Orkesta cache, proveedor externo e historial de búsquedas.
package search_flights

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/shared/encoding"
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

// UseCase orchestrates flight search with caching and history recording.
type UseCase struct {
	provider  domain.FlightProvider
	cache     Cache
	repo      domain.SearchHistoryRepository
	searchTTL time.Duration
	wg        sync.WaitGroup
}

// UseCaseDeps bundles dependencies for the search flights use case.
type UseCaseDeps struct {
	Provider  domain.FlightProvider
	Cache     Cache
	Repo      domain.SearchHistoryRepository
	SearchTTL time.Duration
}

// NewUseCase creates a new search flights use case.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		provider:  deps.Provider,
		cache:     deps.Cache,
		repo:      deps.Repo,
		searchTTL: deps.SearchTTL,
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

// Execute performs the flight search with caching and history recording.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	start := time.Now()

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
			resp.CachedAt = new(time.Now())

			// Capture full lengths before slicing (for history + pagination gating)
			resultCount := len(resp.BestFlights) + len(resp.OtherFlights)
			maxLen := max(len(resp.BestFlights), len(resp.OtherFlights))

			// Fire-and-forget: save to search history async (full count)
			// Use WithoutCancel so the goroutine survives handler return.
			saveCtx := context.WithoutCancel(ctx)
			elapsedMs := int(time.Since(start).Milliseconds())
			uc.wg.Go(func() {
				uc.saveSearchHistory(saveCtx, domainReq, resultCount, true,
					new(elapsedMs), cmd.IPAddress, cmd.UserAgent)
			})

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

	// 5. Call provider
	resp, err := uc.provider.Search(ctx, domainReq)
	if err != nil {
		return nil, fmt.Errorf("flight search: %w", err)
	}

	// 6. Cache the full (uncut) response — marshal before slicing to avoid data race
	fullData, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		slog.ErrorContext(ctx, "cache marshal failed",
			slog.String("key", cacheKey),
			slog.Any("err", marshalErr),
		)
	} else {
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

	// 7. Save to search history async — fire-and-forget (full count before slicing)
	resultCount := len(resp.BestFlights) + len(resp.OtherFlights)
	elapsedMs := int(time.Since(start).Milliseconds())
	saveCtx := context.WithoutCancel(ctx)
	uc.wg.Go(func() {
		uc.saveSearchHistory(saveCtx, domainReq, resultCount, false,
			new(elapsedMs), cmd.IPAddress, cmd.UserAgent)
	})

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
	raw, err := json.Marshal(req)
	if err != nil {
		return fmt.Sprintf("search:fallback:%s:%s:%s:%s:%s",
			req.TripType, req.Departure, req.Arrival,
			req.OutboundDate, req.ReturnDate)
	}
	return "search:" + domain.HashKey(raw)
}

// =============================================================================
// Historial de Búsqueda
// =============================================================================

func (uc *UseCase) saveSearchHistory(ctx context.Context, req domain.FlightSearchRequest, resultCount int, cacheHit bool, executionTimeMs *int, ipAddress, userAgent string) {
	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	rawQuery, _ := json.Marshal(req)

	entry := domain.SearchHistoryEntry{
		ID:              uuid.Must(uuid.NewV7()),
		QueryType:       "structured",
		RawQuery:        fmt.Sprintf("%s → %s", req.Departure, req.Arrival),
		ParsedParams:    rawQuery,
		ResultCount:     resultCount,
		ExecutionTimeMs: executionTimeMs,
		CacheHit:        cacheHit,
		IPAddress:       ipAddress,
		UserAgent:       userAgent,
		CreatedAt:       time.Now(),
	}

	if err := uc.repo.Save(bgCtx, entry); err != nil {
		slog.ErrorContext(bgCtx, "search history save failed",
			slog.String("query_type", entry.QueryType),
			slog.String("raw_query", entry.RawQuery),
			slog.Any("err", err),
		)
	}
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
	offset, _ := encoding.DecodeCursor(*cursor)
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
		meta.PrevCursor = new(encoding.EncodeCursor(max(0, offset-limit)))
	}

	if meta.HasNext {
		meta.NextCursor = new(encoding.EncodeCursor(offset + limit))
	}

	return meta
}
