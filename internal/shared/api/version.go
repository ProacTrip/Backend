package api

import "strings"

// Prefix devuelve el prefijo de versión de la API (ej: "/v1")
func Prefix(version string) string {
	if version == "" {
		return "/v1"
	}
	// "1.0.0" → "/v1"
	parts := strings.Split(version, ".")
	if len(parts) > 0 && parts[0] != "" {
		return "/v" + parts[0]
	}
	return "/v1"
}
