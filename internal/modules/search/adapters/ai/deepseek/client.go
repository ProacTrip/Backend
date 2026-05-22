// Cliente HTTP para DeepSeek V4 Flash.
// Comunica con la API OpenAI-compatible de DeepSeek para interpretación de lenguaje natural.
package deepseek

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL   = "https://api.deepseek.com"
	defaultTimeout   = 30 * time.Second
	defaultMaxTokens = 4096
)

// chatMessage represents a message in the OpenAI-compatible chat API.
type chatMessage struct {
	Role      string     `json:"role"`                // "system", "user", "assistant", "tool"
	Content   string     `json:"content,omitzero"`    // text content (empty for tool messages with ToolCallID)
	ToolCalls []ToolCall `json:"tool_calls,omitzero"` // tool calls requested by assistant
	ToolCallID string   `json:"tool_call_id,omitzero"` // tool use: which tool call this responds to
}

// chatCompletionRequest is the request body for /chat/completions (v4 Flash endpoint).
type chatCompletionRequest struct {
	Model           string          `json:"model"`
	Messages        []chatMessage   `json:"messages"`
	Temperature     float64         `json:"temperature"`
	MaxTokens       int             `json:"max_tokens"`
	TopP            float64         `json:"top_p,omitzero"`
	ResponseFormat  responseFormat  `json:"response_format"`
	Thinking        thinkingConfig  `json:"thinking"`
	ReasoningEffort string          `json:"reasoning_effort,omitzero"`
	StreamOptions   *streamOptions  `json:"stream_options,omitzero"`
	Tools           []ToolDef       `json:"tools,omitzero"`       // function definitions for tool calling
	ToolChoice      string          `json:"tool_choice,omitzero"` // "none", "auto", or specific function name
	Stream          bool            `json:"stream"`
}

// responseFormat requests JSON output from the model (when supported).
type responseFormat struct {
	Type string `json:"type"` // "json_object" or "text"
}

// thinkingConfig controls DeepSeek's reasoning/thinking capability.
type thinkingConfig struct {
	Type string `json:"type"` // "enabled" or "disabled"
}

// streamOptions controls streaming behavior (v4 Flash).
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// ToolDef represents a function tool definition for the OpenAI-compatible chat API.
type ToolDef struct {
	Type     string       `json:"type"`     // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction describes the function name, description, and JSON Schema parameters.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitzero"`
	Parameters  json.RawMessage `json:"parameters,omitzero"`
}

// ToolCall represents a tool call from the AI model in a chat response.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ChatCompletionParams holds per-request configurable parameters.
// Different use cases (exact search vs discovery) tune these differently.
type ChatCompletionParams struct {
	Temperature     float64
	MaxTokens       int
	TopP            float64
	ResponseFormat  string // "json_object" or "text"
	Thinking        thinkingConfig
	ReasoningEffort string  // "high" or "max" for v4 Flash reasoning
	StreamOptions   *streamOptions
	Tools           []ToolDef // function definitions for tool calling
	ToolChoice      string    // "none", "auto", or specific function name
}

// DefaultExactParams returns params tuned for exact parameter extraction:
// low temperature, JSON output, thinking disabled, no reasoning_effort
// (DeepSeek v4 rejects reasoning_effort when thinking is disabled).
func DefaultExactParams() ChatCompletionParams {
	return ChatCompletionParams{
		Temperature:    0.1,
		MaxTokens:      defaultMaxTokens,
		ResponseFormat: "json_object",
		Thinking:       thinkingConfig{Type: "disabled"},
	}
}

// DefaultDiscoveryParams returns params tuned for creative discovery:
// higher temperature, text output, thinking enabled, streaming support.
func DefaultDiscoveryParams() ChatCompletionParams {
	return ChatCompletionParams{
		Temperature:     0.7,
		MaxTokens:       defaultMaxTokens,
		TopP:            0.9,
		ResponseFormat:  "text",
		Thinking:        thinkingConfig{Type: "enabled"},
		ReasoningEffort: "high",
		StreamOptions:   &streamOptions{IncludeUsage: true},
	}
}

// chatCompletionChoice is a single choice in the response.
type chatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// chatCompletionUsage tracks token consumption.
type chatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// chatCompletionResponse is the response from /v1/chat/completions.
type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   chatCompletionUsage    `json:"usage"`
}

// apiErrorResponse is the error body returned by the API on failure.
type apiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Client performs HTTP calls to the DeepSeek OpenAI-compatible chat completions API.
type Client struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// ClientOpt is a functional option for Client construction.
type ClientOpt func(*Client)

// WithBaseURL overrides the default DeepSeek API base URL.
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

// NewClient creates a new DeepSeek HTTP client.
// apiKey can be empty — requests will fail with an authentication error at call time.
func NewClient(apiKey string, timeout time.Duration, opts ...ClientOpt) *Client {
	if apiKey == "" {
		slog.Warn("DEEPSEEK_API_KEY is empty — deepseek requests will fail")
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	c := &Client{
		baseURL: defaultBaseURL,
		apiKey:  apiKey,
		model:   "", // set via WithModel option or left empty
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ChatCompletion sends a chat completion request and returns the assistant's response text.
func (c *Client) ChatCompletion(ctx context.Context, messages []chatMessage, params ChatCompletionParams) (string, error) {
	reqBody := chatCompletionRequest{
		Model:           c.model,
		Messages:        messages,
		Temperature:     params.Temperature,
		MaxTokens:       params.MaxTokens,
		TopP:            params.TopP,
		ResponseFormat:  responseFormat{Type: params.ResponseFormat},
		Thinking:        params.Thinking,
		ReasoningEffort: params.ReasoningEffort,
		Tools:           params.Tools,
		ToolChoice:      params.ToolChoice,
		Stream:          false,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("deepseek marshal request: %w", err)
	}

	endpoint := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("deepseek create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	slog.DebugContext(ctx, "deepseek request",
		slog.String("model", c.model),
		slog.Int("messages", len(messages)),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("deepseek request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr apiErrorResponse
		if decodeErr := json.NewDecoder(resp.Body).Decode(&apiErr); decodeErr == nil && apiErr.Error.Message != "" {
			return "", fmt.Errorf("deepseek API error (HTTP %d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return "", fmt.Errorf("deepseek API returned HTTP %d", resp.StatusCode)
	}

	var result chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("deepseek decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("deepseek returned no choices")
	}

	slog.DebugContext(ctx, "deepseek response",
		slog.Int("prompt_tokens", result.Usage.PromptTokens),
		slog.Int("completion_tokens", result.Usage.CompletionTokens),
		slog.String("finish_reason", result.Choices[0].FinishReason),
	)

	return result.Choices[0].Message.Content, nil
}

// SSEEvent represents a single Server-Sent Event chunk from the streaming API.
type SSEEvent struct {
	Delta        string     `json:"delta"`                    // incremental text from the model
	ToolCalls    []ToolCall `json:"tool_calls,omitzero"`      // accumulated tool calls (on finish_reason: "tool_calls")
	FinishReason string     `json:"finish_reason,omitzero"`   // "stop", "tool_calls", etc.
	Done         bool       `json:"done"`                     // true when the stream is complete
	FullText     string     `json:"full_text,omitempty"`      // accumulated full response (only on final chunk)
}

// ChatCompletionStream sends a streaming chat completion request.
// Returns a channel of SSE events that the caller reads until the channel is closed.
// The caller MUST read the channel to completion or cancel the context to avoid leaks.
func (c *Client) ChatCompletionStream(ctx context.Context, messages []chatMessage, params ChatCompletionParams) (<-chan SSEEvent, error) {
	reqBody := chatCompletionRequest{
		Model:           c.model,
		Messages:        messages,
		Temperature:     params.Temperature,
		MaxTokens:       params.MaxTokens,
		TopP:            params.TopP,
		ResponseFormat:  responseFormat{Type: params.ResponseFormat},
		Thinking:        params.Thinking,
		ReasoningEffort: params.ReasoningEffort,
		StreamOptions:   params.StreamOptions,
		Tools:           params.Tools,
		ToolChoice:      params.ToolChoice,
		Stream:          true,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("deepseek marshal request: %w", err)
	}

	endpoint := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("deepseek create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek stream request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		var apiErr apiErrorResponse
		if decodeErr := json.NewDecoder(resp.Body).Decode(&apiErr); decodeErr == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("deepseek API error (HTTP %d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("deepseek API returned HTTP %d", resp.StatusCode)
	}

	ch := make(chan SSEEvent, 16)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		// Increase buffer for larger SSE lines
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		var fullText strings.Builder
		var accumulatedToolCalls []ToolCall
		// toolCallAcc accumulates fragmented tool_call arguments during streaming.
		type toolCallAccumulator struct {
			ID        string
			Type      string
			FuncName  string
			Arguments strings.Builder
		}
		toolCallAcc := make(map[int]*toolCallAccumulator)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()

			// SSE data lines start with "data: "
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			// "[DONE]" signals end of stream
			if data == "[DONE]" {
				ch <- SSEEvent{Done: true, FullText: fullText.String()}
				return
			}

			// Parse the JSON chunk
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content   string `json:"content"`
						ToolCalls []struct {
							Index    int    `json:"index"`
							ID       string `json:"id"`
							Type     string `json:"type"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
			}

			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				slog.WarnContext(ctx, "deepseek stream: unmarshal chunk failed",
					slog.String("error", err.Error()),
				)
				continue
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta.Content
			if delta != "" {
				fullText.WriteString(delta)
				ch <- SSEEvent{Delta: delta}
			}

			// Accumulate tool call deltas
			for _, tcDelta := range chunk.Choices[0].Delta.ToolCalls {
				if acc, exists := toolCallAcc[tcDelta.Index]; exists {
					// Append arguments fragment
					acc.Arguments.WriteString(tcDelta.Function.Arguments)
				} else {
					acc := &toolCallAccumulator{
						ID:       tcDelta.ID,
						Type:     tcDelta.Type,
						FuncName: tcDelta.Function.Name,
					}
					acc.Arguments.WriteString(tcDelta.Function.Arguments)
					toolCallAcc[tcDelta.Index] = acc
				}
			}

			// Check if the model signalled finish
			if chunk.Choices[0].FinishReason != nil {
				finishReason := *chunk.Choices[0].FinishReason

				// If tool_calls finish reason → resolve accumulated tool calls
				if finishReason == "tool_calls" {
					for idx := 0; idx < len(toolCallAcc); idx++ {
						if acc, ok := toolCallAcc[idx]; ok {
							tc := ToolCall{
								ID:   acc.ID,
								Type: acc.Type,
								Function: ToolFunction{
									Name: acc.FuncName,
								},
							}
							// Store the accumulated arguments JSON as a raw message
							argsJSON := acc.Arguments.String()
							if argsJSON != "" {
								tc.Function.Parameters = json.RawMessage(argsJSON)
							}
							accumulatedToolCalls = append(accumulatedToolCalls, tc)
						}
					}
					ch <- SSEEvent{
						Done:         true,
						FinishReason: finishReason,
						ToolCalls:    accumulatedToolCalls,
						FullText:     fullText.String(),
					}
					return
				}

				ch <- SSEEvent{Done: true, FullText: fullText.String(), FinishReason: finishReason}
				return
			}
		}

		// Scanner ended (connection closed or error) — send final event
		if err := scanner.Err(); err != nil {
			slog.WarnContext(ctx, "deepseek stream: scanner error",
				slog.String("error", err.Error()),
			)
		}
		ch <- SSEEvent{Done: true, FullText: fullText.String()}
	}()

	return ch, nil
}
