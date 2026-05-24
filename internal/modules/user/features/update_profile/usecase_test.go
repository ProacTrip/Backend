// RED — Test del usecase update_profile.
package update_profile

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type mockProfileRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error)
	updateFn      func(ctx context.Context, profile *domain.UserProfile) error
}

func (m *mockProfileRepo) Create(ctx context.Context, p *domain.UserProfile) error  { return nil }
func (m *mockProfileRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockProfileRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
	return nil, nil
}
func (m *mockProfileRepo) Update(ctx context.Context, p *domain.UserProfile) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, p)
	}
	return nil
}
func (m *mockProfileRepo) UpdateLocale(ctx context.Context, userID uuid.UUID, tz, lang, cur, loc string) error {
	return nil
}
func (m *mockProfileRepo) UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error {
	return nil
}

type mockEventPublisher struct {
	publishFn func(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

func (m *mockEventPublisher) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	if m.publishFn != nil {
		return m.publishFn(ctx, stream, payload)
	}
	return "msg-1", nil
}

// =============================================================================
// Tests
// =============================================================================

func TestUpdateProfile_HappyPath(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	firstName := "Juan"
	lastName := "Pérez"
	bio := "Viajero frecuente"

	cmd := Command{
		UserID:    userID.String(),
		FirstName: &firstName,
		LastName:  &lastName,
		Bio:       &bio,
	}

	called := false
	uc := NewUseCase(UseCaseDeps{
		ProfileRepo: &mockProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
				return domain.NewUserProfile(userID, ""), nil
			},
			updateFn: func(ctx context.Context, p *domain.UserProfile) error {
				called = true
				if p.FirstName == nil || *p.FirstName != "Juan" {
					t.Error("FirstName no fue actualizado correctamente")
				}
				if p.LastName == nil || *p.LastName != "Pérez" {
					t.Error("LastName no fue actualizado correctamente")
				}
				return nil
			},
		},
		EventPublisher: &mockEventPublisher{},
	})

	err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !called {
		t.Error("Update debería haber sido llamado")
	}
}

func TestUpdateProfile_InvalidGender(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	invalidGender := "helicoptero"

	cmd := Command{
		UserID: userID.String(),
		Gender: &invalidGender,
	}

	uc := NewUseCase(UseCaseDeps{
		ProfileRepo: &mockProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
				return domain.NewUserProfile(userID, ""), nil
			},
		},
	})

	err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error de género inválido")
	}
	if !errors.Is(err, domain.ErrInvalidGender) {
		t.Errorf("error = %v, se esperaba ErrInvalidGender", err)
	}
}

func TestUpdateProfile_ValidGenderValues(t *testing.T) {
	tests := []struct {
		name   string
		gender string
		valid  bool
	}{
		{"male", "male", true},
		{"female", "female", true},
		{"non_binary", "non_binary", true},
		{"prefer_not_to_say", "prefer_not_to_say", true},
		{"invalid", "other", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			userID := uuid.Must(uuid.NewV7())
			cmd := Command{
				UserID: userID.String(),
				Gender: &tc.gender,
			}

			uc := NewUseCase(UseCaseDeps{
				ProfileRepo: &mockProfileRepo{
					getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
						return domain.NewUserProfile(userID, ""), nil
					},
				},
			})

			err := uc.Execute(t.Context(), cmd)
			if tc.valid && err != nil {
				t.Errorf("no se esperaba error para %s: %v", tc.gender, err)
			}
			if !tc.valid && !errors.Is(err, domain.ErrInvalidGender) {
				t.Errorf("se esperaba ErrInvalidGender para %s, obtuve %v", tc.gender, err)
			}
		})
	}
}

func TestUpdateProfile_ProfileNotFound(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		ProfileRepo: &mockProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
				return nil, domain.ErrProfileNotFound
			},
		},
	})

	cmd := Command{UserID: uuid.Must(uuid.NewV7()).String()}
	err := uc.Execute(t.Context(), cmd)
	if !errors.Is(err, domain.ErrProfileNotFound) {
		t.Errorf("se esperaba ErrProfileNotFound, obtuve %v", err)
	}
}

func TestUpdateProfile_NationalityValidation(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name        string
		nationality string
		valid       bool
	}{
		{"AR válido", "AR", true},
		{"ES válido", "ES", true},
		{"US válido", "US", true},
		{"inválido largo", "Argentina", false},
		{"inválido 1 char", "A", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := Command{
				UserID:      userID.String(),
				Nationality: &tc.nationality,
			}
			uc := NewUseCase(UseCaseDeps{
				ProfileRepo: &mockProfileRepo{
					getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
						return domain.NewUserProfile(userID, ""), nil
					},
				},
			})
			err := uc.Execute(t.Context(), cmd)
			if tc.valid && err != nil {
				t.Errorf("no se esperaba error para %s: %v", tc.nationality, err)
			}
			if !tc.valid && err == nil {
				t.Errorf("se esperaba error para %s", tc.nationality)
			}
		})
	}
}

func TestUpdateProfile_DateOfBirthFormat(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	dob := DateOnly(time.Date(1990, 5, 15, 0, 0, 0, 0, time.UTC))

	cmd := Command{
		UserID:      userID.String(),
		DateOfBirth: &dob,
	}

	called := false
	uc := NewUseCase(UseCaseDeps{
		ProfileRepo: &mockProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
				return domain.NewUserProfile(userID, ""), nil
			},
			updateFn: func(ctx context.Context, p *domain.UserProfile) error {
				called = true
				if p.DateOfBirth == nil || p.DateOfBirth.Format("2006-01-02") != "1990-05-15" {
					t.Error("DateOfBirth mal actualizado")
				}
				return nil
			},
		},
	})

	err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !called {
		t.Error("Update debería haber sido llamado")
	}
}

// =============================================================================
// T-2.3 / T-2.4: E.164 phone validation
// =============================================================================

func TestUpdateProfile_PhoneValidation_ValidE164(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	validPhones := []string{"+5491123456789", "+12025550123", "+8613800138000", "+34123456789"}

	for _, phone := range validPhones {
		t.Run(phone, func(t *testing.T) {
			cmd := Command{
				UserID: userID.String(),
				Phone:  &phone,
			}
			uc := NewUseCase(UseCaseDeps{
				ProfileRepo: &mockProfileRepo{
					getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
						return domain.NewUserProfile(userID, ""), nil
					},
					updateFn: func(ctx context.Context, p *domain.UserProfile) error { return nil },
				},
			})
			err := uc.Execute(t.Context(), cmd)
			if err != nil {
				t.Errorf("teléfono válido %s fue rechazado: %v", phone, err)
			}
		})
	}
}

func TestUpdateProfile_PhoneValidation_InvalidE164(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	invalidPhones := []string{
		"1123456789",       // sin +
		"++5491123456789",   // doble +
		"+0",               // empieza con 0 después del +
		"+",                // solo +
		"+54 911 23456789", // espacios
		"+54-911-23456789", // guiones
		"12345",            // sin + ni código de país
	}

	for _, phone := range invalidPhones {
		t.Run(phone, func(t *testing.T) {
			ph := phone // local copy
			cmd := Command{
				UserID: userID.String(),
				Phone:  &ph,
			}
			uc := NewUseCase(UseCaseDeps{
				ProfileRepo: &mockProfileRepo{
					getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
						return domain.NewUserProfile(userID, ""), nil
					},
				},
			})
			err := uc.Execute(t.Context(), cmd)
			if err == nil {
				t.Errorf("teléfono inválido %q debería haber sido rechazado", phone)
			}
			if !errors.Is(err, domain.ErrInvalidPhone) {
				t.Errorf("error = %v, se esperaba ErrInvalidPhone para %q", err, phone)
			}
		})
	}
}

func TestUpdateProfile_PhoneValidation_NilPhone(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	firstName := "Juan"

	cmd := Command{
		UserID:    userID.String(),
		FirstName: &firstName,
		Phone:     nil,
	}
	uc := NewUseCase(UseCaseDeps{
		ProfileRepo: &mockProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
				return domain.NewUserProfile(userID, ""), nil
			},
			updateFn: func(ctx context.Context, p *domain.UserProfile) error { return nil },
		},
	})
	err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Errorf("phone nil debería ser aceptado (no phone = skip validation): %v", err)
	}
}
