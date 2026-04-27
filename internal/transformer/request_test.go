package transformer

import (
	"encoding/json"
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
