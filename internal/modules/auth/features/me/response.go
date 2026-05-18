package me

import "github.com/google/uuid"

type Response struct {
	User UserResponse `json:"user"`
}

type UserResponse struct {
	ID            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	RoleName      string    `json:"role_name"`
	AvatarURL     *string   `json:"avatar_url,omitzero"`
}
