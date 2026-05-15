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
	"github.com/ProacTrip/Backend/internal/modules/user/adapters/hash"
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
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Module contiene las dependencias del módulo User
type Module struct {
	// Repositorios (implementaciones de dominio)
	profileRepo        domain.ProfileRepository
	travelPrefsRepo    domain.TravelPrefsRepository
	notifPrefsRepo     domain.NotificationPrefsRepository
	medicalProfileRepo domain.MedicalProfileRepository
	medicalPendingRepo domain.MedicalPendingUpdateRepository

	// Servicios
	encryptionService *encryption.Service
	r2Storage         *storage.R2Storage
	redisClient       *redis.Client

	// Use Cases (legacy)
	upsertProfileUseCase *upsert_profile.UseCase

	// Handlers (Phase 2)
	getProfileHandler        *get_profile.Handler
	updateProfileHandler     *update_profile.Handler
	updateLocaleHandler      *update_locale.Handler
	updateTravelPrefsHandler *update_travel_prefs.Handler
	updateNotifPrefsHandler  *update_notif_prefs.Handler

	// Handlers (Phase 3 — Medical)
	getMedicalProfileHandler     *get_medical_profile.Handler
	updateMedicalProfileHandler  *update_medical_profile.Handler
	listPendingMedicalHandler    *list_pending_medical.Handler
	resolveMedicalPendingHandler *resolve_medical_pending.Handler

	// Handlers (Phase 4 — Avatar)
	uploadAvatarHandler       *upload_avatar.Handler
	confirmAvatarUploadHandler *confirm_avatar_upload.Handler

	// Repositorios (Phase 5 — Documentos)
	documentRepo *postgres.DocumentRepository

	// Repositorios (Phase 6 — Búsquedas y Favoritos)
	savedSearchRepo *postgres.SavedSearchRepository
	favoriteRepo    *postgres.FavoriteRepository

	// Servicio OCR (Phase 5)
	ocrService domain.OCRService

	// Handlers (Phase 5 — Documentos)
	uploadDocumentHandler    *upload_document.Handler
	documentTypesHandler     *document_types.Handler
	listDocumentsHandler     *list_documents.Handler
	getDocumentHandler       *get_document.Handler
	downloadDocumentHandler  *download_document.Handler
	deleteDocumentHandler    *delete_document.Handler
	verifyDocumentHandler    *verify_document.Handler
	documentEventsHandler    *document_events.Handler

	// Handlers (Phase 6 — Búsquedas Guardadas y Favoritos)
	createSavedSearchHandler *create_saved_search.Handler
	listSavedSearchesHandler *list_saved_searches.Handler
	updateSavedSearchHandler *update_saved_search.Handler
	deleteSavedSearchHandler *delete_saved_search.Handler
	toggleAlertHandler       *toggle_alert.Handler
	addFavoriteHandler       *add_favorite.Handler
	listFavoritesHandler     *list_favorites.Handler
	deleteFavoriteHandler    *delete_favorite.Handler

	// Pipelines (Phase 4 — Avatar)
	avatarValidator *pipeline.AvatarValidator

	// Pipelines (Phase 5 — Documentos)
	validatorWorker  *pipeline.ValidatorWorker
	sanitizerWorker  *pipeline.SanitizerWorker
	ocrWorker        *pipeline.OCRWorker

	// Event Consumer
	eventConsumer *consumer.UserEventConsumer

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
	RateLimiter   *ratelimit.RateLimiter // opcional — rate limiting para uploads
}

// SavedSearchRepo expone el repositorio de búsquedas guardadas.
// Accedido por user/adapters/search_resolver.go vía bootstrap.
func (m *Module) SavedSearchRepo() *postgres.SavedSearchRepository {
	return m.savedSearchRepo
}

// EventConsumer expone el consumer de eventos de usuario.
// Accedido por bootstrap para iniciar el consumer.
func (m *Module) EventConsumer() *consumer.UserEventConsumer {
	return m.eventConsumer
}

// NewModule crea e inicializa el módulo User con todas sus dependencias.
func NewModule(cfg Config) (*Module, error) {
	m := &Module{}

	// 0. Run pending migrations BEFORE any DB operation
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := runMigrations(ctx, cfg.PostgresPool); err != nil {
		return nil, fmt.Errorf("user migrations: %w", err)
	}

	// 1. Inicializar Repository (PostgreSQL adapter) — implementa ProfileRepository
	profileRepo := postgres.NewUserRepository(cfg.PostgresPool)
	m.profileRepo = profileRepo

	// 2. Inicializar repos de apoyo
	m.travelPrefsRepo = postgres.NewTravelPrefsRepository(cfg.PostgresPool)
	m.notifPrefsRepo = postgres.NewNotificationPrefsRepository(cfg.PostgresPool)

	// 3. Inicializar repositorios médicos (Phase 3)
	m.medicalProfileRepo = postgres.NewMedicalProfileRepository(cfg.PostgresPool)
	m.medicalPendingRepo = postgres.NewMedicalPendingUpdateRepository(cfg.PostgresPool)

	// 4. Inicializar servicio de encriptación (Phase 3)
	if len(cfg.EncryptionKey) > 0 {
		encSvc, err := encryption.NewService(cfg.EncryptionKey)
		if err != nil {
			return nil, err
		}
		m.encryptionService = encSvc
	}

	// 5. Inicializar R2 Storage (Phase 4 — Avatar)
	m.r2Storage = cfg.R2Storage

	// Guardar Redis client para Start()
	m.redisClient = cfg.RedisClient

	// 6. Inicializar Use Case legacy (with Dragonfly cache for profile prefs)
	m.upsertProfileUseCase = upsert_profile.NewUseCaseWithCache(profileRepo, cfg.RedisClient)

	// 7. Inicializar Event Consumer (Dragonfly Streams)
	// Creates all 4 profile rows (user_profiles + travel/medical/notif defaults)
	// when UserRegistered event arrives.
	m.eventConsumer = consumer.NewUserEventConsumer(
		cfg.RedisClient, profileRepo,
		m.travelPrefsRepo, m.medicalProfileRepo, m.notifPrefsRepo,
	)

	// 8. Inicializar Features (Phase 2)
	getProfileUC := get_profile.NewUseCase(get_profile.UseCaseDeps{
		ProfileRepo:    m.profileRepo,
		TravelPrefsRepo: m.travelPrefsRepo,
		NotifPrefsRepo:  m.notifPrefsRepo,
	})
	m.getProfileHandler = get_profile.NewHandler(getProfileUC)

	updateProfileUC := update_profile.NewUseCase(update_profile.UseCaseDeps{
		ProfileRepo:    m.profileRepo,
		EventPublisher: cfg.EventBus,
	})
	m.updateProfileHandler = update_profile.NewHandler(updateProfileUC)

	updateLocaleUC := update_locale.NewUseCase(update_locale.UseCaseDeps{
		ProfileRepo:    m.profileRepo,
		EventPublisher: cfg.EventBus,
		RedisClient:    cfg.RedisClient,
	})
	m.updateLocaleHandler = update_locale.NewHandler(updateLocaleUC)

	updateTravelPrefsUC := update_travel_prefs.NewUseCase(update_travel_prefs.UseCaseDeps{
		TravelPrefsRepo: m.travelPrefsRepo,
		EventPublisher:  cfg.EventBus,
	})
	m.updateTravelPrefsHandler = update_travel_prefs.NewHandler(updateTravelPrefsUC)

	updateNotifPrefsUC := update_notif_prefs.NewUseCase(update_notif_prefs.UseCaseDeps{
		NotifPrefsRepo: m.notifPrefsRepo,
		EventPublisher: cfg.EventBus,
	})
	m.updateNotifPrefsHandler = update_notif_prefs.NewHandler(updateNotifPrefsUC)

	// 9. Inicializar Features Médicas (Phase 3)
	if m.encryptionService != nil {
		getMedicalProfileUC := get_medical_profile.NewUseCase(get_medical_profile.UseCaseDeps{
			MedicalProfileRepo: m.medicalProfileRepo,
			EncryptionService:  m.encryptionService,
			MedicalPendingRepo: m.medicalPendingRepo,
		})
		m.getMedicalProfileHandler = get_medical_profile.NewHandler(getMedicalProfileUC)

		updateMedicalProfileUC := update_medical_profile.NewUseCase(update_medical_profile.UseCaseDeps{
			MedicalProfileRepo: m.medicalProfileRepo,
			EncryptionService:  m.encryptionService,
			EventPublisher:     cfg.EventBus,
		})
		m.updateMedicalProfileHandler = update_medical_profile.NewHandler(updateMedicalProfileUC)

		resolveMedicalPendingUC := resolve_medical_pending.NewUseCase(resolve_medical_pending.UseCaseDeps{
			MedicalPendingRepo: m.medicalPendingRepo,
			MedicalProfileRepo: m.medicalProfileRepo,
			EncryptionService:  m.encryptionService,
			EventPublisher:     cfg.EventBus,
		})
		m.resolveMedicalPendingHandler = resolve_medical_pending.NewHandler(resolveMedicalPendingUC)
	}

	listPendingMedicalUC := list_pending_medical.NewUseCase(list_pending_medical.UseCaseDeps{
		MedicalPendingRepo: m.medicalPendingRepo,
	})
	m.listPendingMedicalHandler = list_pending_medical.NewHandler(listPendingMedicalUC)

	// 10. Inicializar Features de Avatar (Phase 4)
	if m.r2Storage != nil && cfg.EventBus != nil {
		// Upload Avatar — genera URL prefirmada
		uploadUC := upload_avatar.NewUseCase(upload_avatar.UseCaseDeps{
			Storage:     m.r2Storage,
			RateLimiter: cfg.RateLimiter,
		})
		m.uploadAvatarHandler = upload_avatar.NewHandler(uploadUC)

		// Confirm Avatar — verifica existencia y publica evento de validación
		confirmUC := confirm_avatar_upload.NewUseCase(confirm_avatar_upload.UseCaseDeps{
			Storage:        m.r2Storage,
			EventPublisher: cfg.EventBus,
		})
		m.confirmAvatarUploadHandler = confirm_avatar_upload.NewHandler(confirmUC)

		// Avatar Validator — consumer de Dragonfly Streams
		m.avatarValidator = pipeline.NewAvatarValidator(cfg.RedisClient, m.profileRepo)
	}

	// 11. Inicializar repositorio de documentos (Phase 5)
	docRepo := postgres.NewDocumentRepository(cfg.PostgresPool)
	m.documentRepo = docRepo

	// 12. Inicializar OCR service (Phase 5)
	if cfg.OCRAPIKey != "" {
		m.ocrService = deepseekocr.NewOCRClient(cfg.OCRAPIKey)
	}

	// 13. Inicializar Features de Documentos (Phase 5)
	if m.r2Storage != nil && cfg.EventBus != nil {
		// Upload Document — subida multipart con magic bytes y pipeline
		uploadDocUC := upload_document.NewUseCase(upload_document.UseCaseDeps{
			DocRepo:             docRepo,
			Storage:             m.r2Storage,
			EventPublisher:      cfg.EventBus,
			Dragonfly:           cfg.RedisClient,
			MaxDocumentsPerUser: upload_document.MaxDocumentsPerUser(),
			RateLimitMax:        upload_document.RateLimitMax(),
			RateLimitWindowSecs: upload_document.RateLimitWindowSecs(),
		})
		m.uploadDocumentHandler = upload_document.NewHandler(uploadDocUC)
	}

	// Document Types — catálogo público (siempre disponible)
	docTypesUC := document_types.NewUseCase(document_types.UseCaseDeps{
		TypeRepo: docRepo,
	})
	m.documentTypesHandler = document_types.NewHandler(docTypesUC)

	// List Documents
	listDocsUC := list_documents.NewUseCase(list_documents.UseCaseDeps{
		DocRepo: docRepo,
	})
	m.listDocumentsHandler = list_documents.NewHandler(listDocsUC)

	// Get Document
	getDocUC := get_document.NewUseCase(get_document.UseCaseDeps{
		DocRepo: docRepo,
	})
	m.getDocumentHandler = get_document.NewHandler(getDocUC)

	// Download Document
	if m.r2Storage != nil {
		downloadDocUC := download_document.NewUseCase(download_document.UseCaseDeps{
			DocRepo: docRepo,
			Storage: m.r2Storage,
		})
		m.downloadDocumentHandler = download_document.NewHandler(downloadDocUC)
	}

	// Delete Document
	if m.r2Storage != nil {
		deleteDocUC := delete_document.NewUseCase(delete_document.UseCaseDeps{
			DocRepo: docRepo,
			R2:      m.r2Storage,
		})
		m.deleteDocumentHandler = delete_document.NewHandler(deleteDocUC)
	}

	// Verify Document (admin only)
	verifyDocUC := verify_document.NewUseCase(verify_document.UseCaseDeps{
		DocRepo:   docRepo,
		Dragonfly: cfg.RedisClient,
	})
	m.verifyDocumentHandler = verify_document.NewHandler(verifyDocUC)

	// Document Events (SSE)
	m.documentEventsHandler = document_events.NewHandler(docRepo, cfg.RedisClient)

	// 14. Inicializar Repositorios de Búsquedas y Favoritos (Phase 6)
	searchRepo := postgres.NewSavedSearchRepository(cfg.PostgresPool)
	m.savedSearchRepo = searchRepo

	favRepo := postgres.NewFavoriteRepository(cfg.PostgresPool)
	m.favoriteRepo = favRepo

	// Hash Service (blake3) para deduplicación de búsquedas guardadas
	hashSvc := hash.NewBlake3Service()

	// 15. Inicializar Features de Búsquedas Guardadas (Phase 6)
	createSavedSearchUC := create_saved_search.NewUseCase(create_saved_search.UseCaseDeps{
		SavedSearchRepo: searchRepo,
		HashService:     hashSvc,
	})
	m.createSavedSearchHandler = create_saved_search.NewHandler(createSavedSearchUC)

	listSavedSearchesUC := list_saved_searches.NewUseCase(list_saved_searches.UseCaseDeps{
		SavedSearchRepo: searchRepo,
	})
	m.listSavedSearchesHandler = list_saved_searches.NewHandler(listSavedSearchesUC)

	updateSavedSearchUC := update_saved_search.NewUseCase(update_saved_search.UseCaseDeps{
		SavedSearchRepo: searchRepo,
		HashService:     hashSvc,
	})
	m.updateSavedSearchHandler = update_saved_search.NewHandler(updateSavedSearchUC)

	deleteSavedSearchUC := delete_saved_search.NewUseCase(delete_saved_search.UseCaseDeps{
		SavedSearchRepo: searchRepo,
	})
	m.deleteSavedSearchHandler = delete_saved_search.NewHandler(deleteSavedSearchUC)

	toggleAlertUC := toggle_alert.NewUseCase(toggle_alert.UseCaseDeps{
		SavedSearchRepo: searchRepo,
	})
	m.toggleAlertHandler = toggle_alert.NewHandler(toggleAlertUC)

	// 16. Inicializar Features de Favoritos (Phase 6)
	addFavoriteUC := add_favorite.NewUseCase(add_favorite.UseCaseDeps{
		FavoriteRepo: favRepo,
	})
	m.addFavoriteHandler = add_favorite.NewHandler(addFavoriteUC)

	listFavoritesUC := list_favorites.NewUseCase(list_favorites.UseCaseDeps{
		FavoriteRepo: favRepo,
	})
	m.listFavoritesHandler = list_favorites.NewHandler(listFavoritesUC)

	deleteFavoriteUC := delete_favorite.NewUseCase(delete_favorite.UseCaseDeps{
		FavoriteRepo: favRepo,
	})
	m.deleteFavoriteHandler = delete_favorite.NewHandler(deleteFavoriteUC)

	// 17. Inicializar Document Pipelines (Phase 5)
	if cfg.RedisClient != nil && docRepo != nil {
		// Validator Worker
		m.validatorWorker = pipeline.NewValidatorWorker(cfg.RedisClient, docRepo, m.r2Storage)

		// Sanitizer Worker
		if m.r2Storage != nil {
			m.sanitizerWorker = pipeline.NewSanitizerWorker(cfg.RedisClient, m.r2Storage, docRepo)
		}

		// OCR Worker
		if m.ocrService != nil && m.r2Storage != nil && m.encryptionService != nil {
			m.ocrWorker = pipeline.NewOCRWorker(
				cfg.RedisClient,
				m.r2Storage,
				m.ocrService,
				docRepo,
				m.medicalProfileRepo,
				m.medicalPendingRepo,
				m.encryptionService,
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
	g.GET("/profile", m.getProfileHandler.Handle, authMW)
	g.PUT("/profile", m.updateProfileHandler.Handle, authMW)
	g.PUT("/profile/locale", m.updateLocaleHandler.Handle, authMW)
	g.PUT("/profile/travel-preferences", m.updateTravelPrefsHandler.Handle, authMW)
	g.PUT("/profile/notifications", m.updateNotifPrefsHandler.Handle, authMW)

	// Phase 3 — Medical
	// GetMedicalProfile y UpdateMedicalProfile requieren encriptación
	if m.getMedicalProfileHandler != nil {
		g.GET("/profile/medical", m.getMedicalProfileHandler.Handle, authMW)
		g.PUT("/profile/medical", m.updateMedicalProfileHandler.Handle, authMW)
		g.POST("/profile/medical/pending/resolve", m.resolveMedicalPendingHandler.Handle, authMW)
	}
	// ListPendingMedicalHandler NO requiere encriptación — gate independiente
	if m.listPendingMedicalHandler != nil {
		g.GET("/profile/medical/pending", m.listPendingMedicalHandler.Handle, authMW)
	}

	// Phase 4 — Avatar
	if m.uploadAvatarHandler != nil {
		g.POST("/profile/avatar", m.uploadAvatarHandler.Handle, authMW)
	}
	if m.confirmAvatarUploadHandler != nil {
		g.POST("/profile/avatar/confirm", m.confirmAvatarUploadHandler.Handle, authMW)
	}

	// Phase 5 — Documentos
	// GET /documents/types — público, sin auth
	if m.documentTypesHandler != nil {
		g.GET("/documents/types", m.documentTypesHandler.Handle)
	}
	// Resto de endpoints de documentos requieren auth
	if m.uploadDocumentHandler != nil {
		g.POST("/documents", m.uploadDocumentHandler.Handle, authMW)
	}
	if m.listDocumentsHandler != nil {
		g.GET("/documents", m.listDocumentsHandler.Handle, authMW)
	}
	if m.getDocumentHandler != nil {
		g.GET("/documents/:document_id", m.getDocumentHandler.Handle, authMW)
	}
	if m.downloadDocumentHandler != nil {
		g.GET("/documents/:document_id/download", m.downloadDocumentHandler.Handle, authMW)
	}
	if m.deleteDocumentHandler != nil {
		g.DELETE("/documents/:document_id", m.deleteDocumentHandler.Handle, authMW)
	}
	if m.verifyDocumentHandler != nil {
		g.PUT("/documents/:document_id/verify", m.verifyDocumentHandler.Handle, authMW, middleware.RequireAdmin())
	}
	if m.documentEventsHandler != nil {
		g.GET("/documents/:document_id/events", m.documentEventsHandler.Handle, authMW)
	}

	// Phase 6 — Búsquedas Guardadas y Favoritos
	if m.createSavedSearchHandler != nil {
		g.POST("/saved-searches", m.createSavedSearchHandler.Handle, authMW)
	}
	if m.listSavedSearchesHandler != nil {
		g.GET("/saved-searches", m.listSavedSearchesHandler.Handle, authMW)
	}
	if m.updateSavedSearchHandler != nil {
		g.PUT("/saved-searches/:search_id", m.updateSavedSearchHandler.Handle, authMW)
	}
	if m.deleteSavedSearchHandler != nil {
		g.DELETE("/saved-searches/:search_id", m.deleteSavedSearchHandler.Handle, authMW)
	}
	if m.toggleAlertHandler != nil {
		g.PUT("/saved-searches/:search_id/alert", m.toggleAlertHandler.Handle, authMW)
	}
	if m.addFavoriteHandler != nil {
		g.POST("/favorites", m.addFavoriteHandler.Handle, authMW)
	}
	if m.listFavoritesHandler != nil {
		g.GET("/favorites", m.listFavoritesHandler.Handle, authMW)
	}
	if m.deleteFavoriteHandler != nil {
		g.DELETE("/favorites/:favorite_id", m.deleteFavoriteHandler.Handle, authMW)
	}
}

// Start inicia los pipelines asíncronos del módulo.
// Debe ser llamado después de que el módulo esté completamente inicializado.
func (m *Module) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	var errs []error

	// Ensure DLQ streams exist before starting workers
	if m.redisClient != nil {
		if err := pipeline.EnsureDLQStreams(m.ctx, m.redisClient); err != nil {
			errs = append(errs, fmt.Errorf("ensure DLQ streams: %w", err))
		}
	}

	if m.avatarValidator != nil {
		m.wg.Go(func() {
			if err := m.avatarValidator.Run(m.ctx); err != nil {
				slog.Error("avatar validator failed", "error", err)
			}
		})
	}
	if m.validatorWorker != nil {
		m.wg.Go(func() {
			if err := m.validatorWorker.Run(m.ctx); err != nil {
				slog.Error("validator worker failed", "error", err)
			}
		})
	}
	if m.sanitizerWorker != nil {
		m.wg.Go(func() {
			if err := m.sanitizerWorker.Run(m.ctx); err != nil {
				slog.Error("sanitizer worker failed", "error", err)
			}
		})
	}
	if m.ocrWorker != nil {
		m.wg.Go(func() {
			if err := m.ocrWorker.Run(m.ctx); err != nil {
				slog.Error("OCR worker failed", "error", err)
			}
		})
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Shutdown detiene gracefulmente los pipelines asíncronos.
// Cancela el contexto y espera a que todas las goroutines orphan terminen
// usando sus canales done, con un timeout pasado vía ctx.
func (m *Module) Shutdown(ctx context.Context) error {
	if m.cancel == nil {
		return nil
	}
	m.cancel()

	// Esperar a que cada worker señale completion vía su canal orphanDone.
	// El contexto del caller provee el deadline.
	for _, done := range m.workerDoneChannels() {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (m *Module) workerDoneChannels() []<-chan struct{} {
	var chans []<-chan struct{}
	if m.validatorWorker != nil {
		chans = append(chans, m.validatorWorker.OrphanDone())
	}
	if m.sanitizerWorker != nil {
		chans = append(chans, m.sanitizerWorker.OrphanDone())
	}
	if m.ocrWorker != nil {
		chans = append(chans, m.ocrWorker.OrphanDone())
	}
	if m.avatarValidator != nil {
		chans = append(chans, m.avatarValidator.OrphanDone())
	}
	return chans
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
		case errors.Is(err, domain.ErrTravelPrefsNotFound):
			return serrors.ErrNotFound("Preferencias de viaje no encontradas", err)
		case errors.Is(err, domain.ErrNotifPrefsNotFound):
			return serrors.ErrNotFound("Preferencias de notificación no encontradas", err)

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
		case errors.Is(err, domain.ErrInvalidEnum):
			return serrors.ErrBadRequest(err.Error(), err)
		case errors.Is(err, domain.ErrInvalidPreferredClass):
			return serrors.ErrBadRequest("Clase de cabina inválida. Valores permitidos: economy, premium_economy, business, first", err)
		case errors.Is(err, domain.ErrInvalidSeatPreference):
			return serrors.ErrBadRequest("Preferencia de asiento inválida. Valores permitidos: window, aisle, middle, no_preference", err)
		case errors.Is(err, domain.ErrInvalidChannel):
			return serrors.ErrBadRequest("Canal de notificación inválido. Valores permitidos: email, sms, websocket", err)
		case errors.Is(err, domain.ErrInvalidNotificationType):
			return serrors.ErrBadRequest("Tipo de notificación inválido. Valores permitidos: booking_confirmation, flight_reminder, promotional", err)
		case errors.Is(err, domain.ErrInvalidBloodType):
			return serrors.ErrBadRequest("Tipo de sangre inválido", err)

		case errors.Is(err, domain.ErrEncryptionError), errors.Is(err, domain.ErrDecryptionError):
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
			return serrors.ErrBadRequest(err.Error(), err)
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
			return serrors.ErrBadRequest("Tipo de entidad favorita inválido. Valores permitidos: hotel, flight, activity", err)

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
