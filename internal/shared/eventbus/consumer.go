package eventbus

// =============================================================================
// Consumidores de eventos usando Redis Consumer Groups
// Implementa workers que leen de streams y hacen XACK
// =============================================================================

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// EnsureConsumerGroup creates a Redis Consumer Group if it does not already exist.
// It is idempotent — returns nil if the group already exists (BUSYGROUP).
// The group starts reading from "$" (latest) for first-time consumers, preventing
// replay of historical stream messages on initial consumer group creation.
func EnsureConsumerGroup(ctx context.Context, rdb *redis.Client, stream, group string) error {
	err := rdb.XGroupCreateMkStream(ctx, stream, group, "$").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// StreamWorker runs a blocking consumer loop reading from a Redis Stream with
// XREADGROUP. Messages are dispatched to handler. On handler errors, the message
// is acknowledged to prevent PEL growth. Uses exponential backoff on read errors.
func StreamWorker(ctx context.Context, rdb *redis.Client, stream, group, workerID string, handler func(msg redis.XMessage) error) {
	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 30 * time.Second
	)
	backoff := initialBackoff

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		messages, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: workerID,
			Streams:  []string{stream, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()

		if err == redis.Nil {
			continue
		}
		if err != nil {
			slog.ErrorContext(ctx, "stream read error, backing off",
				slog.String("stream", stream),
				slog.String("worker", workerID),
				slog.Duration("backoff", backoff),
				slog.Any("error", err),
			)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Reset backoff on success
		backoff = initialBackoff

		for _, s := range messages {
			for _, msg := range s.Messages {
				if err := handler(msg); err != nil {
					slog.ErrorContext(ctx, "message handler failed, acknowledging to prevent PEL growth",
						slog.String("stream", stream),
						slog.String("group", group),
						slog.String("worker", workerID),
						slog.String("message_id", msg.ID),
						slog.Any("error", err),
					)
					rdb.XAck(ctx, stream, group, msg.ID)
					continue
				}
				rdb.XAck(ctx, stream, group, msg.ID)
			}
		}
	}
}

// RescueOrphanedMessages reclaims messages that have been idle longer than
// idleTimeout (e.g. because the original consumer crashed before ACKing).
// Uses XAUTOCLAIM to reassign them to a rescue-worker consumer.
func RescueOrphanedMessages(ctx context.Context, rdb *redis.Client, stream, group string, idleTimeout time.Duration) ([]redis.XMessage, error) {
	messages, _, err := rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   stream,
		Group:    group,
		MinIdle:  idleTimeout,
		Start:    "0-0",
		Count:    100,
		Consumer: "rescue-worker",
	}).Result()
	if err != nil {
		return nil, err
	}
	return messages, nil
}
