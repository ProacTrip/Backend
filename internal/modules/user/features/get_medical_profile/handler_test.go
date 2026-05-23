package get_medical_profile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

type testGMMedicalRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfile, error)
}
func (m *testGMMedicalRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfile, error) {
	if m.getByUserIDFn != nil { return m.getByUserIDFn(ctx, userID) }; return nil, nil
}
func (m *testGMMedicalRepo) Update(ctx context.Context, p *domain.MedicalProfile) error { return nil }

type testGMEncryption struct{ decryptFn func(ciphertext []byte) (string, error) }
func (m *testGMEncryption) Decrypt(ciphertext []byte) (string, error) {
	if m.decryptFn != nil { return m.decryptFn(ciphertext) }; return string(ciphertext), nil
}

type testGMPendingCounter struct{ countPendingFn func(ctx context.Context, userID uuid.UUID) (int, error) }
func (m *testGMPendingCounter) CountPending(ctx context.Context, userID uuid.UUID) (int, error) {
	if m.countPendingFn != nil { return m.countPendingFn(ctx, userID) }; return 0, nil
}

func TestGetMedicalProfileHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", JTI: uuid.Must(uuid.NewV7())}
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; mp *testGMMedicalRepo; enc *testGMEncryption; pc *testGMPendingCounter; wantStatus int
	}{
		{"debe retornar 200 con perfil medico", tc, &testGMMedicalRepo{getByUserIDFn: func(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfile, error) {
			return &domain.MedicalProfile{ID: uuid.Must(uuid.NewV7()), UserID: userID, Data: map[string]*domain.MedicalFieldValue{}}, nil
		}}, &testGMEncryption{}, &testGMPendingCounter{}, http.StatusOK},
		{"debe retornar error sin claims", nil, &testGMMedicalRepo{}, &testGMEncryption{}, &testGMPendingCounter{}, http.StatusUnauthorized},
		{"debe retornar error cuando perfil no existe", tc, &testGMMedicalRepo{getByUserIDFn: func(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfile, error) {
			return nil, domain.ErrMedicalProfileNotFound
		}}, &testGMEncryption{}, &testGMPendingCounter{}, http.StatusInternalServerError}, // mapper no registrado en test aislado
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodGet, "/v1/user/profile/medical", nil)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{MedicalProfileRepo: tt.mp, EncryptionService: tt.enc, MedicalPendingRepo: tt.pc})
			h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
