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

	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/config"

	"github.com/ProacTrip/Backend/internal/modules/user/adapters/encryption"
	deepseekocr "github.com/ProacTrip/Backend/internal/modules/user/adapters/ocr/deepseek"
	"github.com/ProacTrip/Backend/internal/modules/user/adapters/postgres"
	"github.com/ProacTrip/Backend/internal/modules/user/adapters/storage"
	"github.com/ProacTrip/Backend/internal/modules/user/consumer"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/modules/user/features/confirm_avatar_upload"
	"github.com/ProacTrip/Backend/internal/modules/user/features/delete_document"
	"github.com/ProacTrip/Backend/internal/modules/user/features/document_types"
	"github.com/ProacTrip/Backend/internal/modules/user/features/get_document"
	"github.com/ProacTrip/Backend/internal/modules/user/features/get_document_download_url"
	"github.com/ProacTrip/Backend/internal/modules/user/features/get_medical_conflict"
	"github.com/ProacTrip/Backend/internal/modules/user/features/get_medical_profile"
	"github.com/ProacTrip/Backend/internal/modules/user/features/get_profile"
	"github.com/ProacTrip/Backend/internal/modules/user/features/get_travel_preferences"
	"github.com/ProacTrip/Backend/internal/modules/user/features/list_documents"
	"github.com/ProacTrip/Backend/internal/modules/user/features/list_medical_conflicts"
	"github.com/ProacTrip/Backend/internal/modules/user/features/resolve_medical_conflict"
	"github.com/ProacTrip/Backend/internal/modules/user/features/update_medical_profile"
	"github.com/ProacTrip/Backend/internal/modules/user/features/update_profile"
	"github.com/ProacTrip/Backend/internal/modules/user/features/update_travel_prefs"
	"github.com/ProacTrip/Backend/internal/modules/user/features/upload_avatar"
	"github.com/ProacTrip/Backend/internal/modules/user/features/upload_document"
	"github.com/ProacTrip/Backend/internal/modules/user/features/upsert_profile"
	"github.com/ProacTrip/Backend/internal/modules/user/pipeline"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Module contiene las dependencias del módulo User
type Module struct {
	// Repositorios (implementaciones de dominio)
	profileRepo        domain.ProfileRepository
	travelPrefsRepo    domain.TravelPrefsRepository
	medicalProfileRepo domain.MedicalProfileRepository
	medicalPendingRepo domain.MedicalPendingUpdateRepository

	// Servicios
	encryptionService *encryption.Service
	r2Storage         *storage.R2Storage
	redisClient       *redis.Client

	// Use Cases (legacy)
	upsertProfileUseCase *upsert_profile.UseCase

	// Handlers (Phase 2)
	getProfileHandler         *get_profile.Handler
	updateProfileHandler      *update_profile.Handler
	updateTravelPrefsHandler  *update_travel_prefs.Handler
	getTravelPrefsHandler     *get_travel_preferences.Handler

	// Handlers (Phase 3 — Medical)
	getMedicalProfileHandler      *get_medical_profile.Handler
	updateMedicalProfileHandler   *update_medical_profile.Handler
	listMedicalConflictsHandler   *list_medical_conflicts.Handler
	getMedicalConflictHandler     *get_medical_conflict.Handler
	resolveMedicalConflictHandler *resolve_medical_conflict.Handler

	// Handlers (Phase 4 — Avatar)
	uploadAvatarHandler       *upload_avatar.Handler
	confirmAvatarUploadHandler *confirm_avatar_upload.Handler

	// Repositorios (Phase 5 — Documentos)
	documentRepo *postgres.DocumentRepository

	// Servicio OCR (Phase 5)
	ocrService domain.OCRService

	// Handlers (Phase 5 — Documentos)
	uploadDocumentHandler    *upload_document.Handler
	documentTypesHandler     *document_types.Handler
	listDocumentsHandler     *list_documents.Handler
	getDocumentHandler       *get_document.Handler
	downloadDocURLHandler    *get_document_download_url.Handler
	deleteDocumentHandler    *delete_document.Handler

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
	OCRConfig     config.AIOCRConfig // Configuración del servicio OCR (modelo, API key, base URL)
	RateLimiter   *ratelimit.RateLimiter // opcional — rate limiting para uploads
}

// ProfileRepo expone el repositorio de perfiles de usuario.
// Usado por el módulo auth para enriquecer GET /v1/auth/me con avatar_url
// sin acoplar auth → user (Ports & Adapters via UserProfileProvider interface).
func (m *Module) ProfileRepo() domain.ProfileRepository {
	return m.profileRepo
}

// TravelPrefsRepo exposes the travel preferences repository.
func (m *Module) TravelPrefsRepo() domain.TravelPrefsRepository {
	return m.travelPrefsRepo
}

// MedicalProfileRepo exposes the medical profile repository.
func (m *Module) MedicalProfileRepo() domain.MedicalProfileRepository {
	return m.medicalProfileRepo
}

// DocumentRepo exposes the document repository.
func (m *Module) DocumentRepo() domain.DocumentRepository {
	return m.documentRepo
}

// EventConsumer expone el consumer de eventos de usuario.
// Accedido por bootstrap para iniciar el consumer.
func (m *Module) EventConsumer() *consumer.UserEventConsumer {
	return m.eventConsumer
}

// NewModule crea e inicializa el módulo User con todas sus dependencias.
// Las migraciones se ejecutan desde bootstrap antes de llamar a NewModule.
func NewModule(cfg Config) (*Module, error) {
	m := &Module{}

	// 1. Inicializar Repository (PostgreSQL adapter) — implementa ProfileRepository
	profileRepo := postgres.NewUserRepository(cfg.PostgresPool)
	m.profileRepo = profileRepo

	// 2. Inicializar repos de apoyo
	m.travelPrefsRepo = postgres.NewTravelPrefsRepository(cfg.PostgresPool)

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
		m.travelPrefsRepo, m.medicalProfileRepo,
	)

	// 8. Inicializar Features (Phase 2)
	getProfileUC := get_profile.NewUseCase(get_profile.UseCaseDeps{
		ProfileRepo: m.profileRepo,
	})
	m.getProfileHandler = get_profile.NewHandler(getProfileUC)

	updateProfileUC := update_profile.NewUseCase(update_profile.UseCaseDeps{
		ProfileRepo:    m.profileRepo,
		EventPublisher: cfg.EventBus,
	})
	m.updateProfileHandler = update_profile.NewHandler(updateProfileUC)

	updateTravelPrefsUC := update_travel_prefs.NewUseCase(update_travel_prefs.UseCaseDeps{
		TravelPrefsRepo: m.travelPrefsRepo,
		EventPublisher:  cfg.EventBus,
	})
	m.updateTravelPrefsHandler = update_travel_prefs.NewHandler(updateTravelPrefsUC)

	getTravelPrefsUC := get_travel_preferences.NewUseCase(get_travel_preferences.UseCaseDeps{
		TravelPrefsRepo: m.travelPrefsRepo,
	})
	m.getTravelPrefsHandler = get_travel_preferences.NewHandler(getTravelPrefsUC)

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

		resolveMedicalConflictUC := resolve_medical_conflict.NewUseCase(resolve_medical_conflict.UseCaseDeps{
			MedicalPendingRepo: m.medicalPendingRepo,
			MedicalProfileRepo: m.medicalProfileRepo,
			EncryptionService:  m.encryptionService,
			EventPublisher:     cfg.EventBus,
		})
		m.resolveMedicalConflictHandler = resolve_medical_conflict.NewHandler(resolveMedicalConflictUC)

		getMedicalConflictUC := get_medical_conflict.NewUseCase(get_medical_conflict.UseCaseDeps{
			MedicalPendingRepo: m.medicalPendingRepo,
		})
		m.getMedicalConflictHandler = get_medical_conflict.NewHandler(getMedicalConflictUC)
	}

	listMedicalConflictsUC := list_medical_conflicts.NewUseCase(list_medical_conflicts.UseCaseDeps{
		MedicalPendingRepo: m.medicalPendingRepo,
	})
	m.listMedicalConflictsHandler = list_medical_conflicts.NewHandler(listMedicalConflictsUC)

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

		// Avatar Validator — consumer de Dragonfly Streams (genera presigned URLs)
		m.avatarValidator = pipeline.NewAvatarValidator(cfg.RedisClient, m.profileRepo, m.r2Storage, storage.AssetsBucket())
	}

	// 11. Inicializar repositorio de documentos (Phase 5)
	docRepo := postgres.NewDocumentRepository(cfg.PostgresPool)
	m.documentRepo = docRepo

	// 12. Inicializar OCR service (Phase 5)
	if cfg.OCRConfig.APIKey != "" {
		m.ocrService = deepseekocr.NewOCRClient(
			cfg.OCRConfig.APIKey,
			deepseekocr.WithBaseURL(cfg.OCRConfig.BaseURL),
			deepseekocr.WithModel(cfg.OCRConfig.Model),
		)
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

	// Download Document URL — genera URL prefirmada
	if m.r2Storage != nil {
		downloadDocURLUC := get_document_download_url.NewUseCase(get_document_download_url.UseCaseDeps{
			DocRepo: docRepo,
			Storage: m.r2Storage,
		})
		m.downloadDocURLHandler = get_document_download_url.NewHandler(downloadDocURLUC)
	}

	// Delete Document
	if m.r2Storage != nil {
		deleteDocUC := delete_document.NewUseCase(delete_document.UseCaseDeps{
			DocRepo: docRepo,
			R2:      m.r2Storage,
		})
		m.deleteDocumentHandler = delete_document.NewHandler(deleteDocUC)
	}

	// 14. Inicializar Document Pipelines (Phase 5)
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

	// 15. Registrar mapeos de errores de dominio
	registerUserErrorMappings()

	slog.Info("User module initialized",
		"features", []string{
			"upsert_profile", "get_profile", "update_profile",
			"update_travel_prefs", "get_travel_preferences",
			"get_medical_profile", "update_medical_profile",
			"list_medical_conflicts", "get_medical_conflict", "resolve_medical_conflict",
			"upload_avatar", "confirm_avatar_upload",
			"upload_document", "document_types", "list_documents",
			"get_document", "get_document_download_url", "delete_document",
		},
		"consumer", "user-event-consumer",
		"pipelines", []string{
			"avatar-validator", "doc-validator", "doc-sanitizer", "doc-ocr",
		},
	)

	return m, nil
}

// RegisterRoutes registra las rutas del módulo en los grupos proporcionados.
// authMW es el middleware de autenticación (cookie-based).
// publicG es el grupo sin middleware de auth para rutas públicas.
func (m *Module) RegisterRoutes(g *echo.Group, publicG *echo.Group, authMW echo.MiddlewareFunc) {
	_ = authMW // auth ya se aplica a nivel de grupo en bootstrap/app.go

	// Phase 2
	g.GET("/profile", m.getProfileHandler.Handle)
	g.PATCH("/profile", m.updateProfileHandler.Handle)
	g.PATCH("/profile/travel-preferences", m.updateTravelPrefsHandler.Handle)

	// GET travel preferences
	if m.getTravelPrefsHandler != nil {
		g.GET("/profile/travel-preferences", m.getTravelPrefsHandler.Handle)
	}

	// Phase 3 — Medical
	// GetMedicalProfile y UpdateMedicalProfile requieren encriptación
	if m.getMedicalProfileHandler != nil {
		g.GET("/profile/medical", m.getMedicalProfileHandler.Handle)
		g.PATCH("/profile/medical", m.updateMedicalProfileHandler.Handle)
	}
	// Medical conflicts — requiere encriptación para resolve, no para list/get
	if m.getMedicalConflictHandler != nil {
		g.GET("/profile/medical-conflicts/:conflict_id", m.getMedicalConflictHandler.Handle)
		g.POST("/profile/medical-conflicts/:conflict_id/resolve", m.resolveMedicalConflictHandler.Handle)
	}
	// ListMedicalConflictsHandler NO requiere encriptación — gate independiente
	if m.listMedicalConflictsHandler != nil {
		g.GET("/profile/medical-conflicts", m.listMedicalConflictsHandler.Handle)
	}

	// Phase 4 — Avatar
	if m.uploadAvatarHandler != nil {
		g.POST("/profile/avatar", m.uploadAvatarHandler.Handle)
	}
	if m.confirmAvatarUploadHandler != nil {
		g.POST("/profile/avatar/confirm", m.confirmAvatarUploadHandler.Handle)
	}

	// Phase 5 — Documentos
	// GET /profile/documents/types — público, sin auth
	if m.documentTypesHandler != nil {
		publicG.GET("/profile/documents/types", m.documentTypesHandler.Handle)
	}
	// Resto de endpoints de documentos requieren auth
	if m.uploadDocumentHandler != nil {
		g.POST("/profile/documents", m.uploadDocumentHandler.Handle)
	}
	if m.listDocumentsHandler != nil {
		g.GET("/profile/documents", m.listDocumentsHandler.Handle)
	}
	if m.getDocumentHandler != nil {
		g.GET("/profile/documents/:document_id", m.getDocumentHandler.Handle)
	}
	if m.downloadDocURLHandler != nil {
		g.GET("/profile/documents/:document_id/download-url", m.downloadDocURLHandler.Handle)
	}
	if m.deleteDocumentHandler != nil {
		g.DELETE("/profile/documents/:document_id", m.deleteDocumentHandler.Handle)
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
