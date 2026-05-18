package me

import (
	"context"

	"github.com/google/uuid"
)

// Profile representa los datos del perfil de usuario externalizados desde
// el módulo user. Solo incluye los campos que el feature me necesita exponer.
type Profile struct {
	AvatarURL *string
}

// UserProfileProvider provee acceso al perfil del usuario desde el módulo user.
// Se implementa como adapter en el módulo auth (auth/module.go) para mantener
// los módulos desacoplados siguiendo Ports & Adapters / Hexagonal.
type UserProfileProvider interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*Profile, error)
}
