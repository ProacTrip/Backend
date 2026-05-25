// Adapter: OCR vía Cloudflare Workers AI (modelo de visión llama-4-scout).
// Recibe URL prefirmada de R2, extrae texto y devuelve JSON estructurado
// en una sola llamada — sin DeepSeek intermedio.
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
		client:    &http.Client{Timeout: 60 * time.Second},
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
	slog.Info("ocr: calling Workers AI vision model", "url", fileURL[:min(len(fileURL), 80)])

	body := map[string]interface{}{
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You are a travel document OCR parser. Extract all fields from the document image. Return ONLY valid JSON, no other text. Use null for missing fields.",
			},
			{
				"role":    "user",
				"content": "Extract from this travel document: document_type (passport, national_id, drivers_license, visa, travel_insurance, vaccination_cert, boarding_pass, receipt, or unknown), document_number, full_name, date_of_birth (YYYY-MM-DD), expiry_date (YYYY-MM-DD), issuing_country (ISO 3166-1 alpha-2), nationality (ISO 3166-1 alpha-2). Return as JSON.",
			},
		},
		"image":      fileURL,
		"max_tokens": 512,
	}

	jsonBody, _ := json.Marshal(body)
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/run/@cf/meta/llama-4-scout-17b-16e-instruct", c.accountID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vision request: %w", err)
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vision API error (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}

	var vr visionResponse
	if err := json.Unmarshal(respBytes, &vr); err != nil {
		return nil, fmt.Errorf("parse vision response: %w", err)
	}
	if !vr.Success {
		return nil, fmt.Errorf("vision failed: %s", string(respBytes))
	}

	slog.Info("ocr: vision response", "text", vr.Result.Response[:min(len(vr.Result.Response), 200)])
	return parseVisionJSON(vr.Result.Response), nil
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

func parseVisionJSON(raw string) *domain.ExtractedData {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var pf parsedFields
	if err := json.Unmarshal([]byte(raw), &pf); err != nil {
		return &domain.ExtractedData{
			DocumentType:  "unknown",
			RawResponse:   raw,
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
		RawResponse:    raw,
		OCRConfidence:  0.85,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
