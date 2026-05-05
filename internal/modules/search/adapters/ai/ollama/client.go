// Cliente HTTP para Ollama local.
// Comunica con la API nativa de Ollama para interpretación de lenguaje natural.
// Ollama es un runtime local de LLMs — no requiere API key.
//
// API nativa de Ollama:
//
//	POST /api/chat → {"model":"llama3","messages":[...],"stream":false,"format":"json"}
//	Respuesta:       {"model":"llama3","message":{"role":"assistant","content":"..."},"done":true}
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const (
	defaultBaseURL     = "http://localhost:11434"
	defaultModel       = "llama3.2"
	defaultTimeout     = 60 * time.Second
)

// =============================================================================
// Ollama native API types
// =============================================================================

// chatMessage represents a message in the Ollama native chat API.
type chatMessage struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"`
}

// ollamaChatRequest is the request body for POST /api/chat (Ollama native API).
type ollamaChatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Format   string        `json:"format,omitempty"`
}

// ollamaChatResponse is the response from POST /api/chat (Ollama native API).
type ollamaChatResponse struct {
	Model     string       `json:"model"`
	Message   chatMessage  `json:"message"`
	Done      bool         `json:"done"`
	DoneReason string      `json:"done_reason,omitempty"`
}

// ollamaErrorResponse is the error body returned by Ollama on failure.
type ollamaErrorResponse struct {
	Error string `json:"error"`
}

// =============================================================================
// Client
// =============================================================================

// Client performs HTTP calls to the Ollama native chat API.
type Client struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// ClientOpt is a functional option for Client construction.
type ClientOpt func(*Client)

// WithBaseURL overrides the default Ollama API base URL (no /v1 suffix).
func WithBaseURL(url string) ClientOpt {
	return func(c *Client) { c.baseURL = url }
}

// WithModel overrides the default model name.
func WithModel(model string) ClientOpt {
	return func(c *Client) { c.model = model }
}

// WithHTTPClient sets a custom http.Client (useful for testing).
func WithHTTPClient(hc *http.Client) ClientOpt {
	return func(c *Client) { c.httpClient = hc }
}

// NewClient creates a new Ollama HTTP client.
// Ollama is local — no API key required.
func NewClient(timeout time.Duration, opts ...ClientOpt) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	c := &Client{
		baseURL: defaultBaseURL,
		model:   defaultModel,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ChatCompletion sends a chat request to Ollama's native /api/chat endpoint
// and returns the assistant's response text.
func (c *Client) ChatCompletion(ctx context.Context, messages []chatMessage) (string, error) {
	reqBody := ollamaChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
		Format:   "json",
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ollama marshal request: %w", err)
	}

	// Ollama native endpoint: POST /api/chat (not /v1/chat/completions)
	endpoint := c.baseURL + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("ollama create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	slog.DebugContext(ctx, "ollama request",
		slog.String("model", c.model),
		slog.Int("messages", len(messages)),
		slog.String("endpoint", endpoint),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr ollamaErrorResponse
		if decodeErr := json.NewDecoder(resp.Body).Decode(&apiErr); decodeErr == nil && apiErr.Error != "" {
			return "", fmt.Errorf("ollama API error (HTTP %d): %s", resp.StatusCode, apiErr.Error)
		}
		return "", fmt.Errorf("ollama API returned HTTP %d", resp.StatusCode)
	}

	var result ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama decode response: %w", err)
	}

	if result.Message.Content == "" {
		return "", fmt.Errorf("ollama returned empty response content (done_reason=%q)", result.DoneReason)
	}

	slog.DebugContext(ctx, "ollama response",
		slog.String("model", result.Model),
		slog.Bool("done", result.Done),
		slog.String("done_reason", result.DoneReason),
	)

	return result.Message.Content, nil
}
