// Command para DELETE /v1/user/documents/:document_id.
package delete_document

// DeleteDocumentCommand contiene los datos necesarios para eliminar un documento.
type DeleteDocumentCommand struct {
	DocumentID string `json:"-"`
	UserID     string `json:"-"`
}
