-- +migrate Up
-- Clean duplicates keeping oldest row per (user_id, entity_id, entity_type)
DELETE FROM user_favorites
WHERE id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (PARTITION BY user_id, entity_id, entity_type ORDER BY created_at ASC) AS rn
        FROM user_favorites
    ) ranked
    WHERE rn > 1
);
ALTER TABLE user_favorites ADD CONSTRAINT uq_user_favorites_entity UNIQUE (user_id, entity_id, entity_type);

-- +migrate Down
ALTER TABLE user_favorites DROP CONSTRAINT IF EXISTS uq_user_favorites_entity;
