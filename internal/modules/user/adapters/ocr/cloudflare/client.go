// Adapter: OCR vía Cloudflare Workers AI toMarkdown.
// Convierte documentos (PDF, imágenes) a texto markdown estructurado.
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

// OCRClient implementa domain.OCRService usando Cloudflare Workers AI toMarkdown.
type OCRClient struct {
	accountID string
	apiToken  string
	client    *http.Client
}

// NewOCRClient crea un nuevo cliente OCR para Cloudflare Workers AI.
func NewOCRClient(accountID, apiToken string) *OCRClient {
	return &OCRClient{
		accountID: accountID,
		apiToken:  apiToken,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// =============================================================================
// domain.OCRService
// =============================================================================

// toMarkdownResponse es la respuesta de la API toMarkdown.
type toMarkdownResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		Name     string `json:"name"`
		MimeType string `json:"mimeType"`
		Format   string `json:"format"`
		Tokens   int    `json:"tokens"`
		Data     string `json:"data"`
	} `json:"result"`
	Errors []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// ExtractFromDocument descarga el archivo de R2 y lo envía a toMarkdown.
func (c *OCRClient) ExtractFromDocument(ctx context.Context, fileURL string) (*domain.ExtractedData, error) {
	// 1. Descargar archivo desde la URL prefirmada de R2
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("crear request descarga: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("descargar archivo para OCR: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("descarga R2 falló con status %d", resp.StatusCode)
	}

	fileBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("leer archivo descargado: %w", err)
	}

	// 2. Enviar a toMarkdown como multipart
	markdown, err := c.sendToMarkdown(ctx, fileBytes, "document.pdf")
	if err != nil {
		return nil, fmt.Errorf("toMarkdown OCR: %w", err)
	}

	// 3. Parsear markdown a ExtractedData
	return parseMarkdown(markdown), nil
}

// sendToMarkdown envía el archivo a la API toMarkdown de Cloudflare.
func (c *OCRClient) sendToMarkdown(ctx context.Context, fileBytes []byte, fileName string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", fileName)
	if err != nil {
		return "", fmt.Errorf("crear form file: %w", err)
	}
	if _, err := part.Write(fileBytes); err != nil {
		return "", fmt.Errorf("escribir archivo en form: %w", err)
	}
	writer.Close()

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/tomarkdown", c.accountID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", fmt.Errorf("crear request toMarkdown: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("toMarkdown request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("leer respuesta toMarkdown: %w", err)
	}

	var result toMarkdownResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("parse toMarkdown response: %w", err)
	}

	if !result.Success || len(result.Result) == 0 {
		errMsg := "toMarkdown: respuesta vacía"
		if len(result.Errors) > 0 {
			errMsg = result.Errors[0].Message
		}
		return "", fmt.Errorf("%s (HTTP %d): %s", errMsg, resp.StatusCode, string(respBytes))
	}

	return result.Result[0].Data, nil
}

// =============================================================================
// Parseo de Markdown → ExtractedData
// =============================================================================
// toMarkdown devuelve texto en formato markdown. Extraemos campos estructurados
// usando heurísticas simples sobre el texto.

func parseMarkdown(markdown string) *domain.ExtractedData {
	data := &domain.ExtractedData{
		RawResponse: markdown,
	}

	lines := strings.Split(markdown, "\n")

	// Detectar tipo de documento por palabras clave
	fullText := strings.ToLower(markdown)
	switch {
	case strings.Contains(fullText, "passport") || strings.Contains(fullText, "pasaporte"):
		data.DocumentType = "passport"
	case strings.Contains(fullText, "national id") || strings.Contains(fullText, "dni"):
		data.DocumentType = "national_id"
	case strings.Contains(fullText, "driver") || strings.Contains(fullText, "licence") || strings.Contains(fullText, "conducir"):
		data.DocumentType = "drivers_license"
	case strings.Contains(fullText, "visa"):
		data.DocumentType = "visa"
	case strings.Contains(fullText, "vaccin") || strings.Contains(fullText, "vacuna"):
		data.DocumentType = "vaccination_cert"
	case strings.Contains(fullText, "insurance") || strings.Contains(fullText, "seguro"):
		data.DocumentType = "travel_insurance"
	default:
		data.DocumentType = "unknown"
	}

	// Extraer campos comunes de documentos de viaje
	for _, line := range lines {
		lineLower := strings.ToLower(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(lineLower, "name:") || strings.HasPrefix(lineLower, "nombre:"):
			if v := extractValue(line); v != "" {
				data.FullName = &v
			}
		case strings.HasPrefix(lineLower, "date of birth:") || strings.HasPrefix(lineLower, "fecha de nacimiento:"):
			if v := extractValue(line); v != "" {
				data.DateOfBirth = &v
			}
		case strings.HasPrefix(lineLower, "expiry") || strings.HasPrefix(lineLower, "expira") || strings.HasPrefix(lineLower, "valido hasta"):
			if v := extractValue(line); v != "" {
				data.ExpiryDate = &v
			}
		case strings.HasPrefix(lineLower, "document number:") || strings.HasPrefix(lineLower, "passport no") || strings.HasPrefix(lineLower, "número"):
			if v := extractValue(line); v != "" {
				data.DocumentNumber = &v
			}
		case strings.HasPrefix(lineLower, "nationality:") || strings.HasPrefix(lineLower, "nacionalidad:"):
			if v := extractValue(line); v != "" {
				data.Nationality = &v
			}
		case strings.HasPrefix(lineLower, "issuing") || strings.HasPrefix(lineLower, "country:") || strings.HasPrefix(lineLower, "país:"):
			if v := extractValue(line); v != "" {
				data.IssuingCountry = &v
			}
		}
	}

	data.OCRConfidence = 0.85 // toMarkdown no devuelve confianza; usamos valor fijo
	return data
}

// extractValue extrae el valor después de ":" en una línea.
func extractValue(line string) string {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}
