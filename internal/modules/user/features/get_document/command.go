// Command para GET /v1/user/documents/:document_id.
package get_document

// GetDocumentCommand contiene los datos necesarios para obtener un documento.
type GetDocumentCommand struct {
	DocumentID string `json:"-"`
	UserID     string `json:"-"`
}
