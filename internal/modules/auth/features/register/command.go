package register

// DTOs del registro.
// Tokens van en cookies HTTP, no en el JSON.

type Command struct {
	//govalid:required
	//govalid:email
	Email string `json:"email"`
	//govalid:required
	//govalid:minlength=8
	Password string `json:"password"`
}

// =============================================================================
// Response - DTO de salida del registro
// Según AUTH_API.md: solo message en JSON, tokens van en cookies

type Response struct {
	Message      string `json:"message"`
	AccessToken  string `json:"-"` // Para Set-Cookie, no en JSON
	RefreshToken string `json:"-"` // Para Set-Cookie, no en JSON
}
