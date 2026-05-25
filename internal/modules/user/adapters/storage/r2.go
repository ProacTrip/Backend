// Adapter: Almacenamiento R2 (compatible con S3) vía AWS SDK v2.
// Maneja buckets de documentos seguros y assets públicos.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
)

// =============================================================================
// Configuración de buckets desde env vars
// =============================================================================

// SecureBucket retorna el nombre del bucket para documentos sensibles.
func SecureBucket() string {
	if v := os.Getenv("R2_SECURE_BUCKET"); v != "" {
		return v
	}
	return "proactrip-secure"
}

// AssetsBucket retorna el nombre del bucket para assets públicos.
func AssetsBucket() string {
	if v := os.Getenv("R2_ASSETS_BUCKET"); v != "" {
		return v
	}
	return "proactrip-assets"
}

// SSEBaseURL retorna la URL base para Server-Sent Events.
func SSEBaseURL() string {
	if v := os.Getenv("SSE_BASE_URL"); v != "" {
		return v
	}
	return "/v1/realtime/events"
}

// =============================================================================
// R2Storage — Adaptador de almacenamiento R2 (S3-compatible)
// =============================================================================

// R2Storage implementa operaciones de almacenamiento contra R2 vía AWS SDK v2.
type R2Storage struct {
	client   *s3.Client
	presign  *s3.PresignClient
	endpoint string
}

// NewR2Storage crea un nuevo cliente de almacenamiento R2 usando AWS SDK v2.
func NewR2Storage(endpoint, accessKey, secretKey string, useSSL bool) (*R2Storage, error) {
	scheme := "https://"
	if !useSSL {
		scheme = "http://"
	}
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("auto"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("crear config AWS: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(scheme + endpoint)
		o.UsePathStyle = true
	})

	return &R2Storage{
		client:   client,
		presign:  s3.NewPresignClient(client),
		endpoint: endpoint,
	}, nil
}

// =============================================================================
// Operaciones de objeto
// =============================================================================

// Upload sube un archivo directamente al storage.
func (s *R2Storage) Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          reader,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("subir objeto: %w", err)
	}
	return nil
}

// Download descarga un archivo del storage. El caller debe cerrar el reader.
func (s *R2Storage) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("descargar objeto: %w", err)
	}
	return out.Body, nil
}

// StatObject obtiene los metadatos de un objeto en el storage.
func (s *R2Storage) StatObject(ctx context.Context, bucket, key string) (*ObjectMeta, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("stat object: %w", err)
	}
	size := int64(0)
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	ct := ""
	if out.ContentType != nil {
		ct = *out.ContentType
	}
	return &ObjectMeta{ContentType: ct, Size: size}, nil
}

// HeadContentType returns just the ContentType from R2 object metadata.
func (s *R2Storage) HeadContentType(ctx context.Context, bucket, key string) (string, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return "", nil
		}
		return "", fmt.Errorf("head object: %w", err)
	}
	if out.ContentType != nil {
		return *out.ContentType, nil
	}
	return "", nil
}

// Delete elimina un archivo del storage.
func (s *R2Storage) Delete(ctx context.Context, bucket, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("eliminar objeto: %w", err)
	}
	return nil
}

// Exists verifica si un archivo existe en el storage.
func (s *R2Storage) Exists(ctx context.Context, bucket, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return false, nil
		}
		return false, fmt.Errorf("verificar existencia: %w", err)
	}
	return true, nil
}

// ListObjects lista objetos con un prefijo dado.
func (s *R2Storage) ListObjects(ctx context.Context, bucket, prefix string) ([]string, error) {
	var keys []string
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}
		for _, obj := range page.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
	}
	return keys, nil
}

// =============================================================================
// URLs prefirmadas
// =============================================================================

// GenerateUploadURL genera una URL prefirmada para subir un archivo.
func (s *R2Storage) GenerateUploadURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("generar upload URL: %w", err)
	}
	return req.URL, nil
}

// GenerateDownloadURL genera una URL prefirmada para descargar un archivo.
func (s *R2Storage) GenerateDownloadURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("generar download URL: %w", err)
	}
	return req.URL, nil
}

// =============================================================================
// Tipos
// =============================================================================

// ObjectMeta contiene metadatos de un objeto en R2.
type ObjectMeta struct {
	ContentType string
	Size        int64
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
