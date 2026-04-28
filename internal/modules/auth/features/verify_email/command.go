package verify_email

// Request body para verificación de email.

// Command representa el request body del endpoint verify-email
type Command struct {
	Token string `json:"token"`
}
