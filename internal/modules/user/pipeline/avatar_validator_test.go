// Tests del AvatarValidator: validación de avatares vía Dragonfly Streams.
// Simula miniredis y mockea ProfileRepository para verificar flujo completo.
package pipeline_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/modules/user/pipeline"
)

// =============================================================================
// Mock: ProfileRepository para avatar validation
// =============================================================================

type avatarMockRepo struct {
	mu          sync.Mutex
	avatars     map[uuid.UUID]string // userID → avatarURL
	updateErr   error
	updateDone chan struct{} // señala cuando UpdateAvatar es llamado
	updateCalls int
}

func newAvatarMockRepo() *avatarMockRepo {
	return &avatarMockRepo{avatars: make(map[uuid.UUID]string), updateDone: make(chan struct{}, 1)}
}

func (m *avatarMockRepo) Create(ctx context.Context, profile *domain.UserProfile) error { return nil }
func (m *avatarMockRepo) UpsertProfile(ctx context.Context, profile *domain.UserProfile) error {
	return nil
}
func (m *avatarMockRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	return nil, nil
}
func (m *avatarMockRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
	return nil, nil
}
func (m *avatarMockRepo) Update(ctx context.Context, profile *domain.UserProfile) error {
	return nil
}
func (m *avatarMockRepo) UpdateLocale(ctx context.Context, userID uuid.UUID, language, currency string) error {
	return nil
}
func (m *avatarMockRepo) UpdatePreferences(ctx context.Context, userID uuid.UUID, language, currency string) error {
	return nil
}

func (m *avatarMockRepo) UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	if m.updateErr != nil {
		return m.updateErr
	}
	m.avatars[userID] = avatarURL
	select { case m.updateDone <- struct{}{}: default: }
	return nil
}

// =============================================================================
// Test: flujo exitoso de validación de avatar
// =============================================================================

func TestAvatarValidator_ProcesaAvatarCorrectamente(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newAvatarMockRepo()
	userID := uuid.Must(uuid.NewV7())

	// Crear el worker
	worker := pipeline.NewAvatarValidator(rdb, repo, "")

	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	// Verificar que está corriendo
	if !worker.IsRunning() {
		t.Fatal("worker debería estar corriendo")
	}

	// Publicar mensaje en el stream de avatar
	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:avatar:validate",
		ID:     "*",
		Values: map[string]interface{}{
			"user_id":     userID.String(),
			"storage_key": "avatars/prod/user123/avatar.jpg",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	// Esperar procesamiento (el worker usa Block: 5s)
	<-time.After(200 * time.Millisecond)

	// Cancelar contexto y esperar cleanup
	cancel()
	<-time.After(200 * time.Millisecond)

	// Verificar que UpdateAvatar fue llamado
	repo.mu.Lock()
	defer repo.mu.Unlock()

	if repo.updateCalls == 0 {
		t.Fatal("UpdateAvatar nunca fue llamado")
	}

	avatarURL, ok := repo.avatars[userID]
	if !ok {
		t.Fatal("no se registró avatar para el usuario")
	}
	expectedURL := "https://cdn.proactrip.com/avatars/prod/user123/avatar.jpg"
	if avatarURL != expectedURL {
		t.Errorf("avatarURL = %q, want %q", avatarURL, expectedURL)
	}
}

// =============================================================================
// Test: mensaje sin user_id → XACK inmediato
// =============================================================================

func TestAvatarValidator_MensajeSinUserID_RechazaInmediato(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newAvatarMockRepo()
	worker := pipeline.NewAvatarValidator(rdb, repo, "")

	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	// Publicar mensaje SIN user_id
	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:avatar:validate",
		ID:     "*",
		Values: map[string]interface{}{
			"storage_key": "avatars/avatar.jpg",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(200 * time.Millisecond)
	cancel()
	<-time.After(200 * time.Millisecond)

	// Verificar que UpdateAvatar NO fue llamado
	repo.mu.Lock()
	defer repo.mu.Unlock()

	if repo.updateCalls > 0 {
		t.Errorf("UpdateAvatar fue llamado %d veces con mensaje sin user_id", repo.updateCalls)
	}
}

// =============================================================================
// Test: mensaje sin storage_key → XACK inmediato
// =============================================================================

func TestAvatarValidator_MensajeSinStorageKey_RechazaInmediato(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newAvatarMockRepo()
	worker := pipeline.NewAvatarValidator(rdb, repo, "")

	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:avatar:validate",
		ID:     "*",
		Values: map[string]interface{}{
			"user_id": uuid.Must(uuid.NewV7()).String(),
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(200 * time.Millisecond)
	cancel()
	<-time.After(200 * time.Millisecond)

	repo.mu.Lock()
	defer repo.mu.Unlock()

	if repo.updateCalls > 0 {
		t.Errorf("UpdateAvatar fue llamado %d veces con mensaje sin storage_key", repo.updateCalls)
	}
}

// =============================================================================
// Test: UUID inválido → XACK inmediato
// =============================================================================

func TestAvatarValidator_UserIDInvalido_RechazaInmediato(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newAvatarMockRepo()
	worker := pipeline.NewAvatarValidator(rdb, repo, "")

	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:avatar:validate",
		ID:     "*",
		Values: map[string]interface{}{
			"user_id":     "no-es-un-uuid",
			"storage_key": "avatars/avatar.jpg",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(200 * time.Millisecond)
	cancel()
	<-time.After(200 * time.Millisecond)

	repo.mu.Lock()
	defer repo.mu.Unlock()

	if repo.updateCalls > 0 {
		t.Errorf("UpdateAvatar fue llamado con UUID inválido")
	}
}

// =============================================================================
// Test: fallo de UpdateAvatar → mensaje queda en PEL (NO XACK)
// =============================================================================

func TestAvatarValidator_FalloUpdateAvatar_QuedaEnPEL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newAvatarMockRepo()
	repo.updateErr = errors.New("database connection refused")

	userID := uuid.Must(uuid.NewV7())
	worker := pipeline.NewAvatarValidator(rdb, repo, "")

	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:avatar:validate",
		ID:     "*",
		Values: map[string]interface{}{
			"user_id":     userID.String(),
			"storage_key": "avatars/avatar.jpg",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	// Esperar que el worker intente procesar
	<-time.After(200 * time.Millisecond)

	// Verificar que el mensaje está en PEL (NO fue XACkeado porque falló UpdateAvatar)
	pending, err := rdb.XPending(ctx, "{events}:avatar:validate", "avatar-validator-group").Result()
	if err != nil {
		t.Fatalf("XPending falló: %v", err)
	}

	// Al menos 1 mensaje en PEL (el nuestro)
	if pending.Count == 0 {
		t.Error("mensaje debería estar en PEL porque UpdateAvatar falló")
	}

	cancel()
	<-time.After(200 * time.Millisecond)
}

// =============================================================================
// Test: ID del worker y running status
// =============================================================================

func TestAvatarValidator_NombreYEstado(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newAvatarMockRepo()
	worker := pipeline.NewAvatarValidator(rdb, repo, "")

	// El nombre es fijo
	if worker.Name() != "avatar-validator" {
		t.Errorf("Name() = %q, want 'avatar-validator'", worker.Name())
	}

	// No está corriendo hasta que se llama Run
	if worker.IsRunning() {
		t.Error("IsRunning() debería ser false antes de Run()")
	}
}
