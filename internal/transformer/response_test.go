package transformer

import (
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
