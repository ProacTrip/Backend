SKILL: Load dragonfly
SKILL: Load go

# event-consumer

## Trigger
When the user needs to create a Dragonfly/Redis Streams event consumer that reacts to events published by other modules. Keywords: "consumer", "event consumer", "escuchar eventos", "XREADGROUP", "XACK", "Dragonfly streams", "consumir eventos", "orphan rescue"

## Questions to Ask (ALWAYS ask first — never generate code without answers)

1. ¿En qué módulo estás y qué stream vas a consumir? (ej: `{events}:auth.user.registered`)
2. ¿Qué tipos de eventos vas a manejar? (user_registered, booking_confirmed, etc.) y ¿qué usecase se ejecuta para cada uno?
3. ¿Qué campos necesitás extraer del payload? (user_id, email, booking_id, etc.)
4. ¿El consumer debe hacer algo especial si falla? (¿reintentar? ¿loggear y seguir? ¿mover a dead letter?)
5. ¿El consumer necesita publicar algún evento como resultado del procesamiento?

## Rules (Non-Negotiable — fail if violated)

### Event Consumer Rules (E1-E5 from spec)
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| E1 | Consumer Naming | MUST | Consumer group: `"{module}-service"`, consumer: `"{module}-worker-{unix_millis}"` |
| E2 | XREADGROUP Config | MUST | Block = 5s, Count = 10 messages per batch |
| E3 | Acknowledgment | CRITICAL | XACK on success, NO XACK on failure (message stays in PEL for retry) |
| E4 | Orphan Rescue | MUST | Goroutine every 30s, XAutoClaim messages idle > 5min |
| E5 | Graceful Shutdown | MUST | Respect `ctx.Done()`, set `running = false`, stop consuming |

### Global Rules (R1-R9)
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| R1 | Module Isolation | CRITICAL | Modules communicate via events — consumer receives events, not imports |
| R3 | Shared Boundaries | CRITICAL | `shared/` MUST NOT import from `modules/` |
| R5 | DI | MUST | Manual constructor injection, zero globals, zero singletons |
| R6 | Testing | MUST | Generate `_test.go` |
| R7 | Go 1.26 | MUST | `omitzero`, `new(expr)`, `errors.AsType` |

### Additional Consumer Conventions
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| C1 | Atomic Flag | MUST | `atomic.Bool` for `running` state, exposed via `IsRunning() bool` |
| C2 | Unique Consumer | MUST | Consumer name includes Unix milliseconds for uniqueness |
| C3 | Malformed Messages | MUST | ACK messages that can't be parsed (to avoid PEL stuck) |
| C4 | Error Logging | MUST | `slog.ErrorContext(ctx, ...)` on processing failures |
| C5 | Rescue Orphans | MUST | `XAutoClaim` with idle threshold > 5min |

## Patterns

### Pattern: Consumer with XReadGroup Loop (from notification/consumer/notification_consumer.go)
```go
type Consumer struct {
    rdb    *redis.Client
    stream string
    group  string
    name   string
    running atomic.Bool
    handler func(ctx context.Context, msg map[string]interface{}) error
}

func (c *Consumer) Start(ctx context.Context) error {
    if err := eventbus.EnsureConsumerGroup(ctx, c.rdb, c.stream, c.group); err != nil {
        return fmt.Errorf("consumer group: %w", err)
    }
    c.running.Store(true)
    go c.consume(ctx)
    go c.rescueOrphans(ctx)
    return nil
}

func (c *Consumer) consume(ctx context.Context) {
    for c.running.Load() {
        select {
        case <-ctx.Done():
            c.running.Store(false)
            return
        default:
            msgs, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
                Group:    c.group,
                Consumer: c.name,
                Streams:  []string{c.stream, ">"},
                Count:    10,
                Block:    5 * time.Second,
            }).Result()
            if err != nil { continue }
            for _, msg := range msgs[0].Messages {
                c.processMessage(ctx, c.stream, c.group, msg)
            }
        }
    }
}
```

## Templates

### Template: Consumer struct
```go
type {{.ConsumerName}} struct {
    rdb      *redis.Client
    stream   string
    group    string
    consumer string
    usecase  *{{.UseCasePackage}}.{{.UseCaseType}}
    running  atomic.Bool
}
```

### Template: NewConsumer constructor
```go
func New{{.ConsumerName}}(rdb *redis.Client, uc *{{.UseCasePackage}}.{{.UseCaseType}}) *{{.ConsumerName}} {
    return &{{.ConsumerName}}{
        rdb:      rdb,
        stream:   eventbus.StreamName("{{.StreamName}}"),
        group:    "{{.ModuleName}}-service",
        consumer: fmt.Sprintf("{{.ModuleName}}-worker-%d", time.Now().UnixMilli()),
        usecase:  uc,
    }
}
```

### Template: Start method
```go
func (c *{{.ConsumerName}}) Start(ctx context.Context) error {
    if err := eventbus.EnsureConsumerGroup(ctx, c.rdb, c.stream, c.group); err != nil {
        return fmt.Errorf("consumer group %s: %w", c.group, err)
    }
    c.running.Store(true)
    go c.consume(ctx)
    go c.rescueOrphans(ctx)
    slog.InfoContext(ctx, "consumer started", "stream", c.stream, "group", c.group)
    return nil
}
```

### Template: Consume loop
```go
func (c *{{.ConsumerName}}) consume(ctx context.Context) {
    for c.running.Load() {
        select {
        case <-ctx.Done():
            c.running.Store(false)
            return
        default:
            msgs, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
                Group:    c.group,
                Consumer: c.consumer,
                Streams:  []string{c.stream, ">"},
                Count:    10,
                Block:    5 * time.Second,
            }).Result()
            if err != nil {
                if errors.Is(err, context.Canceled) { return }
                slog.ErrorContext(ctx, "XREADGROUP error", "error", err)
                time.Sleep(1 * time.Second)
                continue
            }
            for _, stream := range msgs {
                for _, msg := range stream.Messages {
                    c.processMessage(ctx, c.stream, c.group, msg)
                }
            }
        }
    }
}
```

### Template: processMessage
```go
func (c *{{.ConsumerName}}) processMessage(ctx context.Context, stream, group string, msg redis.XMessage) {
    eventType, ok := msg.Values["event_type"].(string)
    if !ok {
        slog.ErrorContext(ctx, "missing event_type in message", "msg_id", msg.ID)
        c.rdb.XAck(ctx, stream, group, msg.ID) // ACK malformed
        return
    }

    var err error
    switch eventType {
    {{range .EventTypes}}
    case "{{.Type}}":
        err = c.handle{{.Name}}(ctx, msg.Values)
    {{end}}
    default:
        slog.WarnContext(ctx, "unknown event type", "type", eventType)
    }

    if err != nil {
        slog.ErrorContext(ctx, "message processing failed",
            "event", eventType, "msg_id", msg.ID, "error", err)
        return // NO XACK — stays in PEL
    }

    if xackErr := c.rdb.XAck(ctx, stream, group, msg.ID).Err(); xackErr != nil {
        slog.ErrorContext(ctx, "XACK failed", "msg_id", msg.ID, "error", xackErr)
    }
}
```

### Template: Orphan Rescue
```go
func (c *{{.ConsumerName}}) rescueOrphans(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            rescued, err := eventbus.RescueOrphanedMessages(ctx, c.rdb, c.stream, c.group, c.consumer, 5*time.Minute)
            if err != nil {
                slog.ErrorContext(ctx, "orphan rescue failed", "error", err)
                continue
            }
            for _, msg := range rescued {
                c.processMessage(ctx, c.stream, c.group, msg)
            }
        }
    }
}
```

### Template: Event handler method
```go
func (c *{{.ConsumerName}}) handle{{.EventName}}(ctx context.Context, values map[string]interface{}) error {
    {{.EntityVar}}ID, err := uuid.Parse(values["{{.EntityVar}}_id"].(string))
    if err != nil {
        return fmt.Errorf("invalid {{.EntityVar}}_id: %w", err)
    }
    return c.usecase.Execute(ctx, {{.EntityVar}}ID)
}
```

### Template: IsRunning + Name
```go
func (c *{{.ConsumerName}}) IsRunning() bool { return c.running.Load() }
func (c *{{.ConsumerName}}) Name() string    { return "{{.ModuleName}}-consumer" }
```

## Uses Skills
| Skill | When |
|-------|------|
| `dragonfly` | Always loaded — Dragonfly Streams, XREADGROUP, XACK, XAutoClaim |
| `feature-slice` | When the consumer needs to call an existing usecase |
| `domain-core` | When processing domain events with entity references |

## Verification
```bash
# 1. Compile check
go build ./internal/modules/{{.ModuleName}}/... && echo "PASS: compiles"

# 2. Check consumer group naming
grep 'group.*{{.ModuleName}}-service' internal/modules/{{.ModuleName}}/consumer/*.go || echo "WARN: consumer group naming"

# 3. Check XACK on success, no XACK on failure
grep 'XAck' internal/modules/{{.ModuleName}}/consumer/*.go || echo "WARN: no XACK found"

# 4. Check orphan rescue
grep 'RescueOrphanedMessages\|XAutoClaim' internal/modules/{{.ModuleName}}/consumer/*.go || echo "WARN: no orphan rescue"

# 5. Check graceful shutdown
grep 'ctx.Done\|running.Store(false)' internal/modules/{{.ModuleName}}/consumer/*.go || echo "WARN: no graceful shutdown"

# 6. Check atomic running flag
grep 'atomic.Bool' internal/modules/{{.ModuleName}}/consumer/*.go || echo "WARN: no atomic running flag"

# 7. Check IsRunning exposed
grep 'func.*IsRunning.*bool' internal/modules/{{.ModuleName}}/consumer/*.go || echo "WARN: IsRunning not exposed"
```
