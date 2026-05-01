package login

// DTOs del login según AUTH_API.md.

type Command struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type Response struct {
	AccessToken  string        `json:"-"`
	RefreshToken string        `json:"-"`
	User         *UserResponse `json:"user"`
}

type UserResponse struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	RoleName      string `json:"role_name"`
}
