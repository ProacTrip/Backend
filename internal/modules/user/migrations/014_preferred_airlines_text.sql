-- +migrate Up
-- MIGRACIÓN 014: Cambia preferred_airlines de UUID[] a TEXT[] (códigos IATA).
-- Los UUIDs existentes se convierten a su representación textual.
-- NOTA: los UUIDs no se pueden mapear a códigos IATA automáticamente
-- porque no hay una tabla de aerolíneas en la DB. Los usuarios deberán
-- re-guardar sus preferencias con códigos IATA desde el frontend.
ALTER TABLE user_travel_preferences
  ALTER COLUMN preferred_airlines TYPE TEXT[] USING preferred_airlines::TEXT[];

-- +migrate Down
ALTER TABLE user_travel_preferences
  ALTER COLUMN preferred_airlines TYPE UUID[] USING preferred_airlines::UUID[];
