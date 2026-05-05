// Lógica de negocio para obtener detalles de un hotel.
// Orquesta cache y proveedor externo via HotelProvider interface.
package hotel_details

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
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

// UseCase orchestrates hotel details retrieval with caching.
type UseCase struct {
	hotelProvider domain.HotelProvider
	cache         Cache
	rateLimiter   *ratelimit.RateLimiter
	detailsTTL    time.Duration
	wg            sync.WaitGroup
}

// UseCaseDeps bundles dependencies for the hotel details use case.
type UseCaseDeps struct {
	Provider    domain.HotelProvider
	Cache       Cache
	RateLimiter *ratelimit.RateLimiter
	DetailsTTL  time.Duration
}

// NewUseCase creates a new hotel details use case.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		hotelProvider: deps.Provider,
		cache:         deps.Cache,
		rateLimiter:   deps.RateLimiter,
		detailsTTL:    deps.DetailsTTL,
	}
}

// Wait blocks until all fire-and-forget goroutines have completed.
func (uc *UseCase) Wait() {
	uc.wg.Wait()
}

// =============================================================================
// Ejecución Principal
// =============================================================================

// Execute retrieves hotel details with caching.
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
			resp.CachedAt = new(time.Now().UTC().Format(time.RFC3339))
			return &resp, nil
		}
		slog.WarnContext(ctx, "hotel details cache unmarshal failed, falling through to provider",
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

	// 6. Cache miss — call provider via HotelProvider interface
	resp, err := uc.hotelProvider.GetHotelDetails(ctx, domainReq)
	if err != nil {
		return nil, fmt.Errorf("hotel details: %w", err)
	}

	// 7. Save to cache async — fire-and-forget
	bgCtx := context.WithoutCancel(ctx)
	uc.wg.Go(func() {
		data, err := json.Marshal(resp)
		if err != nil {
			slog.ErrorContext(bgCtx, "hotel details cache marshal failed",
				slog.String("key", cacheKey),
				slog.Any("err", err),
			)
			return
		}
		if err := uc.cache.Set(bgCtx, cacheKey, string(data), uc.detailsTTL); err != nil {
			slog.ErrorContext(bgCtx, "hotel details cache set failed",
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
// the hotel details response. Uses blake3 hash for fixed-size keys.
func generateCacheKey(req domain.HotelDetailsRequest) string {
	raw, err := json.Marshal(req)
	if err != nil {
		return fmt.Sprintf("hotel-detail:fallback:%s:%s:%s:%s",
			req.ID, req.CheckInDate, req.CheckOutDate, ptrStr(req.Currency))
	}
	return "hotel-detail:" + domain.HashKey(raw)
}

// ptrStr returns the dereferenced string, or "" if nil.
func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
