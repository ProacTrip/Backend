// Caso de uso: Obtener perfil de usuario agregado.
// Consulta profile + travel_preferences + notification_preferences.
package get_profile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports (interfaces requeridas por el usecase)
// =============================================================================

type ProfileRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error)
}

type TravelPrefsRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.TravelPreferences, error)
}

type NotifPrefsRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error)
}

// =============================================================================
// UseCase — Agrega datos de 3 repositorios
// =============================================================================

type UseCaseDeps struct {
	ProfileRepo    ProfileRepo
	TravelPrefsRepo TravelPrefsRepo
	NotifPrefsRepo  NotifPrefsRepo
}

type UseCase struct {
	profileRepo    ProfileRepo
	travelPrefsRepo TravelPrefsRepo
	notifPrefsRepo  NotifPrefsRepo
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		profileRepo:    deps.ProfileRepo,
		travelPrefsRepo: deps.TravelPrefsRepo,
		notifPrefsRepo:  deps.NotifPrefsRepo,
	}
}

// Execute consulta el perfil, las preferencias de viaje y las preferencias
// de notificación del usuario. Las preferencias de viaje y notificación
// son opcionales: si no existen, se retornan como nil/empty.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*GetProfileResponse, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	// 1. Obtener perfil, travel prefs y notif prefs en paralelo
	g, gctx := errgroup.WithContext(ctx)
	var profile *domain.UserProfile
	var travelPrefs *domain.TravelPreferences
	var notifPrefs []*domain.NotificationPreference

	g.Go(func() error {
		var err error
		profile, err = uc.profileRepo.GetByUserID(gctx, userID)
		return err
	})
	g.Go(func() error {
		var err error
		travelPrefs, err = uc.travelPrefsRepo.GetByUserID(gctx, userID)
		if errors.Is(err, domain.ErrTravelPrefsNotFound) {
			return nil
		}
		return err
	})
	g.Go(func() error {
		var err error
		notifPrefs, err = uc.notifPrefsRepo.GetByUserID(gctx, userID)
		if errors.Is(err, domain.ErrNotifPrefsNotFound) {
			return nil
		}
		return err
	})
	if err := g.Wait(); err != nil {
		if errors.Is(err, domain.ErrProfileNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}
	if profile == nil {
		return nil, domain.ErrProfileNotFound
	}

	// 2. Construir respuesta flat
	resp := &GetProfileResponse{
		ID:            profile.ID,
		UserID:        profile.UserID,
		Email:         profile.Email,
		FirstName:     profile.FirstName,
		LastName:      profile.LastName,
		Gender:        genderToString(profile.Gender),
		Nationality:   profile.Nationality,
		Phone:         profile.Phone,
		Bio:           profile.Bio,
		AvatarURL:     profile.AvatarURL,
		IsPublic:      profile.IsPublic,
		Location:      buildLocation(profile.CurrentLocation, profile.TimezoneName, profile.CurrencyCode, profile.LanguageCode),
		NotificationPreferences: buildNotifPrefsMap(notifPrefs),
	}

	if profile.DateOfBirth != nil {
		dobStr := profile.DateOfBirth.Format("2006-01-02")
		resp.DateOfBirth = &dobStr
	}

	// 3. Construir respuesta de preferencias de viaje (opcional)
	if travelPrefs != nil {
		resp.TravelPreferences = buildTravelPrefsResponse(travelPrefs)
	}

	return resp, nil
}

// =============================================================================
// Helpers
// =============================================================================

func genderToString(g *domain.Gender) *string {
	if g == nil {
		return nil
	}
	s := string(*g)
	return &s
}

// buildLocation parsea el current_location string en un objeto LocationResponse.
// current_location suele ser "Ciudad, País". Extrae city y country.
// Si no hay coma, todo va a city. El resto de campos se rellenan con
// datos del perfil (timezone, currency, language) o se dejan en zero.
func buildLocation(rawLocation *string, timezone, currency, language string) LocationResponse {
	loc := LocationResponse{
		Timezone: timezone,
		Currency: currency,
		Language: language,
	}

	if rawLocation == nil || *rawLocation == "" {
		return loc
	}

	parts := strings.SplitN(*rawLocation, ",", 2)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	if len(parts) == 1 {
		loc.City = parts[0]
	} else {
		loc.City = parts[0]
		loc.Country = parts[1]
	}

	return loc
}

func buildTravelPrefsResponse(tp *domain.TravelPreferences) *TravelPreferencesResponse {
	resp := &TravelPreferencesResponse{
		PreferredClass:    string(tp.PreferredClass),
		AvoidLayovers:     tp.AvoidLayovers,
		MaxLayoverDuration: tp.MaxLayoverDuration,
	}

	if tp.SeatPreference != nil {
		s := string(*tp.SeatPreference)
		resp.SeatPreference = &s
	}
	resp.MealPreference = tp.MealPreference
	resp.SpecialAssistance = tp.SpecialAssistance

	if len(tp.PreferredAirlines) > 0 {
		airlines := make([]string, len(tp.PreferredAirlines))
		for i, a := range tp.PreferredAirlines {
			airlines[i] = a.String()
		}
		resp.PreferredAirlines = airlines
	}
	resp.PreferredHotels = tp.PreferredHotels

	return resp
}

// buildNotifPrefsMap convierte el array de NotificationPreference en un mapa
// keyed por notification_type, agrupando los canales habilitados.
func buildNotifPrefsMap(prefs []*domain.NotificationPreference) NotificationPreferencesResponse {
	result := make(NotificationPreferencesResponse)

	for _, np := range prefs {
		key := string(np.NotificationType)
		entry, exists := result[key]
		if !exists {
			entry = ChannelPreferences{}
		}

		switch np.Channel {
		case domain.NotifChannelEmail:
			entry.Email = np.Enabled
		case domain.NotifChannelSMS:
			entry.SMS = np.Enabled
		case domain.NotifChannelWebSocket:
			entry.Websocket = np.Enabled
		}

		result[key] = entry
	}

	if len(result) == 0 {
		return nil
	}

	return result
}
