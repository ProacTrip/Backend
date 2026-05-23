// Command de entrada para obtener el clima de un destino en una fecha específica.
package get_destination_weather

import (
	"fmt"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
)

// Command representa los parámetros de entrada del caso de uso get_destination_weather.
// Lat y Lng son coordenadas geográficas, Date es una fecha futura en formato YYYY-MM-DD.
type Command struct {
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
	Date string  `json:"date"` // YYYY-MM-DD
}

// Validate verifica que los parámetros del comando estén dentro de rangos válidos.
// Retorna nil si todo es válido, o un error de validación con detalles.
func (cmd Command) Validate() error {
	if cmd.Lat < -90 || cmd.Lat > 90 {
		return fmt.Errorf("%w: latitud %.6f fuera de rango [-90, 90]", domain.ErrInvalidParameterRange, cmd.Lat)
	}
	if cmd.Lng < -180 || cmd.Lng > 180 {
		return fmt.Errorf("%w: longitud %.6f fuera de rango [-180, 180]", domain.ErrInvalidParameterRange, cmd.Lng)
	}
	if cmd.Date == "" {
		return fmt.Errorf("%w: date es requerido", domain.ErrInvalidParameterRange)
	}

	// Validar formato YYYY-MM-DD
	targetDate, err := time.Parse("2006-01-02", cmd.Date)
	if err != nil {
		return fmt.Errorf("%w: date '%s' no es una fecha válida (YYYY-MM-DD)", domain.ErrInvalidParameterRange, cmd.Date)
	}

	// Rechazar fechas en el pasado
	now := time.Now().Truncate(24 * time.Hour)
	if targetDate.Before(now) {
		return fmt.Errorf("%w: date '%s' está en el pasado", domain.ErrInvalidParameterRange, cmd.Date)
	}

	return nil
}
