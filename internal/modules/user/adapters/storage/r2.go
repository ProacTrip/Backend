// Adapter: Almacenamiento R2 (compatible con S3) vía MinIO.
// Maneja buckets de documentos seguros y assets públicos.
package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// =============================================================================
// Configuración de buckets desde env vars
// =============================================================================

// SecureBucket retorna el nombre del bucket para documentos sensibles.
// Configurable via R2_SECURE_BUCKET (default "proactrip-secure").
func SecureBucket() string {
	if v := os.Getenv("R2_SECURE_BUCKET"); v != "" {
		return v
	}
	return "proactrip-secure"
}

// AssetsBucket retorna el nombre del bucket para assets públicos.
// Configurable via R2_ASSETS_BUCKET (default "proactrip-assets").
func AssetsBucket() string {
	if v := os.Getenv("R2_ASSETS_BUCKET"); v != "" {
		return v
	}
	return "proactrip-assets"
}

// SSEBaseURL retorna la URL base para Server-Sent Events.
// Configurable via SSE_BASE_URL (default "/v1/realtime/events").
func SSEBaseURL() string {
	if v := os.Getenv("SSE_BASE_URL"); v != "" {
		return v
	}
	return "/v1/realtime/events"
}

// =============================================================================
// R2Storage — Adaptador de almacenamiento R2 (S3-compatible)
// =============================================================================

// R2Storage implementa operaciones de almacenamiento contra R2 (S3-compatible).
type R2Storage struct {
	client *minio.Client
}

// NewR2Storage crea un nuevo cliente de almacenamiento R2.
// endpoint: URL del endpoint S3 (ej. "https://account.r2.cloudflarestorage.com" o "account.r2.cloudflarestorage.com")
// accessKey: clave de acceso
// secretKey: clave secreta
// useSSL: true para HTTPS, false para HTTP (dev local)
func NewR2Storage(endpoint, accessKey, secretKey string, useSSL bool) (*R2Storage, error) {
	// MinIO SDK espera solo host:port, no URL completa con scheme
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
		Region: "auto",
	})
	if err != nil {
		return nil, fmt.Errorf("crear cliente R2: %w", err)
	}

	return &R2Storage{client: client}, nil
}

// GenerateUploadURL genera una URL prefirmada para subir un archivo.
func (s *R2Storage) GenerateUploadURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	url, err := s.client.PresignedPutObject(ctx, bucket, key, expiry)
	if err != nil {
		return "", fmt.Errorf("generar upload URL: %w", err)
	}
	return url.String(), nil
}

// Download descarga un archivo del storage.
// El caller es responsable de cerrar el reader.
func (s *R2Storage) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("descargar objeto: %w", err)
	}
	return obj, nil
}

// GenerateDownloadURL genera una URL prefirmada para descargar un archivo.
func (s *R2Storage) GenerateDownloadURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	url, err := s.client.PresignedGetObject(ctx, bucket, key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("generar download URL: %w", err)
	}
	return url.String(), nil
}

// Delete elimina un archivo del storage.
func (s *R2Storage) Delete(ctx context.Context, bucket, key string) error {
	err := s.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("eliminar objeto: %w", err)
	}
	return nil
}

// ListObjects lista objetos con un prefijo dado.
// Retorna una slice con las keys de los objetos encontrados.
func (s *R2Storage) ListObjects(ctx context.Context, bucket, prefix string) ([]string, error) {
	objCh := s.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	var keys []string
	for obj := range objCh {
		if obj.Err != nil {
			return nil, fmt.Errorf("list objects: %w", obj.Err)
		}
		keys = append(keys, obj.Key)
	}
	return keys, nil
}

// Exists verifica si un archivo existe en el storage.
func (s *R2Storage) Exists(ctx context.Context, bucket, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("verificar existencia: %w", err)
	}
	return true, nil
}

// ObjectMeta contiene metadatos de un objeto en R2.
type ObjectMeta struct {
	ContentType string
	Size        int64
}

// Upload sube un archivo directamente al storage (para uploads server-side).
func (s *R2Storage) Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, bucket, key, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("subir objeto: %w", err)
	}
	return nil
}

// StatObject obtiene los metadatos de un objeto en el storage.
func (s *R2Storage) StatObject(ctx context.Context, bucket, key string) (*ObjectMeta, error) {
	info, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return nil, nil
		}
		return nil, fmt.Errorf("stat object: %w", err)
	}
	return &ObjectMeta{
		ContentType: info.ContentType,
		Size:        info.Size,
	}, nil
}

// HeadContentType returns just the ContentType from R2 object metadata.
// Used by validators that don't need the full ObjectMeta.
func (s *R2Storage) HeadContentType(ctx context.Context, bucket, key string) (string, error) {
	info, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return "", nil
		}
		return "", fmt.Errorf("head object: %w", err)
	}
	return info.ContentType, nil
}

// =============================================================================
// Helpers para construir keys de storage
// =============================================================================

// AvatarKey construye la key de storage para un avatar.
func AvatarKey(userID uuid.UUID, ext string) string {
	return fmt.Sprintf("avatars/%s.%s", userID.String(), ext)
}

// DocumentRawKey construye la key del documento original.
func DocumentRawKey(userID, docID uuid.UUID, ext string) string {
	return fmt.Sprintf("documents/%s/%s/raw.%s", userID.String(), docID.String(), ext)
}

// DocumentProcessedKey construye la key del documento procesado (sanitized).
func DocumentProcessedKey(userID, docID uuid.UUID, ext string) string {
	return fmt.Sprintf("documents/%s/%s/processed.%s", userID.String(), docID.String(), ext)
}

// DocumentResultsKey construye la key de los resultados OCR/extracción.
func DocumentResultsKey(userID, docID uuid.UUID) string {
	return fmt.Sprintf("documents/%s/%s/results.json", userID.String(), docID.String())
}
