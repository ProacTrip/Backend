// ToolCallStreamer adapter — wraps Adapter to implement domain.ToolCallStreamer.
// Converts domain types (ChatMessage, map[string]interface{}) to the deepseek
// adapter's internal types (chatMessage, ToolDef) and back.
package deepseek

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Compile-time interface check
// =============================================================================

var _ domain.ToolCallStreamer = (*ToolCallStreamerAdapter)(nil)

// =============================================================================
// ToolCallStreamerAdapter
// =============================================================================

// ToolCallStreamerAdapter implements domain.ToolCallStreamer by wrapping
// the deepseek Adapter and converting between domain and internal types.
type ToolCallStreamerAdapter struct {
	adapter *Adapter
}

// NewToolCallStreamer creates a domain.ToolCallStreamer from a deepseek Adapter.
// Returns nil if adapter is nil (caller should handle this case).
func NewToolCallStreamer(adapter *Adapter) domain.ToolCallStreamer {
	if adapter == nil {
		return nil
	}
	return &ToolCallStreamerAdapter{adapter: adapter}
}

// ChatWithTools converts domain types to deepseek internal types, calls the
// adapter's streaming ChatWithTools, and converts the result back to domain types.
func (s *ToolCallStreamerAdapter) ChatWithTools(ctx context.Context, messages []domain.ChatMessage, tools []map[string]interface{}) (*domain.ToolCallStreamResult, error) {
	return s.ChatWithToolsStream(ctx, messages, tools, nil)
}

// ChatWithToolsStream calls the adapter's ChatWithToolsStream with an onChunk
// callback for real-time token streaming.
func (s *ToolCallStreamerAdapter) ChatWithToolsStream(ctx context.Context, messages []domain.ChatMessage, tools []map[string]interface{}, onChunk func(text string)) (*domain.ToolCallStreamResult, error) {
	// Convert domain.ChatMessage → chatMessage
	chatMsgs, err := toChatMessages(messages)
	if err != nil {
		return nil, fmt.Errorf("convert messages: %w", err)
	}

	// Convert []map[string]interface{} → []ToolDef via JSON roundtrip
	toolDefs, err := toToolDefs(tools)
	if err != nil {
		return nil, fmt.Errorf("convert tools: %w", err)
	}

	// Call the adapter with streaming callback
	result, err := s.adapter.ChatWithToolsStream(ctx, chatMsgs, toolDefs, onChunk)
	if err != nil {
		return nil, err
	}

	// Convert AdapterToolCallResult → domain.ToolCallStreamResult
	return toStreamResult(result), nil
}

// =============================================================================
// Type converters
// =============================================================================

// toChatMessages converts []domain.ChatMessage to []chatMessage.
// Tool calls in assistant messages are converted from domain.ToolCall to
// deepseek ToolCall (with Function.Parameters holding the JSON-encoded arguments).
func toChatMessages(messages []domain.ChatMessage) ([]chatMessage, error) {
	chatMsgs := make([]chatMessage, len(messages))
	for i, m := range messages {
		chatMsgs[i] = chatMessage{
			Role:             m.Role,
			Content:          m.Content,
			ToolCallID:       m.ToolCallID,
			ReasoningContent: m.ReasoningContent,
		}
		if len(m.ToolCalls) > 0 {
			chatMsgs[i].ToolCalls = make([]ToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				tcOut := ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: ToolFunction{
						Name: tc.Name,
					},
				}
				if tc.Arguments != nil {
					argsJSON, err := json.Marshal(tc.Arguments)
					if err != nil {
						return nil, fmt.Errorf("marshal tool call %d arguments: %w", j, err)
					}
					tcOut.Function.Arguments = string(argsJSON)
				}
				chatMsgs[i].ToolCalls[j] = tcOut
			}
		}
	}
	return chatMsgs, nil
}

// toToolDefs converts []map[string]interface{} to []ToolDef via JSON roundtrip.
// The input maps are produced by buildDefaultTools() in the handler layer,
// which serializes ai_search.ToolDef types to JSON and back to maps.
func toToolDefs(tools []map[string]interface{}) ([]ToolDef, error) {
	toolDefs := make([]ToolDef, len(tools))
	for i, t := range tools {
		raw, err := json.Marshal(t)
		if err != nil {
			return nil, fmt.Errorf("marshal tool %d: %w", i, err)
		}
		if err := json.Unmarshal(raw, &toolDefs[i]); err != nil {
			return nil, fmt.Errorf("unmarshal tool %d: %w", i, err)
		}
	}
	return toolDefs, nil
}

// toStreamResult converts an AdapterToolCallResult to a domain.ToolCallStreamResult.
func toStreamResult(result *AdapterToolCallResult) *domain.ToolCallStreamResult {
	streamResult := &domain.ToolCallStreamResult{
		AssistantText:    result.AssistantMessage,
		ReasoningContent: result.ReasoningContent,
	}
	if len(result.ToolCalls) > 0 {
		streamResult.ToolCalls = make([]domain.ToolCall, len(result.ToolCalls))
		for i, tc := range result.ToolCalls {
			streamResult.ToolCalls[i] = domain.ToolCall{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			}
		}
	}
	return streamResult
}
