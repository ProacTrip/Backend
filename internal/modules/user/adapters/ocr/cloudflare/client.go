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
	"log/slog"
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
	docTypes       []domain.DocumentType
	client         *http.Client
}

// NewOCRClient crea un nuevo cliente OCR.
func NewOCRClient(cfAccountID, cfAPIToken, deepseekAPIKey string, docTypes []domain.DocumentType) *OCRClient {
	return &OCRClient{
		cfAccountID:    cfAccountID,
		cfAPIToken:     cfAPIToken,
		deepseekAPIKey: deepseekAPIKey,
		docTypes:       docTypes,
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
// Paso 1: Cloudflare Workers AI vision model extrae texto de la imagen vía URL prefirmada.
// Paso 2: DeepSeek V4 Flash clasifica el tipo de documento.
func (c *OCRClient) ExtractFromDocument(ctx context.Context, fileURL string) (*domain.ExtractedData, error) {
	// 1. Paso 1: Workers AI vision model — extraer texto de la imagen
	slog.Info("ocr: calling Workers AI vision model")
	rawText, err := c.extractTextWithVision(ctx, fileURL)
	if err != nil {
		return nil, fmt.Errorf("vision extraction: %w", err)
	}
	slog.Info("ocr: vision extraction completed", "text_len", len(rawText), "text_preview", rawText[:min(len(rawText), 300)])

	// 2. Paso 2: DeepSeek V4 Flash — clasificar y estructurar
	slog.Info("ocr: calling DeepSeek for classification")
	result, err := c.classifyWithDeepSeek(ctx, rawText)
	if err != nil {
		return nil, err
	}
	slog.Info("ocr: DeepSeek classification result", "doc_type", result.DocumentType, "is_travel", result.IsTravelDocument())
	return result, nil
}

// extractTextWithVision envía la URL de la imagen al modelo de visión de Workers AI.
func (c *OCRClient) extractTextWithVision(ctx context.Context, imageURL string) (string, error) {
	slog.Info("ocr: vision image URL", "url", imageURL[:min(len(imageURL), 100)])
	body := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "system", "content": "Extract all text from this document image exactly as it appears. Return only the extracted text, no additional commentary."},
			{"role": "user", "content": "Extract all text from this document."},
		},
		"image":      imageURL,
		"max_tokens": 1024,
	}
	jsonBody, _ := json.Marshal(body)

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/run/@cf/meta/llama-4-scout-17b-16e-instruct", c.cfAccountID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer "+c.cfAPIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vision request: %w", err)
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vision API error (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Success bool `json:"success"`
		Result  struct {
			Response string `json:"response"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("parse vision response: %w", err)
	}
	if !result.Success {
		return "", fmt.Errorf("vision failed: %s", string(respBytes))
	}
	return result.Result.Response, nil
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
			{Role: "system", Content: c.buildSystemPrompt()},
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("deepseek API error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	slog.Info("ocr: deepseek response received", "status", resp.StatusCode)
	var dsResp deepseekResponse
	if err := json.Unmarshal(respBytes, &dsResp); err != nil {
		return nil, fmt.Errorf("parse deepseek: %w", err)
	}
	if len(dsResp.Choices) == 0 {
		return nil, fmt.Errorf("deepseek: respuesta vacía")
	}

	content := dsResp.Choices[0].Message.Content
	slog.Info("ocr: deepseek raw response", "content", content[:min(len(content), 200)])
	return parseDeepSeekJSON(content, rawText), nil
}

// =============================================================================
// Prompts
// =============================================================================

func (c *OCRClient) buildSystemPrompt() string {
	var types []string
	for _, t := range c.docTypes {
		types = append(types, fmt.Sprintf("%s: %s", t.Code, t.Name))
	}
	typeList := strings.Join(types, ", ")

	return fmt.Sprintf(`You are a travel document OCR classifier. Extract structured fields from the provided text.
Valid document types: %s

Return ONLY valid JSON with this exact schema:
{
  "document_type": "one of the valid types above or 'unknown'",
  "document_number": "string or null",
  "full_name": "string or null",
  "date_of_birth": "YYYY-MM-DD or null",
  "expiry_date": "YYYY-MM-DD or null",
  "issuing_country": "ISO 3166-1 alpha-2 code or null",
  "nationality": "ISO 3166-1 alpha-2 code or null"
}
Do NOT include any text outside the JSON. Do NOT use markdown code blocks.`, typeList)
}

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
func min(a, b int) int { if a < b { return a }; return b }
