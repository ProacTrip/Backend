// Caso de uso: Actualizar perfil médico del usuario (PUT /v1/user/profile/medical).
// Encripta transparentemente los campos sensibles antes de almacenarlos.
package update_medical_profile

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Ports
// =============================================================================

// MedicalProfileRepo permite leer y actualizar el perfil médico.
type MedicalProfileRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfileV2, error)
	Update(ctx context.Context, profile *domain.MedicalProfileV2) error
}

// EncryptionSvc permite encriptar valores médicos.
type EncryptionSvc interface {
	Encrypt(plaintext string) ([]byte, error)
}

// EventPublisher permite publicar eventos de dominio.
type EventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

// =============================================================================
// UpdateMedicalProfileResponse
// =============================================================================

// UpdateMedicalProfileResponse es la respuesta del endpoint.
type UpdateMedicalProfileResponse struct {
	Message       string   `json:"message"`
	AppliedFields []string `json:"applied_fields"`
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	MedicalProfileRepo MedicalProfileRepo
	EncryptionService  EncryptionSvc
	EventPublisher     EventPublisher
}

// UseCase implementa la actualización del perfil médico.
type UseCase struct {
	medicalProfileRepo MedicalProfileRepo
	encryptionService  EncryptionSvc
	eventPublisher     EventPublisher
	wg                 sync.WaitGroup
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		medicalProfileRepo: deps.MedicalProfileRepo,
		encryptionService:  deps.EncryptionService,
		eventPublisher:     deps.EventPublisher,
	}
}

// Wait espera a que todos los eventos publicados asíncronamente terminen.
func (uc *UseCase) Wait() { uc.wg.Wait() }

// Execute valida el comando, encripta campos y actualiza el perfil médico.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*UpdateMedicalProfileResponse, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	// 1. Verificar que el perfil existe
	existing, err := uc.medicalProfileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, domain.ErrMedicalProfileNotFound
	}

	// 2. Validar blood_type si se envió
	if cmd.BloodType != nil && !ValidBloodTypes[*cmd.BloodType] {
		return nil, domain.ErrInvalidBloodType
	}

	// 3. Aplicar cambios — encriptar campos de texto, blood_type en texto plano
	appliedFields := make([]string, 0)
	now := time.Now()

	// blood_type: NO encriptado
	if cmd.BloodType != nil {
		existing.Data["blood_type"] = &domain.MedicalFieldValue{
			Value:     *cmd.BloodType,
			Source:    domain.MedicalSourceProfile,
			UpdatedAt: now,
		}
		appliedFields = append(appliedFields, "blood_type")
	}

	// Campos encriptados: allergies, medications, conditions, vaccinations, emergency_contact, insurance_info
	encryptedFields := map[string]*string{
		"allergies":        cmd.Allergies,
		"medications":      cmd.Medications,
		"conditions":       cmd.Conditions,
		"vaccinations":     cmd.Vaccinations,
		"emergency_contact": cmd.EmergencyContact,
		"insurance_info":   cmd.InsuranceInfo,
	}

	for fieldName, fieldValue := range encryptedFields {
		if fieldValue == nil {
			continue
		}

		// Encriptar el valor
		encrypted, err := uc.encryptionService.Encrypt(*fieldValue)
		if err != nil {
			return nil, fmt.Errorf("%w: encrypt %s: %w", domain.ErrEncryptionError, fieldName, err)
		}

		// Codificar en base64 para almacenar como string JSONB
		encodedValue := base64.StdEncoding.EncodeToString(encrypted)

		existing.Data[fieldName+"_enc"] = &domain.MedicalFieldValue{
			Value:     encodedValue,
			Source:    domain.MedicalSourceProfile,
			UpdatedAt: now,
		}
		appliedFields = append(appliedFields, fieldName)
	}

	// is_shared
	if cmd.IsShared != nil {
		existing.IsShared = *cmd.IsShared
		appliedFields = append(appliedFields, "is_shared")
	}

	// 4. Guardar en DB
	existing.UpdatedAt = now
	if err := uc.medicalProfileRepo.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("update medical profile: %w", err)
	}

	// Emit event (best-effort)
	if uc.eventPublisher != nil {
		uc.wg.Go(func() {
			bgCtx := context.WithoutCancel(ctx)
			_, err := uc.eventPublisher.Publish(bgCtx,
				eventbus.StreamName("user.medical_profile.updated"),
				map[string]interface{}{
					"user_id": userID.String(),
				},
			)
			if err != nil {
				slog.WarnContext(bgCtx, "publish medical profile updated event failed",
					slog.String("user_id", userID.String()),
					slog.String("error", err.Error()),
				)
			}
		})
	}

	return &UpdateMedicalProfileResponse{
		Message:       "Perfil médico actualizado correctamente",
		AppliedFields: appliedFields,
	}, nil
}
