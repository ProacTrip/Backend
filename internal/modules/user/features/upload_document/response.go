// Response para upload de documentos.
package upload_document

// UploadDocumentResponse es la respuesta HTTP 202 del endpoint de upload.
type UploadDocumentResponse struct {
	DocumentID string `json:"document_id"`
	Status     string `json:"status"`
	EventsURL  string `json:"events_url"`
	Message    string `json:"message"`
}
