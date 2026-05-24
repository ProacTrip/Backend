// Tests de validación para VerifyStatusCommand (document_verification).
// DV-2.5, DV-2.6, DV-2.7 — validación de estados, reason length, campos requeridos.
package document_verification_test

import (
	"testing"

	"github.com/google/uuid"

	docverification "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/document_verification"
)

func validUUID() uuid.UUID  { return uuid.Must(uuid.NewV7()) }
func nilUUID() uuid.UUID    { return uuid.Nil }

// =============================================================================
// VerifyCommand Tests
// =============================================================================

func TestVerifyCommand_Validate_Valid(t *testing.T) {
	cmd := docverification.VerifyCommand{DocumentID: validUUID()}
	if err := cmd.Validate(); err != nil {
		t.Errorf("expected no error for valid command, got: %v", err)
	}
}

func TestVerifyCommand_Validate_NilUUID(t *testing.T) {
	cmd := docverification.VerifyCommand{DocumentID: nilUUID()}
	if err := cmd.Validate(); err == nil {
		t.Fatal("expected error for nil UUID, got nil")
	}
}

// =============================================================================
// VerifyStatusCommand Tests — table-driven
// =============================================================================

func TestVerifyStatusCommand_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cmd     docverification.VerifyStatusCommand
		wantErr bool
	}{
		{
			name: "DV-2.1: valid approval",
			cmd: docverification.VerifyStatusCommand{
				DocumentID: validUUID(),
				Status:     "verified",
				Reason:     ptr("MRZ válido"),
				VerifiedBy: validUUID(),
			},
			wantErr: false,
		},
		{
			name: "DV-2.2: valid rejection without reason",
			cmd: docverification.VerifyStatusCommand{
				DocumentID: validUUID(),
				Status:     "rejected",
				VerifiedBy: validUUID(),
			},
			wantErr: false,
		},
		{
			name: "DV-2.3: manual_review valid",
			cmd: docverification.VerifyStatusCommand{
				DocumentID: validUUID(),
				Status:     "manual_review",
				VerifiedBy: validUUID(),
			},
			wantErr: false,
		},
		{
			name: "DV-2.4: suspicious valid",
			cmd: docverification.VerifyStatusCommand{
				DocumentID: validUUID(),
				Status:     "suspicious",
				VerifiedBy: validUUID(),
			},
			wantErr: false,
		},
		{
			name: "DV-2.5: pending rejected (read-only initial state)",
			cmd: docverification.VerifyStatusCommand{
				DocumentID: validUUID(),
				Status:     "pending",
				VerifiedBy: validUUID(),
			},
			wantErr: true,
		},
		{
			name: "DV-2.6: reason > 500 chars",
			cmd: docverification.VerifyStatusCommand{
				DocumentID: validUUID(),
				Status:     "verified",
				Reason:     ptr(string(make([]byte, 501))),
				VerifiedBy: validUUID(),
			},
			wantErr: true,
		},
		{
			name: "DV-2.7: empty status",
			cmd: docverification.VerifyStatusCommand{
				DocumentID: validUUID(),
				Status:     "",
				VerifiedBy: validUUID(),
			},
			wantErr: true,
		},
		{
			name: "nil document ID",
			cmd: docverification.VerifyStatusCommand{
				DocumentID: nilUUID(),
				Status:     "verified",
				VerifiedBy: validUUID(),
			},
			wantErr: true,
		},
		{
			name: "nil verified_by",
			cmd: docverification.VerifyStatusCommand{
				DocumentID: validUUID(),
				Status:     "verified",
				VerifiedBy: nilUUID(),
			},
			wantErr: true,
		},
		{
			name: "reason exactly 500 chars is valid",
			cmd: docverification.VerifyStatusCommand{
				DocumentID: validUUID(),
				Status:     "verified",
				Reason:     ptr(string(make([]byte, 500))),
				VerifiedBy: validUUID(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
