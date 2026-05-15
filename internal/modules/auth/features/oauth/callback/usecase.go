package callback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// Lógica de negocio del callback OAuth.
// Flujo: validar state → intercambiar código → obtener user info → crear/vincular usuario → generar tokens.

// OAuthStateTokenService valida tokens de estado OAuth (PASETO).
type OAuthStateTokenService interface {
	ValidateOAuthStateToken(tokenString string) (string, error)
}

// TokenService genera pares de tokens (access + refresh).
type TokenService interface {
	GenerateTokenPair(userID uuid.UUID, email string, role string, roleID, sessionID uuid.UUID) (*token.TokenPair, error)
}

// EventPublisher publica eventos en Dragonfly Streams.
type EventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]any) (string, error)
}

// OAuthProviderSelector resuelve el proveedor OAuth por código.
type OAuthProviderSelector interface {
	GetProvider(providerCode string) (domain.OAuthProvider, error)
}

// UseCase — lógica de negocio del callback OAuth.
type UseCase struct {
	repo               domain.UserRepository
	oauthRepo          domain.OAuthRepository
	stateTokenSvc      OAuthStateTokenService
	providerSel        OAuthProviderSelector
	tokenSvc           TokenService
	dragonfly          *redis.Client
	eventPublisher     EventPublisher
}

type UseCaseDeps struct {
	Repo           domain.UserRepository
	OAuthRepo      domain.OAuthRepository
	StateTokenSvc  OAuthStateTokenService
	ProviderSel    OAuthProviderSelector
	TokenSvc       TokenService
	Dragonfly      *redis.Client
	EventPublisher EventPublisher
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		repo:           deps.Repo,
		oauthRepo:      deps.OAuthRepo,
		stateTokenSvc:  deps.StateTokenSvc,
		providerSel:    deps.ProviderSel,
		tokenSvc:       deps.TokenSvc,
		dragonfly:      deps.Dragonfly,
		eventPublisher: deps.EventPublisher,
	}
}

// Execute procesa el callback OAuth completo.
// 1. Valida el state PASETO (anti-CSRF).
// 2. Recupera el code_verifier de Dragonfly (one-time delete atómico).
// 3. Intercambia el código por tokens con el proveedor.
// 4. Obtiene la información del usuario del proveedor.
// 5. Busca o crea el usuario en la base de datos.
// 6. Crea o actualiza la identidad de autenticación.
// 7. Si es usuario nuevo, publica evento auth.user.registered.
// 8. Genera tokens de sesión y retorna.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	// 1. Validar state PASETO
	stateValue, err := uc.stateTokenSvc.ValidateOAuthStateToken(cmd.State)
	if err != nil {
		slog.WarnContext(ctx, "state OAuth inválido", slog.String("error", err.Error()))
		return nil, domain.ErrOAuthStateInvalid
	}

	// 2. Recuperar code_verifier de Dragonfly (GET+DEL atómico para one-time use)
	codeVerifier, err := uc.getAndDeleteOAuthState(ctx, stateValue)
	if err != nil {
		slog.WarnContext(ctx, "state OAuth no encontrado en caché (posible replay)",
			slog.String("state_value", stateValue),
			slog.String("error", err.Error()),
		)
		return nil, domain.ErrOAuthStateInvalid
	}

	// 3. Obtener el proveedor OAuth
	provider, err := uc.providerSel.GetProvider(cmd.Provider)
	if err != nil {
		return nil, err
	}

	// 4. Intercambiar código por tokens
	oauthToken, err := provider.ExchangeCode(ctx, cmd.ProviderCode, codeVerifier)
	if err != nil {
		return nil, err
	}

	// 5. Obtener información del usuario del proveedor
	userInfo, err := provider.GetUserInfo(ctx, oauthToken.AccessToken)
	if err != nil {
		return nil, err
	}

	// Validar que el email del proveedor esté verificado
	if !userInfo.EmailVerified || userInfo.Email == "" {
		return nil, domain.ErrEmailNotVerified
	}

	// 6. Buscar usuario por email
	existingUser, err := uc.repo.GetByEmail(ctx, userInfo.Email)

	var user *domain.User
	var isNewUser bool

	if err == nil {
		// Usuario existe → login OAuth
		user = existingUser

		// Verificar que el usuario esté activo
		if user.Status != domain.StatusActive {
			switch user.Status {
			case domain.StatusLocked:
				return nil, domain.ErrAccountLocked
			case domain.StatusSuspended:
				return nil, domain.ErrAccountSuspended
			case domain.StatusDisabled:
				// AS-SPEC-007: OAuth callback rechaza cuentas deshabilitadas.
				return nil, domain.ErrAccountDisabled
			case domain.StatusPendingVerification:
				// Usuario se registró con email+password pero nunca verificó.
				// Google ya verificó el email → activamos la cuenta automáticamente.
				user.VerifyEmail()
				if err := uc.repo.Update(ctx, user); err != nil {
					return nil, fmt.Errorf("activar usuario vía OAuth: %w", err)
				}
			slog.InfoContext(ctx, "usuario activado vía OAuth",
				slog.String("user_id", user.ID.String()),
				slog.String("email", user.Email),
				slog.String("provider", cmd.Provider),
			)
			default:
				return nil, domain.ErrAccountInactive
			}
		}
	} else if errors.Is(err, domain.ErrUserNotFound) {
		// Usuario no existe → crear nuevo usuario OAuth
		isNewUser = true

		role, err := uc.repo.GetRoleByName(ctx, "client")
		if err != nil {
			return nil, domain.ErrRoleNotFound
		}

		user = newOAuthUser(userInfo.Email, role.ID)
		if err := uc.repo.Create(ctx, user); err != nil {
			return nil, fmt.Errorf("crear usuario OAuth: %w", err)
		}
		slog.InfoContext(ctx, "usuario creado vía OAuth",
			slog.String("user_id", user.ID.String()),
			slog.String("email", user.Email),
			slog.String("provider", cmd.Provider),
		)
	} else {
		return nil, fmt.Errorf("buscar usuario por email: %w", err)
	}

	// 7. Buscar identidad de autenticación existente
	var identity *domain.AuthIdentity
	existingIdentity, err := uc.oauthRepo.GetAuthIdentityByProvider(ctx, cmd.Provider, userInfo.ProviderUserID)
	if err == nil {
		// Identidad ya existe → actualizar tokens y datos
		identity = existingIdentity
		identity.DisplayName = userInfo.Name
		identity.AvatarURL = userInfo.Picture
		identity.Email = userInfo.Email
		identity.AccessTokenEnc = oauthToken.AccessToken
		identity.RefreshTokenEnc = oauthToken.RefreshToken
		if oauthToken.ExpiresIn > 0 {
			exp := time.Now().Add(time.Duration(oauthToken.ExpiresIn) * time.Second)
			identity.TokenExpiresAt = &exp
		}
		identity.LastUsedAt = time.Now()
		identity.UpdatedAt = time.Now()

		if err := uc.oauthRepo.UpdateAuthIdentity(ctx, identity); err != nil {
			slog.ErrorContext(ctx, "error al actualizar identidad OAuth",
				slog.String("error", err.Error()),
			)
		}
	} else if errors.Is(err, domain.ErrIdentityNotFound) {
		// Identidad no existe → crear nueva
		identity = domain.NewAuthIdentity(user.ID, cmd.Provider, userInfo.ProviderUserID,
			userInfo.Email, userInfo.Name, userInfo.Picture)
		identity.AccessTokenEnc = oauthToken.AccessToken
		identity.RefreshTokenEnc = oauthToken.RefreshToken
		if oauthToken.ExpiresIn > 0 {
			exp := time.Now().Add(time.Duration(oauthToken.ExpiresIn) * time.Second)
			identity.TokenExpiresAt = &exp
		}
		if err := uc.oauthRepo.CreateAuthIdentity(ctx, identity); err != nil {
			slog.ErrorContext(ctx, "error al crear identidad OAuth",
				slog.String("error", err.Error()),
			)
		}
	} else {
		return nil, fmt.Errorf("buscar identidad OAuth: %w", err)
	}

	// 8. Publicar evento auth.user.registered si es usuario nuevo
	if isNewUser && uc.eventPublisher != nil {
		streamName := eventbus.StreamName("auth.user.registered")

		// Generar token de verificación para el notification consumer.
		// Para usuarios OAuth el email ya está verificado por el proveedor,
		// pero se incluye el token para que el consumer pueda armar el enlace del email.
		verificationToken := uuid.Must(uuid.NewV7()).String()

		flatPayload := map[string]any{
			"event_type":         "user_registered",
			"event_version":      int64(1),
			"aggregate_id":       user.ID.String(),
			"timestamp":          time.Now().UnixMilli(),
			"user_id":            user.ID.String(),
			"email":              user.Email,
			"provider":           cmd.Provider,
			"verification_token": verificationToken,
			"first_name":         userInfo.GivenName, // nombre de pila de Google, vacío si no está disponible
		}
		if _, err := uc.eventPublisher.Publish(ctx, streamName, flatPayload); err != nil {
			slog.ErrorContext(ctx, "failed to publish auth user event",
				slog.String("event", "auth.user.registered"),
				slog.Any("error", err),
			)
		}
	}

	// 9. Registrar login exitoso
	user.RecordLogin()
	if err := uc.repo.Update(ctx, user); err != nil {
		slog.ErrorContext(ctx, "failed to update user login record after OAuth",
			slog.String("user_id", user.ID.String()),
			slog.Any("error", err),
		)
	}

	// 10. Generar tokens de sesión
	sessionID := uuid.Must(uuid.NewV7())
	tokenPair, err := uc.tokenSvc.GenerateTokenPair(user.ID, user.Email, user.RoleName, user.RoleID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("generar tokens: %w", err)
	}

	return &Response{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		User: &UserResponse{
			UserID:        user.ID,
			Email:         user.Email,
			EmailVerified: true,
			RoleName:      user.RoleName,
		},
	}, nil
}

// getAndDeleteOAuthState recupera y elimina atómicamente el estado OAuth de Dragonfly.
// Usa GET+DEL en pipeline para simular atomicidad (one-time use).
func (uc *UseCase) getAndDeleteOAuthState(ctx context.Context, stateValue string) (string, error) {
	cacheKey := fmt.Sprintf("{auth}:oauth:state:%s", stateValue)

	// GET + DEL en un pipeline para one-time use (evita replay attacks)
	pipe := uc.dragonfly.Pipeline()
	getCmd := pipe.Get(ctx, cacheKey)
	pipe.Del(ctx, cacheKey)
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return "", fmt.Errorf("recuperar estado OAuth de Dragonfly: %w", err)
	}

	val, err := getCmd.Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", domain.ErrOAuthStateInvalid
		}
		return "", fmt.Errorf("leer estado OAuth de Dragonfly: %w", err)
	}

	var state domain.OAuthState
	if err := json.Unmarshal([]byte(val), &state); err != nil {
		return "", fmt.Errorf("deserializar estado OAuth: %w", err)
	}

	return state.CodeVerifier, nil
}

// newOAuthUser crea un nuevo usuario para registro vía OAuth.
// El email ya está verificado por el proveedor, por lo que el estado es active directamente.
// password_hash queda vacío (NULL en DB) porque el usuario se autentica vía OAuth.
func newOAuthUser(email string, roleID uuid.UUID) *domain.User {
	now := time.Now()
	return &domain.User{
		ID:                  uuid.Must(uuid.NewV7()),
		Email:               email,
		EmailVerified:       true,
		EmailVerifiedAt:     &now,
		PasswordHash:        "", // sin contraseña — autenticación vía OAuth
		RoleID:              roleID,
		RoleName:            "client",
		Status:              domain.StatusActive, // email ya verificado por Google
		LoginCount:          0,
		FailedLoginAttempts: 0,
		MFAEnabled:          false,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}
