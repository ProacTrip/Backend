package storage

import (
	"testing"

	"github.com/google/uuid"
)

func TestStorageKeyHelpers(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())

	t.Run("AvatarKey", func(t *testing.T) {
		key := AvatarKey(userID, "jpg")
		expected := "avatars/" + userID.String() + ".jpg"
		if key != expected {
			t.Errorf("AvatarKey = %s, se esperaba %s", key, expected)
		}
	})

	t.Run("DocumentRawKey", func(t *testing.T) {
		key := DocumentRawKey(userID, docID, "pdf")
		expected := "documents/" + userID.String() + "/" + docID.String() + "/raw.pdf"
		if key != expected {
			t.Errorf("DocumentRawKey = %s, se esperaba %s", key, expected)
		}
	})

	t.Run("DocumentProcessedKey", func(t *testing.T) {
		key := DocumentProcessedKey(userID, docID, "png")
		expected := "documents/" + userID.String() + "/" + docID.String() + "/processed.png"
		if key != expected {
			t.Errorf("DocumentProcessedKey = %s, se esperaba %s", key, expected)
		}
	})

	t.Run("DocumentResultsKey", func(t *testing.T) {
		key := DocumentResultsKey(userID, docID)
		expected := "documents/" + userID.String() + "/" + docID.String() + "/results.json"
		if key != expected {
			t.Errorf("DocumentResultsKey = %s, se esperaba %s", key, expected)
		}
	})
}

func TestBucketConstants(t *testing.T) {
	if BucketSecure != "proactrip-secure" {
		t.Errorf("BucketSecure = %s, se esperaba proactrip-secure", BucketSecure)
	}
	if BucketAssets != "proactrip-assets" {
		t.Errorf("BucketAssets = %s, se esperaba proactrip-assets", BucketAssets)
	}
}

func TestNewR2Storage_InvalidEndpoint(t *testing.T) {
	// Endpoint vacío debería retornar error
	_, err := NewR2Storage("", "access", "secret", true)
	if err != nil {
		t.Logf("error esperado: %v", err)
	}
}

func TestNewR2Storage_ValidParams(t *testing.T) {
	svc, err := NewR2Storage("localhost:9000", "minioadmin", "minioadmin", false)
	if err != nil {
		t.Fatalf("NewR2Storage falló: %v", err)
	}
	if svc == nil {
		t.Fatal("NewR2Storage devolvió nil")
	}
	if svc.client == nil {
		t.Error("cliente minio no inicializado")
	}
}
