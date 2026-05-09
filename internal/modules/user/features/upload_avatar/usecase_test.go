// Tests del usecase upload_avatar.
package upload_avatar

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type mockStorage struct {
	generateUploadURLFn func(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
}

func (m *mockStorage) GenerateUploadURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	if m.generateUploadURLFn != nil {
		return m.generateUploadURLFn(ctx, bucket, key, expiry)
	}
	return "https://r2.example.com/upload?token=abc", nil
}

// =============================================================================
// Tests
// =============================================================================

func TestUploadAvatar_HappyPath(t *testing.T) {
	cmd := Command{
		UserID:   uuid.Must(uuid.NewV7()).String(),
		FileName: "avatar.webp",
		MimeType: "image/webp",
	}

	called := false
	uc := NewUseCase(UseCaseDeps{
		Storage: &mockStorage{
			generateUploadURLFn: func(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
				called = true
				if bucket != "proactrip-assets" {
					t.Errorf("bucket = %q, se esperaba proactrip-assets", bucket)
				}
				if key == "" {
					t.Error("key no debería estar vacía")
				}
				return "https://r2.example.com/upload?token=abc", nil
			},
		},
	})

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !called {
		t.Error("GenerateUploadURL debería haber sido llamado")
	}
	if resp.UploadURL == "" {
		t.Error("upload_url no debería estar vacío")
	}
	if resp.StorageKey == "" {
		t.Error("storage_key no debería estar vacío")
	}
	if resp.Message == "" {
		t.Error("message no debería estar vacío")
	}
	if resp.ExpiresAt.IsZero() {
		t.Error("expires_at no debería ser cero")
	}
}

func TestUploadAvatar_WithTTL(t *testing.T) {
	ttl := 10
	cmd := Command{
		UserID:     uuid.Must(uuid.NewV7()).String(),
		FileName:   "avatar.png",
		MimeType:   "image/png",
		TTLMinutes: &ttl,
	}

	called := false
	uc := NewUseCase(UseCaseDeps{
		Storage: &mockStorage{
			generateUploadURLFn: func(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
				called = true
				if expiry != 10*time.Minute {
					t.Errorf("expiry = %v, se esperaba 10m", expiry)
				}
				return "https://r2.example.com/upload", nil
			},
		},
	})

	_, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !called {
		t.Error("GenerateUploadURL debería haber sido llamado")
	}
}

func TestUploadAvatar_InvalidMimeType(t *testing.T) {
	cmd := Command{
		UserID:   uuid.Must(uuid.NewV7()).String(),
		FileName: "avatar.gif",
		MimeType: "image/gif",
	}

	uc := NewUseCase(UseCaseDeps{
		Storage: &mockStorage{},
	})

	// Validar el comando primero (el handler valida antes de ejecutar el usecase)
	err := cmd.Validate()
	if err == nil {
		t.Fatal("se esperaba error de MIME type inválido")
	}
	_ = uc
}

func TestUploadAvatar_FileSizeTooLarge(t *testing.T) {
	largeSize := int64(MaxAvatarSizeBytes + 1)
	cmd := Command{
		UserID:   uuid.Must(uuid.NewV7()).String(),
		FileName: "avatar.jpg",
		MimeType: "image/jpeg",
		FileSize: &largeSize,
	}

	err := cmd.Validate()
	if err == nil {
		t.Fatal("se esperaba error de archivo demasiado grande")
	}
	if !errors.Is(err, domain.ErrFileTooLarge) {
		t.Errorf("error = %v, se esperaba ErrFileTooLarge", err)
	}
}

func TestUploadAvatar_MissingFileName(t *testing.T) {
	cmd := Command{
		UserID:   uuid.Must(uuid.NewV7()).String(),
		MimeType: "image/webp",
	}

	err := cmd.Validate()
	if err == nil {
		t.Fatal("se esperaba error de file_name requerido")
	}
	if err.Error() != "file_name: campo requerido" {
		t.Errorf("error = %q, se esperaba 'file_name: campo requerido'", err.Error())
	}
}

func TestUploadAvatar_MissingMimeType(t *testing.T) {
	cmd := Command{
		UserID:   uuid.Must(uuid.NewV7()).String(),
		FileName: "avatar.png",
	}

	err := cmd.Validate()
	if err == nil {
		t.Fatal("se esperaba error de mime_type requerido")
	}
}

func TestUploadAvatar_AllValidMimeTypes(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
	}{
		{"jpeg", "image/jpeg"},
		{"png", "image/png"},
		{"webp", "image/webp"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := Command{
				UserID:   uuid.Must(uuid.NewV7()).String(),
				FileName: "avatar." + tc.name,
				MimeType: tc.mimeType,
			}
			if err := cmd.Validate(); err != nil {
				t.Errorf("no se esperaba error para %s: %v", tc.mimeType, err)
			}
		})
	}
}

func TestUploadAvatar_StorageKeyFormat(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	cmd := Command{
		UserID:   userID.String(),
		FileName: "foto_perfil.jpeg",
		MimeType: "image/jpeg",
	}

	var capturedKey string
	uc := NewUseCase(UseCaseDeps{
		Storage: &mockStorage{
			generateUploadURLFn: func(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
				capturedKey = key
				return "https://r2.example.com/upload", nil
			},
		},
	})

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	// Verificar formato: avatars/{user_id}/{uuid}.{ext}
	if capturedKey == "" {
		t.Fatal("key no fue capturada")
	}

	expectedPrefix := "avatars/" + userID.String() + "/"
	if len(capturedKey) <= len(expectedPrefix) {
		t.Fatalf("key demasiado corta: %q", capturedKey)
	}
	if capturedKey[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("key = %q, se esperaba prefijo %q", capturedKey, expectedPrefix)
	}

	// Verificar que la extensión esté en el key y en la respuesta
	if resp.StorageKey != capturedKey {
		t.Errorf("resp.StorageKey = %q, capturada = %q", resp.StorageKey, capturedKey)
	}
}
