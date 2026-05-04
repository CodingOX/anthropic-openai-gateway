package tokenizer

import (
	"testing"

	"anthropic-openai-gateway/pkg/types"
)

func TestEmbeddedBPELoaderLoadsRequiredEncodings(t *testing.T) {
	loader := embeddedBPELoader{}
	cases := []struct {
		name    string
		minSize int
	}{
		{"https://openaipublic.blob.core.windows.net/encodings/o200k_base.tiktoken", 100000},
		{"https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken", 100000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ranks, err := loader.LoadTiktokenBpe(tc.name)
			if err != nil {
				t.Fatalf("LoadTiktokenBpe() error = %v", err)
			}
			if got := len(ranks); got < tc.minSize {
				t.Fatalf("len(ranks) = %d, want >= %d", got, tc.minSize)
			}
		})
	}
}

func TestResolveEncodingName(t *testing.T) {
	cases := []struct {
		model    string
		wantName string
	}{
		{"gpt-4o", "o200k_base"},
		{"gpt-4.1", "o200k_base"},
		{"gpt-4.5-preview", "o200k_base"},
		{"gpt-5", "o200k_base"},
		{"o3-mini", "o200k_base"},
		{"deepseek-v4-pro", "o200k_base"},
		{"deepseek-v4-flash", "o200k_base"},
		{"gpt-4", "cl100k_base"},
		{"glm-5", "cl100k_base"},
		{"kimi-k2.5", "cl100k_base"},
		{"qwen3.6-plus", "cl100k_base"},
		{"claude-sonnet-4-6", "cl100k_base"},
		{"unknown-model", "cl100k_base"},
	}
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			gotName := resolveEncodingName(tc.model)
			if gotName != tc.wantName {
				t.Fatalf("resolveEncodingName(%q) = %q, want %q", tc.model, gotName, tc.wantName)
			}
		})
	}
}

func TestCountTokensSimple(t *testing.T) {
	n := CountTokens(&types.MessageRequest{
		Model: "gpt-4o",
		Messages: []types.Message{
			{Role: "user", Content: "hello world"},
		},
	})
	if n != 7 {
		t.Fatalf("CountTokens = %d, want 7", n)
	}
}

func TestCountTokensWithSystem(t *testing.T) {
	n := CountTokens(&types.MessageRequest{
		Model:  "gpt-4o",
		System: "You are a helpful assistant.",
		Messages: []types.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if n != 12 {
		t.Fatalf("CountTokens = %d, want 12", n)
	}
}

func TestCountTokensWithTools(t *testing.T) {
	n := CountTokens(&types.MessageRequest{
		Model: "gpt-4o",
		Messages: []types.Message{
			{Role: "user", Content: "what's the weather"},
		},
		Tools: []types.Tool{
			{
				Name:        "get_weather",
				Description: "Get current weather",
				InputSchema: types.JSONSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"location": map[string]interface{}{"type": "string"},
					},
					Required: []string{"location"},
				},
			},
		},
	})
	if n != 37 {
		t.Fatalf("CountTokens = %d, want 37", n)
	}
}

func TestCountTokensEmptyRequestReturnsOne(t *testing.T) {
	// 没有任何 content 的空请求应返回 1
	n := CountTokens(&types.MessageRequest{
		Model: "gpt-4o",
	})
	if n != 1 {
		t.Fatalf("CountTokens = %d, want 1", n)
	}
}

func TestCountTokensEmptyMessageContent(t *testing.T) {
	// 空内容消息：role + 格式开销
	n := CountTokens(&types.MessageRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: ""}},
	})
	if n != 5 {
		t.Fatalf("CountTokens = %d, want 5", n)
	}
}

func TestCountTokensContentBlocks(t *testing.T) {
	n := CountTokens(&types.MessageRequest{
		Model: "gpt-4o",
		Messages: []types.Message{
			{
				Role: "user",
				Content: []interface{}{
					map[string]interface{}{"type": "text", "text": "hello"},
					map[string]interface{}{"type": "text", "text": "world"},
				},
			},
		},
	})
	if n != 7 {
		t.Fatalf("CountTokens = %d, want 7", n)
	}
}

func TestCountTokensChineseText(t *testing.T) {
	n := CountTokens(&types.MessageRequest{
		Model: "deepseek-v4-pro",
		Messages: []types.Message{
			{Role: "user", Content: "你好，今天天气怎么样？"},
		},
	})
	if n != 12 {
		t.Fatalf("CountTokens = %d, want 12", n)
	}
}

func TestCountChatCompletionTokens(t *testing.T) {
	n := CountChatCompletionTokens(&types.ChatCompletionRequest{
		Model: "deepseek-v4-flash",
		Messages: []types.ChatMessage{
			{Role: "system", Content: "You are helpful."},
			{Role: "assistant", Content: "prior answer", ReasoningContent: strPtr("reasoning")},
			{Role: "user", Content: []types.ChatContentPart{
				{Type: "text", Text: "hello"},
			}},
		},
		Tools: []types.OpenAITool{
			{
				Type: "function",
				Function: types.OpenAIFunctionDef{
					Name:        "lookup",
					Description: "Lookup info",
					Parameters: map[string]interface{}{
						"type": "object",
					},
				},
			},
		},
	})
	if n <= 0 {
		t.Fatalf("CountChatCompletionTokens = %d, want > 0", n)
	}
}

func strPtr(value string) *string {
	return &value
}
