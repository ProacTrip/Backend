// Lógica de negocio para obtener detalles de un vuelo.
// Orquesta cache y proveedor externo.
package flight_details

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

// UseCase orchestrates flight details retrieval with caching.
type UseCase struct {
	provider    domain.FlightProvider
	cache       Cache
	rateLimiter *ratelimit.RateLimiter
	detailsTTL  time.Duration
	wg          sync.WaitGroup
}

// UseCaseDeps bundles dependencies for the flight details use case.
type UseCaseDeps struct {
	Provider    domain.FlightProvider
	Cache       Cache
	RateLimiter *ratelimit.RateLimiter
	DetailsTTL  time.Duration
}

// NewUseCase creates a new flight details use case.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		provider:    deps.Provider,
		cache:       deps.Cache,
		rateLimiter: deps.RateLimiter,
		detailsTTL:  deps.DetailsTTL,
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

	// 2. Construir request de dominio para proveedor y cache key
	providerReq := domain.FlightDetailsRequest{
		BookingToken: cmd.BookingToken,
		Adults:       cmd.Adults,
		DepartureID:  cmd.Departure,
		ArrivalID:    cmd.Arrival,
		OutboundDate: cmd.OutboundDate,
		ReturnDate:   cmd.ReturnDate,
		GL:           searchshared.PtrOrEmpty(cmd.GL),
		HL:           searchshared.PtrOrEmpty(cmd.HL),
		Currency:     searchshared.PtrOrEmpty(cmd.Currency),
	}

	// 3. Generate cache key — includes ALL params that affect results
	cacheKey := generateCacheKey(providerReq)

	// 4. Try cache
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

	// 5. Rate limit check — after cache miss, before provider call
	if uc.rateLimiter != nil {
		if result, err := uc.rateLimiter.ProviderAllow(ctx, "serpapi"); err != nil {
			slog.ErrorContext(ctx, "rate limit check failed", slog.Any("error", err))
			return nil, serrors.ErrInternalError("rate limit service unavailable", err)
		} else if !result.Allowed {
			return nil, domain.ErrRateLimitExceeded
		}
	}

	// 6. Cache miss — call provider
	resp, err := uc.provider.GetFlightDetails(ctx, providerReq)
	if err != nil {
		return nil, fmt.Errorf("flight details: %w", err)
	}

	// 7. Set timestamp, marshal BEFORE spawning goroutine to prevent
	// data race: handler writes resp.FromCache=false after return, but
	// goroutine reads resp via json.Marshal. Pre-marshaling avoids the race.
	resp.CachedAt = new(time.Now()) // timestamp del fetch original
	fullData, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		slog.ErrorContext(ctx, "flight details cache marshal failed",
			slog.String("key", cacheKey),
			slog.Any("err", marshalErr),
		)
	} else {
		bgCtx := context.WithoutCancel(ctx)
		uc.wg.Go(func() {
			if err := uc.cache.Set(bgCtx, cacheKey, string(fullData), uc.detailsTTL); err != nil {
				slog.ErrorContext(bgCtx, "flight details cache set failed",
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

// generateCacheKey construye una clave de cache determinista a partir del request
// de dominio. Usa blake3 para claves de tamaño fijo.
func generateCacheKey(req domain.FlightDetailsRequest) string {
	raw, err := json.Marshal(req)
	if err != nil {
		// Fallback: clave limitada (no debería ocurrir).
		return fmt.Sprintf("details:fallback:%s:%s", req.BookingToken, req.Currency)
	}
	return "details:" + domain.HashKey(raw)
}
