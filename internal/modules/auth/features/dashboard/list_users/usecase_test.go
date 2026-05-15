// RED phase — tests for list_users usecase.
// These reference types and functions that do NOT exist yet.
// They MUST fail to compile initially.
package list_users_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	listusers "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/list_users"
	"github.com/ProacTrip/Backend/internal/shared/pagination"
)

// =============================================================================
// Stub — implementa list_users.UserListRepo
// =============================================================================

type stubUserListRepo struct {
	listFn func(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error)
}

func (s *stubUserListRepo) ListUsers(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error) {
	return s.listFn(ctx, filters)
}

// =============================================================================
// Fixtures
// =============================================================================

func userRow(id uuid.UUID, email string, status domain.UserStatus, roleID uuid.UUID, roleName string) listusers.UserRow {
	return listusers.UserRow{
		ID:            id,
		Email:         email,
		Status:        string(status),
		RoleID:        roleID,
		RoleName:      roleName,
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

func newUseCase(listFn func(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error)) *listusers.UseCase {
	repo := &stubUserListRepo{listFn: listFn}
	return listusers.NewUseCase(repo)
}

// =============================================================================
// Tests
// =============================================================================

// TestExecute_FirstPage returns users + meta with has_next and no prev_cursor.
func TestExecute_FirstPage(t *testing.T) {
	ctx := t.Context()

	ids := []uuid.UUID{uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())}
	roleID := uuid.Must(uuid.NewV7())

	uc := newUseCase(func(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error) {
		return []listusers.UserRow{
			userRow(ids[0], "alice@test.com", domain.StatusActive, roleID, "client"),
			userRow(ids[1], "bob@test.com", domain.StatusActive, roleID, "client"),
		}, 2, nil
	})

	cmd := listusers.Command{Limit: 2}
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(resp.Users))
	}
	if resp.Meta.PrevCursor != nil {
		t.Errorf("first page should have nil prev_cursor, got %q", *resp.Meta.PrevCursor)
	}
	if resp.Meta.HasNext {
		t.Error("has_next should be false with 2 users and limit=2")
	}
	if resp.Meta.NextCursor != nil {
		t.Errorf("last page should have nil next_cursor, got %q", *resp.Meta.NextCursor)
	}
	if resp.Meta.Limit != 2 {
		t.Errorf("meta.limit = %d, expected 2", resp.Meta.Limit)
	}
}

// TestExecute_HasNext returns has_next=true when total > limit.
func TestExecute_HasNext(t *testing.T) {
	ctx := t.Context()
	roleID := uuid.Must(uuid.NewV7())

	uc := newUseCase(func(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error) {
		return []listusers.UserRow{
			userRow(uuid.Must(uuid.NewV7()), "a@test.com", domain.StatusActive, roleID, "client"),
		}, 10, nil // 1 returned, 10 total
	})

	cmd := listusers.Command{Limit: 1}
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.Meta.HasNext {
		t.Error("has_next should be true when total (10) > limit (1)")
	}
	if resp.Meta.NextCursor == nil {
		t.Error("next_cursor should be present when has_next is true")
	}
	if resp.Meta.Limit != 1 {
		t.Errorf("meta.limit = %d, expected 1", resp.Meta.Limit)
	}
}

// TestExecute_CursorPagination decodes cursor and returns correct offset page.
func TestExecute_CursorPagination(t *testing.T) {
	ctx := t.Context()
	roleID := uuid.Must(uuid.NewV7())

	uc := newUseCase(func(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error) {
		return []listusers.UserRow{
			userRow(uuid.Must(uuid.NewV7()), "c@test.com", domain.StatusActive, roleID, "client"),
		}, 5, nil
	})

	cursor := pagination.EncodeCursor(2)
	cmd := listusers.Command{Limit: 2, Cursor: cursor}
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Meta.PrevCursor == nil {
		t.Error("page 2 should have prev_cursor")
	}
	if len(resp.Users) != 1 {
		t.Errorf("expected 1 user on page 2, got %d", len(resp.Users))
	}
}

// TestExecute_EmptyResult returns empty slice + meta with has_next=false.
func TestExecute_EmptyResult(t *testing.T) {
	ctx := t.Context()

	uc := newUseCase(func(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error) {
		return nil, 0, nil
	})

	cmd := listusers.Command{Limit: 10}
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Users) != 0 {
		t.Errorf("expected 0 users, got %d", len(resp.Users))
	}
	if resp.Meta.HasNext {
		t.Error("has_next should be false for empty results")
	}
	if resp.Meta.NextCursor != nil {
		t.Error("next_cursor should be nil for empty results")
	}
}

// TestExecute_RepoError returns wrapped error.
func TestExecute_RepoError(t *testing.T) {
	ctx := t.Context()

	repoErr := errors.New("db connection refused")
	uc := newUseCase(func(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error) {
		return nil, 0, repoErr
	})

	cmd := listusers.Command{Limit: 10}
	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repo error to be wrapped, got %v", err)
	}
}

// TestValidate_DefaultLimit sets limit to 10 when zero.
func TestValidate_DefaultLimit(t *testing.T) {
	cmd := listusers.Command{}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if cmd.Limit != 10 {
		t.Errorf("expected default limit=10, got %d", cmd.Limit)
	}
}

// TestValidate_MaxLimit rejects limit > 100.
func TestValidate_MaxLimit(t *testing.T) {
	cmd := listusers.Command{Limit: 101}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for limit > 100, got nil")
	}
}

// TestValidate_InvalidStatus rejects unknown status.
func TestValidate_InvalidStatus(t *testing.T) {
	cmd := listusers.Command{Status: "invalid_status", Limit: 10}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for invalid status, got nil")
	}
}

// TestValidate_AllValidStatuses accepts every status in the CHECK constraint.
func TestValidate_AllValidStatuses(t *testing.T) {
	validStatuses := []string{"active", "inactive", "suspended", "pending_verification", "locked", "disabled"}
	for _, s := range validStatuses {
		t.Run("status="+s, func(t *testing.T) {
			cmd := listusers.Command{Status: s, Limit: 10}
			if err := cmd.Validate(); err != nil {
				t.Errorf("expected status %q to be valid, got error: %v", s, err)
			}
		})
	}
}

// TestExecute_EmptyCursorFirstPage treats empty string cursor as first page.
func TestExecute_EmptyCursorFirstPage(t *testing.T) {
	ctx := t.Context()
	roleID := uuid.Must(uuid.NewV7())

	uc := newUseCase(func(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error) {
		// Verify offset is 0 when cursor is empty
		if filters.Offset != 0 {
			t.Errorf("expected offset=0 for empty cursor, got %d", filters.Offset)
		}
		return []listusers.UserRow{
			userRow(uuid.Must(uuid.NewV7()), "a@test.com", domain.StatusActive, roleID, "client"),
		}, 1, nil
	})

	cmd := listusers.Command{Limit: 10, Cursor: ""}
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Meta.PrevCursor != nil {
		t.Errorf("first page should have nil prev_cursor")
	}
}

// TestExecute_PrevCursorCalculates correct previous page offset.
func TestExecute_PrevCursorCalculates(t *testing.T) {
	ctx := t.Context()
	roleID := uuid.Must(uuid.NewV7())

	uc := newUseCase(func(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error) {
		return []listusers.UserRow{
			userRow(uuid.Must(uuid.NewV7()), "x@test.com", domain.StatusActive, roleID, "client"),
		}, 10, nil
	})

	// Page 2: offset=5, limit=5 → prev should be offset 0
	cursor := pagination.EncodeCursor(5)
	cmd := listusers.Command{Limit: 5, Cursor: cursor}
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Meta.PrevCursor == nil {
		t.Fatal("page 2 should have prev_cursor")
	}
	prevOffset, _ := pagination.DecodeCursor(*resp.Meta.PrevCursor)
	if prevOffset != 0 {
		t.Errorf("prev_cursor offset = %d, expected 0", prevOffset)
	}
}

// TestValidate_NegativeLimit rejects negative limit.
func TestValidate_NegativeLimit(t *testing.T) {
	cmd := listusers.Command{Limit: -1}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for negative limit, got nil")
	}
}
