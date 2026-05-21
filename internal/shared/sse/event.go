// Event representa un evento enviado por el servidor (server-sent event) para el hub de tiempo real.
package sse

// Event es la unidad de datos publicada en y recibida desde el Hub de SSE.
// Type se mapea al campo "event:" de SSE; Data se mapea al campo "data:"
// y es serializable en JSON.
type Event struct {
	Type string
	Data any
}
