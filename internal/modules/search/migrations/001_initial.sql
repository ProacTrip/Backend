-- +migrate Up

-- =============================================================================
-- SEARCH HISTORY
-- Append-only log of searches performed by authenticated and anonymous users.
-- Inserts only — never updated. Drives proactive features (price alerts,
-- intent detection) by recording what users searched and when.
-- NOTE: Actual search results are NOT stored here. They are cached in
-- DragonflyDB with a 15-minute TTL. This table records search intent only.
-- =============================================================================
CREATE TABLE IF NOT EXISTS search_history (
    id                UUID        PRIMARY KEY DEFAULT uuidv7(),
    user_id           UUID,                   -- cross-domain ref to Auth domain (no FK)
    session_id        VARCHAR(100),
    query_type        VARCHAR(20)  NOT NULL DEFAULT 'structured',
    raw_query         TEXT,
    parsed_params     JSONB        NOT NULL DEFAULT '{}',
    result_count      INTEGER      NOT NULL DEFAULT 0,
    execution_time_ms INTEGER,
    cache_hit         BOOLEAN      NOT NULL DEFAULT FALSE,
    ip_address        INET,
    user_agent        TEXT,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_search_identity CHECK (
        user_id IS NOT NULL OR session_id IS NOT NULL
    ),
    CONSTRAINT chk_query_type CHECK (
        query_type IN ('natural_language', 'structured')
    )
);

CREATE INDEX idx_search_history_user_id    ON search_history(user_id)
    WHERE user_id IS NOT NULL;
CREATE INDEX idx_search_history_session_id ON search_history(session_id)
    WHERE session_id IS NOT NULL;
CREATE INDEX idx_search_history_created_at ON search_history(created_at DESC);
CREATE INDEX idx_search_history_type       ON search_history(query_type, created_at DESC);