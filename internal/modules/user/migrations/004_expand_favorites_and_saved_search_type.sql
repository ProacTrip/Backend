-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 004: Expande entity types de favoritos y agrega search_type a
-- búsquedas guardadas para compatibilidad con search_ai.
-- =============================================================================

-- +migrate Up
-- =============================================================================

-- 1. Expandir CHECK constraint de user_favorites para los 8 tipos de entidad
ALTER TABLE user_favorites
    DROP CONSTRAINT IF EXISTS chk_favorite_entity_type;

ALTER TABLE user_favorites
    ADD CONSTRAINT chk_favorite_entity_type CHECK (
        entity_type IN (
            'hotel', 'flight', 'airport', 'airline',
            'hotel_chain', 'country', 'destination', 'activity'
        )
    );

-- 2. Agregar search_type a saved_searches (flight|hotel|ai|both)
ALTER TABLE saved_searches
    ADD COLUMN IF NOT EXISTS search_type VARCHAR(10);

-- 2b. Agregar parameters_version (schema version, default 1)
ALTER TABLE saved_searches
    ADD COLUMN IF NOT EXISTS parameters_version INTEGER NOT NULL DEFAULT 1;

-- 3. CHECK constraint para search_type
ALTER TABLE saved_searches
    ADD CONSTRAINT chk_saved_search_type CHECK (
        search_type IS NULL
        OR search_type IN ('flight', 'hotel', 'ai', 'both')
    );

-- +migrate Down
-- =============================================================================

ALTER TABLE saved_searches
    DROP CONSTRAINT IF EXISTS chk_saved_search_type;

ALTER TABLE saved_searches
    DROP COLUMN IF EXISTS search_type;

ALTER TABLE user_favorites
    DROP CONSTRAINT IF EXISTS chk_favorite_entity_type;

ALTER TABLE user_favorites
    ADD CONSTRAINT chk_favorite_entity_type CHECK (
        entity_type IN ('hotel', 'flight', 'destination')
    );

-- +migrate Down
ALTER TABLE saved_searches DROP COLUMN IF EXISTS search_type CASCADE;
ALTER TABLE saved_searches DROP COLUMN IF EXISTS parameters_version CASCADE;
