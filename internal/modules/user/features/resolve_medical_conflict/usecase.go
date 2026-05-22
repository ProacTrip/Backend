// Caso de uso: Resolver conflicto médico (accept/reject/custom).
package resolve_medical_conflict

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

// MedicalPendingRepo permite leer y resolver conflictos médicos.
type MedicalPendingRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error)
	Resolve(ctx context.Context, id uuid.UUID, status domain.MedicalPendingUpdateStatus) error
}

// MedicalProfileRepo permite leer y actualizar el perfil médico.
type MedicalProfileRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfile, error)
	Update(ctx context.Context, profile *domain.MedicalProfile) error
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
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	MedicalPendingRepo MedicalPendingRepo
	MedicalProfileRepo MedicalProfileRepo
	EncryptionService  EncryptionSvc
	EventPublisher     EventPublisher
}

// UseCase implementa la resolución de conflictos médicos.
type UseCase struct {
	medicalPendingRepo MedicalPendingRepo
	medicalProfileRepo MedicalProfileRepo
	encryptionService  EncryptionSvc
	eventPublisher     EventPublisher
	wg                 sync.WaitGroup
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		medicalPendingRepo: deps.MedicalPendingRepo,
		medicalProfileRepo: deps.MedicalProfileRepo,
		encryptionService:  deps.EncryptionService,
		eventPublisher:     deps.EventPublisher,
	}
}

// Wait espera a que todos los eventos publicados asíncronamente terminen.
func (uc *UseCase) Wait() { uc.wg.Wait() }

// Execute resuelve un conflicto médico según la acción solicitada.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*ResolveResponse, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	conflictID, err := uuid.Parse(cmd.ConflictID)
	if err != nil {
		return nil, fmt.Errorf("invalid conflict_id: %w", err)
	}

	// 1. Fetch pending update
	pu, err := uc.medicalPendingRepo.GetByID(ctx, conflictID)
	if err != nil {
		return nil, err
	}
	if pu == nil {
		return nil, domain.ErrPendingUpdateNotFound
	}

	// 2. Check ownership — debe pertenecer al usuario autenticado
	if pu.UserID != userID {
		return nil, domain.ErrPendingUpdateNotFound
	}

	// 3. Check not expired
	if time.Now().After(pu.ExpiresAt) {
		return nil, domain.ErrPendingUpdateExpired
	}

	// 4. Validar acción
	if !ValidActions[cmd.Action] {
		return nil, domain.ErrInvalidPendingAction
	}

	// 5. Fetch medical profile
	mp, err := uc.medicalProfileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if mp == nil {
		return nil, domain.ErrMedicalProfileNotFound
	}

	now := time.Now()
	var resolvedStatus domain.MedicalPendingUpdateStatus

	switch cmd.Action {
	case "accept":
		// Aceptar: setear valor propuesto en el perfil médico
		if err := uc.setMedicalField(mp, pu.FieldName, pu.ProposedValue, pu.SourceType, pu.SourceDocumentID, now); err != nil {
			return nil, err
		}
		if err := uc.medicalProfileRepo.Update(ctx, mp); err != nil {
			return nil, fmt.Errorf("update medical profile: %w", err)
		}
		if err := uc.medicalPendingRepo.Resolve(ctx, conflictID, domain.PendingUpdateAccepted); err != nil {
			return nil, err
		}
		resolvedStatus = domain.PendingUpdateAccepted

	case "reject":
		// Rechazar: mantener valor actual, solo marcar como rechazado
		if err := uc.medicalPendingRepo.Resolve(ctx, conflictID, domain.PendingUpdateRejected); err != nil {
			return nil, err
		}
		resolvedStatus = domain.PendingUpdateRejected

	case "custom":
		// Valor personalizado
		if cmd.Value == nil {
			return nil, domain.ErrInvalidPendingAction
		}
		if err := uc.setMedicalField(mp, pu.FieldName, *cmd.Value, pu.SourceType, pu.SourceDocumentID, now); err != nil {
			return nil, err
		}
		if err := uc.medicalProfileRepo.Update(ctx, mp); err != nil {
			return nil, fmt.Errorf("update medical profile: %w", err)
		}
		if err := uc.medicalPendingRepo.Resolve(ctx, conflictID, domain.PendingUpdateAccepted); err != nil {
			return nil, err
		}
		resolvedStatus = domain.PendingUpdateAccepted
	}

	// Emit event (best-effort)
	if uc.eventPublisher != nil {
		uc.wg.Go(func() {
			bgCtx := context.WithoutCancel(ctx)
			_, err := uc.eventPublisher.Publish(bgCtx,
				eventbus.StreamName("user.medical_pending.resolved"),
				map[string]interface{}{
					"user_id":     userID.String(),
					"pending_id":  conflictID.String(),
					"status":      string(resolvedStatus),
				},
			)
			if err != nil {
				slog.WarnContext(bgCtx, "publish medical pending resolved event failed",
					slog.String("user_id", userID.String()),
					slog.String("pending_id", conflictID.String()),
					slog.String("error", err.Error()),
				)
			}
		})
	}

	return &ResolveResponse{
		Message: "Conflicto médico resuelto correctamente.",
	}, nil
}

// setMedicalField establece un campo en el perfil médico.
// Los campos de texto se encriptan, blood_type se guarda en texto plano.
func (uc *UseCase) setMedicalField(mp *domain.MedicalProfile, fieldName, value, sourceType string, sourceDocumentID *uuid.UUID, now time.Time) error {
	// Construir source detail
	source := domain.SourceToDetail(domain.MedicalSource(sourceType))
	if sourceDocumentID != nil {
		docIDStr := sourceDocumentID.String()
		source.DocumentID = &docIDStr
	}

	// Determinar si el campo se encripta
	encryptableFields := map[string]bool{
		"allergies": true, "medications": true, "conditions": true,
		"vaccinations": true, "emergency_contact": true, "insurance_info": true,
	}

	if encryptableFields[fieldName] {
		// Encriptar
		encrypted, err := uc.encryptionService.Encrypt(value)
		if err != nil {
			return fmt.Errorf("%w: encrypt %s: %w", domain.ErrEncryptionError, fieldName, err)
		}
		encodedValue := base64.StdEncoding.EncodeToString(encrypted)

		mp.Data[fieldName+"_enc"] = &domain.MedicalFieldValue{
			Value:     encodedValue,
			Source:    source,
			UpdatedAt: now,
		}
	} else {
		// Guardar en texto plano (blood_type)
		mp.Data[fieldName] = &domain.MedicalFieldValue{
			Value:     value,
			Source:    source,
			UpdatedAt: now,
		}
	}
	mp.UpdatedAt = now

	return nil
}
