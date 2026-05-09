package domain

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// =============================================================================
// T-1.4: Repository interfaces — compile-time checks
// =============================================================================

// profileRepoStub satisface ProfileRepository (compile-time)
type profileRepoStub struct {
	createFn      func(context.Context, *UserProfile) error
	getByUserIDFn func(context.Context, uuid.UUID) (*UserProfile, error)
	getByIDFn     func(context.Context, uuid.UUID) (*UserProfile, error)
	updateFn      func(context.Context, *UserProfile) error
	updateLocaleFn  func(context.Context, uuid.UUID, string, string, string, string) error
	updateAvatarFn  func(context.Context, uuid.UUID, string) error
}

func (s *profileRepoStub) Create(ctx context.Context, p *UserProfile) error {
	return s.createFn(ctx, p)
}
func (s *profileRepoStub) GetByUserID(ctx context.Context, id uuid.UUID) (*UserProfile, error) {
	return s.getByUserIDFn(ctx, id)
}
func (s *profileRepoStub) GetByID(ctx context.Context, id uuid.UUID) (*UserProfile, error) {
	return s.getByIDFn(ctx, id)
}
func (s *profileRepoStub) Update(ctx context.Context, p *UserProfile) error {
	return s.updateFn(ctx, p)
}
func (s *profileRepoStub) UpdateLocale(ctx context.Context, id uuid.UUID, tz, lang, curr, loc string) error {
	return s.updateLocaleFn(ctx, id, tz, lang, curr, loc)
}
func (s *profileRepoStub) UpdateAvatar(ctx context.Context, id uuid.UUID, url string) error {
	return s.updateAvatarFn(ctx, id, url)
}

// Compile-time: profileRepoStub implements ProfileRepository
var _ ProfileRepository = (*profileRepoStub)(nil)

// travelPrefsRepoStub satisface TravelPrefsRepository
type travelPrefsRepoStub struct {
	createFn      func(context.Context, *TravelPreferences) error
	getByUserIDFn func(context.Context, uuid.UUID) (*TravelPreferences, error)
	updateFn      func(context.Context, *TravelPreferences) error
}

func (s *travelPrefsRepoStub) Create(ctx context.Context, tp *TravelPreferences) error {
	return s.createFn(ctx, tp)
}
func (s *travelPrefsRepoStub) GetByUserID(ctx context.Context, id uuid.UUID) (*TravelPreferences, error) {
	return s.getByUserIDFn(ctx, id)
}
func (s *travelPrefsRepoStub) Update(ctx context.Context, tp *TravelPreferences) error {
	return s.updateFn(ctx, tp)
}

var _ TravelPrefsRepository = (*travelPrefsRepoStub)(nil)

// medicalProfileRepoStub satisface MedicalProfileRepository
type medicalProfileRepoStub struct {
	createFn      func(context.Context, *MedicalProfileV2) error
	getByUserIDFn func(context.Context, uuid.UUID) (*MedicalProfileV2, error)
	updateFn      func(context.Context, *MedicalProfileV2) error
}

func (s *medicalProfileRepoStub) Create(ctx context.Context, mp *MedicalProfileV2) error {
	return s.createFn(ctx, mp)
}
func (s *medicalProfileRepoStub) GetByUserID(ctx context.Context, id uuid.UUID) (*MedicalProfileV2, error) {
	return s.getByUserIDFn(ctx, id)
}
func (s *medicalProfileRepoStub) Update(ctx context.Context, mp *MedicalProfileV2) error {
	return s.updateFn(ctx, mp)
}

var _ MedicalProfileRepository = (*medicalProfileRepoStub)(nil)

// medicalPendingUpdateRepoStub satisface MedicalPendingUpdateRepository
type medicalPendingUpdateRepoStub struct {
	createFn       func(context.Context, *MedicalPendingUpdate) error
	getByUserIDFn  func(context.Context, uuid.UUID) ([]*MedicalPendingUpdate, error)
	getByIDFn      func(context.Context, uuid.UUID) (*MedicalPendingUpdate, error)
	resolveFn      func(context.Context, uuid.UUID, MedicalPendingUpdateStatus) error
	countPendingFn func(context.Context, uuid.UUID) (int, error)
}

func (s *medicalPendingUpdateRepoStub) Create(ctx context.Context, pu *MedicalPendingUpdate) error {
	return s.createFn(ctx, pu)
}
func (s *medicalPendingUpdateRepoStub) GetByUserID(ctx context.Context, id uuid.UUID) ([]*MedicalPendingUpdate, error) {
	return s.getByUserIDFn(ctx, id)
}
func (s *medicalPendingUpdateRepoStub) GetByID(ctx context.Context, id uuid.UUID) (*MedicalPendingUpdate, error) {
	return s.getByIDFn(ctx, id)
}
func (s *medicalPendingUpdateRepoStub) Resolve(ctx context.Context, id uuid.UUID, status MedicalPendingUpdateStatus) error {
	return s.resolveFn(ctx, id, status)
}
func (s *medicalPendingUpdateRepoStub) CountPending(ctx context.Context, id uuid.UUID) (int, error) {
	return s.countPendingFn(ctx, id)
}

var _ MedicalPendingUpdateRepository = (*medicalPendingUpdateRepoStub)(nil)

// notificationPrefsRepoStub satisface NotificationPrefsRepository
type notificationPrefsRepoStub struct {
	createFn      func(context.Context, *NotificationPreference) error
	getByUserIDFn func(context.Context, uuid.UUID) ([]*NotificationPreference, error)
	upsertFn      func(context.Context, *NotificationPreference) error
	deleteFn      func(context.Context, uuid.UUID, NotificationChannel, NotificationType) error
}

func (s *notificationPrefsRepoStub) Create(ctx context.Context, np *NotificationPreference) error {
	return s.createFn(ctx, np)
}
func (s *notificationPrefsRepoStub) GetByUserID(ctx context.Context, id uuid.UUID) ([]*NotificationPreference, error) {
	return s.getByUserIDFn(ctx, id)
}
func (s *notificationPrefsRepoStub) Upsert(ctx context.Context, np *NotificationPreference) error {
	return s.upsertFn(ctx, np)
}
func (s *notificationPrefsRepoStub) Delete(ctx context.Context, userID uuid.UUID, ch NotificationChannel, nt NotificationType) error {
	return s.deleteFn(ctx, userID, ch, nt)
}

var _ NotificationPrefsRepository = (*notificationPrefsRepoStub)(nil)

// documentRepoStub satisface DocumentRepository
type documentRepoStub struct {
	createFn       func(context.Context, *UserDocument) error
	getByIDFn      func(context.Context, uuid.UUID) (*UserDocument, error)
	getByUserIDFn  func(context.Context, uuid.UUID) ([]*UserDocument, error)
	countByUserIDFn func(context.Context, uuid.UUID) (int, error)
	updateFn       func(context.Context, *UserDocument) error
	deleteFn       func(context.Context, uuid.UUID) error
	getTypesFn     func(context.Context) ([]DocumentType, error)
}

func (s *documentRepoStub) Create(ctx context.Context, d *UserDocument) error {
	return s.createFn(ctx, d)
}
func (s *documentRepoStub) GetByID(ctx context.Context, id uuid.UUID) (*UserDocument, error) {
	return s.getByIDFn(ctx, id)
}
func (s *documentRepoStub) GetByUserID(ctx context.Context, id uuid.UUID) ([]*UserDocument, error) {
	return s.getByUserIDFn(ctx, id)
}
func (s *documentRepoStub) CountByUserID(ctx context.Context, id uuid.UUID) (int, error) {
	return s.countByUserIDFn(ctx, id)
}
func (s *documentRepoStub) Update(ctx context.Context, d *UserDocument) error {
	return s.updateFn(ctx, d)
}
func (s *documentRepoStub) Delete(ctx context.Context, id uuid.UUID) error {
	return s.deleteFn(ctx, id)
}
func (s *documentRepoStub) GetTypes(ctx context.Context) ([]DocumentType, error) {
	return s.getTypesFn(ctx)
}

var _ DocumentRepository = (*documentRepoStub)(nil)

// savedSearchRepoStub satisface SavedSearchRepository
type savedSearchRepoStub struct {
	createFn      func(context.Context, *SavedSearch) error
	getByUserIDFn func(context.Context, uuid.UUID) ([]*SavedSearch, error)
	getByHashFn   func(context.Context, uuid.UUID, string) (*SavedSearch, error)
	getByIDFn     func(context.Context, uuid.UUID) (*SavedSearch, error)
	updateFn      func(context.Context, *SavedSearch) error
	deleteFn      func(context.Context, uuid.UUID) error
	setAlertEnabledFn func(context.Context, uuid.UUID, bool) error
}

func (s *savedSearchRepoStub) Create(ctx context.Context, ss *SavedSearch) error {
	return s.createFn(ctx, ss)
}
func (s *savedSearchRepoStub) GetByUserID(ctx context.Context, id uuid.UUID) ([]*SavedSearch, error) {
	return s.getByUserIDFn(ctx, id)
}
func (s *savedSearchRepoStub) GetByHash(ctx context.Context, userID uuid.UUID, hash string) (*SavedSearch, error) {
	return s.getByHashFn(ctx, userID, hash)
}
func (s *savedSearchRepoStub) GetByID(ctx context.Context, id uuid.UUID) (*SavedSearch, error) {
	return s.getByIDFn(ctx, id)
}
func (s *savedSearchRepoStub) Update(ctx context.Context, ss *SavedSearch) error {
	return s.updateFn(ctx, ss)
}
func (s *savedSearchRepoStub) Delete(ctx context.Context, id uuid.UUID) error {
	return s.deleteFn(ctx, id)
}
func (s *savedSearchRepoStub) SetAlertEnabled(ctx context.Context, id uuid.UUID, enabled bool) error {
	return s.setAlertEnabledFn(ctx, id, enabled)
}

var _ SavedSearchRepository = (*savedSearchRepoStub)(nil)

// favoriteRepoStub satisface FavoriteRepository
type favoriteRepoStub struct {
	createFn      func(context.Context, *Favorite) error
	getByUserIDFn func(context.Context, uuid.UUID) ([]*Favorite, error)
	deleteFn      func(context.Context, uuid.UUID) error
}

func (s *favoriteRepoStub) Create(ctx context.Context, f *Favorite) error {
	return s.createFn(ctx, f)
}
func (s *favoriteRepoStub) GetByUserID(ctx context.Context, id uuid.UUID) ([]*Favorite, error) {
	return s.getByUserIDFn(ctx, id)
}
func (s *favoriteRepoStub) Delete(ctx context.Context, id uuid.UUID) error {
	return s.deleteFn(ctx, id)
}

var _ FavoriteRepository = (*favoriteRepoStub)(nil)

// encryptionServiceStub satisface EncryptionService
type encryptionServiceStub struct {
	encryptFn func(string) ([]byte, error)
	decryptFn func([]byte) (string, error)
}

func (s *encryptionServiceStub) Encrypt(plaintext string) ([]byte, error) {
	return s.encryptFn(plaintext)
}
func (s *encryptionServiceStub) Decrypt(ciphertext []byte) (string, error) {
	return s.decryptFn(ciphertext)
}

var _ EncryptionService = (*encryptionServiceStub)(nil)

// TestRepositoryInterfaces_CompileTime verifies all interfaces have stubs
func TestRepositoryInterfaces_CompileTime(t *testing.T) {
	// Los compile-time checks con var _ son suficientes
	// Este test existe para asegurar que el archivo compila
	t.Log("Todas las interfaces de repositorio satisfacen compile-time checks")
}

// TestRepositorySignatures_ContextFirst verifies context.Context is first param
func TestRepositorySignatures_ContextFirst(t *testing.T) {
	ctx := context.Background()
	uid := uuid.Must(uuid.NewV7())

	// Stub simple para verificar que context es primer parámetro
	stub := &profileRepoStub{
		getByUserIDFn: func(c context.Context, id uuid.UUID) (*UserProfile, error) {
			if c != ctx {
				return nil, ErrProfileNotFound
			}
			return &UserProfile{UserID: id}, nil
		},
	}
	p, err := stub.GetByUserID(ctx, uid)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if p.UserID != uid {
		t.Errorf("UserID = %v, se esperaba %v", p.UserID, uid)
	}
}
