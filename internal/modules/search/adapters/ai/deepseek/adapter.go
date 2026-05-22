// Adaptador DeepSeek — implementa domain.AIInterpreter.
// Interpreta lenguaje natural de viajes usando DeepSeek V4 Flash
// via API OpenAI-compatible. Incluye reparación JSON y reintentos.
package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Timezone
// =============================================================================

// madridLocation is used to format today's date in the system prompt.
// All dates are communicated in Europe/Madrid timezone (CET/CEST).
var madridLocation = mustLoadLocation("Europe/Madrid")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		// Fallback to UTC — should never happen in production containers.
		return time.UTC
	}
	return loc
}

// =============================================================================
// Compile-time interface checks
// =============================================================================

var _ domain.AIInterpreter = (*Adapter)(nil)
var _ domain.DiscoveryInterpreter = (*Adapter)(nil)

// =============================================================================
// Adapter
// =============================================================================

// Adapter implements domain.AIInterpreter using DeepSeek V4 Flash.
type Adapter struct {
	client *Client
}

// NewAdapter creates a new DeepSeek AI interpreter adapter.
func NewAdapter(client *Client) *Adapter {
	return &Adapter{client: client}
}

// =============================================================================
// AdapterToolCallResult — result of a tool-calling chat completion
// =============================================================================

// AdapterToolCallResult wraps the AI assistant message and any tool calls it requests.
type AdapterToolCallResult struct {
	AssistantMessage string              `json:"assistant_message"`
	ToolCalls        []AdapterToolCall   `json:"tool_calls,omitzero"`
}

// AdapterToolCall represents a parsed tool call from the AI model.
type AdapterToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"` // "search_hotels" or "search_flights"
	Arguments map[string]interface{} `json:"arguments"`
}

// =============================================================================
// ChatWithTools — streaming chat completion with tool calling
// =============================================================================

// ChatWithTools sends messages and tool definitions to DeepSeek via streaming.
// If the AI responds with finish_reason: "tool_calls", the accumulated tool calls
// are parsed and returned alongside the assistant text.
//
// Returns the assistant text message (accumulated from stream) and any tool calls.
func (a *Adapter) ChatWithTools(ctx context.Context, messages []chatMessage, tools []ToolDef) (*AdapterToolCallResult, error) {
	params := DefaultDiscoveryParams()
	params.Tools = tools

	ch, err := a.client.ChatCompletionStream(ctx, messages, params)
	if err != nil {
		return nil, fmt.Errorf("deepseek chat with tools: %w", err)
	}

	result := &AdapterToolCallResult{}

	for event := range ch {
		if event.Delta != "" {
			result.AssistantMessage += event.Delta
		}
		if event.Done && event.FinishReason == "tool_calls" && len(event.ToolCalls) > 0 {
			for _, tc := range event.ToolCalls {
				// Parse the JSON arguments from Parameters (json.RawMessage)
				var args map[string]interface{}
				if len(tc.Function.Parameters) > 0 {
					if err := json.Unmarshal(tc.Function.Parameters, &args); err != nil {
						args = map[string]interface{}{}
					}
				}
				result.ToolCalls = append(result.ToolCalls, AdapterToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: args,
				})
			}
			break
		}
		if event.Done {
			break
		}
	}

	// Drain remaining events (channel still has data after break)
	for range ch {
	}

	return result, nil
}

// =============================================================================
// DiscoveryPrompt
// =============================================================================

// DiscoveryPrompt carries location and preference context for discovery queries.
type DiscoveryPrompt struct {
	Query       string  `json:"query"`
	Lat         float64 `json:"lat,omitzero"`
	Lng         float64 `json:"lng,omitzero"`
	CountryCode string  `json:"country_code,omitzero"`
	Timezone    string  `json:"timezone,omitzero"`
	Language    string  `json:"language"`
	Currency    string  `json:"currency,omitzero"`
}

// =============================================================================
// Discover (streaming) — sends discovery prompt and returns SSE channel
// =============================================================================

// DiscoverStream sends a discovery prompt to DeepSeek v4 Flash and returns a channel
// of SSE events for the caller to consume. The system prompt instructs the AI
// to act as a travel discovery assistant that recommends destinations.
func (a *Adapter) DiscoverStream(ctx context.Context, prompt DiscoveryPrompt) (<-chan SSEEvent, error) {
	sysPrompt := buildDiscoverySystemPrompt(prompt)
	messages := []chatMessage{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: prompt.Query},
	}
	return a.client.ChatCompletionStream(ctx, messages, DefaultDiscoveryParams())
}

// =============================================================================
// Discover — domain.DiscoveryInterpreter implementation
// =============================================================================

// Discover interprets a natural language discovery query using DeepSeek v4 Flash.
// It builds a system prompt with location context (lat/lng/country/timezone),
// conversation history, and user preferences (currency/language/date), then
// calls the AI for a natural language response.
//
// Returns the complete AI response as a string. For streaming, use DiscoverStream
// and stream the SSE channel at the handler layer.
func (a *Adapter) Discover(ctx context.Context, message string, ctxData domain.DiscoveryContext, history []domain.ConversationMessage) (string, error) {
	prompt := DiscoveryPrompt{
		Query:       message,
		Lat:         ctxData.Lat,
		Lng:         ctxData.Lng,
		CountryCode: ctxData.CountryCode,
		Timezone:    ctxData.Timezone,
		Language:    ctxData.Language,
		Currency:    ctxData.Currency,
	}

	sysPrompt := buildDiscoverySystemPrompt(prompt)

	// Build messages: system prompt + conversation history + current query
	messages := []chatMessage{
		{Role: "system", Content: sysPrompt},
	}

	for _, h := range history {
		role := h.Role
		if role != "user" && role != "assistant" && role != "system" {
			role = "user"
		}
		messages = append(messages, chatMessage{
			Role:    role,
			Content: h.Content,
		})
	}

	messages = append(messages, chatMessage{
		Role:    "user",
		Content: message,
	})

	// Use non-streaming ChatCompletion for simplicity.
	// Streaming SSE chunks are handled at the handler layer.
	return a.client.ChatCompletion(ctx, messages, DefaultDiscoveryParams())
}

// =============================================================================
// Discovery System Prompt
// =============================================================================

const discoverySystemPrompt = `Eres un asistente de descubrimiento de viajes. Tu trabajo es recomendar destinos y experiencias de viaje basándote en las preferencias y contexto del usuario.

Contexto del usuario:
- Ubicación: {location_info}
- Moneda preferida: {currency}
- Idioma: {language}

Tu respuesta debe ser en lenguaje natural, atractiva y útil. Estructura tus recomendaciones así:
1. Un resumen de lo que entendiste sobre las preferencias del usuario
2. 3-5 recomendaciones de destinos o experiencias, cada una con:
   - Nombre del destino
   - Por qué coincide con las preferencias del usuario
   - Mejor época para visitar
   - Rango de presupuesto estimado en {currency}
3. Una sugerencia final o pregunta para refinar la búsqueda

Reglas:
- Sé específico con nombres de lugares reales (ciudades, regiones, parques nacionales)
- Adapta las recomendaciones a la ubicación del usuario (destinos cercanos o bien conectados)
- Considera la temporada actual al recomendar
- Si el usuario no especificó preferencias claras, recomienda destinos populares y variados
- Responde SIEMPRE en {language}
- No inventes precios exactos, usa rangos: $, $$, $$$`

// buildDiscoverySystemPrompt fills placeholders in the discovery system prompt
// with actual location and preference values.
func buildDiscoverySystemPrompt(prompt DiscoveryPrompt) string {
	locInfo := "No especificada"
	if prompt.Lat != 0 || prompt.Lng != 0 {
		locInfo = fmt.Sprintf("Lat: %.4f, Lng: %.4f", prompt.Lat, prompt.Lng)
		if prompt.CountryCode != "" {
			locInfo += fmt.Sprintf(", País: %s", prompt.CountryCode)
		}
		if prompt.Timezone != "" {
			locInfo += fmt.Sprintf(", Zona horaria: %s", prompt.Timezone)
		}
	}

	lang := "español"
	if prompt.Language == "en" {
		lang = "English"
	}

	currency := prompt.Currency
	if currency == "" {
		currency = "EUR"
	}

	s := strings.ReplaceAll(discoverySystemPrompt, "{location_info}", locInfo)
	s = strings.ReplaceAll(s, "{currency}", currency)
	s = strings.ReplaceAll(s, "{language}", lang)
	return s
}

// =============================================================================
// System Prompt (Exact Search — parameter extraction)
// =============================================================================

// systemPrompt is the complete system prompt for DeepSeek (~30 lines).
// Kept extremely short so small models (DeepSeek V4 Flash) produce clean JSON.
// The {language} placeholder is replaced at call time by buildSystemPrompt.
const systemPrompt = `You are a travel search assistant. Your ONLY job is to extract search parameters from user messages and return JSON.
Current date: {today_date} (YYYY-MM-DD). Use this year for all dates unless user specifies otherwise.

OUTPUT FORMAT — return ONLY this JSON structure:
{
  "type": "flights|hotels|both|ambiguous|incomplete",
  "confidence": 0.0-1.0,
  "missing_fields": [],
  "follow_up": "friendly question in {language}",
  "flight_params": {
    "departure": "IATA code (MAD, BCN, EZE, CDG, LIM...)",
    "arrival": "IATA code",
    "outbound_date": "YYYY-MM-DD",
    "return_date": "YYYY-MM-DD or empty",
    "trip_type": "round_trip|one_way",
    "adults": number,
    "travel_class": "economy|premium_economy|business|first",
    "stops": "any|nonstop|max_1|max_2",
    "max_price": null or number
  },
  "hotel_params": {
    "query": "City, Country",
    "check_in_date": "YYYY-MM-DD",
    "check_out_date": "YYYY-MM-DD",
    "adults": number,
    "rating": null or 7|8|9,
    "hotel_classes": [2,3,4,5] or [],
    "amenities": [35,4,5,9,10] or [],
    "free_cancellation": true or false,
    "max_price": null or number,
    "vacation_rentals": true or false
  }
}

RULES:
- Map city names to IATA codes: Madrid=MAD, Barcelona=BCN, Buenos Aires=EZE, Paris=CDG, Lima=LIM, London=LHR
- For hotel_params.query (and any city mention), ALWAYS include the country name: "City, Country" (e.g., "Paris, France", "Madrid, España", "Tokyo, Japan"). NEVER return just the city name — SerpAPI requires country for accurate results.
- Default adults=1 for BOTH flights and hotels unless user specifies otherwise. NEVER ask "cuántos adultos?" or "how many adults?" — just use 1.
- Default trip_type=round_trip unless user says "solo ida" or "one way". Don't ask unless completely unclear.
- If user says "vuelo directo" → stops=nonstop
- If user says "hotel 4 estrellas" → hotel_classes=[4]
- If user says "piscina" → amenities=[4,5]
- If user says "wifi" → amenities=[35]
- If user says "spa" → amenities=[10]
- If user says "cancelación gratuita" → free_cancellation=true
- Only mark type=incomplete when REQUIRED fields are missing: departure/arrival for flights, query for hotels, or outbound_date/check_in_date. Do NOT mark incomplete for optional fields (adults, trip_type, travel_class, stops, rating, amenities, free_cancellation, max_price, vacation_rentals).
- If unclear what user wants → type=ambiguous, ask follow_up
- RESPOND IN {language} ALWAYS`

// buildSystemPrompt replaces {today_date} and {language} placeholders with actual values.
// today is the current date formatted as YYYY-MM-DD in Europe/Madrid timezone.
// language: "es" (default) = español, "en" = English.
func buildSystemPrompt(today string, language string) string {
	lang := "español"
	if language == "en" {
		lang = "English"
	}
	prompt := strings.ReplaceAll(systemPrompt, "{today_date}", today)
	return strings.ReplaceAll(prompt, "{language}", lang)
}

// =============================================================================
// Parse — domain.AIInterpreter implementation
// =============================================================================

// maxRetries is the maximum number of retry attempts on JSON parse failure.
const maxRetries = 2

// Parse interprets a user message with optional conversation history
// and returns a structured TravelIntent.
// language is the user's detected language code ("es", "en", etc.) used to
// instruct the AI which language to respond in.
func (a *Adapter) Parse(ctx context.Context, message string, history []domain.ConversationMessage, language string) (*domain.TravelIntent, error) {
	messages := buildMessages(message, history, language)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		raw, err := a.client.ChatCompletion(ctx, messages, DefaultExactParams())
		if err != nil {
			return nil, fmt.Errorf("deepseek parse: %w", err)
		}

		intent, parseErr := parseResponse(raw)
		if parseErr == nil {
			return intent, nil
		}

		// On last attempt, wrap and return the parse error
		if attempt == maxRetries {
			return nil, fmt.Errorf("%w: %v", domain.ErrAIParseFailure, parseErr)
		}

		// Retry: append a repair hint to messages
		messages = append(messages, chatMessage{
			Role:    "assistant",
			Content: raw,
		}, chatMessage{
			Role:    "user",
			Content: "Tu respuesta no es JSON válido. Corregilo y devolvé SOLO el JSON sin markdown ni texto adicional. Error: " + parseErr.Error(),
		})
	}

	// unreachable — loop returns before here
	return nil, domain.ErrAIParseFailure
}

// =============================================================================
// Message building
// =============================================================================

// buildMessages constructs the chat message array from system prompt, history, and current message.
// language is injected into the system prompt so the AI responds in the user's detected language.
// today's date (Europe/Madrid) is injected so the AI uses the correct year for dates.
func buildMessages(message string, history []domain.ConversationMessage, language string) []chatMessage {
	// Compute today's date in Madrid timezone for the system prompt.
	today := time.Now().In(madridLocation).Format("2006-01-02")
	sysPrompt := buildSystemPrompt(today, language)

	messages := []chatMessage{
		{Role: "system", Content: sysPrompt},
	}

	for _, h := range history {
		role := h.Role
		// Only coerce unknown roles to "user". Known roles:
		//   user, assistant, system — pass through unchanged.
		if role != "user" && role != "assistant" && role != "system" {
			role = "user"
		}
		messages = append(messages, chatMessage{
			Role:    role,
			Content: h.Content,
		})
	}

	messages = append(messages, chatMessage{
		Role:    "user",
		Content: message,
	})

	return messages
}

// =============================================================================
// JSON parsing and repair
// =============================================================================

// parseResponse extracts and parses a TravelIntent from the raw LLM response.
// On parse failure, attempts progressive repair and falls back to minimum extraction.
func parseResponse(raw string) (*domain.TravelIntent, error) {
	cleaned := repairJSON(raw)

	var intent domain.TravelIntent
	if err := json.Unmarshal([]byte(cleaned), &intent); err != nil {
		// Progressive repair: try fixing unquoted strings
		cleaned = fixUnquotedStrings(cleaned)
		if err2 := json.Unmarshal([]byte(cleaned), &intent); err2 != nil {
			// Last resort: extract minimum viable response (type + follow_up)
			minimum, minErr := extractMinimumIntent(raw)
			if minErr != nil {
				return nil, fmt.Errorf("parse travel intent: %w", err)
			}
			return minimum, nil
		}
	}

	if intent.Type == "" {
		return nil, errors.New("parsed intent has empty type field")
	}

	return &intent, nil
}

// repairJSON attempts to fix common LLM JSON output issues.
func repairJSON(raw string) string {
	s := strings.TrimSpace(raw)

	// 1. Extract from markdown code blocks (```json ... ``` or ``` ... ```)
	if idx := indexOfCodeBlock(s); idx >= 0 {
		s = extractCodeBlock(s, idx)
	}

	// 2. Find the first '{' and last '}' — strip surrounding text
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		s = s[start : end+1]
	}

	// 3. Remove trailing commas before closing brackets/braces
	s = removeTrailingCommas(s)

	// 4. Fix unquoted string values (common in complex prompts)
	s = fixUnquotedStrings(s)

	return s
}

// indexOfCodeBlock finds the start of a markdown code block: ```json or ```
func indexOfCodeBlock(s string) int {
	if idx := strings.Index(s, "```json"); idx >= 0 {
		return idx
	}
	if idx := strings.Index(s, "```"); idx >= 0 {
		return idx
	}
	return -1
}

// extractCodeBlock extracts content between ``` markers.
func extractCodeBlock(s string, startIdx int) string {
	// Find the end of the opening marker line
	afterMarker := strings.Index(s[startIdx:], "\n")
	if afterMarker < 0 {
		return s
	}
	afterMarker += startIdx + 1

	// Find the closing ```
	closeMarker := strings.Index(s[afterMarker:], "```")
	if closeMarker < 0 {
		return s[afterMarker:]
	}

	return s[afterMarker : afterMarker+closeMarker]
}

// trailingCommaPattern matches a comma followed by optional whitespace before ] or }.
var trailingCommaPattern = regexp.MustCompile(`,\s*([}\]])`)

// removeTrailingCommas removes trailing commas before ] or } including those
// separated by whitespace (common LLM JSON error).
func removeTrailingCommas(s string) string {
	return trailingCommaPattern.ReplaceAllString(s, "$1")
}

// unquotedStringPattern matches a colon followed by a bare word (not a quoted string,
// not a number, not a nested structure). The ReplaceAllStringFunc callback filters out
// JSON literals (true, false, null).
var unquotedStringPattern = regexp.MustCompile(`:\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*([,}\]])`)

// fixUnquotedStrings wraps bare string values in quotes.
// DeepSeek V4 Flash sometimes omits quotes on enum values under complex schemas.
// Skips JSON literals (true, false, null) to avoid corrupting valid values.
func fixUnquotedStrings(s string) string {
	return unquotedStringPattern.ReplaceAllStringFunc(s, func(match string) string {
		sub := unquotedStringPattern.FindStringSubmatch(match)
		if len(sub) != 3 {
			return match
		}
		value := sub[1]
		if value == "true" || value == "false" || value == "null" {
			return match
		}
		return `: "` + value + `"` + sub[2]
	})
}

// extractTypePattern captures the "type" field value.
var extractTypePattern = regexp.MustCompile(`"type"\s*:\s*"(flights|hotels|both|ambiguous|incomplete)"`)

// extractFollowUpPattern captures the "follow_up" field value.
var extractFollowUpPattern = regexp.MustCompile(`"follow_up"\s*:\s*"([^"]*)"`)

// extractMinimumIntent tries to salvage a response when JSON is completely malformed.
// Extracts just the "type" and "follow_up" fields via regex as a last resort,
// so the conversation doesn't die with an error. Returns "incomplete" as the type
// if the original type can't be trusted (not ambiguous/incomplete).
func extractMinimumIntent(raw string) (*domain.TravelIntent, error) {
	typeMatch := extractTypePattern.FindStringSubmatch(raw)
	if typeMatch == nil {
		return nil, errors.New("cannot extract type field from response")
	}

	intentType := typeMatch[1]

	// If the original type was flights/hotels/both but we can't verify the params,
	// downgrade to "incomplete" so the conversation continues safely.
	if intentType != "incomplete" && intentType != "ambiguous" {
		intentType = "incomplete"
	}

	intent := &domain.TravelIntent{
		Type:       intentType,
		Confidence: 0.0,
	}

	if followMatch := extractFollowUpPattern.FindStringSubmatch(raw); followMatch != nil {
		intent.FollowUp = followMatch[1]
	}

	return intent, nil
}
