-- +migrate Up
-- =============================================================================
-- MIGRACIÓN 002: Reemplaza wishlists/wishlist_items con user_favorites
-- Migra el modelo de wishlists a un modelo simplificado de favoritos.
-- =============================================================================

-- +migrate Up
-- =============================================================================

-- 1. Eliminar tablas antiguas de wishlists
DROP TABLE IF EXISTS wishlist_items CASCADE;
DROP TABLE IF EXISTS wishlists CASCADE;

-- 2. Crear tabla de favoritos (reemplaza wishlists + wishlist_items)
CREATE TABLE IF NOT EXISTS user_favorites (
    id          UUID        PRIMARY KEY DEFAULT uuidv7(),
    user_id     UUID        NOT NULL REFERENCES user_profiles(user_id) ON DELETE CASCADE,
    entity_id   UUID        NOT NULL,
    entity_type VARCHAR(50) NOT NULL,
    title       VARCHAR(255) NOT NULL,
    notes       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_favorite_entity_type CHECK (
        entity_type IN (
            'hotel', 'flight', 'airport', 'airline',
            'hotel_chain', 'country', 'destination', 'activity'
        )
    )
);

CREATE TRIGGER trg_user_favorites_updated_at
    BEFORE UPDATE ON user_favorites
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_user_favorites_user_id ON user_favorites(user_id);
CREATE INDEX idx_user_favorites_entity ON user_favorites(user_id, entity_type);

-- +migrate Down
-- =============================================================================
DROP TABLE IF EXISTS user_favorites CASCADE;

-- Restaurar tablas originales de wishlists
CREATE TABLE IF NOT EXISTS wishlists (
    id          UUID        PRIMARY KEY DEFAULT uuidv7(),
    user_id     UUID        NOT NULL REFERENCES user_profiles(user_id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    is_public   BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER trg_wishlists_updated_at
    BEFORE UPDATE ON wishlists
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_wishlists_user_id  ON wishlists(user_id);
CREATE INDEX idx_wishlists_is_public ON wishlists(is_public);

CREATE TABLE IF NOT EXISTS wishlist_items (
    id             UUID        PRIMARY KEY DEFAULT uuidv7(),
    wishlist_id    UUID        NOT NULL REFERENCES wishlists(id) ON DELETE CASCADE,
    entity_id      UUID        NOT NULL,
    entity_type    VARCHAR(50) NOT NULL,
    price_snapshot DOUBLE PRECISION,
    currency_code  VARCHAR(3),
    notes          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT uq_wishlist_entity UNIQUE (wishlist_id, entity_id, entity_type)
);

CREATE INDEX idx_wishlist_items_wishlist_id ON wishlist_items(wishlist_id);
