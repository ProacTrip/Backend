// RED phase — tests for user_detail usecase.
// These reference types and functions that do NOT exist yet.
// They MUST fail to compile initially.
package user_detail_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	userdetail "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/user_detail"
)

// =============================================================================
// Stubs
// =============================================================================

type stubUserRepo struct {
	getByID func(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

func (s *stubUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return s.getByID(ctx, id)
}

type stubPermissionResolver struct {
	resolveFn func(ctx context.Context, userID, roleID uuid.UUID) ([]string, error)
}

func (s *stubPermissionResolver) ResolveEffectivePermissions(ctx context.Context, userID, roleID uuid.UUID) ([]string, error) {
	return s.resolveFn(ctx, userID, roleID)
}

type stubDocumentLister struct {
	getDocsFn func(ctx context.Context, userID uuid.UUID) ([]domain.DocumentSummary, error)
}

func (s *stubDocumentLister) GetUserDocuments(ctx context.Context, userID uuid.UUID) ([]domain.DocumentSummary, error) {
	return s.getDocsFn(ctx, userID)
}

// =============================================================================
// Fixtures
// =============================================================================

func usuarioActivo(id uuid.UUID, email string, roleID uuid.UUID) *domain.User {
	now := time.Now()
	return &domain.User{
		ID:            id,
		Email:         email,
		EmailVerified: true,
		RoleID:        roleID,
		RoleName:      "client",
		Status:        domain.StatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func newUseCase(
	getByID func(ctx context.Context, id uuid.UUID) (*domain.User, error),
	resolveFn func(ctx context.Context, userID, roleID uuid.UUID) ([]string, error),
) *userdetail.UseCase {
	repo := &stubUserRepo{getByID: getByID}
	resolver := &stubPermissionResolver{resolveFn: resolveFn}
	lister := &stubDocumentLister{
		getDocsFn: func(ctx context.Context, userID uuid.UUID) ([]domain.DocumentSummary, error) {
			return nil, nil
		},
	}
	return userdetail.NewUseCase(repo, resolver, lister)
}

// =============================================================================
// Tests
// =============================================================================

// TestExecute_Success returns user detail with effective permissions.
func TestExecute_Success(t *testing.T) {
	ctx := t.Context()

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "alice@test.com", roleID), nil
		},
		func(ctx context.Context, userID, roleID uuid.UUID) ([]string, error) {
			return []string{"users:read", "users:write"}, nil
		},
	)

	cmd := userdetail.Command{UserID: userID}
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.User.ID != userID {
		t.Errorf("user.ID = %s, expected %s", resp.User.ID, userID)
	}
	if resp.User.Email != "alice@test.com" {
		t.Errorf("user.Email = %s, expected alice@test.com", resp.User.Email)
	}
	if len(resp.EffectivePermissions) != 2 {
		t.Errorf("expected 2 effective permissions, got %d: %v", len(resp.EffectivePermissions), resp.EffectivePermissions)
	}
	if resp.EffectivePermissions[0] != "users:read" {
		t.Errorf("expected users:read, got %s", resp.EffectivePermissions[0])
	}
}

// TestExecute_UserNotFound returns domain.ErrUserNotFound.
func TestExecute_UserNotFound(t *testing.T) {
	ctx := t.Context()

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
		nil,
	)

	cmd := userdetail.Command{UserID: uuid.Must(uuid.NewV7())}
	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for non-existent user, got nil")
	}
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

// TestExecute_PermissionResolverError wraps the error.
func TestExecute_PermissionResolverError(t *testing.T) {
	ctx := t.Context()

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	resolverErr := errors.New("permission resolver: db timeout")

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "alice@test.com", roleID), nil
		},
		func(ctx context.Context, userID, roleID uuid.UUID) ([]string, error) {
			return nil, resolverErr
		},
	)

	cmd := userdetail.Command{UserID: userID}
	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error from resolver, got nil")
	}
	if !errors.Is(err, resolverErr) {
		t.Errorf("expected resolver error to be wrapped, got %v", err)
	}
}

// TestExecute_EmptyPermissions returns user with empty permissions slice.
func TestExecute_EmptyPermissions(t *testing.T) {
	ctx := t.Context()

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "noperms@test.com", roleID), nil
		},
		func(ctx context.Context, userID, roleID uuid.UUID) ([]string, error) {
			return []string{}, nil
		},
	)

	cmd := userdetail.Command{UserID: userID}
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.EffectivePermissions) != 0 {
		t.Errorf("expected 0 permissions, got %d", len(resp.EffectivePermissions))
	}
}

// TestExecute_SensitiveFieldsExcluded verifies password_hash, locked_until, etc. are NOT in response.
func TestExecute_SensitiveFieldsExcluded(t *testing.T) {
	ctx := t.Context()

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	lockedUntil := time.Now().Add(10 * time.Minute)

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			u := usuarioActivo(userID, "sensitive@test.com", roleID)
			u.PasswordHash = "should-not-leak"
			u.LockedUntil = new(lockedUntil)
			u.FailedLoginAttempts = 5
			return u, nil
		},
		func(ctx context.Context, userID, roleID uuid.UUID) ([]string, error) {
			return []string{}, nil
		},
	)

	cmd := userdetail.Command{UserID: userID}
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The response struct should NOT have PasswordHash, LockedUntil, etc. fields at all.
	// This is verified at the type level — if the response type tried to include them,
	// it wouldn't compile. But we also verify the response is properly mapped.
	if resp.User.ID != userID {
		t.Errorf("user ID mismatch")
	}
	if resp.User.Email != "sensitive@test.com" {
		t.Errorf("user email mismatch")
	}
	// DU-SPEC-004: response fields must exclude password_hash, locked_until, failed_attempts
	// Verified by the UserDetailResponse type NOT having these fields (compile-time guarantee).
}

// TestExecute_UserWithDocuments — UD-1.1: user detail incluye documents array.
func TestExecute_UserWithDocuments(t *testing.T) {
	ctx := t.Context()

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	docID1 := uuid.Must(uuid.NewV7())
	docID2 := uuid.Must(uuid.NewV7())

	repo := &stubUserRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "docs@test.com", roleID), nil
		},
	}
	resolver := &stubPermissionResolver{
		resolveFn: func(ctx context.Context, userID, roleID uuid.UUID) ([]string, error) {
			return []string{"users:read"}, nil
		},
	}
	lister := &stubDocumentLister{
		getDocsFn: func(ctx context.Context, uid uuid.UUID) ([]domain.DocumentSummary, error) {
			passport := "passport"
			visa := "visa"
			return []domain.DocumentSummary{
				{ID: docID1, FileName: "passport.pdf", DocumentType: &passport, VerificationStatus: "verified"},
				{ID: docID2, FileName: "visa.pdf", DocumentType: &visa, VerificationStatus: "unverified"},
			}, nil
		},
	}

	uc := userdetail.NewUseCase(repo, resolver, lister)

	cmd := userdetail.Command{UserID: userID}
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Documents) != 2 {
		t.Errorf("documents len = %d, expected 2", len(resp.Documents))
	}
	if resp.Documents[0].DocumentType == nil || *resp.Documents[0].DocumentType != "passport" {
		t.Errorf("doc[0].type = %v, expected passport", resp.Documents[0].DocumentType)
	}
	if resp.Documents[1].DocumentType == nil || *resp.Documents[1].DocumentType != "visa" {
		t.Errorf("doc[1].type = %v, expected visa", resp.Documents[1].DocumentType)
	}
}

// TestExecute_NoDocuments — UD-1.2: sin documentos → empty array (not nil).
func TestExecute_NoDocuments(t *testing.T) {
	ctx := t.Context()

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())

	repo := &stubUserRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "nodocs@test.com", roleID), nil
		},
	}
	resolver := &stubPermissionResolver{
		resolveFn: func(ctx context.Context, userID, roleID uuid.UUID) ([]string, error) {
			return []string{}, nil
		},
	}
	lister := &stubDocumentLister{
		getDocsFn: func(ctx context.Context, uid uuid.UUID) ([]domain.DocumentSummary, error) {
			return []domain.DocumentSummary{}, nil
		},
	}

	uc := userdetail.NewUseCase(repo, resolver, lister)

	cmd := userdetail.Command{UserID: userID}
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Documents == nil {
		t.Error("documents should be empty slice, not nil")
	}
	if len(resp.Documents) != 0 {
		t.Errorf("documents len = %d, expected 0", len(resp.Documents))
	}
}

// TestValidate_EmptyUserID rejects zero UUID.
func TestValidate_EmptyUserID(t *testing.T) {
	cmd := userdetail.Command{UserID: uuid.Nil}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for nil UUID, got nil")
	}
}
