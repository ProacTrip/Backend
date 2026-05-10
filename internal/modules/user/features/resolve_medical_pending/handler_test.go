package resolve_medical_pending

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

type testRMPPendingRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error)
	resolveFn func(ctx context.Context, id uuid.UUID, status domain.MedicalPendingUpdateStatus) error
}
func (m *testRMPPendingRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error) {
	if m.getByIDFn != nil { return m.getByIDFn(ctx, id) }; return nil, nil
}
func (m *testRMPPendingRepo) Resolve(ctx context.Context, id uuid.UUID, status domain.MedicalPendingUpdateStatus) error {
	if m.resolveFn != nil { return m.resolveFn(ctx, id, status) }; return nil
}

type testRMPProfileRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfileV2, error)
	updateFn      func(ctx context.Context, p *domain.MedicalProfileV2) error
}
func (m *testRMPProfileRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfileV2, error) {
	if m.getByUserIDFn != nil { return m.getByUserIDFn(ctx, userID) }; return nil, nil
}
func (m *testRMPProfileRepo) Update(ctx context.Context, p *domain.MedicalProfileV2) error {
	if m.updateFn != nil { return m.updateFn(ctx, p) }; return nil
}

type testRMPEncryption struct{ encryptFn func(plaintext string) ([]byte, error) }
func (m *testRMPEncryption) Encrypt(plaintext string) ([]byte, error) {
	if m.encryptFn != nil { return m.encryptFn(plaintext) }; return []byte(plaintext), nil
}

type testRMPEventPub struct{ publishFn func(ctx context.Context, stream string, payload map[string]interface{}) (string, error) }
func (m *testRMPEventPub) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	if m.publishFn != nil { return m.publishFn(ctx, stream, payload) }; return "id", nil
}

func TestResolveMedicalPendingHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	pid := uuid.Must(uuid.NewV7()).String()
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; body string; pp *testRMPPendingRepo; mp *testRMPProfileRepo; enc *testRMPEncryption; ep *testRMPEventPub; wantStatus int
	}{
		{"debe retornar 200 con accion accept", tc, `{"pending_update_id":"`+pid+`","action":"accept"}`,
			&testRMPPendingRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error) {
				return &domain.MedicalPendingUpdate{ID: id, UserID: uid, FieldName: "blood_type", ProposedValue: "O+", SourceType: "ocr", ExpiresAt: time.Now().Add(time.Hour)}, nil
			}},
			&testRMPProfileRepo{getByUserIDFn: func(ctx context.Context, uid2 uuid.UUID) (*domain.MedicalProfileV2, error) {
				return &domain.MedicalProfileV2{ID: uuid.Must(uuid.NewV7()), UserID: uid2, Data: map[string]*domain.MedicalFieldValue{}}, nil
			}},
			&testRMPEncryption{}, &testRMPEventPub{}, http.StatusOK},
		{"debe retornar error sin claims", nil, `{"pending_update_id":"`+pid+`","action":"accept"}`, &testRMPPendingRepo{}, &testRMPProfileRepo{}, &testRMPEncryption{}, &testRMPEventPub{}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodPost, "/v1/user/profile/medical/pending/resolve", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{MedicalPendingRepo: tt.pp, MedicalProfileRepo: tt.mp, EncryptionService: tt.enc, EventPublisher: tt.ep})
			h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d. body = %s", rec.Code, tt.wantStatus, rec.Body.String()) }
		})
	}
}
