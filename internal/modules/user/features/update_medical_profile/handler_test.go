package update_medical_profile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

type testUMPMedicalRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfile, error)
	updateFn      func(ctx context.Context, p *domain.MedicalProfile) error
}
func (m *testUMPMedicalRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfile, error) {
	if m.getByUserIDFn != nil { return m.getByUserIDFn(ctx, userID) }; return nil, nil
}
func (m *testUMPMedicalRepo) Update(ctx context.Context, p *domain.MedicalProfile) error {
	if m.updateFn != nil { return m.updateFn(ctx, p) }; return nil
}

type testUMPEncryption struct{ encryptFn func(plaintext string) ([]byte, error) }
func (m *testUMPEncryption) Encrypt(plaintext string) ([]byte, error) {
	if m.encryptFn != nil { return m.encryptFn(plaintext) }; return []byte(plaintext), nil
}

type testUMPEventPub struct{ publishFn func(ctx context.Context, stream string, payload map[string]interface{}) (string, error) }
func (m *testUMPEventPub) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	if m.publishFn != nil { return m.publishFn(ctx, stream, payload) }; return "id", nil
}

func TestUpdateMedicalProfileHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; body string; mp *testUMPMedicalRepo; enc *testUMPEncryption; ep *testUMPEventPub; wantStatus int
	}{
		{"debe retornar 200 con blood_type A+", tc, `{"blood_type":"A+"}`,
			&testUMPMedicalRepo{getByUserIDFn: func(ctx context.Context, uid uuid.UUID) (*domain.MedicalProfile, error) {
				return &domain.MedicalProfile{ID: uuid.Must(uuid.NewV7()), UserID: uid, Data: map[string]*domain.MedicalFieldValue{}}, nil
			}},
			&testUMPEncryption{}, &testUMPEventPub{}, http.StatusOK},
		{"debe retornar error sin claims", nil, `{"blood_type":"A+"}`, &testUMPMedicalRepo{}, &testUMPEncryption{}, &testUMPEventPub{}, http.StatusUnauthorized},
		{"debe retornar error cuando perfil no existe", tc, `{"blood_type":"O+"}`,
			&testUMPMedicalRepo{getByUserIDFn: func(ctx context.Context, uid uuid.UUID) (*domain.MedicalProfile, error) {
				return nil, domain.ErrMedicalProfileNotFound
			}},
			&testUMPEncryption{}, &testUMPEventPub{}, http.StatusInternalServerError}, // mapper no registrado en test aislado
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodPut, "/v1/user/profile/medical", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{MedicalProfileRepo: tt.mp, EncryptionService: tt.enc, EventPublisher: tt.ep})
			h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
