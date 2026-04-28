package logout

// DTO de logout.
// LogoutAll: invalida todas las sesiones del usuario.

type Command struct {
	Token     string `json:"token"`
	LogoutAll bool   `json:"logout_all"`
}
