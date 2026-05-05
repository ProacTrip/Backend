// Adaptador Ollama — implementa domain.AIInterpreter.
// Interpreta lenguaje natural de viajes usando un LLM local via Ollama
// (API OpenAI-compatible). Incluye reparación JSON y reintentos más agresivos
// (modelos locales como llama3 pueden no devolver JSON perfecto).
package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Compile-time interface check
// =============================================================================

var _ domain.AIInterpreter = (*Adapter)(nil)

// =============================================================================
// Adapter
// =============================================================================

// Adapter implements domain.AIInterpreter using a local LLM via Ollama.
type Adapter struct {
	client *Client
}

// NewAdapter creates a new Ollama AI interpreter adapter.
func NewAdapter(client *Client) *Adapter {
	return &Adapter{client: client}
}

// =============================================================================
// System Prompt
// =============================================================================

// systemPrompt is the complete system prompt for Ollama (~30 lines).
// Kept extremely short so small local models (llama3, mistral) produce clean JSON.
// The {language} placeholder is replaced at call time by buildSystemPrompt.
const systemPrompt = `You are a travel search assistant. Your ONLY job is to extract search parameters from user messages and return JSON.

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
- Default adults=1 unless specified
- Default trip_type=round_trip unless user says "solo ida"/"one way"
- If user says "vuelo directo" → stops=nonstop
- If user says "hotel 4 estrellas" → hotel_classes=[4]
- If user says "piscina" → amenities=[4,5]
- If user says "wifi" → amenities=[35]
- If user says "spa" → amenities=[10]
- If user says "cancelación gratuita" → free_cancellation=true
- If info is missing → type=incomplete, list missing fields, ask follow_up
- If unclear what user wants → type=ambiguous, ask follow_up
- RESPOND IN {language} ALWAYS`

// buildSystemPrompt replaces the {language} placeholder with the user's language.
func buildSystemPrompt(language string) string {
	lang := "español"
	if language == "en" {
		lang = "English"
	}
	return strings.ReplaceAll(systemPrompt, "{language}", lang)
}

// =============================================================================
// Parse — domain.AIInterpreter implementation
// =============================================================================

// maxRetries is higher for Ollama because local models (llama3) may produce
// malformed JSON more often than cloud APIs.
const maxRetries = 3

// Parse interprets a user message with optional conversation history
// and returns a structured TravelIntent.
// language is the user's detected language code ("es", "en") used to
// instruct the AI which language to respond in.
func (a *Adapter) Parse(ctx context.Context, message string, history []domain.ConversationMessage, language string) (*domain.TravelIntent, error) {
	messages := buildMessages(message, history, language)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		raw, err := a.client.ChatCompletion(ctx, messages)
		if err != nil {
			return nil, fmt.Errorf("ollama parse: %w", err)
		}

		intent, parseErr := parseResponse(raw)
		if parseErr == nil {
			return intent, nil
		}

		// On last attempt, wrap and return the parse error
		if attempt == maxRetries {
			return nil, fmt.Errorf("%w: %v", domain.ErrAIParseFailure, parseErr)
		}

		// Retry: append a repair hint to messages — more explicit for local models
		messages = append(messages, chatMessage{
			Role:    "assistant",
			Content: raw,
		}, chatMessage{
			Role:    "user",
			Content: "ERROR: Tu respuesta no es JSON válido. Responde EXCLUSIVAMENTE con el JSON, sin texto, sin markdown, sin explicaciones. Solo el objeto JSON. Error técnico: " + parseErr.Error(),
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
func buildMessages(message string, history []domain.ConversationMessage, language string) []chatMessage {
	// Build system prompt with language directive prepended.
	sysPrompt := buildSystemPrompt(language)

	messages := []chatMessage{
		{Role: "system", Content: sysPrompt},
	}

	for _, h := range history {
		role := h.Role
		if role != "user" && role != "assistant" {
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
// Ollama models (llama3, mistral) are more prone to malformed JSON:
// - Text before/after the JSON block
// - Markdown code fences
// - Trailing commas
// - Missing closing braces (truncated output)
// - Double-quote issues in string values
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

	// 4. Fix missing closing braces (truncated output)
	s = fixMissingClosingBraces(s)

	// 5. Fix unquoted string values (common in complex prompts)
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

// fixMissingClosingBraces adds missing closing braces for truncated JSON.
// Counts { and [ vs } and ] — if unbalanced, appends the missing characters.
func fixMissingClosingBraces(s string) string {
	var braceDepth, bracketDepth int

	for _, ch := range s {
		switch ch {
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		}
	}

	// Close any unclosed brackets first, then braces
	var suffix strings.Builder
	for bracketDepth > 0 {
		suffix.WriteByte(']')
		bracketDepth--
	}
	for braceDepth > 0 {
		suffix.WriteByte('}')
		braceDepth--
	}

	if suffix.Len() > 0 {
		s = s + suffix.String()
	}

	return s
}

// unquotedStringPattern matches a colon followed by a bare word (not a quoted string,
// not a number, not a nested structure). The ReplaceAllStringFunc callback filters out
// JSON literals (true, false, null).
var unquotedStringPattern = regexp.MustCompile(`:\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*([,}\]])`)

// fixUnquotedStrings wraps bare string values in quotes.
// Local models (llama3, mistral) often omit quotes on enum values under complex schemas.
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
