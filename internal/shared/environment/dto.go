// Tipos compartidos para el contrato de caché env:{ip} en DragonflyDB.
// Usados por los módulos environment (escritor) y search (lector) para
// garantizar compatibilidad de formato en tiempo de compilación.
package environment

// =============================================================================
// CacheEntry — Entrada canónica del caché env:{ip}
// =============================================================================

// CacheEntry es el tipo canónico para el caché env:{ip} en DragonflyDB.
// El módulo environment serializa este tipo al escribir en caché,
// y el módulo search lo deserializa al leer.
type CacheEntry struct {
	Location LocationDTO `json:"location"`
	Weather  *WeatherDTO `json:"weather"`
}

// =============================================================================
// LocationDTO — Datos de ubicación en el contrato de caché
// =============================================================================

// LocationDTO contiene los datos de ubicación para el contrato de caché.
// Es un DTO separado de domain.LocationData para mantener aislamiento
// cero-conocimiento entre módulos.
type LocationDTO struct {
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	City        string  `json:"city"`
	State       string  `json:"state"`
	Zipcode     string  `json:"zipcode"`
	Timezone    string  `json:"timezone"`
	Currency    string  `json:"currency"`
	Language    string  `json:"language"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

// =============================================================================
// WeatherDTO — Datos meteorológicos en el contrato de caché
// =============================================================================

// WeatherDTO contiene los datos meteorológicos para el contrato de caché.
type WeatherDTO struct {
	Temp        float64 `json:"temp"`
	FeelsLike   float64 `json:"feels_like"`
	Description string  `json:"description"`
	Icon        string  `json:"icon"`
	IconURL     string  `json:"icon_url"`
	Humidity    int     `json:"humidity"`
	WindSpeed   float64 `json:"wind_speed"`
}

// =============================================================================
// CacheKey — Formato canónico de clave de caché
// =============================================================================

// CacheKey retorna la clave DragonflyDB para el caché de entorno.
// Formato: "env:{ip}" — usado por ambos módulos para garantizar
// que leen y escriben la misma clave.
func CacheKey(ip string) string {
	return "env:" + ip
}
