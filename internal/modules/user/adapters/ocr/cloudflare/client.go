// Adapter: OCR vía Cloudflare Workers AI (modelo de visión llama-4-scout).
// Recibe URL prefirmada de R2, descarga el archivo, si es PDF lo convierte
// a imágenes PNG por página, y envía cada página al modelo de visión.
// Combina el texto de todas las páginas y lo clasifica.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/gen2brain/go-fitz"
)

// OCRClient implementa domain.OCRService usando Workers AI vision model.
type OCRClient struct {
	accountID string
	apiToken  string
	client    *http.Client
}

func NewOCRClient(accountID, apiToken string, _ string, _ []domain.DocumentType) *OCRClient {
	return &OCRClient{
		accountID: accountID,
		apiToken:  apiToken,
		client:    &http.Client{Timeout: 120 * time.Second},
	}
}

// =============================================================================
// domain.OCRService
// =============================================================================

type visionResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Response string `json:"response"`
	} `json:"result"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *OCRClient) ExtractFromDocument(ctx context.Context, fileURL string) (*domain.ExtractedData, error) {
	// 1. Descargar archivo de R2
	slog.Info("ocr: downloading file from R2")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	fileBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	contentType := http.DetectContentType(fileBytes)
	slog.Info("ocr: file downloaded", "size", len(fileBytes), "type", contentType)

	// 2. Extraer imágenes de cada página (PDF) o usar la imagen directamente
	var pageImages []string // base64 encoded PNGs
	if strings.HasPrefix(contentType, "application/pdf") || strings.HasSuffix(fileURL, ".pdf") {
		images, err := c.pdfToImages(fileBytes)
		if err != nil {
			return nil, fmt.Errorf("pdf to images: %w", err)
		}
		pageImages = images
		slog.Info("ocr: PDF converted to images", "pages", len(pageImages))
	} else {
		// JPEG/PNG — usar directamente como base64
		pageImages = []string{base64.StdEncoding.EncodeToString(fileBytes)}
		slog.Info("ocr: using image directly")
	}

	// 3. Enviar cada página al modelo de visión
	var allText strings.Builder
	for i, img := range pageImages {
		slog.Info("ocr: processing page", "page", i+1, "total", len(pageImages))
		text, err := c.extractTextFromImage(ctx, img)
		if err != nil {
			slog.Warn("ocr: page extraction failed", "page", i+1, "error", err)
			continue
		}
		allText.WriteString(text)
		allText.WriteString("\n")
	}

	combinedText := strings.TrimSpace(allText.String())
	if combinedText == "" {
		return &domain.ExtractedData{
			DocumentType:  "unknown",
			RawResponse:   "no text extracted",
			OCRConfidence: 0,
		}, nil
	}

	slog.Info("ocr: combined text", "len", len(combinedText), "preview", combinedText[:min(len(combinedText), 200)])

	// 4. Clasificar el texto combinado con una segunda llamada al modelo
	return c.classifyText(ctx, combinedText)
}

// pdfToImages convierte un PDF a una lista de imágenes PNG en base64 (una por página).
func (c *OCRClient) pdfToImages(pdfBytes []byte) ([]string, error) {
	doc, err := fitz.NewFromMemory(pdfBytes)
	if err != nil {
		return nil, fmt.Errorf("open PDF: %w", err)
	}
	defer doc.Close()

	var images []string
	for i := 0; i < doc.NumPage(); i++ {
		img, err := doc.Image(i)
		if err != nil {
			slog.Warn("ocr: failed to render PDF page", "page", i+1, "error", err)
			continue
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			continue
		}
		images = append(images, base64.StdEncoding.EncodeToString(buf.Bytes()))
	}
	return images, nil
}

// extractTextFromImage envía una imagen base64 al modelo de visión para extraer texto.
func (c *OCRClient) extractTextFromImage(ctx context.Context, base64Image string) (string, error) {
	dataURI := "data:image/png;base64," + base64Image
	body := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "system", "content": "Extract ALL text from this document image exactly as written. Preserve names, numbers, dates, and codes. Return only the extracted text, no commentary."},
			{"role": "user", "content": "Extract every piece of text from this document page."},
		},
		"image":      dataURI,
		"max_tokens": 1024,
	}
	return c.callVisionAPI(ctx, body)
}

// classifyText envía el texto extraído al modelo para clasificarlo como JSON estructurado.
func (c *OCRClient) classifyText(ctx context.Context, text string) (*domain.ExtractedData, error) {
	body := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "system", "content": "You are a travel document classifier. Return ONLY valid JSON with this schema: {\"document_type\":\"passport|national_id|drivers_license|visa|travel_insurance|vaccination_cert|boarding_pass|receipt|unknown\",\"document_number\":null,\"full_name\":null,\"date_of_birth\":null,\"expiry_date\":null,\"issuing_country\":null,\"nationality\":null}. Use null for missing fields."},
			{"role": "user", "content": fmt.Sprintf("Classify this document and extract structured fields:\n\n%s", text[:min(len(text), 3000)])},
		},
		"max_tokens": 512,
	}

	rawJSON, err := c.callVisionAPI(ctx, body)
	if err != nil {
		return nil, err
	}
	return parseVisionJSON(rawJSON, text), nil
}

// callVisionAPI hace una llamada al modelo de visión de Workers AI.
func (c *OCRClient) callVisionAPI(ctx context.Context, body map[string]interface{}) (string, error) {
	jsonBody, _ := json.Marshal(body)
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/run/@cf/meta/llama-4-scout-17b-16e-instruct", c.accountID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
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

	var vr visionResponse
	if err := json.Unmarshal(respBytes, &vr); err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	if !vr.Success {
		return "", fmt.Errorf("api returned error: %s", string(respBytes))
	}
	return vr.Result.Response, nil
}

// =============================================================================
// Parseo
// =============================================================================

type parsedFields struct {
	DocumentType   string  `json:"document_type"`
	DocumentNumber *string `json:"document_number"`
	FullName       *string `json:"full_name"`
	DateOfBirth    *string `json:"date_of_birth"`
	ExpiryDate     *string `json:"expiry_date"`
	IssuingCountry *string `json:"issuing_country"`
	Nationality    *string `json:"nationality"`
}

func parseVisionJSON(raw, rawResponse string) *domain.ExtractedData {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var pf parsedFields
	if err := json.Unmarshal([]byte(raw), &pf); err != nil {
		return &domain.ExtractedData{
			DocumentType:  "unknown",
			RawResponse:   rawResponse,
			OCRConfidence: 0.5,
		}
	}
	return &domain.ExtractedData{
		DocumentType:   pf.DocumentType,
		DocumentNumber: pf.DocumentNumber,
		FullName:       pf.FullName,
		DateOfBirth:    pf.DateOfBirth,
		ExpiryDate:     pf.ExpiryDate,
		IssuingCountry: pf.IssuingCountry,
		Nationality:    pf.Nationality,
		RawResponse:    rawResponse,
		OCRConfidence:  0.85,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
