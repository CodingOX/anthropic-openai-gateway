package transformer

import (
	"encoding/json"
	"strings"
	"testing"

	"anthropic-openai-gateway/pkg/types"
)

func TestTransformRequestSplitsMultipleToolResultsAndKeepsUserText(t *testing.T) {
	req := &types.MessageRequest{
		Model:     "gpt-4o",
		MaxTokens: 128,
		Messages: []types.Message{
			{
				Role: "user",
				Content: []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "call_1",
						"content":     "first result",
					},
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "call_2",
						"content": []interface{}{
							map[string]interface{}{"type": "text", "text": "second result"},
						},
					},
					map[string]interface{}{"type": "text", "text": "continue"},
				},
			},
		},
	}

	openaiReq, err := NewRequestTransformer().TransformRequest(req)
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 3; got != want {
		t.Fatalf("len(Messages) = %d, want %d: %#v", got, want, openaiReq.Messages)
	}
	if got, want := openaiReq.Messages[0].Role, "tool"; got != want {
		t.Fatalf("message[0].Role = %q, want %q", got, want)
	}
	if got, want := openaiReq.Messages[0].ToolCallID, "call_1"; got != want {
		t.Fatalf("message[0].ToolCallID = %q, want %q", got, want)
	}
	if got, want := openaiReq.Messages[0].Content, "first result"; got != want {
		t.Fatalf("message[0].Content = %#v, want %q", got, want)
	}
	if got, want := openaiReq.Messages[1].ToolCallID, "call_2"; got != want {
		t.Fatalf("message[1].ToolCallID = %q, want %q", got, want)
	}
	if got, want := openaiReq.Messages[1].Content, "second result"; got != want {
		t.Fatalf("message[1].Content = %#v, want %q", got, want)
	}
	if got, want := openaiReq.Messages[2].Role, "user"; got != want {
		t.Fatalf("message[2].Role = %q, want %q", got, want)
	}
	if got, want := openaiReq.Messages[2].Content, "continue"; got != want {
		t.Fatalf("message[2].Content = %#v, want %q", got, want)
	}
}

func TestTransformRequestPreservesThinkingAsReasoningContent(t *testing.T) {
	req := &types.MessageRequest{
		Model:     "gpt-4o",
		MaxTokens: 128,
		Messages: []types.Message{
			{
				Role: "assistant",
				Content: []interface{}{
					map[string]interface{}{"type": "thinking", "thinking": "reason carefully"},
					map[string]interface{}{"type": "text", "text": "answer"},
				},
			},
		},
	}

	openaiReq, err := NewRequestTransformer().TransformRequest(req)
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	msg := openaiReq.Messages[0]
	if msg.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want non-nil")
	}
	if got, want := *msg.ReasoningContent, "reason carefully"; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}

	body, err := json.Marshal(openaiReq)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !json.Valid(body) || !containsJSONField(body, "reasoning_content") {
		t.Fatalf("serialized request missing reasoning_content: %s", body)
	}
}

func TestTransformRequestKeepsAssistantMessageWithOnlyThinking(t *testing.T) {
	req := &types.MessageRequest{
		Model:     "gpt-4o",
		MaxTokens: 128,
		Messages: []types.Message{
			{
				Role: "assistant",
				Content: []interface{}{
					map[string]interface{}{"type": "thinking", "thinking": "intermediate reasoning"},
				},
			},
		},
	}

	openaiReq, err := NewRequestTransformer().TransformRequest(req)
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d: %#v", got, want, openaiReq.Messages)
	}
	msg := openaiReq.Messages[0]
	if got, want := msg.Role, "assistant"; got != want {
		t.Fatalf("Role = %q, want %q", got, want)
	}
	if msg.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want preserved thinking-only assistant message")
	}
	if got, want := *msg.ReasoningContent, "intermediate reasoning"; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}
}

func TestTransformRequestKeepsAssistantMessageWithOmittedThinking(t *testing.T) {
	req := &types.MessageRequest{
		Model:     "gpt-4o",
		MaxTokens: 128,
		Messages: []types.Message{
			{
				Role: "assistant",
				Content: []interface{}{
					map[string]interface{}{"type": "thinking", "thinking": "", "signature": "sig_123"},
					map[string]interface{}{"type": "text", "text": "tool follow-up"},
				},
			},
		},
	}

	openaiReq, err := NewRequestTransformer().TransformRequest(req)
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d: %#v", got, want, openaiReq.Messages)
	}
	msg := openaiReq.Messages[0]
	if msg.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want placeholder for omitted thinking block")
	}
	if got := *msg.ReasoningContent; strings.TrimSpace(got) == "" {
		t.Fatalf("ReasoningContent = %q, want non-blank placeholder", got)
	}
	if got, want := *msg.ReasoningContent, reasoningReplayPlaceholder; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}
	if got, want := msg.Content, "tool follow-up"; got != want {
		t.Fatalf("Content = %#v, want %q", got, want)
	}
}

func TestTransformRequestKeepsAssistantMessageWithRedactedThinking(t *testing.T) {
	req := &types.MessageRequest{
		Model:     "gpt-4o",
		MaxTokens: 128,
		Messages: []types.Message{
			{
				Role: "assistant",
				Content: []interface{}{
					map[string]interface{}{"type": "redacted_thinking", "data": "opaque"},
					map[string]interface{}{"type": "tool_use", "id": "call_1", "name": "lookup", "input": map[string]interface{}{"q": "x"}},
				},
			},
		},
	}

	openaiReq, err := NewRequestTransformer().TransformRequest(req)
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d: %#v", got, want, openaiReq.Messages)
	}
	msg := openaiReq.Messages[0]
	if msg.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want placeholder for redacted thinking block")
	}
	if got := *msg.ReasoningContent; strings.TrimSpace(got) == "" {
		t.Fatalf("ReasoningContent = %q, want non-blank placeholder", got)
	}
	if got, want := *msg.ReasoningContent, reasoningReplayPlaceholder; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}
	if got := len(msg.ToolCalls); got != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", got)
	}
}

func TestTransformRequestAddsReasoningPlaceholderWhenTopLevelThinkingEnabled(t *testing.T) {
	req := &types.MessageRequest{
		Model:     "gpt-4o",
		MaxTokens: 128,
		Thinking: map[string]interface{}{
			"type": "enabled",
		},
		Messages: []types.Message{
			{
				Role:    "assistant",
				Content: "answer without explicit thinking block",
			},
		},
	}

	openaiReq, err := NewRequestTransformer().TransformRequest(req)
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	msg := openaiReq.Messages[0]
	if msg.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want placeholder when top-level thinking is enabled")
	}
	if got := *msg.ReasoningContent; strings.TrimSpace(got) == "" {
		t.Fatalf("ReasoningContent = %q, want non-blank placeholder", got)
	}
	if got, want := *msg.ReasoningContent, reasoningReplayPlaceholder; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}
	if got, want := msg.Content, "answer without explicit thinking block"; got != want {
		t.Fatalf("Content = %#v, want %q", got, want)
	}
	body, err := json.Marshal(openaiReq)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !containsJSONField(body, "reasoning_content") {
		t.Fatalf("serialized request missing reasoning_content placeholder: %s", body)
	}
}

func TestTransformRequestAddsReasoningPlaceholderForReplayModels(t *testing.T) {
	models := []string{
		"deepseek-v4-flash",
		"qwen3.6-plus",
		"glm-5.1",
		"kimi-k2.6",
		"kimi-k2.5",
	}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			req := &types.MessageRequest{
				Model:     model,
				MaxTokens: 128,
				Messages: []types.Message{
					{
						Role:    "assistant",
						Content: "compressed prior answer without explicit thinking block",
					},
					{
						Role:    "user",
						Content: "continue",
					},
				},
			}

			openaiReq, err := NewRequestTransformer().TransformRequest(req)
			if err != nil {
				t.Fatalf("TransformRequest() error = %v", err)
			}

			msg := openaiReq.Messages[0]
			if msg.ReasoningContent == nil {
				t.Fatal("ReasoningContent = nil, want placeholder for reasoning replay")
			}
			if got := *msg.ReasoningContent; strings.TrimSpace(got) == "" {
				t.Fatalf("ReasoningContent = %q, want non-blank placeholder", got)
			}
			if got, want := *msg.ReasoningContent, reasoningReplayPlaceholder; got != want {
				t.Fatalf("ReasoningContent = %q, want %q", got, want)
			}
			if got, want := msg.Content, "compressed prior answer without explicit thinking block"; got != want {
				t.Fatalf("Content = %#v, want %q", got, want)
			}
		})
	}
}

func containsJSONField(body []byte, field string) bool {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	raw, _ := json.Marshal(payload)
	return json.Valid(raw) && jsonContains(raw, `"`+field+`"`)
}

func jsonContains(body []byte, needle string) bool {
	for i := 0; i+len(needle) <= len(body); i++ {
		if string(body[i:i+len(needle)]) == needle {
			return true
		}
	}
	return false
}

func TestTransformRequestUsesDeveloperRoleForOAndGPT5Series(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{"gpt-5 uses developer", "gpt-5", "developer"},
		{"gpt-5.4 uses developer", "gpt-5.4", "developer"},
		{"gpt-5-mini uses developer", "gpt-5-mini", "developer"},
		{"o3 uses developer", "o3", "developer"},
		{"o4-mini uses developer", "o4-mini", "developer"},
		{"gpt-4o uses system", "gpt-4o", "system"},
		{"gpt-4.1 uses system", "gpt-4.1", "system"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &types.MessageRequest{
				Model:     tt.model,
				System:    "You are helpful.",
				MaxTokens: 128,
				Messages:  []types.Message{{Role: "user", Content: "hi"}},
			}
			got, err := NewRequestTransformer().TransformRequest(req)
			if err != nil {
				t.Fatalf("TransformRequest() error = %v", err)
			}
			if len(got.Messages) == 0 || got.Messages[0].Role != tt.want {
				t.Fatalf("first message role = %q, want %q for model %s",
					got.Messages[0].Role, tt.want, tt.model)
			}
		})
	}
}

func TestTransformRequestUsesMaxTokensForUpstreamCompatibility(t *testing.T) {
	req := &types.MessageRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: 50,
		Messages:  []types.Message{{Role: "user", Content: "hi"}},
	}

	openaiReq, err := NewRequestTransformer().TransformRequest(req)
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.MaxTokens == nil {
		t.Fatal("MaxTokens = nil, want non-nil")
	}
	if got, want := *openaiReq.MaxTokens, 50; got != want {
		t.Fatalf("MaxTokens = %d, want %d", got, want)
	}
	if openaiReq.MaxCompletionTokens != nil {
		t.Fatalf("MaxCompletionTokens = %v, want nil", *openaiReq.MaxCompletionTokens)
	}

	body, err := json.Marshal(openaiReq)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !containsJSONField(body, "max_tokens") {
		t.Fatalf("serialized request missing max_tokens: %s", body)
	}
	if containsJSONField(body, "max_completion_tokens") {
		t.Fatalf("serialized request should not contain max_completion_tokens: %s", body)
	}
}

func TestTransformRequestPreservesCacheControlOnUserContentPart(t *testing.T) {
	req := &types.MessageRequest{
		Model:     "deepseek-v4-flash",
		MaxTokens: 128,
		Messages: []types.Message{
			{
				Role: "user",
				Content: []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "stable prefix",
						"cache_control": map[string]interface{}{
							"type": "ephemeral",
						},
					},
				},
			},
		},
	}

	openaiReq, err := NewRequestTransformer().TransformRequest(req)
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	parts, ok := openaiReq.Messages[0].Content.([]types.ChatContentPart)
	if !ok {
		t.Fatalf("Content type = %T, want []types.ChatContentPart", openaiReq.Messages[0].Content)
	}
	if got, want := len(parts), 1; got != want {
		t.Fatalf("len(parts) = %d, want %d", got, want)
	}
	if parts[0].CacheControl == nil {
		t.Fatal("CacheControl = nil, want preserved cache_control")
	}
	if got, want := parts[0].CacheControl.Type, "ephemeral"; got != want {
		t.Fatalf("CacheControl.Type = %q, want %q", got, want)
	}
}

func TestTransformRequestPreservesCacheControlOnSystemContentPart(t *testing.T) {
	req := &types.MessageRequest{
		Model:     "deepseek-v4-flash",
		MaxTokens: 128,
		System: []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "system prefix",
				"cache_control": map[string]interface{}{
					"type": "ephemeral",
				},
			},
		},
		Messages: []types.Message{
			{Role: "user", Content: "hi"},
		},
	}

	openaiReq, err := NewRequestTransformer().TransformRequest(req)
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 2; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}
	if got, want := openaiReq.Messages[0].Role, "system"; got != want {
		t.Fatalf("message[0].Role = %q, want %q", got, want)
	}

	parts, ok := openaiReq.Messages[0].Content.([]types.ChatContentPart)
	if !ok {
		t.Fatalf("system content type = %T, want []types.ChatContentPart", openaiReq.Messages[0].Content)
	}
	if got, want := len(parts), 1; got != want {
		t.Fatalf("len(system parts) = %d, want %d", got, want)
	}
	if parts[0].CacheControl == nil {
		t.Fatal("system CacheControl = nil, want preserved cache_control")
	}
	if got, want := parts[0].CacheControl.Type, "ephemeral"; got != want {
		t.Fatalf("system CacheControl.Type = %q, want %q", got, want)
	}
}
