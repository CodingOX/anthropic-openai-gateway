package transformer

import (
	"encoding/json"
	"testing"

	"anthropic-openai-gateway/pkg/types"
)

func TestTransformResponsePreservesReasoningAndCacheUsage(t *testing.T) {
	reasoning := "think first"
	resp := &types.ChatCompletionResponse{
		ID:    "chatcmpl_1",
		Model: "gpt-4o",
		Choices: []types.ChatChoice{
			{
				Message: types.ChatMessage{
					Content:          "final",
					ReasoningContent: &reasoning,
				},
				FinishReason: "stop",
			},
		},
		Usage: &types.ChatUsage{
			PromptTokens:          10,
			CompletionTokens:      5,
			PromptCacheHitTokens:  7,
			PromptCacheMissTokens: 3,
		},
	}

	got, err := NewResponseTransformer().TransformResponse(resp, "gpt-4o")
	if err != nil {
		t.Fatalf("TransformResponse() error = %v", err)
	}

	if len(got.Content) != 2 {
		t.Fatalf("len(Content) = %d, want 2: %#v", len(got.Content), got.Content)
	}
	if got.Content[0].Type != "thinking" || got.Content[0].Thinking != reasoning {
		t.Fatalf("thinking block = %#v, want reasoning %q", got.Content[0], reasoning)
	}
	if got.Usage.CacheReadInputTokens != 7 {
		t.Fatalf("CacheReadInputTokens = %d, want 7", got.Usage.CacheReadInputTokens)
	}
	if got.Usage.CacheCreationInputTokens != 3 {
		t.Fatalf("CacheCreationInputTokens = %d, want 3", got.Usage.CacheCreationInputTokens)
	}
}

func TestTransformResponseNormalizesCacheUsageFallbacks(t *testing.T) {
	var resp types.ChatCompletionResponse
	raw := []byte(`{
		"id":"chatcmpl_2",
		"model":"gpt-4o",
		"choices":[{"index":0,"message":{"content":"final"},"finish_reason":"stop"}],
		"usage":{
			"prompt_tokens":12,
			"completion_tokens":4,
			"prompt_tokens_details":{"cached_tokens":9},
			"cache_creation_input_tokens":2
		}
	}`)
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	got, err := NewResponseTransformer().TransformResponse(&resp, "gpt-4o")
	if err != nil {
		t.Fatalf("TransformResponse() error = %v", err)
	}
	if got.Usage.CacheReadInputTokens != 9 {
		t.Fatalf("CacheReadInputTokens = %d, want 9", got.Usage.CacheReadInputTokens)
	}
	if got.Usage.CacheCreationInputTokens != 2 {
		t.Fatalf("CacheCreationInputTokens = %d, want 2", got.Usage.CacheCreationInputTokens)
	}
}
