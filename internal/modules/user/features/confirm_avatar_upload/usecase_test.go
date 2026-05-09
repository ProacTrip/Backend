// Tests del usecase confirm_avatar_upload.
package confirm_avatar_upload

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type mockStorage struct {
	existsFn func(ctx context.Context, bucket, key string) (bool, error)
}

func (m *mockStorage) Exists(ctx context.Context, bucket, key string) (bool, error) {
	if m.existsFn != nil {
		return m.existsFn(ctx, bucket, key)
	}
	return true, nil
}

type mockEventPublisher struct {
	publishFn func(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

func (m *mockEventPublisher) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	if m.publishFn != nil {
		return m.publishFn(ctx, stream, payload)
	}
	return "msg-1", nil
}

// =============================================================================
// Tests
// =============================================================================

func TestConfirmAvatar_HappyPath(t *testing.T) {
	cmd := Command{
		UserID:     uuid.Must(uuid.NewV7()).String(),
		StorageKey: "avatars/user123/abc123.webp",
	}

	fileChecked := false
	eventPublished := false
	uc := NewUseCase(UseCaseDeps{
		Storage: &mockStorage{
			existsFn: func(ctx context.Context, bucket, key string) (bool, error) {
				fileChecked = true
				if bucket != "proactrip-assets" {
					t.Errorf("bucket = %q, se esperaba proactrip-assets", bucket)
				}
				return true, nil
			},
		},
		EventPublisher: &mockEventPublisher{
			publishFn: func(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
				eventPublished = true
				if stream != "{events}:avatar:validate" {
					t.Errorf("stream = %q, se esperaba {events}:avatar:validate", stream)
				}
				if payload["user_id"] == nil || payload["storage_key"] == nil {
					t.Error("payload debe contener user_id y storage_key")
				}
				return "msg-1", nil
			},
		},
	})

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !fileChecked {
		t.Error("Exists debería haber sido llamado")
	}
	if !eventPublished {
		t.Error("Publish debería haber sido llamado")
	}
	if resp.Status != "validating" {
		t.Errorf("status = %q, se esperaba validating", resp.Status)
	}
	if resp.Message == "" {
		t.Error("message no debería estar vacío")
	}
}

func TestConfirmAvatar_FileNotFound(t *testing.T) {
	cmd := Command{
		UserID:     uuid.Must(uuid.NewV7()).String(),
		StorageKey: "avatars/user123/nonexistent.webp",
	}

	uc := NewUseCase(UseCaseDeps{
		Storage: &mockStorage{
			existsFn: func(ctx context.Context, bucket, key string) (bool, error) {
				return false, nil
			},
		},
		EventPublisher: &mockEventPublisher{},
	})

	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error de avatar no encontrado")
	}
	if !errors.Is(err, domain.ErrAvatarNotFound) {
		t.Errorf("error = %v, se esperaba ErrAvatarNotFound", err)
	}
}

func TestConfirmAvatar_MissingStorageKey(t *testing.T) {
	cmd := Command{
		UserID: uuid.Must(uuid.NewV7()).String(),
	}

	err := cmd.Validate()
	if err == nil {
		t.Fatal("se esperaba error de storage_key requerido")
	}
}

func TestConfirmAvatar_R2Error(t *testing.T) {
	cmd := Command{
		UserID:     uuid.Must(uuid.NewV7()).String(),
		StorageKey: "avatars/user123/error.webp",
	}

	uc := NewUseCase(UseCaseDeps{
		Storage: &mockStorage{
			existsFn: func(ctx context.Context, bucket, key string) (bool, error) {
				return false, errors.New("R2 connection failed")
			},
		},
		EventPublisher: &mockEventPublisher{},
	})

	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error de verificación en R2")
	}
}

func TestConfirmAvatar_EventPublishFails(t *testing.T) {
	cmd := Command{
		UserID:     uuid.Must(uuid.NewV7()).String(),
		StorageKey: "avatars/user123/valid.webp",
	}

	uc := NewUseCase(UseCaseDeps{
		Storage: &mockStorage{
			existsFn: func(ctx context.Context, bucket, key string) (bool, error) {
				return true, nil
			},
		},
		EventPublisher: &mockEventPublisher{
			publishFn: func(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
				return "", errors.New("stream not available")
			},
		},
	})

	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error al publicar evento")
	}
}
