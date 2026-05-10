package resend_verification

// Response DTO de salida del resend-verification.
// Mensaje genérico anti-enumeración: nunca revela si el email existe o no.
type Response struct {
	Message string `json:"message"`
}
