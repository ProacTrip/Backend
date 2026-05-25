// Adapter: OCR vía Cloudflare Workers AI vision model.
// Descarga el archivo de R2, convierte PDF a JPEG, envía base64 al modelo.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/gen2brain/go-fitz"
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

type visionResponse struct {
	Success bool   `json:"success"`
	Result  struct{ Response string } `json:"result"`
	Errors  []struct{ Message string } `json:"errors"`
}

func (c *OCRClient) ExtractFromDocument(ctx context.Context, fileURL string) (*domain.ExtractedData, error) {
	// 1. Descargar
	slog.Info("ocr: downloading")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	fileBytes, _ := io.ReadAll(resp.Body)

	// 2. PDF → JPEG (todas las páginas)
	var pageB64 []string
	if strings.HasPrefix(http.DetectContentType(fileBytes), "application/pdf") {
		doc, err := fitz.NewFromMemory(fileBytes)
		if err != nil {
			return nil, fmt.Errorf("open PDF: %w", err)
		}
		defer doc.Close()
		for i := 0; i < doc.NumPage(); i++ {
			img, err := doc.Image(i)
			if err != nil {
				continue
			}
			var buf bytes.Buffer
			jpeg.Encode(&buf, img, &jpeg.Options{Quality: 30})
			pageB64 = append(pageB64, base64.StdEncoding.EncodeToString(buf.Bytes()))
		}
		slog.Info("ocr: PDF to images", "pages", len(pageB64))
	} else {
		pageB64 = []string{base64.StdEncoding.EncodeToString(fileBytes)}
	}

	// 3. Extraer texto de cada página
	var allText strings.Builder
	for i, b64 := range pageB64 {
		slog.Info("ocr: page", "n", i+1, "b64_len", len(b64))
		text, err := c.extractText(ctx, b64)
		if err != nil {
			slog.Warn("ocr: page failed", "page", i+1, "error", err)
			continue
		}
		allText.WriteString(text + "\n")
	}

	combined := strings.TrimSpace(allText.String())
	if combined == "" {
		return &domain.ExtractedData{DocumentType: "unknown", OCRConfidence: 0}, nil
	}
	slog.Info("ocr: text", "len", len(combined), "preview", combined[:min(len(combined), 200)])
	return c.classify(ctx, combined)
}

func (c *OCRClient) extractText(ctx context.Context, b64 string) (string, error) {
	return c.visionCall(ctx, map[string]interface{}{
		"messages": []map[string]string{
			{"role": "user", "content": "Extract ALL text from this document image exactly as written. Preserve names, numbers, dates. Return only text, no commentary."},
		},
		"image":      "data:image/jpeg;base64," + b64,
		"max_tokens": 1024,
	})
}

func (c *OCRClient) classify(ctx context.Context, text string) (*domain.ExtractedData, error) {
	raw, err := c.visionCall(ctx, map[string]interface{}{
		"messages": []map[string]string{
			{"role": "system", "content": "Return ONLY JSON: {\"document_type\":\"passport|national_id|drivers_license|visa|travel_insurance|vaccination_cert|boarding_pass|receipt|unknown\",\"document_number\":null,\"full_name\":null,\"date_of_birth\":null,\"expiry_date\":null,\"issuing_country\":null,\"nationality\":null}"},
			{"role": "user", "content": fmt.Sprintf("Classify and extract:\n\n%s", text[:min(len(text), 3000)])},
		},
		"max_tokens": 512,
	})
	if err != nil {
		return nil, err
	}
	return parseJSON(raw, text), nil
}

func (c *OCRClient) visionCall(ctx context.Context, body map[string]interface{}) (string, error) {
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
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	var vr visionResponse
	json.Unmarshal(b, &vr)
	if !vr.Success {
		return "", fmt.Errorf("API error: %s", string(b))
	}
	return vr.Result.Response, nil
}

type parsedFields struct {
	DocumentType   string  `json:"document_type"`
	DocumentNumber *string `json:"document_number"`
	FullName       *string `json:"full_name"`
	DateOfBirth    *string `json:"date_of_birth"`
	ExpiryDate     *string `json:"expiry_date"`
	IssuingCountry *string `json:"issuing_country"`
	Nationality    *string `json:"nationality"`
}

func parseJSON(raw, rawResp string) *domain.ExtractedData {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var pf parsedFields
	if json.Unmarshal([]byte(raw), &pf) != nil {
		return &domain.ExtractedData{DocumentType: "unknown", RawResponse: rawResp, OCRConfidence: 0.5}
	}
	return &domain.ExtractedData{
		DocumentType: pf.DocumentType, DocumentNumber: pf.DocumentNumber,
		FullName: pf.FullName, DateOfBirth: pf.DateOfBirth,
		ExpiryDate: pf.ExpiryDate, IssuingCountry: pf.IssuingCountry,
		Nationality: pf.Nationality, RawResponse: rawResp, OCRConfidence: 0.85,
	}
}

func min(a, b int) int { if a < b { return a }; return b }
