package eventbus

// =============================================================================
// Consumidores de eventos usando Redis Consumer Groups
// Implementa workers que leen de streams y hacen XACK
// =============================================================================

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

func EnsureConsumerGroup(ctx context.Context, rdb *redis.Client, stream, group string) error {
	err := rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

func StreamWorker(ctx context.Context, rdb *redis.Client, stream, group, workerID string, handler func(msg redis.XMessage) error) {
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
			continue
		}

		for _, s := range messages {
			for _, msg := range s.Messages {
				if err := handler(msg); err != nil {
					continue
				}
				rdb.XAck(ctx, stream, group, msg.ID)
			}
		}
	}
}

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
