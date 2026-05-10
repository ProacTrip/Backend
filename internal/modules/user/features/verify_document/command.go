// Command para PUT /v1/user/documents/:document_id/verify.
package verify_document

// VerifyCommand contiene los datos del body del request.
type VerifyCommand struct {
	DocumentID string `json:"-"`
	IsVerified bool   `json:"is_verified"`
	VerifiedBy string `json:"-"`
}
