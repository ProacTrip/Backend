// Adapter: OCR vía Cloudflare Workers AI toMarkdown + DeepSeek V4 Flash.
// toMarkdown extrae texto raw de PDFs/imágenes.
// DeepSeek V4 Flash clasifica el tipo de documento y extrae campos estructurados.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Cliente
// =============================================================================

// OCRClient implementa domain.OCRService usando toMarkdown + DeepSeek.
type OCRClient struct {
	cfAccountID    string
	cfAPIToken     string
	deepseekAPIKey string
	client         *http.Client
}

// NewOCRClient crea un nuevo cliente OCR.
func NewOCRClient(cfAccountID, cfAPIToken, deepseekAPIKey string) *OCRClient {
	return &OCRClient{
		cfAccountID:    cfAccountID,
		cfAPIToken:     cfAPIToken,
		deepseekAPIKey: deepseekAPIKey,
		client:         &http.Client{Timeout: 60 * time.Second},
	}
}

// =============================================================================
// domain.OCRService
// =============================================================================

type toMarkdownResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		Name     string `json:"name"`
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	} `json:"result"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type deepseekRequest struct {
	Model    string           `json:"model"`
	Messages []deepseekMsg    `json:"messages"`
	Stream   bool             `json:"stream"`
}

type deepseekMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepseekResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// ExtractFromDocument implementa el pipeline OCR de dos pasos.
func (c *OCRClient) ExtractFromDocument(ctx context.Context, fileURL string) (*domain.ExtractedData, error) {
	// 1. Descargar archivo de R2
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("crear request descarga: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("descargar archivo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("descarga R2 falló status %d", resp.StatusCode)
	}
	fileBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("leer archivo: %w", err)
	}

	// 2. Paso 1: toMarkdown — extraer texto raw
	rawText, err := c.sendToMarkdown(ctx, fileBytes, "document.pdf")
	if err != nil {
		return nil, fmt.Errorf("toMarkdown: %w", err)
	}

	// 3. Paso 2: DeepSeek V4 Flash — clasificar y estructurar
	return c.classifyWithDeepSeek(ctx, rawText)
}

// sendToMarkdown envía el archivo a Cloudflare toMarkdown.
func (c *OCRClient) sendToMarkdown(ctx context.Context, fileBytes []byte, fileName string) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("files", fileName)
	part.Write(fileBytes)
	w.Close()

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/tomarkdown", c.cfAccountID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	req.Header.Set("Authorization", "Bearer "+c.cfAPIToken)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	var r toMarkdownResponse
	if err := json.Unmarshal(b, &r); err != nil {
		return "", fmt.Errorf("parse toMarkdown: %w", err)
	}
	if !r.Success || len(r.Result) == 0 {
		return "", fmt.Errorf("toMarkdown failed: %s", string(b))
	}
	return r.Result[0].Data, nil
}

// classifyWithDeepSeek envía el texto raw a DeepSeek V4 Flash para clasificar.
func (c *OCRClient) classifyWithDeepSeek(ctx context.Context, rawText string) (*domain.ExtractedData, error) {
	prompt := buildClassificationPrompt(rawText)

	body := deepseekRequest{
		Model: "deepseek-chat",
		Messages: []deepseekMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.deepseek.com/v1/chat/completions",
		bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.deepseekAPIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek request: %w", err)
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepseek API error (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}

	var dsResp deepseekResponse
	if err := json.Unmarshal(respBytes, &dsResp); err != nil {
		return nil, fmt.Errorf("parse deepseek: %w", err)
	}
	if len(dsResp.Choices) == 0 {
		return nil, fmt.Errorf("deepseek: respuesta vacía")
	}

	content := dsResp.Choices[0].Message.Content
	return parseDeepSeekJSON(content, rawText), nil
}

// =============================================================================
// Prompts
// =============================================================================

const systemPrompt = `You are a travel document OCR classifier. Extract structured fields from the provided text.
Return ONLY valid JSON with this exact schema:
{
  "document_type": "passport|national_id|drivers_license|visa|travel_insurance|vaccination_cert|boarding_pass|receipt|unknown",
  "document_number": "string or null",
  "full_name": "string or null",
  "date_of_birth": "YYYY-MM-DD or null",
  "expiry_date": "YYYY-MM-DD or null",
  "issuing_country": "ISO 3166-1 alpha-2 code or null",
  "nationality": "ISO 3166-1 alpha-2 code or null"
}
Do NOT include any text outside the JSON. Do NOT use markdown code blocks.`

func buildClassificationPrompt(rawText string) string {
	// Truncar a ~3000 caracteres para no exceder el contexto
	text := rawText
	if len(text) > 3000 {
		text = text[:3000]
	}

	return fmt.Sprintf(`Classify this travel document and extract structured fields.

Document text:
%s

Return only the JSON object.`, text)
}

// =============================================================================
// Parseo de DeepSeek JSON → ExtractedData
// =============================================================================

func parseDeepSeekJSON(jsonStr, rawText string) *domain.ExtractedData {
	// Limpiar posibles markdown code blocks
	jsonStr = strings.TrimSpace(jsonStr)
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimPrefix(jsonStr, "```")
	jsonStr = strings.TrimSuffix(jsonStr, "```")
	jsonStr = strings.TrimSpace(jsonStr)

	data := &domain.ExtractedData{
		RawResponse:  rawText,
		OCRConfidence: 0.90,
	}

	var parsed struct {
		DocumentType   string  `json:"document_type"`
		DocumentNumber *string `json:"document_number"`
		FullName       *string `json:"full_name"`
		DateOfBirth    *string `json:"date_of_birth"`
		ExpiryDate     *string `json:"expiry_date"`
		IssuingCountry *string `json:"issuing_country"`
		Nationality    *string `json:"nationality"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		// Si falla el parseo JSON, devolver datos mínimos
		data.DocumentType = "unknown"
		return data
	}

	data.DocumentType = parsed.DocumentType
	data.DocumentNumber = parsed.DocumentNumber
	data.FullName = parsed.FullName
	data.DateOfBirth = parsed.DateOfBirth
	data.ExpiryDate = parsed.ExpiryDate
	data.IssuingCountry = parsed.IssuingCountry
	data.Nationality = parsed.Nationality

	if data.DocumentType == "" {
		data.DocumentType = "unknown"
	}
	return data
}
