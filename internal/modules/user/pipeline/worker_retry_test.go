// Tests de lógica de reintento y Dead Letter Queue (DLQ).
// Prueba RetryBackoff (exponencial), ShouldRetry, MoveToDLQ y EnsureDLQStreams.
package pipeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/pipeline"
)

// =============================================================================
// RetryBackoff — backoff exponencial
// =============================================================================

func TestRetryBackoff_CalculaBackoffExponencial(t *testing.T) {
	tests := []struct {
		nombre   string
		retry    int
		esperado time.Duration
	}{
		{
			nombre:   "retry 0 devuelve base wait",
			retry:    0,
			esperado: 5 * time.Second,
		},
		{
			nombre:   "retry negativo devuelve base wait",
			retry:    -1,
			esperado: 5 * time.Second,
		},
		{
			nombre:   "retry 1 = 10s (base * 2^1)",
			retry:    1,
			esperado: 10 * time.Second,
		},
		{
			nombre:   "retry 2 = 20s (base * 2^2)",
			retry:    2,
			esperado: 20 * time.Second,
		},
		{
			nombre:   "retry 3 = 40s (base * 2^3)",
			retry:    3,
			esperado: 40 * time.Second,
		},
		{
			nombre:   "retry 10 capped at max wait (60s)",
			retry:    10,
			esperado: 60 * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.nombre, func(t *testing.T) {
			got := pipeline.RetryBackoff(tc.retry)
			if got != tc.esperado {
				t.Errorf("RetryBackoff(%d) = %v, want %v", tc.retry, got, tc.esperado)
			}
		})
	}
}

// =============================================================================
// ShouldRetry — límite MaxRetries
// =============================================================================

func TestShouldRetry_RespetaMaxRetries(t *testing.T) {
	tests := []struct {
		nombre      string
		retryCount  interface{} // valor de _retry_count en el mensaje
		shouldRetry bool
	}{
		{
			nombre:      "sin campo _retry_count → debe reintentar (retry 0 < 3)",
			retryCount:  nil,
			shouldRetry: true,
		},
		{
			nombre:      "retry 0 < MaxRetries(3) → debe reintentar",
			retryCount:  0,
			shouldRetry: true,
		},
		{
			nombre:      "retry 1 < MaxRetries(3) → debe reintentar",
			retryCount:  1,
			shouldRetry: true,
		},
		{
			nombre:      "retry 2 < MaxRetries(3) → debe reintentar",
			retryCount:  2,
			shouldRetry: true,
		},
		{
			nombre:      "retry 3 >= MaxRetries(3) → NO reintentar (DLQ)",
			retryCount:  3,
			shouldRetry: false,
		},
		{
			nombre:      "retry 5 >> MaxRetries → NO reintentar",
			retryCount:  5,
			shouldRetry: false,
		},
		{
			nombre:      "retry como string '2' → parsea y reintenta",
			retryCount:  "2",
			shouldRetry: true,
		},
		{
			nombre:      "retry como int64 2 → reintenta",
			retryCount:  int64(2),
			shouldRetry: true,
		},
		{
			nombre:      "retry como float64 3 → NO reintenta (DLQ)",
			retryCount:  float64(3),
			shouldRetry: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.nombre, func(t *testing.T) {
			msg := redis.XMessage{ID: "test-1", Values: map[string]interface{}{}}
			if tc.retryCount != nil {
				msg.Values["_retry_count"] = tc.retryCount
			}
			got := pipeline.ShouldRetry(msg)
			if got != tc.shouldRetry {
				t.Errorf("ShouldRetry() = %v, want %v (retry=%v)", got, tc.shouldRetry, tc.retryCount)
			}
		})
	}
}

// =============================================================================
// MoveToDLQ — mensaje a Dead Letter Queue
// =============================================================================

func TestMoveToDLQ_MueveMensajeAColaMuerta(t *testing.T) {
	tests := []struct {
		nombre       string
		sourceStream string
		sourceGroup  string
		dlqStream    string
		errMsg       string
	}{
		{
			nombre:       "DLQ para doc validator",
			sourceStream: "{events}:doc:validate",
			sourceGroup:  "doc-validate-group",
			dlqStream:    "{events}:doc:dlq",
			errMsg:       "validación fallida: MIME no soportado",
		},
		{
			nombre:       "DLQ para avatar validator",
			sourceStream: "{events}:avatar:validate",
			sourceGroup:  "avatar-validator-group",
			dlqStream:    "{events}:avatar:dlq",
			errMsg:       "archivo no encontrado en R2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.nombre, func(t *testing.T) {
			ctx := context.Background()
			mr := miniredis.RunT(t)
			rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { rdb.Close() })

			// Crear stream y consumer group de origen
			requireStreamGroup(t, ctx, rdb, tc.sourceStream, tc.sourceGroup)
			// Crear stream DLQ
			requireStreamGroup(t, ctx, rdb, tc.dlqStream, tc.dlqStream+"-dlq")

			// Publicar mensaje en stream de origen
			msgID, err := rdb.XAdd(ctx, &redis.XAddArgs{
				Stream: tc.sourceStream,
				ID:     "*",
				Values: map[string]interface{}{
					"document_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
					"user_id":     "a47ac10b-58cc-4372-a567-0e02b2c3d479",
					"storage_key": "raw/doc.pdf",
				},
			}).Result()
			if err != nil {
				t.Fatalf("no se pudo publicar mensaje: %v", err)
			}

			// Leer mensaje para obtener estructura completa
			msgs, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    tc.sourceGroup,
				Consumer: "test-consumer",
				Streams:  []string{tc.sourceStream, ">"},
				Count:    1,
				Block:    0,
			}).Result()
			if err != nil {
				t.Fatalf("XReadGroup falló: %v", err)
			}
			if len(msgs) == 0 || len(msgs[0].Messages) == 0 {
				t.Fatal("no se leyó ningún mensaje del stream")
			}

			msg := msgs[0].Messages[0]

			// Ejecutar MoveToDLQ
			err = pipeline.MoveToDLQ(ctx, rdb, tc.sourceStream, tc.sourceGroup, tc.dlqStream, msgID, msg, tc.errMsg)
			if err != nil {
				t.Fatalf("MoveToDLQ falló: %v", err)
			}

			// Verificar que el mensaje fue removido del stream de origen (XACK)
			pending, err := rdb.XPending(ctx, tc.sourceStream, tc.sourceGroup).Result()
			if err != nil {
				t.Fatalf("XPending falló: %v", err)
			}
			if pending.Count > 0 {
				t.Errorf("mensaje aún en PEL del stream origen: count=%d", pending.Count)
			}

			// Verificar que el mensaje está en el DLQ
			dlqLen, err := rdb.XLen(ctx, tc.dlqStream).Result()
			if err != nil {
				t.Fatalf("XLen DLQ falló: %v", err)
			}
			if dlqLen != 1 {
				t.Fatalf("DLQ debería tener 1 mensaje, tiene %d", dlqLen)
			}

			// Verificar contenido del mensaje en DLQ
			dlqMsgs, err := rdb.XRead(ctx, &redis.XReadArgs{
				Streams: []string{tc.dlqStream, "0"},
				Count:   1,
				Block:   0,
			}).Result()
			if err != nil {
				t.Fatalf("XRead DLQ falló: %v", err)
			}
			if len(dlqMsgs) == 0 || len(dlqMsgs[0].Messages) == 0 {
				t.Fatal("no se encontró mensaje en DLQ")
			}

			dlqMsg := dlqMsgs[0].Messages[0]
			dlqErr, ok := dlqMsg.Values["_dlq_error"].(string)
			if !ok {
				t.Fatal("campo _dlq_error ausente en mensaje DLQ")
			}
			if dlqErr != tc.errMsg {
				t.Errorf("_dlq_error = %q, want %q", dlqErr, tc.errMsg)
			}

			sourceStream, ok := dlqMsg.Values["_dlq_source_stream"].(string)
			if !ok || sourceStream != tc.sourceStream {
				t.Errorf("_dlq_source_stream = %q, want %q", sourceStream, tc.sourceStream)
			}
		})
	}
}

// =============================================================================
// EnsureDLQStreams — crea streams DLQ y sus consumer groups
// =============================================================================

func TestEnsureDLQStreams_CreaStreamsYConsumerGroups(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	// Primera llamada: debe crear los streams
	err := pipeline.EnsureDLQStreams(ctx, rdb)
	if err != nil {
		t.Fatalf("EnsureDLQStreams primera llamada falló: %v", err)
	}

	// Verificar que doc DLQ stream existe
	docLen, err := rdb.XLen(ctx, "{events}:doc:dlq").Result()
	if err != nil {
		t.Fatalf("XLen doc:dlq falló: %v", err)
	}
	if docLen != 0 {
		t.Errorf("doc:dlq debería estar vacío, tiene %d mensajes", docLen)
	}

	// Verificar que avatar DLQ stream existe
	avatarLen, err := rdb.XLen(ctx, "{events}:avatar:dlq").Result()
	if err != nil {
		t.Fatalf("XLen avatar:dlq falló: %v", err)
	}
	if avatarLen != 0 {
		t.Errorf("avatar:dlq debería estar vacío, tiene %d mensajes", avatarLen)
	}

	// Verificar consumer groups
	groups, err := rdb.XInfoGroups(ctx, "{events}:doc:dlq").Result()
	if err != nil {
		t.Fatalf("XInfoGroups doc:dlq falló: %v", err)
	}
	if len(groups) != 1 {
		t.Errorf("doc:dlq debería tener 1 consumer group, tiene %d", len(groups))
	}
	if groups[0].Name != "doc-dlq-group" {
		t.Errorf("doc:dlq group name = %q, want 'doc-dlq-group'", groups[0].Name)
	}

	avatarGroups, err := rdb.XInfoGroups(ctx, "{events}:avatar:dlq").Result()
	if err != nil {
		t.Fatalf("XInfoGroups avatar:dlq falló: %v", err)
	}
	if len(avatarGroups) != 1 {
		t.Errorf("avatar:dlq debería tener 1 consumer group, tiene %d", len(avatarGroups))
	}

	// Segunda llamada: debe ser idempotente (no error)
	err = pipeline.EnsureDLQStreams(ctx, rdb)
	if err != nil {
		t.Fatalf("EnsureDLQStreams segunda llamada (idempotente) falló: %v", err)
	}
}

// =============================================================================
// Helpers
// =============================================================================

// requireStreamGroup crea un stream y consumer group, fallando el test si hay error.
func requireStreamGroup(t *testing.T, ctx context.Context, rdb *redis.Client, stream, group string) {
	t.Helper()
	err := rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		t.Fatalf("no se pudo crear grupo %s en stream %s: %v", group, stream, err)
	}
}
