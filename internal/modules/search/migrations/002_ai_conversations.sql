-- +migrate Up

-- =============================================================================
-- CONVERSACIONES DE IA — Persistencia durable para sesiones multi-turno.
-- Solo se almacenan conversaciones de usuarios autenticados (UserID != "").
-- Usuarios anónimos son solo Dragonfly (Hash con HEXPIRE de 10 min).
--
-- Escritura vía Dragonfly Streams:
--   SaveConversation() -> Dragonfly Hash (primario, baja latencia)
--                      -> eventBus.Publish("{events}:search.conversation.saved")
--   ConversationConsumer -> XREADGROUP -> pgStore.SaveConversationHistory()
--                        -> XACK en éxito, queda en PEL en fallo
--                        -> XAutoClaim rescata huérfanos cada 30s (idle > 5min)
-- =============================================================================
CREATE TABLE IF NOT EXISTS ai_conversations (
    conversation_id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id         UUID NOT NULL,
    messages        JSONB NOT NULL DEFAULT '[]',
    intent          JSONB,
    results         JSONB,
    turn_count      INTEGER NOT NULL DEFAULT 0,
    max_turns       INTEGER NOT NULL DEFAULT 10,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_ai_conversations_turn_count CHECK (turn_count >= 0),
    CONSTRAINT chk_ai_conversations_max_turns CHECK (max_turns > 0)
);

-- Trigger para actualizar updated_at automáticamente
CREATE TRIGGER trg_ai_conversations_updated_at
    BEFORE UPDATE ON ai_conversations
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Índice para consultas por usuario (dashboard, historial de conversaciones)
CREATE INDEX idx_ai_conversations_user_id ON ai_conversations(user_id);

-- Índice para listar conversaciones recientes (dashboard, paginación)
CREATE INDEX idx_ai_conversations_updated_at ON ai_conversations(updated_at DESC);
