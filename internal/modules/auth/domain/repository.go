package domain

import (
	"context"

	"github.com/google/uuid"
)

// Interfaz de persistencia para usuarios.
// Implementación: adapters/postgres/user_repository.go

// UserRepository define los métodos necesarios para persistencia de usuarios
type UserRepository interface {
	// Create crea un nuevo usuario
	Create(ctx context.Context, user *User) error

	// GetByID obtiene un usuario por su ID
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)

	// GetByEmail obtiene un usuario por su email
	GetByEmail(ctx context.Context, email string) (*User, error)

	// Update actualiza un usuario existente
	Update(ctx context.Context, user *User) error

	// GetRoleByName obtiene un rol por su nombre
	GetRoleByName(ctx context.Context, name string) (*Role, error)
}

// Role representa un rol del sistema
type Role struct {
	ID          uuid.UUID
	Name        string
	Description string
	IsSystem    bool
	Permissions []string
}
