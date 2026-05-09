// Worker retry logic with Dead Letter Queue (DLQ).
// All pipeline workers (Validator, Sanitizer, OCR, Avatar) use this.
//
// On failure: increment retry count in message metadata.
//   - If retries < MaxRetries: wait exponential backoff, message stays in PEL
//   - If retries >= MaxRetries: XACK (remove from PEL), produce to DLQ stream
//
// DLQ streams:
//   - {events}:doc:dlq for document workers (Validator, Sanitizer, OCR)
//   - {events}:avatar:dlq for avatar worker
package pipeline

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Retry configuration
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
// Streams start from "0" to preserve all dead-lettered messages.
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
// Retry logic
// =============================================================================

// retryCount extracts the retry count from message metadata.
// Returns 0 if not present or unparseable.
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

// RetryBackoff calculates exponential backoff: base * 2^retry, capped at max.
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

// ShouldRetry returns true if the message can be retried.
// When false, the message should be moved to DLQ.
func ShouldRetry(msg redis.XMessage) bool {
	return retryCount(msg) < MaxRetries
}

// MoveToDLQ XACKs the message from its source stream and produces
// a dead-letter entry with original payload + error + retry count + timestamp.
func MoveToDLQ(ctx context.Context, rdb *redis.Client, sourceStream, sourceGroup, dlqStream, msgID string, msg redis.XMessage, errMsg string) error {
	// Build DLQ payload: original values + metadata
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

	// Produce to DLQ stream
	if _, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: dlqStream,
		ID:     "*",
		Values: dlqPayload,
	}).Result(); err != nil {
		return fmt.Errorf("dlq: xadd to %s: %w", dlqStream, err)
	}

	return nil
}
