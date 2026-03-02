package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	eventport "github.com/ProacTrip/Backend/internal/shared/domain/ports/eventbus"
)

// Aseguramos en tiempo de compilación que cumplimos las interfaces
var _ eventport.Bus = (*dragonflyBus)(nil)
var _ eventport.Subscription = (*dragonflySubscription)(nil)

// dragonflyBus es la implementación concreta para DragonflyDB usando Redis Streams
type dragonflyBus struct {
	client *redis.Client
	maxLen int64
}

// NewDragonflyBus crea una nueva instancia del bus conectado a DragonflyDB
func NewDragonflyBus(ctx context.Context, redisURL string, poolSize int, maxLen int64) (eventport.Bus, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("URL de eventbus inválida: %w", err)
	}

	opts.PoolSize = poolSize
	client := redis.NewClient(opts)

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("error conectando a Dragonfly EventBus: %w", err)
	}

	slog.Info("Conexión a EventBus (Dragonfly Streams) establecida exitosamente")

	return &dragonflyBus{
		client: client,
		maxLen: maxLen,
	}, nil
}

// Publish emite un evento al stream. Usa JSON para serializar el Message completo.
func (b *dragonflyBus) Publish(ctx context.Context, topic string, message *eventport.Message) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("error serializando mensaje: %w", err)
	}

	args := &redis.XAddArgs{
		Stream: topic,
		MaxLen: b.maxLen,
		Approx: true, // Optimización de poda de Dragonfly
		Values: map[string]interface{}{
			"payload": string(data),
		},
	}

	_, err = b.client.XAdd(ctx, args).Result()
	if err != nil {
		return fmt.Errorf("error publicando en %s: %w", topic, err)
	}

	return nil
}

// Subscribe crea una suscripción simple (sin Consumer Group)
func (b *dragonflyBus) Subscribe(ctx context.Context, topic string) (eventport.Subscription, error) {
	return &dragonflySubscription{
		client:  b.client,
		topic:   topic,
		isGroup: false,
		lastID:  "$", // '$' significa "leer solo mensajes nuevos a partir de ahora"
	}, nil
}

// SubscribeWithGroup crea una suscripción con balanceo de carga
func (b *dragonflyBus) SubscribeWithGroup(ctx context.Context, topic, group string) (eventport.Subscription, error) {
	// 1. Intentamos crear el grupo y el stream si no existen
	err := b.client.XGroupCreateMkStream(ctx, topic, group, "$").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		// Ignoramos el error "BUSYGROUP" (el grupo ya existe), si es otro, fallamos.
		return nil, fmt.Errorf("error creando consumer group: %w", err)
	}

	// 2. Generamos un nombre único para este consumidor activo
	consumerName := fmt.Sprintf("consumer-%s", uuid.New().String())

	return &dragonflySubscription{
		client:       b.client,
		topic:        topic,
		group:        group,
		consumerName: consumerName,
		isGroup:      true,
	}, nil
}

func (b *dragonflyBus) Close() error {
	slog.Info("Cerrando conexión global del EventBus")
	return b.client.Close()
}

// dragonflySubscription es el suscriptor que se conecta a un grupo de consumidores de DragonflyDB
type dragonflySubscription struct {
	client       *redis.Client
	topic        string
	group        string
	consumerName string
	isGroup      bool
	lastID       string
}

// Receive lee el siguiente mensaje del grupo de consumidores
func (s *dragonflySubscription) Receive(ctx context.Context) (*eventport.Message, error) {
	// Usamos un bucle con un block time corto (2s) para no bloquear el hilo de Go para siempre.
	// Esto nos permite revisar si el `ctx.Done()` fue cancelado de manera limpia.
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			var streams []redis.XStream
			var err error

			if s.isGroup {
				// Lectura con Consumer Group (">" lee mensajes no asignados a nadie aún)
				streams, err = s.client.XReadGroup(ctx, &redis.XReadGroupArgs{
					Group:    s.group,
					Consumer: s.consumerName,
					Streams:  []string{s.topic, ">"},
					Count:    1,
					Block:    2 * time.Second,
				}).Result()
			} else {
				// Lectura Fan-Out simple
				streams, err = s.client.XRead(ctx, &redis.XReadArgs{
					Streams: []string{s.topic, s.lastID},
					Count:   1,
					Block:   2 * time.Second,
				}).Result()
			}

			// Manejo del Timeout por Block (no hay mensajes nuevos en estos 2 segundos)
			if errors.Is(err, redis.Nil) {
				continue // Volvemos a iterar el for
			}
			if err != nil {
				return nil, fmt.Errorf("error leyendo stream: %w", err)
			}

			// Si hay datos, procesamos el primer mensaje
			if len(streams) > 0 && len(streams[0].Messages) > 0 {
				redisMsg := streams[0].Messages[0]

				// Actualizamos lastID por si es una suscripción simple
				s.lastID = redisMsg.ID

				// Deserializamos el payload guardado
				payloadStr, ok := redisMsg.Values["payload"].(string)
				if !ok {
					return nil, eventport.ErrInvalidMessage
				}

				var domainMsg eventport.Message
				if err := json.Unmarshal([]byte(payloadStr), &domainMsg); err != nil {
					return nil, fmt.Errorf("%w: parse error: %v", eventport.ErrInvalidMessage, err)
				}

				// IMPORTANTE: Sobrescribimos el UUID con el ID real de Dragonfly
				// solo para propósitos de ACK, o usamos metadata.
				// Lo más seguro es mandar el redisMsg.ID en la metadata o un campo temporal
				// para que el usuario pueda usarlo en el Acknowledge.
				domainMsg.Metadata.Set("_stream_id", redisMsg.ID)

				return &domainMsg, nil
			}
		}
	}
}

// Acknowledge confirma el procesamiento de un mensaje
func (s *dragonflySubscription) Acknowledge(ctx context.Context, messageIDs ...string) error {
	if !s.isGroup {
		// En suscripciones simples (Fan-Out) no existe el concepto de ACK.
		return nil
	}

	if len(messageIDs) == 0 {
		return nil
	}

	err := s.client.XAck(ctx, s.topic, s.group, messageIDs...).Err()
	if err != nil {
		return fmt.Errorf("error confirmando mensajes en stream %s: %w", s.topic, err)
	}

	return nil
}

// Close cierra el suscriptor
func (s *dragonflySubscription) Close() error {
	slog.Info("Cerrando suscripción activa", "topic", s.topic, "group", s.group)
	// Aquí podrías opcionalmente borrar el consumidor de Dragonfly (XGROUP DELCONSUMER)
	// pero en arquitecturas efímeras a veces se confía en el garbage collection de Redis.
	return nil
}

// HealthCheck verifica que el cliente Redis/Dragonfly responda
func (b *dragonflyBus) HealthCheck(ctx context.Context) error {
	return b.client.Ping(ctx).Err()
}
