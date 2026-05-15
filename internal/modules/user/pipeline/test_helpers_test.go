// Helpers compartidos para tests del pipeline.
// Reemplazan time.Sleep con sincronización basada en streams (XREAD BLOCK)
// o señales via channel.
package pipeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// waitForStreamEvent usa XREAD BLOCK para esperar un evento en un stream.
// Prueba el event flow real del pipeline sin sleeps.
// Usa "$" para esperar SOLO eventos nuevos (no lee históricos).
func waitForStreamEvent(t *testing.T, rdb *redis.Client, streamKey string, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	streams, err := rdb.XRead(ctx, &redis.XReadArgs{
		Streams: []string{streamKey, "$"},
		Count:   1,
		Block:   timeout,
	}).Result()
	if err != nil {
		t.Fatalf("XRead error on %s: %v", streamKey, err)
	}
	if len(streams) == 0 {
		t.Fatalf("no output event on %s within %v", streamKey, timeout)
	}
}

// waitForDocEvent espera un evento en el stream {events}:doc:events:{docID}.
func waitForDocEvent(t *testing.T, rdb *redis.Client, docID string, timeout time.Duration) {
	t.Helper()
	waitForStreamEvent(t, rdb, "{events}:doc:events:"+docID, timeout)
}

// waitForChannel espera una señal en un channel sin bloquear el test runner.
func waitForChannel(t *testing.T, ch <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-ch:
		return
	case <-time.After(timeout):
		t.Fatalf("channel did not signal within %v", timeout)
	}
}
