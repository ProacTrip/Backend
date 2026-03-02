package health

import (
	"context"
	"sync"
	"time"

	"github.com/ProacTrip/Backend/internal/infrastructure/database"
	cacheport "github.com/ProacTrip/Backend/internal/shared/domain/ports/cache"
	eventport "github.com/ProacTrip/Backend/internal/shared/domain/ports/eventbus"
)

type Checker struct {
	db    *database.PoolManager
	cache cacheport.Cache
	eb    eventport.Bus
}

type Response struct {
	Status    string            `json:"status"` // "healthy" o "degraded"
	Timestamp time.Time         `json:"timestamp"`
	Database  map[string]string `json:"database"` // auth, booking, etc.
	Cache     string            `json:"cache"`
	EventBus  string            `json:"eventbus"`
}

func NewChecker(db *database.PoolManager, c cacheport.Cache, eb eventport.Bus) *Checker {
	return &Checker{db: db, cache: c, eb: eb}
}

func (h *Checker) Check(ctx context.Context) Response {
	resp := Response{
		Timestamp: time.Now().UTC(),
		Database:  make(map[string]string),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	wg.Add(2)

	// === Cache (Se ejecuta en 2do plano) ===
	go func() {
		defer wg.Done()
		err := h.cache.HealthCheck(ctx)

		mu.Lock()
		if err == nil {
			resp.Cache = "healthy"
		} else {
			resp.Cache = "unhealthy"
		}
		mu.Unlock()
	}()

	// === EventBus (Se ejecuta en 2do plano) ===
	go func() {
		defer wg.Done()
		err := h.eb.HealthCheck(ctx)

		mu.Lock()
		if err == nil {
			resp.EventBus = "healthy"
		} else {
			resp.EventBus = "unhealthy"
		}
		mu.Unlock()
	}()

	// === Database (todas las 8 DBs) ===
	dbResults := h.db.HealthCheck(ctx)
	allDBHealthy := true
	for dbType, err := range dbResults {
		key := string(dbType)
		if err == nil {
			resp.Database[key] = "healthy"
		} else {
			resp.Database[key] = "unhealthy"
			allDBHealthy = false
		}
	}

	wg.Wait()

	// === Estado general ===
	if allDBHealthy && resp.Cache == "healthy" && resp.EventBus == "healthy" {
		resp.Status = "healthy"
	} else {
		resp.Status = "degraded"
	}

	return resp
}
