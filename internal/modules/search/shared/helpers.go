// Helpers compartidos para el módulo search.
// Funciones puras sin dependencias externas — usadas por múltiples feature packages.
package shared

import "strings"

// PtrOrEmpty devuelve el valor del string apuntado, o "" si el puntero es nil.
func PtrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// TrimSpaces elimina espacios, tabs y newlines al inicio y final del string.
func TrimSpaces(s string) string {
	return strings.TrimSpace(s)
}
