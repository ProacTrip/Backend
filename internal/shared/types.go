package shared

// =============================================================================
// Tipos compartidos: paginación y respuesta
// =============================================================================

import (
	"encoding/base64"
	"encoding/json"
)

// =============================================================================
// Pagination - Cursor-based (opaque cursors for high-performance)
// =============================================================================

// PaginationParams son los parámetros de paginación con cursor
type PaginationParams struct {
	Limit  int    `query:"limit" validate:"gte=1,lte=100"`
	Cursor string `query:"cursor"`
}

// GetLimit retorna el límite con valor por defecto
func (p *PaginationParams) GetLimit() int {
	if p.Limit <= 0 {
		return 20
	}
	return p.Limit
}

// CursorData representa los datos codificados en el cursor
type CursorData struct {
	ID        string `json:"id"`           // ID del último item
	Timestamp int64  `json:"ts,omitempty"` // Timestamp para ordenamiento
}

// EncodeCursor codifica CursorData a string opaco (base64)
func EncodeCursor(data *CursorData) string {
	if data == nil {
		return ""
	}
	bytes, _ := json.Marshal(data)
	return base64.URLEncoding.EncodeToString(bytes)
}

// DecodeCursor decodifica un cursor opaco a CursorData
func DecodeCursor(cursor string) (*CursorData, error) {
	if cursor == "" {
		return nil, nil
	}
	bytes, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, err
	}
	var data CursorData
	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// PaginationMeta contiene metadatos de paginación para la respuesta
type PaginationMeta struct {
	NextCursor *string `json:"next_cursor"` // nil si no hay más páginas
	PrevCursor *string `json:"prev_cursor"` // nil si es la primera página
	HasNext    bool    `json:"has_next"`
	Limit      int     `json:"limit"`
}

// NewPaginationMeta crea un nuevo PaginationMeta
func NewPaginationMeta(limit int) *PaginationMeta {
	empty := ""
	return &PaginationMeta{
		NextCursor: nil,
		PrevCursor: &empty,
		HasNext:    false,
		Limit:      limit,
	}
}

// SetNextCursor establece el cursor para la siguiente página
func (m *PaginationMeta) SetNextCursor(cursor string) {
	if cursor == "" {
		m.NextCursor = nil
	} else {
		m.NextCursor = &cursor
	}
}

// SetPrevCursor establece el cursor para la página anterior
func (m *PaginationMeta) SetPrevCursor(cursor string) {
	if cursor == "" {
		m.PrevCursor = nil
	} else {
		m.PrevCursor = &cursor
	}
}

// =============================================================================
// Response wrappers - RFC 7807 via shared/errors
// Los handlers deben retornar errores tipo *Problem ( RFC 7807 )
// y Data directamente (sin wrapper Success)
// =============================================================================
