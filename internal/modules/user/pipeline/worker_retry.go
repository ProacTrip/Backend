// Lógica de reintento de workers con Dead Letter Queue (DLQ).
// Todos los workers del pipeline (Validator, Sanitizer, OCR, Avatar) usan esto.
//
// En caso de fallo: incrementa contador de reintentos en metadatos del mensaje.
//   - Si reintentos < MaxRetries: espera backoff exponencial, mensaje permanece en PEL
//   - Si reintentos >= MaxRetries: XACK (remover de PEL), producir al stream DLQ
//
// Streams DLQ:
//   - {events}:doc:dlq para workers de documentos (Validator, Sanitizer, OCR)
//   - {events}:avatar:dlq para el worker de avatar
package pipeline

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Configuración de reintentos
// =============================================================================

const (
	MaxRetries    = 3
	RetryBaseWait = 5 * time.Second
	RetryMaxWait  = 60 * time.Second
)

// DLQ stream names
const (
	DocDLQStream    = "{events}:doc:dlq"
	AvatarDLQStream = "{events}:avatar:dlq"
)

// DLQ consumer group names
const (
	DocDLQGroup    = "doc-dlq-group"
	AvatarDLQGroup = "avatar-dlq-group"
)

// =============================================================================
// DLQ initialization
// =============================================================================

// EnsureDLQStreams creates the DLQ streams and consumer groups if they don't exist.
// Los streams comienzan desde "0" para preservar todos los mensajes dead-lettered.
func EnsureDLQStreams(ctx context.Context, rdb *redis.Client) error {
	for _, cfg := range []struct {
		stream string
		group  string
	}{
		{DocDLQStream, DocDLQGroup},
		{AvatarDLQStream, AvatarDLQGroup},
	} {
		err := rdb.XGroupCreateMkStream(ctx, cfg.stream, cfg.group, "0").Err()
		if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
			return fmt.Errorf("ensure DLQ stream %s group %s: %w", cfg.stream, cfg.group, err)
		}
	}
	return nil
}

// =============================================================================
// Lógica de reintentos
// =============================================================================

// retryCount extracts the retry count from message metadata.
// Devuelve 0 si no está presente o no se puede parsear.
func retryCount(msg redis.XMessage) int {
	val, ok := msg.Values["_retry_count"]
	if !ok {
		return 0
	}
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case string:
		var n int
		fmt.Sscanf(v, "%d", &n)
		return n
	case float64:
		return int(v)
	default:
		return 0
	}
}

// RetryBackoff calcula backoff exponencial: base * 2^reintento, limitado a max.
func RetryBackoff(retry int) time.Duration {
	if retry <= 0 {
		return RetryBaseWait
	}
	d := RetryBaseWait * time.Duration(int64(math.Pow(2, float64(retry))))
	if d > RetryMaxWait {
		return RetryMaxWait
	}
	return d
}

// ShouldRetry devuelve true si el mensaje puede ser reintentado.
// Cuando es false, el mensaje debe moverse a DLQ.
func ShouldRetry(msg redis.XMessage) bool {
	return retryCount(msg) < MaxRetries
}

// MoveToDLQ hace XACK del mensaje de su stream origen y produce
// una entrada dead-letter con payload original + error + reintentos + timestamp.
func MoveToDLQ(ctx context.Context, rdb *redis.Client, sourceStream, sourceGroup, dlqStream, msgID string, msg redis.XMessage, errMsg string) error {
	// Construir payload DLQ: valores originales + metadatos
	dlqPayload := make(map[string]interface{})
	for k, v := range msg.Values {
		dlqPayload[k] = v
	}
	dlqPayload["_dlq_error"] = errMsg
	dlqPayload["_dlq_retry_count"] = retryCount(msg)
	dlqPayload["_dlq_timestamp"] = time.Now().UnixMilli()
	dlqPayload["_dlq_source_stream"] = sourceStream
	dlqPayload["_dlq_original_msg_id"] = msgID

	// XACK first to remove from PEL
	if err := rdb.XAck(ctx, sourceStream, sourceGroup, msgID).Err(); err != nil {
		return fmt.Errorf("dlq: xack source: %w", err)
	}

	// Producir al stream DLQ
	if _, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: dlqStream,
		ID:     "*",
		Values: dlqPayload,
	}).Result(); err != nil {
		return fmt.Errorf("dlq: xadd to %s: %w", dlqStream, err)
	}

	return nil
}
