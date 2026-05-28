// Adapter: OCR vía Cloudflare Workers AI con llama-3.2-11b-vision-instruct.
// Modelo con soporte nativo de visión + JSON Mode (response_format).
package cloudflare

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/gen2brain/go-fitz"
)

const cloudflareModel = "@cf/meta/llama-3.2-11b-vision-instruct"

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

// Cloudflare Workers AI espera el JSON Schema directamente, sin el wrapper
// de OpenAI {name, schema}. Ver: https://developers.cloudflare.com/workers-ai/
var ocrResponseSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"document_type":   map[string]interface{}{"type": "string", "enum": []string{"passport", "national_id", "drivers_license", "visa", "travel_insurance", "vaccination_cert", "boarding_pass", "receipt", "unknown"}},
		"document_number": map[string]interface{}{"type": "string"},
		"full_name":       map[string]interface{}{"type": "string"},
		"date_of_birth":   map[string]interface{}{"type": "string"},
		"expiry_date":     map[string]interface{}{"type": "string"},
		"issuing_country": map[string]interface{}{"type": "string"},
		"nationality":     map[string]interface{}{"type": "string"},
		"gender":          map[string]interface{}{"type": "string"},
	},
	"required": []string{"document_type"},
}

const ocrSystemPrompt = `Eres un experto en OCR de documentos de viaje. Extrae los datos como JSON estructurado.

Tipos de documento: passport (pasaporte), national_id (DNI/identidad), drivers_license (licencia de conducir), visa, travel_insurance (seguro de viaje), vaccination_cert (certificado médico/vacunación), boarding_pass (tarjeta de embarque), receipt (recibo/factura), unknown (desconocido).

Campos a extraer (string vacío "" si no está presente):
- document_type: uno del enum
- document_number: número de documento
- full_name: nombre completo (APELLIDOS Nombres)
- date_of_birth: YYYY-MM-DD
- expiry_date: YYYY-MM-DD
- issuing_country: código o nombre del país emisor
- nationality: código o nombre de nacionalidad
- gender: M o F`

type visionResponse struct {
	Success bool              `json:"success"`
	Result  struct {
		// Response puede ser string (sin response_format) u objeto JSON
		// (con response_format: json_schema o json_object).
		Response json.RawMessage `json:"response"`
	} `json:"result"`
	Errors []struct{ Message string } `json:"errors"`
}

type parsedFields struct {
	DocumentType   string `json:"document_type"`
	DocumentNumber string `json:"document_number"`
	FullName       string `json:"full_name"`
	DateOfBirth    string `json:"date_of_birth"`
	ExpiryDate     string `json:"expiry_date"`
	IssuingCountry string `json:"issuing_country"`
	Nationality    string `json:"nationality"`
	Gender         string `json:"gender"`
}

func (c *OCRClient) ExtractFromDocument(ctx context.Context, fileURL string) (*domain.ExtractedData, error) {
	slog.Info("ocr: downloading")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	fileBytes, _ := io.ReadAll(resp.Body)

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
			// Resize to max 1600px — Cloudflare AI has limits on input size.
			// Large scanned documents (300+ DPI) produce huge images that cause
			// empty responses from the model.
			img = resizeToMax(img, 1600)
			var buf bytes.Buffer
			jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
			pageB64 = append(pageB64, base64.StdEncoding.EncodeToString(buf.Bytes()))
		}
		slog.Info("ocr: PDF to images", "pages", len(pageB64))
	} else {
		pageB64 = []string{base64.StdEncoding.EncodeToString(fileBytes)}
	}

	for i, b64 := range pageB64 {
		slog.Info("ocr: page", "n", i+1, "b64_len", len(b64))
		result, err := c.extractJSON(ctx, b64)
		if err != nil {
			slog.Warn("ocr: page failed", "page", i+1, "error", err)
			continue
		}
		if result.DocumentType != "unknown" {
			slog.Info("ocr: result", "type", result.DocumentType,
				"name", result.FullName, "dob", result.DateOfBirth,
				"gender", result.Gender, "nationality", result.Nationality)
			return result, nil
		}
	}

	return &domain.ExtractedData{DocumentType: "unknown", OCRConfidence: 0}, nil
}

func (c *OCRClient) extractJSON(ctx context.Context, b64 string) (*domain.ExtractedData, error) {
	// Formato documentado de Cloudflare Workers AI para modelos vision:
	// "image" como campo top-level con data URI, "messages" para el texto,
	// y "response_format" para structured output.
	// Ver: https://developers.cloudflare.com/workers-ai/configuration/structured-outputs/
	raw, err := c.call(ctx, map[string]interface{}{
		"messages": []map[string]string{
			{"role": "system", "content": ocrSystemPrompt},
			{"role": "user", "content": "Extrae los datos estructurados de este documento de viaje. Responde SOLO con el JSON, sin explicaciones ni markdown."},
		},
		"image": "data:image/jpeg;base64," + b64,
		"response_format": map[string]interface{}{
			"type":        "json_schema",
			"json_schema": ocrResponseSchema,
		},
		"max_tokens": 1024,
	})
	if err != nil {
		return nil, err
	}
	return parseStructured(raw), nil
}

func parseStructured(raw string) *domain.ExtractedData {
	raw = strings.TrimSpace(raw)

	// 0. Remove markdown code fences anywhere in the response
	// (AI often wraps JSON in ```json ... ``` or ``` ... ``` inline)
	raw = removeMarkdownCodeFences(raw)

	// 1. Intentar JSON primero
	jsonStr := extractJSONObject(raw)
	if jsonStr != "" {
		var pf parsedFields
		if err := json.Unmarshal([]byte(jsonStr), &pf); err == nil {
			return fieldsToResult(pf, raw)
		} else {
			slog.Warn("ocr: JSON parse failed, falling back to markdown", "json", jsonStr[:min(len(jsonStr), 200)], "error", err)
		}
	}

	// 2. Parsear markdown: **campo**: valor o campo: valor
	pf := parseMarkdownFields(raw)
	if pf.DocumentType != "" {
		return fieldsToResult(pf, raw)
	}

	slog.Warn("ocr: no JSON or markdown fields found", "raw", raw[:min(len(raw), 200)])
	return &domain.ExtractedData{DocumentType: "unknown", OCRConfidence: 0.3}
}

// removeMarkdownCodeFences strips ```json and ``` markers from the string,
// even if they're not at the very start/end.
func removeMarkdownCodeFences(raw string) string {
	// Remove ```json or ``` at the start
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```json") {
		raw = strings.TrimPrefix(raw, "```json")
	} else if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```")
	}
	raw = strings.TrimSpace(raw)
	// Remove trailing ```
	if idx := strings.LastIndex(raw, "```"); idx >= 0 && idx > len(raw)-10 {
		raw = strings.TrimSpace(raw[:idx])
	}
	return raw
}

// parseMarkdownFields extrae campos de formato **key**: value o key: value.
func parseMarkdownFields(raw string) parsedFields {
	var pf parsedFields
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "- ")
		// Remover bold markers
		line = strings.ReplaceAll(line, "**", "")
		line = strings.ReplaceAll(line, "__", "")

		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		// Limpiar comillas, puntuación y espacios
		val = strings.Trim(val, "\"'")
		val = strings.Trim(val, ",;.")

		switch key {
		case "document_type", "tipo de documento", "document type":
			pf.DocumentType = strings.ToLower(val)
		case "document_number", "número de documento", "document number":
			pf.DocumentNumber = val
		case "full_name", "nombre completo", "full name":
			pf.FullName = val
		case "date_of_birth", "fecha de nacimiento", "date of birth":
			pf.DateOfBirth = val
		case "expiry_date", "fecha de vencimiento", "expiry date":
			pf.ExpiryDate = val
		case "issuing_country", "país emisor", "issuing country":
			pf.IssuingCountry = val
		case "nationality", "nacionalidad":
			pf.Nationality = val
		case "gender", "género", "sexo", "sex":
			pf.Gender = val
		}
	}
	return pf
}

func fieldsToResult(pf parsedFields, raw string) *domain.ExtractedData {
	out := &domain.ExtractedData{DocumentType: pf.DocumentType, OCRConfidence: 0.9, RawResponse: raw}
	if pf.DocumentNumber != "" {
		out.DocumentNumber = &pf.DocumentNumber
	}
	if pf.FullName != "" {
		out.FullName = &pf.FullName
	}
	if pf.DateOfBirth != "" {
		out.DateOfBirth = &pf.DateOfBirth
	}
	if pf.ExpiryDate != "" {
		out.ExpiryDate = &pf.ExpiryDate
	}
	if pf.IssuingCountry != "" {
		out.IssuingCountry = &pf.IssuingCountry
	}
	if pf.Nationality != "" {
		out.Nationality = &pf.Nationality
	}
	if pf.Gender != "" {
		out.Gender = &pf.Gender
	}
	return out
}

func (c *OCRClient) call(ctx context.Context, body map[string]interface{}) (string, error) {
	jsonBody, _ := json.Marshal(body)
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/run/%s", c.accountID, cloudflareModel)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	// Auto-aceptar licencia del modelo si es necesario (one-time per account)
	if resp.StatusCode == 403 && strings.Contains(string(b), "Model Agreement") {
		slog.Info("ocr: accepting model license agreement")
		c.acceptLicense(ctx)
		// Retry una vez
		req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
		req2.Header.Set("Authorization", "Bearer "+c.apiToken)
		req2.Header.Set("Content-Type", "application/json")
		resp2, err2 := c.client.Do(req2)
		if err2 != nil {
			return "", err2
		}
		defer resp2.Body.Close()
		b2, _ := io.ReadAll(resp2.Body)
		if resp2.StatusCode != 200 {
			return "", fmt.Errorf("HTTP %d after license: %s", resp2.StatusCode, string(b2))
		}
		b = b2
	} else if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	var vr visionResponse
	if err := json.Unmarshal(b, &vr); err != nil {
		return "", fmt.Errorf("Cloudflare response parse error: %w (body: %.200s)", err, string(b))
	}
	if !vr.Success {
		return "", fmt.Errorf("Cloudflare API error (success=false): %s", string(b))
	}
	// El campo response puede venir como string (sin structured output)
	// o como objeto JSON (con response_format: json_schema/json_object).
	raw := string(vr.Result.Response)
	if raw == "" || raw == "null" {
		slog.Warn("ocr: Cloudflare returned empty/null response",
			"status", resp.StatusCode,
			"body_preview", string(b)[:min(len(string(b)), 300)],
		)
		return "", nil
	}
	// Si es un objeto JSON (empieza con '{'), usarlo tal cual.
	// Si es un string (empieza con '"'), extraer el contenido.
	if raw[0] == '{' || raw[0] == '[' {
		return raw, nil
	}
	// Intentar desempaquetar si viene como string JSON escapado
	var strContent string
	if err := json.Unmarshal(vr.Result.Response, &strContent); err == nil {
		return strContent, nil
	}
	return raw, nil
}

func (c *OCRClient) acceptLicense(ctx context.Context) {
	agreeBody, _ := json.Marshal(map[string]string{"prompt": "agree"})
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/run/%s", c.accountID, cloudflareModel)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(agreeBody))
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		slog.Warn("ocr: license accept failed", "error", err)
		return
	}
	resp.Body.Close()
}

func min(a, b int) int { if a < b { return a }; return b }

// resizeToMax scales an image down so its longest side is at most maxPx.
// Uses nearest-neighbor for speed (OCR text remains readable at 1600px).
func resizeToMax(src image.Image, maxPx int) *image.RGBA {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= maxPx && h <= maxPx {
		if rgba, ok := src.(*image.RGBA); ok {
			return rgba
		}
		// Convert to RGBA
		dst := image.NewRGBA(bounds)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(x, y, src.At(x, y))
			}
		}
		return dst
	}
	var nw, nh int
	if w > h {
		nw = maxPx
		nh = h * maxPx / w
	} else {
		nh = maxPx
		nw = w * maxPx / h
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			sx := x * w / nw
			sy := y * h / nh
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// extractJSONObject extrae el primer objeto JSON de una respuesta que puede
// tener texto, markdown o código alrededor (ej: "El documento es... ```json\n{...}\n```").
func extractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)

	// If it starts with {, extract balanced JSON
	if strings.HasPrefix(raw, "{") {
		return extractBalancedJSON(raw)
	}

	// Find first { and extract from there
	idx := strings.Index(raw, "{")
	if idx >= 0 {
		return extractBalancedJSON(raw[idx:])
	}

	return ""
}

func extractBalancedJSON(s string) string {
	depth := 0
	inString := false
	escaped := false
	for i, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inString {
			escaped = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if r == '{' {
			depth++
		} else if r == '}' {
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return ""
}
