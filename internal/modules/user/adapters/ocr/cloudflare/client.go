// Adapter: OCR vía Cloudflare Workers AI vision model.
// Recibe URLs prefirmadas de R2 (ya convertidas a JPEG por el worker).
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

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
// domain.OCRService — ahora recibe múltiples URLs (una por página)
// =============================================================================

type visionResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Response string `json:"response"`
	} `json:"result"`
	Errors []struct{ Message string `json:"message"` } `json:"errors"`
}

// ExtractFromDocument procesa las URLs de imágenes (JPEG) de cada página.
func (c *OCRClient) ExtractFromDocument(ctx context.Context, fileURL string) (*domain.ExtractedData, error) {
	// El worker ahora pasa una sola URL (primera página) temporalmente.
	// TODO: extender interfaz para múltiples URLs.
	return c.extractFromURLs(ctx, []string{fileURL})
}

func (c *OCRClient) extractFromURLs(ctx context.Context, urls []string) (*domain.ExtractedData, error) {
	var allText strings.Builder
	for i, url := range urls {
		slog.Info("ocr: processing page", "page", i+1, "total", len(urls), "url", url[:min(len(url), 80)])
		text, err := c.extractTextFromURL(ctx, url)
		if err != nil {
			slog.Warn("ocr: page extraction failed", "page", i+1, "error", err)
			continue
		}
		allText.WriteString(text)
		allText.WriteString("\n")
	}

	combined := strings.TrimSpace(allText.String())
	if combined == "" {
		return &domain.ExtractedData{DocumentType: "unknown", RawResponse: "no text", OCRConfidence: 0}, nil
	}
	slog.Info("ocr: combined text", "len", len(combined), "preview", combined[:min(len(combined), 200)])
	return c.classifyText(ctx, combined)
}

func (c *OCRClient) extractTextFromURL(ctx context.Context, imageURL string) (string, error) {
	body := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "user", "content": "Extract ALL text from this document image exactly as written. Preserve names, numbers, dates. Return only text, no commentary."},
		},
		"image":      imageURL,
		"max_tokens": 1024,
	}
	return c.callVisionAPI(ctx, body)
}

func (c *OCRClient) classifyText(ctx context.Context, text string) (*domain.ExtractedData, error) {
	body := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "system", "content": "Return ONLY JSON: {\"document_type\":\"passport|national_id|drivers_license|visa|travel_insurance|vaccination_cert|boarding_pass|receipt|unknown\",\"document_number\":null,\"full_name\":null,\"date_of_birth\":null,\"expiry_date\":null,\"issuing_country\":null,\"nationality\":null}. Use null for missing."},
			{"role": "user", "content": fmt.Sprintf("Classify and extract:\n\n%s", text[:min(len(text), 3000)])},
		},
		"max_tokens": 512,
	}
	rawJSON, err := c.callVisionAPI(ctx, body)
	if err != nil {
		return nil, err
	}
	return parseVisionJSON(rawJSON, text), nil
}

func (c *OCRClient) callVisionAPI(ctx context.Context, body map[string]interface{}) (string, error) {
	jsonBody, _ := json.Marshal(body)
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/run/@cf/meta/llama-4-scout-17b-16e-instruct", c.accountID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	var vr visionResponse
	json.Unmarshal(respBytes, &vr)
	if !vr.Success {
		return "", fmt.Errorf("API error: %s", string(respBytes))
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
		return &domain.ExtractedData{DocumentType: "unknown", RawResponse: rawResponse, OCRConfidence: 0.5}
	}
	return &domain.ExtractedData{
		DocumentType: pf.DocumentType, DocumentNumber: pf.DocumentNumber,
		FullName: pf.FullName, DateOfBirth: pf.DateOfBirth,
		ExpiryDate: pf.ExpiryDate, IssuingCountry: pf.IssuingCountry,
		Nationality: pf.Nationality, RawResponse: rawResponse, OCRConfidence: 0.85,
	}
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
