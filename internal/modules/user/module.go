// Módulo de usuario.
// Maneja perfiles de usuario y sus preferencias.
// Usa PostgreSQL para persistencia.
package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/user/adapters/encryption"
	deepseekocr "github.com/ProacTrip/Backend/internal/modules/user/adapters/ocr/deepseek"
	"github.com/ProacTrip/Backend/internal/modules/user/adapters/postgres"
	"github.com/ProacTrip/Backend/internal/modules/user/adapters/storage"
	"github.com/ProacTrip/Backend/internal/modules/user/consumer"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/modules/user/features/add_favorite"
	"github.com/ProacTrip/Backend/internal/modules/user/features/confirm_avatar_upload"
	"github.com/ProacTrip/Backend/internal/modules/user/features/create_saved_search"
	"github.com/ProacTrip/Backend/internal/modules/user/features/delete_document"
	"github.com/ProacTrip/Backend/internal/modules/user/features/delete_favorite"
	"github.com/ProacTrip/Backend/internal/modules/user/features/delete_saved_search"
	"github.com/ProacTrip/Backend/internal/modules/user/features/document_events"
	"github.com/ProacTrip/Backend/internal/modules/user/features/document_types"
	"github.com/ProacTrip/Backend/internal/modules/user/features/download_document"
	"github.com/ProacTrip/Backend/internal/modules/user/features/get_document"
	"github.com/ProacTrip/Backend/internal/modules/user/features/get_medical_profile"
	"github.com/ProacTrip/Backend/internal/modules/user/features/get_profile"
	"github.com/ProacTrip/Backend/internal/modules/user/features/list_documents"
	"github.com/ProacTrip/Backend/internal/modules/user/features/list_favorites"
	"github.com/ProacTrip/Backend/internal/modules/user/features/list_pending_medical"
	"github.com/ProacTrip/Backend/internal/modules/user/features/list_saved_searches"
	"github.com/ProacTrip/Backend/internal/modules/user/features/resolve_medical_pending"
	"github.com/ProacTrip/Backend/internal/modules/user/features/toggle_alert"
	"github.com/ProacTrip/Backend/internal/modules/user/features/update_locale"
	"github.com/ProacTrip/Backend/internal/modules/user/features/update_medical_profile"
	"github.com/ProacTrip/Backend/internal/modules/user/features/update_notif_prefs"
	"github.com/ProacTrip/Backend/internal/modules/user/features/update_profile"
	"github.com/ProacTrip/Backend/internal/modules/user/features/update_saved_search"
	"github.com/ProacTrip/Backend/internal/modules/user/features/update_travel_prefs"
	"github.com/ProacTrip/Backend/internal/modules/user/features/upload_avatar"
	"github.com/ProacTrip/Backend/internal/modules/user/features/upload_document"
	"github.com/ProacTrip/Backend/internal/modules/user/features/upsert_profile"
	"github.com/ProacTrip/Backend/internal/modules/user/features/verify_document"
	"github.com/ProacTrip/Backend/internal/modules/user/pipeline"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
	"github.com/ProacTrip/Backend/internal/shared/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Module contiene las dependencias del módulo User
type Module struct {
	// Repositorios (implementaciones de dominio)
	ProfileRepo        domain.ProfileRepository
	TravelPrefsRepo    domain.TravelPrefsRepository
	NotifPrefsRepo     domain.NotificationPrefsRepository
	MedicalProfileRepo domain.MedicalProfileRepository
	MedicalPendingRepo domain.MedicalPendingUpdateRepository

	// Servicios
	EncryptionService *encryption.Service
	R2Storage         *storage.R2Storage
	RedisClient       *redis.Client

	// Use Cases (legacy)
	UpsertProfileUseCase *upsert_profile.UseCase

	// Handlers (Phase 2)
	GetProfileHandler        *get_profile.Handler
	UpdateProfileHandler     *update_profile.Handler
	UpdateLocaleHandler      *update_locale.Handler
	UpdateTravelPrefsHandler *update_travel_prefs.Handler
	UpdateNotifPrefsHandler  *update_notif_prefs.Handler

	// Handlers (Phase 3 — Medical)
	GetMedicalProfileHandler     *get_medical_profile.Handler
	UpdateMedicalProfileHandler  *update_medical_profile.Handler
	ListPendingMedicalHandler    *list_pending_medical.Handler
	ResolveMedicalPendingHandler *resolve_medical_pending.Handler

	// Handlers (Phase 4 — Avatar)
	UploadAvatarHandler       *upload_avatar.Handler
	ConfirmAvatarUploadHandler *confirm_avatar_upload.Handler

	// Repositorios (Phase 5 — Documentos)
	DocumentRepo *postgres.DocumentRepository

	// Repositorios (Phase 6 — Búsquedas y Favoritos)
	SavedSearchRepo *postgres.SavedSearchRepository
	FavoriteRepo    *postgres.FavoriteRepository

	// Servicio OCR (Phase 5)
	OCRService domain.OCRService

	// Handlers (Phase 5 — Documentos)
	UploadDocumentHandler    *upload_document.Handler
	DocumentTypesHandler     *document_types.Handler
	ListDocumentsHandler     *list_documents.Handler
	GetDocumentHandler       *get_document.Handler
	DownloadDocumentHandler  *download_document.Handler
	DeleteDocumentHandler    *delete_document.Handler
	VerifyDocumentHandler    *verify_document.Handler
	DocumentEventsHandler    *document_events.Handler

	// Handlers (Phase 6 — Búsquedas Guardadas y Favoritos)
	CreateSavedSearchHandler *create_saved_search.Handler
	ListSavedSearchesHandler *list_saved_searches.Handler
	UpdateSavedSearchHandler *update_saved_search.Handler
	DeleteSavedSearchHandler *delete_saved_search.Handler
	ToggleAlertHandler       *toggle_alert.Handler
	AddFavoriteHandler       *add_favorite.Handler
	ListFavoritesHandler     *list_favorites.Handler
	DeleteFavoriteHandler    *delete_favorite.Handler

	// Pipelines (Phase 4 — Avatar)
	AvatarValidator *pipeline.AvatarValidator

	// Pipelines (Phase 5 — Documentos)
	ValidatorWorker  *pipeline.ValidatorWorker
	SanitizerWorker  *pipeline.SanitizerWorker
	OCRWorker        *pipeline.OCRWorker

	// Event Consumer
	EventConsumer *consumer.UserEventConsumer

	// Lifecycle
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// Config configuración del módulo
type Config struct {
	PostgresPool  *pgxpool.Pool
	RedisClient   *redis.Client
	EventBus      *eventbus.EventBus
	EncryptionKey []byte // 32 bytes para ChaCha20-Poly1305
	R2Storage     *storage.R2Storage
	OCRAPIKey     string // API key para DeepSeek OCR (opcional: sin key, OCR no disponible)
}

// NewModule crea e inicializa el módulo User con todas sus dependencias.
func NewModule(cfg Config) (*Module, error) {
	m := &Module{}

	// 0. Run pending migrations BEFORE any DB operation
	if err := runMigrations(context.Background(), cfg.PostgresPool); err != nil {
		return nil, fmt.Errorf("user migrations: %w", err)
	}

	// 1. Inicializar Repository (PostgreSQL adapter) — implementa ProfileRepository
	profileRepo := postgres.NewUserRepository(cfg.PostgresPool)
	m.ProfileRepo = profileRepo

	// 2. Inicializar repos de apoyo
	m.TravelPrefsRepo = postgres.NewTravelPrefsRepository(cfg.PostgresPool)
	m.NotifPrefsRepo = postgres.NewNotificationPrefsRepository(cfg.PostgresPool)

	// 3. Inicializar repositorios médicos (Phase 3)
	m.MedicalProfileRepo = postgres.NewMedicalProfileRepository(cfg.PostgresPool)
	m.MedicalPendingRepo = postgres.NewMedicalPendingUpdateRepository(cfg.PostgresPool)

	// 4. Inicializar servicio de encriptación (Phase 3)
	if len(cfg.EncryptionKey) > 0 {
		encSvc, err := encryption.NewService(cfg.EncryptionKey)
		if err != nil {
			return nil, err
		}
		m.EncryptionService = encSvc
	}

	// 5. Inicializar R2 Storage (Phase 4 — Avatar)
	m.R2Storage = cfg.R2Storage

	// Guardar Redis client para Start()
	m.RedisClient = cfg.RedisClient

	// 6. Inicializar Use Case legacy (with Dragonfly cache for profile prefs)
	m.UpsertProfileUseCase = upsert_profile.NewUseCaseWithCache(profileRepo, cfg.RedisClient)

	// 7. Inicializar Event Consumer (Dragonfly Streams)
	// Creates all 4 profile rows (user_profiles + travel/medical/notif defaults)
	// when UserRegistered event arrives.
	m.EventConsumer = consumer.NewUserEventConsumer(
		cfg.RedisClient, profileRepo,
		m.TravelPrefsRepo, m.MedicalProfileRepo, m.NotifPrefsRepo,
	)

	// 8. Inicializar Features (Phase 2)
	getProfileUC := get_profile.NewUseCase(get_profile.UseCaseDeps{
		ProfileRepo:    m.ProfileRepo,
		TravelPrefsRepo: m.TravelPrefsRepo,
		NotifPrefsRepo:  m.NotifPrefsRepo,
	})
	m.GetProfileHandler = get_profile.NewHandler(getProfileUC)

	updateProfileUC := update_profile.NewUseCase(update_profile.UseCaseDeps{
		ProfileRepo:    m.ProfileRepo,
		EventPublisher: cfg.EventBus,
	})
	m.UpdateProfileHandler = update_profile.NewHandler(updateProfileUC)

	updateLocaleUC := update_locale.NewUseCase(update_locale.UseCaseDeps{
		ProfileRepo:    m.ProfileRepo,
		EventPublisher: cfg.EventBus,
		RedisClient:    cfg.RedisClient,
	})
	m.UpdateLocaleHandler = update_locale.NewHandler(updateLocaleUC)

	updateTravelPrefsUC := update_travel_prefs.NewUseCase(update_travel_prefs.UseCaseDeps{
		TravelPrefsRepo: m.TravelPrefsRepo,
		EventPublisher:  cfg.EventBus,
	})
	m.UpdateTravelPrefsHandler = update_travel_prefs.NewHandler(updateTravelPrefsUC)

	updateNotifPrefsUC := update_notif_prefs.NewUseCase(update_notif_prefs.UseCaseDeps{
		NotifPrefsRepo: m.NotifPrefsRepo,
		EventPublisher: cfg.EventBus,
	})
	m.UpdateNotifPrefsHandler = update_notif_prefs.NewHandler(updateNotifPrefsUC)

	// 9. Inicializar Features Médicas (Phase 3)
	if m.EncryptionService != nil {
		getMedicalProfileUC := get_medical_profile.NewUseCase(get_medical_profile.UseCaseDeps{
			MedicalProfileRepo: m.MedicalProfileRepo,
			EncryptionService:  m.EncryptionService,
			MedicalPendingRepo: m.MedicalPendingRepo,
		})
		m.GetMedicalProfileHandler = get_medical_profile.NewHandler(getMedicalProfileUC)

		updateMedicalProfileUC := update_medical_profile.NewUseCase(update_medical_profile.UseCaseDeps{
			MedicalProfileRepo: m.MedicalProfileRepo,
			EncryptionService:  m.EncryptionService,
			EventPublisher:     cfg.EventBus,
		})
		m.UpdateMedicalProfileHandler = update_medical_profile.NewHandler(updateMedicalProfileUC)

		resolveMedicalPendingUC := resolve_medical_pending.NewUseCase(resolve_medical_pending.UseCaseDeps{
			MedicalPendingRepo: m.MedicalPendingRepo,
			MedicalProfileRepo: m.MedicalProfileRepo,
			EncryptionService:  m.EncryptionService,
			EventPublisher:     cfg.EventBus,
		})
		m.ResolveMedicalPendingHandler = resolve_medical_pending.NewHandler(resolveMedicalPendingUC)
	}

	listPendingMedicalUC := list_pending_medical.NewUseCase(list_pending_medical.UseCaseDeps{
		MedicalPendingRepo: m.MedicalPendingRepo,
	})
	m.ListPendingMedicalHandler = list_pending_medical.NewHandler(listPendingMedicalUC)

	// 10. Inicializar Features de Avatar (Phase 4)
	if m.R2Storage != nil && cfg.EventBus != nil {
		// Upload Avatar — genera URL prefirmada
		uploadUC := upload_avatar.NewUseCase(upload_avatar.UseCaseDeps{
			Storage: m.R2Storage,
		})
		m.UploadAvatarHandler = upload_avatar.NewHandler(uploadUC)

		// Confirm Avatar — verifica existencia y publica evento de validación
		confirmUC := confirm_avatar_upload.NewUseCase(confirm_avatar_upload.UseCaseDeps{
			Storage:        m.R2Storage,
			EventPublisher: cfg.EventBus,
		})
		m.ConfirmAvatarUploadHandler = confirm_avatar_upload.NewHandler(confirmUC)

		// Avatar Validator — consumer de Dragonfly Streams
		m.AvatarValidator = pipeline.NewAvatarValidator(cfg.RedisClient, m.ProfileRepo)
	}

	// 11. Inicializar repositorio de documentos (Phase 5)
	docRepo := postgres.NewDocumentRepository(cfg.PostgresPool)
	m.DocumentRepo = docRepo

	// 12. Inicializar OCR service (Phase 5)
	if cfg.OCRAPIKey != "" {
		m.OCRService = deepseekocr.NewOCRClient(cfg.OCRAPIKey)
	}

	// 13. Inicializar Features de Documentos (Phase 5)
	if m.R2Storage != nil && cfg.EventBus != nil {
		// Upload Document — subida multipart con magic bytes y pipeline
		uploadDocUC := upload_document.NewUseCase(upload_document.UseCaseDeps{
			DocRepo:             docRepo,
			Storage:             m.R2Storage,
			EventPublisher:      cfg.EventBus,
			Dragonfly:           cfg.RedisClient,
			MaxDocumentsPerUser: upload_document.MaxDocumentsPerUser(),
			RateLimitMax:        upload_document.RateLimitMax(),
			RateLimitWindowSecs: upload_document.RateLimitWindowSecs(),
		})
		m.UploadDocumentHandler = upload_document.NewHandler(uploadDocUC)
	}

	// Document Types — catálogo público (siempre disponible)
	m.DocumentTypesHandler = document_types.NewHandler(docRepo)

	// List Documents
	m.ListDocumentsHandler = list_documents.NewHandler(docRepo)

	// Get Document
	m.GetDocumentHandler = get_document.NewHandler(docRepo)

	// Download Document
	if m.R2Storage != nil {
		m.DownloadDocumentHandler = download_document.NewHandler(docRepo, m.R2Storage)
	}

	// Delete Document
	if m.R2Storage != nil {
		m.DeleteDocumentHandler = delete_document.NewHandler(docRepo, m.R2Storage)
	}

	// Verify Document (admin only)
	m.VerifyDocumentHandler = verify_document.NewHandler(docRepo, cfg.RedisClient)

	// Document Events (SSE)
	m.DocumentEventsHandler = document_events.NewHandler(docRepo, cfg.RedisClient)

	// 14. Inicializar Repositorios de Búsquedas y Favoritos (Phase 6)
	searchRepo := postgres.NewSavedSearchRepository(cfg.PostgresPool)
	m.SavedSearchRepo = searchRepo

	favRepo := postgres.NewFavoriteRepository(cfg.PostgresPool)
	m.FavoriteRepo = favRepo

	// 15. Inicializar Features de Búsquedas Guardadas (Phase 6)
	createSavedSearchUC := create_saved_search.NewUseCase(create_saved_search.UseCaseDeps{
		SavedSearchRepo: searchRepo,
	})
	m.CreateSavedSearchHandler = create_saved_search.NewHandler(createSavedSearchUC)

	listSavedSearchesUC := list_saved_searches.NewUseCase(list_saved_searches.UseCaseDeps{
		SavedSearchRepo: searchRepo,
	})
	m.ListSavedSearchesHandler = list_saved_searches.NewHandler(listSavedSearchesUC)

	updateSavedSearchUC := update_saved_search.NewUseCase(update_saved_search.UseCaseDeps{
		SavedSearchRepo: searchRepo,
	})
	m.UpdateSavedSearchHandler = update_saved_search.NewHandler(updateSavedSearchUC)

	deleteSavedSearchUC := delete_saved_search.NewUseCase(delete_saved_search.UseCaseDeps{
		SavedSearchRepo: searchRepo,
	})
	m.DeleteSavedSearchHandler = delete_saved_search.NewHandler(deleteSavedSearchUC)

	toggleAlertUC := toggle_alert.NewUseCase(toggle_alert.UseCaseDeps{
		SavedSearchRepo: searchRepo,
	})
	m.ToggleAlertHandler = toggle_alert.NewHandler(toggleAlertUC)

	// 16. Inicializar Features de Favoritos (Phase 6)
	addFavoriteUC := add_favorite.NewUseCase(add_favorite.UseCaseDeps{
		FavoriteRepo: favRepo,
	})
	m.AddFavoriteHandler = add_favorite.NewHandler(addFavoriteUC)

	listFavoritesUC := list_favorites.NewUseCase(list_favorites.UseCaseDeps{
		FavoriteRepo: favRepo,
	})
	m.ListFavoritesHandler = list_favorites.NewHandler(listFavoritesUC)

	deleteFavoriteUC := delete_favorite.NewUseCase(delete_favorite.UseCaseDeps{
		FavoriteRepo: favRepo,
	})
	m.DeleteFavoriteHandler = delete_favorite.NewHandler(deleteFavoriteUC)

	// 17. Inicializar Document Pipelines (Phase 5)
	if cfg.RedisClient != nil && docRepo != nil {
		// Validator Worker
		m.ValidatorWorker = pipeline.NewValidatorWorker(cfg.RedisClient, docRepo, m.R2Storage)

		// Sanitizer Worker
		if m.R2Storage != nil {
			m.SanitizerWorker = pipeline.NewSanitizerWorker(cfg.RedisClient, m.R2Storage, docRepo)
		}

		// OCR Worker
		if m.OCRService != nil && m.R2Storage != nil && m.EncryptionService != nil {
			m.OCRWorker = pipeline.NewOCRWorker(
				cfg.RedisClient,
				m.R2Storage,
				m.OCRService,
				docRepo,
				m.MedicalProfileRepo,
				m.MedicalPendingRepo,
				m.EncryptionService,
			)
		}
	}

	// 18. Registrar mapeos de errores de dominio
	registerUserErrorMappings()

	slog.Info("User module initialized",
		"features", []string{
			"upsert_profile", "get_profile", "update_profile", "update_locale",
			"update_travel_prefs", "update_notif_prefs",
			"get_medical_profile", "update_medical_profile",
			"list_pending_medical", "resolve_medical_pending",
			"upload_avatar", "confirm_avatar_upload",
			"upload_document", "document_types", "list_documents",
			"get_document", "download_document", "delete_document",
			"verify_document", "document_events",
			"create_saved_search", "list_saved_searches", "update_saved_search",
			"delete_saved_search", "toggle_alert",
			"add_favorite", "list_favorites", "delete_favorite",
		},
		"consumer", "user-event-consumer",
		"pipelines", []string{
			"avatar-validator", "doc-validator", "doc-sanitizer", "doc-ocr",
		},
	)

	return m, nil
}

// RegisterRoutes registra las rutas del módulo en el grupo proporcionado.
// authMW es el middleware de autenticación (cookie-based).
func (m *Module) RegisterRoutes(g *echo.Group, authMW echo.MiddlewareFunc) {
	// Phase 2
	g.GET("/profile", m.GetProfileHandler.Handle, authMW)
	g.PUT("/profile", m.UpdateProfileHandler.Handle, authMW)
	g.PUT("/profile/locale", m.UpdateLocaleHandler.Handle, authMW)
	g.PUT("/profile/travel-preferences", m.UpdateTravelPrefsHandler.Handle, authMW)
	g.PUT("/profile/notifications", m.UpdateNotifPrefsHandler.Handle, authMW)

	// Phase 3 — Medical (solo si EncryptionService está disponible)
	if m.GetMedicalProfileHandler != nil {
		g.GET("/profile/medical", m.GetMedicalProfileHandler.Handle, authMW)
		g.PUT("/profile/medical", m.UpdateMedicalProfileHandler.Handle, authMW)
		g.GET("/profile/medical/pending", m.ListPendingMedicalHandler.Handle, authMW)
		g.POST("/profile/medical/pending/resolve", m.ResolveMedicalPendingHandler.Handle, authMW)
	}

	// Phase 4 — Avatar
	if m.UploadAvatarHandler != nil {
		g.POST("/profile/avatar", m.UploadAvatarHandler.Handle, authMW)
	}
	if m.ConfirmAvatarUploadHandler != nil {
		g.POST("/profile/avatar/confirm", m.ConfirmAvatarUploadHandler.Handle, authMW)
	}

	// Phase 5 — Documentos
	// GET /documents/types — público, sin auth
	if m.DocumentTypesHandler != nil {
		g.GET("/documents/types", m.DocumentTypesHandler.Handle)
	}
	// Resto de endpoints de documentos requieren auth
	if m.UploadDocumentHandler != nil {
		g.POST("/documents", m.UploadDocumentHandler.Handle, authMW)
	}
	if m.ListDocumentsHandler != nil {
		g.GET("/documents", m.ListDocumentsHandler.Handle, authMW)
	}
	if m.GetDocumentHandler != nil {
		g.GET("/documents/:document_id", m.GetDocumentHandler.Handle, authMW)
	}
	if m.DownloadDocumentHandler != nil {
		g.GET("/documents/:document_id/download", m.DownloadDocumentHandler.Handle, authMW)
	}
	if m.DeleteDocumentHandler != nil {
		g.DELETE("/documents/:document_id", m.DeleteDocumentHandler.Handle, authMW)
	}
	if m.VerifyDocumentHandler != nil {
		g.PUT("/documents/:document_id/verify", m.VerifyDocumentHandler.Handle, authMW, middleware.RequireAdmin())
	}
	if m.DocumentEventsHandler != nil {
		g.GET("/documents/:document_id/events", m.DocumentEventsHandler.Handle, authMW)
	}

	// Phase 6 — Búsquedas Guardadas y Favoritos
	if m.CreateSavedSearchHandler != nil {
		g.POST("/saved-searches", m.CreateSavedSearchHandler.Handle, authMW)
	}
	if m.ListSavedSearchesHandler != nil {
		g.GET("/saved-searches", m.ListSavedSearchesHandler.Handle, authMW)
	}
	if m.UpdateSavedSearchHandler != nil {
		g.PUT("/saved-searches/:search_id", m.UpdateSavedSearchHandler.Handle, authMW)
	}
	if m.DeleteSavedSearchHandler != nil {
		g.DELETE("/saved-searches/:search_id", m.DeleteSavedSearchHandler.Handle, authMW)
	}
	if m.ToggleAlertHandler != nil {
		g.PUT("/saved-searches/:search_id/alert", m.ToggleAlertHandler.Handle, authMW)
	}
	if m.AddFavoriteHandler != nil {
		g.POST("/favorites", m.AddFavoriteHandler.Handle, authMW)
	}
	if m.ListFavoritesHandler != nil {
		g.GET("/favorites", m.ListFavoritesHandler.Handle, authMW)
	}
	if m.DeleteFavoriteHandler != nil {
		g.DELETE("/favorites/:favorite_id", m.DeleteFavoriteHandler.Handle, authMW)
	}
}

// Start inicia los pipelines asíncronos del módulo.
// Debe ser llamado después de que el módulo esté completamente inicializado.
func (m *Module) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	var errs []error

	// Ensure DLQ streams exist before starting workers
	if m.RedisClient != nil {
		if err := pipeline.EnsureDLQStreams(m.ctx, m.RedisClient); err != nil {
			errs = append(errs, fmt.Errorf("ensure DLQ streams: %w", err))
		}
	}

	if m.AvatarValidator != nil {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			if err := m.AvatarValidator.Run(m.ctx); err != nil {
				slog.Error("avatar validator failed", "error", err)
			}
		}()
	}
	if m.ValidatorWorker != nil {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			if err := m.ValidatorWorker.Run(m.ctx); err != nil {
				slog.Error("validator worker failed", "error", err)
			}
		}()
	}
	if m.SanitizerWorker != nil {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			if err := m.SanitizerWorker.Run(m.ctx); err != nil {
				slog.Error("sanitizer worker failed", "error", err)
			}
		}()
	}
	if m.OCRWorker != nil {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			if err := m.OCRWorker.Run(m.ctx); err != nil {
				slog.Error("OCR worker failed", "error", err)
			}
		}()
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Shutdown detiene gracefulmente los pipelines asíncronos.
// Cancela el contexto, espera a que todos los workers drenen
// con un timeout pasado vía ctx.
func (m *Module) Shutdown(ctx context.Context) error {
	if m.cancel == nil {
		return nil
	}
	m.cancel()

	// Poll IsRunning() — Run() is non-blocking so wg.Wait() covers
	// only setup, not the internal consume/rescueOrphans goroutines.
	deadline, hasDeadline := ctx.Deadline()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for !m.allWorkersStopped() {
		if hasDeadline && time.Now().After(deadline) {
			return context.DeadlineExceeded
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (m *Module) allWorkersStopped() bool {
	if m.AvatarValidator != nil && m.AvatarValidator.IsRunning() {
		return false
	}
	if m.ValidatorWorker != nil && m.ValidatorWorker.IsRunning() {
		return false
	}
	if m.SanitizerWorker != nil && m.SanitizerWorker.IsRunning() {
		return false
	}
	if m.OCRWorker != nil && m.OCRWorker.IsRunning() {
		return false
	}
	return true
}

// registerUserErrorMappings registra los mapeos de errores de dominio user
// a respuestas HTTP RFC 9457.
func registerUserErrorMappings() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		case errors.Is(err, domain.ErrProfileNotFound):
			return serrors.ErrNotFound("Perfil de usuario no encontrado", err)
		case errors.Is(err, domain.ErrMedicalProfileNotFound):
			return serrors.ErrNotFound("Perfil médico no encontrado", err)

		case errors.Is(err, domain.ErrInvalidGender):
			return serrors.ErrBadRequest("Género inválido. Valores permitidos: male, female, non_binary, prefer_not_to_say", err)
		case errors.Is(err, domain.ErrInvalidCountryCode):
			return serrors.ErrBadRequest("Código de país inválido. Debe ser ISO 3166-1 alpha-2 (2 letras)", err)
		case errors.Is(err, domain.ErrInvalidLanguageCode):
			return serrors.ErrBadRequest("Código de idioma inválido. Debe tener entre 2 y 5 caracteres", err)
		case errors.Is(err, domain.ErrInvalidCurrencyCode):
			return serrors.ErrBadRequest("Código de moneda inválido. Debe ser ISO 4217 (3 letras)", err)
		case errors.Is(err, domain.ErrInvalidTimezone):
			return serrors.ErrBadRequest("Zona horaria inválida. Formato IANA: Area/City", err)
		case errors.Is(err, domain.ErrInvalidPreferredClass):
			return serrors.ErrBadRequest("Clase de cabina inválida. Valores permitidos: economy, premium_economy, business, first", err)
		case errors.Is(err, domain.ErrInvalidSeatPreference):
			return serrors.ErrBadRequest("Preferencia de asiento inválida. Valores permitidos: window, aisle, middle, no_preference", err)
		case errors.Is(err, domain.ErrInvalidChannel):
			return serrors.ErrBadRequest("Canal de notificación inválido. Valores permitidos: email, sms, websocket", err)
		case errors.Is(err, domain.ErrInvalidNotificationType):
			return serrors.ErrBadRequest("Tipo de notificación inválido. Valores permitidos: price_alert, booking_confirm, travel_reminder, promo_offer, booking_confirmation, flight_reminder, promotional", err)
		case errors.Is(err, domain.ErrInvalidBloodType):
			return serrors.ErrBadRequest("Tipo de sangre inválido", err)

		case errors.Is(err, domain.ErrEncryptionFailed), errors.Is(err, domain.ErrDecryptionFailed):
			return serrors.ErrInternalError("Error interno de encriptación", err)

		case errors.Is(err, domain.ErrInvalidMimeType):
			return serrors.ErrBadRequest("Tipo MIME no permitido. Valores permitidos: image/jpeg, image/png, image/webp", err)
		case errors.Is(err, domain.ErrAvatarNotFound):
			return serrors.ErrNotFound("Archivo de avatar no encontrado en R2", err)

		case errors.Is(err, domain.ErrDocumentNotFound):
			return serrors.ErrNotFound("Documento no encontrado", err)
		case errors.Is(err, domain.ErrInvalidDocumentType):
			return serrors.ErrBadRequest("Tipo de documento inválido", err)
		case errors.Is(err, domain.ErrInvalidFileType):
			return serrors.ErrBadRequest("Tipo de archivo no permitido", err)
		case errors.Is(err, domain.ErrFileTooLarge):
			return serrors.ErrPayloadTooLarge(err.Error(), err)
		case errors.Is(err, domain.ErrDocumentNotReady):
			return serrors.ErrBadRequest("El documento aún no está listo para procesar", err)
		case errors.Is(err, domain.ErrMaxDocumentsReached):
			return serrors.ErrBadRequest("Se alcanzó el límite máximo de documentos", err)
		case errors.Is(err, domain.ErrDuplicateDocument):
			return serrors.ErrConflict("El documento ya fue subido previamente", err)
		case errors.Is(err, domain.ErrRateLimitExceeded):
			return serrors.ErrTooManyRequests("Límite de subidas por minuto excedido. Intente nuevamente en 60 segundos.", err)

		case errors.Is(err, domain.ErrSearchNotFound):
			return serrors.ErrNotFound("Búsqueda guardada no encontrada", err)
		case errors.Is(err, domain.ErrDuplicateSavedSearch):
			return serrors.ErrConflict("Ya existe una búsqueda idéntica guardada", err)

		case errors.Is(err, domain.ErrFavoriteNotFound):
			return serrors.ErrNotFound("Favorito no encontrado", err)
		case errors.Is(err, domain.ErrDuplicateFavorite):
			return serrors.ErrConflict("El favorito ya existe para esta entidad", err)
		case errors.Is(err, domain.ErrInvalidFavoriteEntityType):
			return serrors.ErrBadRequest("Tipo de entidad favorita inválido. Valores permitidos: hotel, flight, airport, airline, hotel_chain, country, destination, activity", err)

		case errors.Is(err, domain.ErrPendingUpdateNotFound):
			return serrors.ErrNotFound("Actualización pendiente no encontrada", err)
		case errors.Is(err, domain.ErrPendingUpdateExpired):
			return serrors.ErrBadRequest("La actualización pendiente ha expirado", err)
		case errors.Is(err, domain.ErrInvalidPendingAction):
			return serrors.ErrBadRequest("Acción no válida para la actualización pendiente", err)

		case errors.Is(err, domain.ErrPermissionDenied):
			return serrors.ErrForbidden("Permiso denegado para esta acción administrativa", err)

		default:
			return nil
		}
	})
}
