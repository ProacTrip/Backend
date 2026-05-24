// Command para GET /v1/user/profile/documents.
package list_documents

// ListDocumentsCommand contiene los datos para listar documentos.
type ListDocumentsCommand struct {
	UserID        string `json:"-"`
	StatusFilter   string `json:"-"`
	DocTypeFilter string `json:"-"`
}
