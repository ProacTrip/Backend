// Cliente HTTP para DeepSeek V4 Flash — OCR de documentos.
// Sigue el mismo patrón que search/adapters/ai/deepseek/client.go.
// Comunica con la API OpenAI-compatible de DeepSeek para extraer datos
// de documentos de viaje (pasaportes, visas, certificados médicos, etc.).
package deepseek

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

const (
	defaultBaseURL     = "https://api.deepseek.com/v1"
	defaultModel       = "deepseek-chat"
	defaultTimeout     = 60 * time.Second // OCR puede tomar más tiempo
	defaultMaxTokens   = 4096
	defaultTemperature = 0.0
)

// =============================================================================
// Tipos de API OpenAI-compatible
// =============================================================================

type chatMessage struct {
	Role    string        `json:"role"`
	Content []messagePart `json:"content"` // multimodal: texto + imagen
}

type messagePart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL    string `json:"url"`    // data:image/jpeg;base64,...
	Detail string `json:"detail"` // "auto", "low", "high"
}

type chatCompletionRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Temperature    float64        `json:"temperature"`
	MaxTokens      int            `json:"max_tokens"`
	ResponseFormat responseFormat `json:"response_format"`
	Thinking       thinkingMode   `json:"thinking"`
	Stream         bool           `json:"stream"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type thinkingMode struct {
	Type string `json:"type"`
}

type chatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      chatMsgResp `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type chatMsgResp struct {
	Content string `json:"content"`
}

type chatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   chatCompletionUsage    `json:"usage"`
}

type apiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// =============================================================================
// OCR Client
// =============================================================================

// OCRClient es el adaptador de OCR usando DeepSeek V4 Flash.
// Implementa domain.OCRService.
type OCRClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// Compile-time interface check
var _ domain.OCRService = (*OCRClient)(nil)

// ClientOpt is a functional option for Client construction.
type ClientOpt func(*OCRClient)

// WithBaseURL overrides the default DeepSeek API base URL.
func WithBaseURL(url string) ClientOpt {
	return func(c *OCRClient) { c.baseURL = url }
}

// WithModel overrides the default model name.
func WithModel(model string) ClientOpt {
	return func(c *OCRClient) { c.model = model }
}

// WithHTTPClient sets a custom http.Client (useful for testing).
func WithHTTPClient(hc *http.Client) ClientOpt {
	return func(c *OCRClient) { c.httpClient = hc }
}

// NewOCRClient creates a new DeepSeek OCR client.
func NewOCRClient(apiKey string, opts ...ClientOpt) *OCRClient {
	if apiKey == "" {
		slog.Warn("DEEPSEEK_API_KEY is empty — deepseek OCR requests will fail")
	}

	c := &OCRClient{
		baseURL: defaultBaseURL,
		apiKey:  apiKey,
		model:   defaultModel,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// =============================================================================
// ExtractFromDocument — implementación de domain.OCRService
// =============================================================================

// ocrSystemPrompt es el prompt de sistema para extracción de documentos.
const ocrSystemPrompt = `Eres un asistente de OCR especializado en documentos de viaje.
Extrae la información estructurada del documento proporcionado.

Responde SIEMPRE en formato JSON con esta estructura:
{
  "document_type": "passport" | "national_id" | "drivers_license" | "visa" | "travel_insurance" | "vaccination_cert" | "boarding_pass" | "receipt" | "unknown",
  "document_number": "string o null",
  "full_name": "string o null",
  "date_of_birth": "YYYY-MM-DD o null",
  "expiry_date": "YYYY-MM-DD o null",
  "issuing_country": "código ISO 3166-1 alpha-2 (2 letras) o null",
  "nationality": "código ISO 3166-1 alpha-2 (2 letras) o null",
  "medical_fields": {
    "blood_type": "string o null",
    "allergies": "string o null",
    "medications": "string o null",
    "conditions": "string o null",
    "vaccinations": "string o null"
  },
  "ocr_confidence": 0.95
}

Reglas:
- Solo extrae campos que estén VISIBLES en el documento
- Para campos no visibles, usa null
- document_type usa uno de los valores de la lista
- ocr_confidence es 0.0 a 1.0: qué tan seguro estás de la precisión
- Si no es un documento de viaje reconocible, document_type="unknown"`

// ExtractFromDocument envía el archivo a DeepSeek V4 Flash para OCR.
func (c *OCRClient) ExtractFromDocument(ctx context.Context, fileBytes []byte, mimeType string) (*domain.ExtractedData, error) {
	// Construir base64 data URL para el modelo multimodal
	base64Data := base64.StdEncoding.EncodeToString(fileBytes)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)

	// Construir mensajes
	messages := []chatMessage{
		{
			Role: "system",
			Content: []messagePart{
				{Type: "text", Text: ocrSystemPrompt},
			},
		},
		{
			Role: "user",
			Content: []messagePart{
				{
					Type: "image_url",
					ImageURL: &imageURL{
						URL:    dataURL,
						Detail: "high",
					},
				},
				{
					Type: "text",
					Text:  "Extrae los datos de este documento de viaje.",
				},
			},
		},
	}

	// Construir request
	reqBody := chatCompletionRequest{
		Model:          c.model,
		Messages:       messages,
		Temperature:    defaultTemperature,
		MaxTokens:      defaultMaxTokens,
		ResponseFormat: responseFormat{Type: "json_object"},
		Thinking:       thinkingMode{Type: "enabled"},
		Stream:         false,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("deepseek ocr marshal request: %w", err)
	}

	endpoint := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("deepseek ocr create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	slog.DebugContext(ctx, "deepseek ocr request",
		slog.String("model", c.model),
		slog.Int("file_size_bytes", len(fileBytes)),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek ocr request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr apiErrorResponse
		if decodeErr := json.NewDecoder(resp.Body).Decode(&apiErr); decodeErr == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("deepseek OCR API error (HTTP %d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("deepseek OCR API returned HTTP %d", resp.StatusCode)
	}

	var result chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("deepseek ocr decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("deepseek ocr returned no choices")
	}

	content := result.Choices[0].Message.Content

	slog.DebugContext(ctx, "deepseek ocr usage",
		slog.Int("prompt_tokens", result.Usage.PromptTokens),
		slog.Int("completion_tokens", result.Usage.CompletionTokens),
	)

	// Limpiar markdown code fences si están presentes
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	// Parsear JSON respuesta a ExtractedData
	var extracted domain.ExtractedData
	if err := json.Unmarshal([]byte(content), &extracted); err != nil {
		return nil, fmt.Errorf("deepseek ocr parse response JSON: %w (raw: %.200s)", err, content)
	}

	extracted.RawResponse = content

	return &extracted, nil
}
