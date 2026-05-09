// Caso de uso: Obtener perfil médico del usuario.
// Desencripta transparentemente los campos médicos antes de retornarlos.
package get_medical_profile

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

// MedicalProfileRepo permite leer el perfil médico.
type MedicalProfileRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfileV2, error)
}

// EncryptionSvc permite desencriptar valores médicos.
type EncryptionSvc interface {
	Decrypt(ciphertext []byte) (string, error)
}

// MedicalPendingCounter permite contar conflictos pendientes.
type MedicalPendingCounter interface {
	CountPending(ctx context.Context, userID uuid.UUID) (int, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	MedicalProfileRepo MedicalProfileRepo
	EncryptionService  EncryptionSvc
	MedicalPendingRepo MedicalPendingCounter
}

// mapSourceToAPI traduce el enum de dominio MedicalSource al formato de API.
// "profile" → "manual", "ocr" → "ocr", "nlp" → "nlp".
func mapSourceToAPI(source domain.MedicalSource) string {
	switch source {
	case domain.MedicalSourceProfile:
		return "manual"
	default:
		return string(source)
	}
}

// UseCase implementa la obtención del perfil médico.
type UseCase struct {
	medicalProfileRepo MedicalProfileRepo
	encryptionService  EncryptionSvc
	medicalPendingRepo MedicalPendingCounter
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		medicalProfileRepo: deps.MedicalProfileRepo,
		encryptionService:  deps.EncryptionService,
		medicalPendingRepo: deps.MedicalPendingRepo,
	}
}

// Execute obtiene el perfil médico, desencripta campos y cuenta conflictos pendientes.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*MedicalProfileResponse, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	// 1. Obtener perfil médico
	mp, err := uc.medicalProfileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if mp == nil {
		return nil, domain.ErrMedicalProfileNotFound
	}

	// 2. Desencriptar campos y construir respuesta
	data := make(map[string]*MedicalFieldEntry, len(mp.Data))
	for key, field := range mp.Data {
		entry := &MedicalFieldEntry{
			Source:    mapSourceToAPI(field.Source),
			UpdatedAt: formatTime(field.UpdatedAt),
		}

		// Campos con sufijo _enc están encriptados (medical fields: allergies_enc, medications_enc, etc.)
		// Campos sin sufijo pasan como texto plano (ej: blood_type, is_shared)
		if strings.HasSuffix(key, "_enc") {
			// Decodificar base64 y desencriptar
			ciphertext, err := base64.StdEncoding.DecodeString(field.Value)
			if err != nil {
				return nil, fmt.Errorf("%w: base64 decode %s: %w", domain.ErrDecryptionFailed, key, err)
			}
			plaintext, err := uc.encryptionService.Decrypt(ciphertext)
			if err != nil {
				return nil, fmt.Errorf("%w: decrypt %s: %w", domain.ErrDecryptionFailed, key, err)
			}
			entry.Value = plaintext

			// Remover sufijo _enc del nombre del campo
			fieldName := strings.TrimSuffix(key, "_enc")
			data[fieldName] = entry
		} else {
			// Campo sin encriptar (ej: blood_type)
			entry.Value = field.Value
			data[key] = entry
		}
	}

	// 3. Contar conflictos pendientes
	pendingCount, err := uc.medicalPendingRepo.CountPending(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("count pending updates: %w", err)
	}

	return &MedicalProfileResponse{
		Data:                 data,
		IsShared:             mp.IsShared,
		HasPendingConflicts:  pendingCount > 0,
		PendingConflictCount: pendingCount,
	}, nil
}
