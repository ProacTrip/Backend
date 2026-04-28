// Utilidades para codificar/decodificar cursores de paginación.
// Convierte offset numérico a string opaco base64 para API.
package encoding

import (
	"encoding/base64"
	"encoding/json"
)

// cursorPayload is the JSON structure encoded in pagination cursors.
type cursorPayload struct {
	Offset int `json:"offset"`
}

// EncodeCursor encodes a numeric offset into a base64-encoded JSON cursor string.
// Returns a cursor suitable for use as a next/prev cursor in paginated responses.
func EncodeCursor(offset int) string {
	payload, _ := json.Marshal(cursorPayload{Offset: offset})
	return base64.StdEncoding.EncodeToString(payload)
}

// DecodeCursor decodes a base64-encoded JSON cursor string back into a numeric offset.
// Returns 0 and nil error on empty or malformed input (graceful first-page fallback).
func DecodeCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	raw, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return 0, nil // graceful: malformed -> first page
	}
	var payload cursorPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, nil
	}
	return payload.Offset, nil
}
