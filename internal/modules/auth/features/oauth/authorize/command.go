package authorize

// Command para la autorización OAuth — no tiene body,
// los parámetros vienen de la URL (path param provider).
type Command struct {
	Provider string // extraído de c.PathParam("provider")
}
