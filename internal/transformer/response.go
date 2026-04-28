// Package transformer - response.go 将 OpenAI 响应转换为 Anthropic 格式。
package transformer

import (
	"encoding/json"
	"fmt"
	"log"

	"anthropic-openai-gateway/pkg/types"
)

// ResponseTransformer 将 OpenAI 响应转换为 Anthropic 响应。
type ResponseTransformer struct{}

// NewResponseTransformer 创建响应转换器。
func NewResponseTransformer() *ResponseTransformer {
	return &ResponseTransformer{}
}

// TransformResponse 执行 OpenAI → Anthropic 非流式响应转换。
func (t *ResponseTransformer) TransformResponse(or *types.ChatCompletionResponse, model string) (*types.MessageResponse, error) {
	log.Printf("[TRANSFORMER] 🔄 开始转换 OpenAI → Anthropic")
	log.Printf("[TRANSFORMER] 📝 响应ID: %s, 模型: %s, 选项数: %d", or.ID, model, len(or.Choices))

	resp := &types.MessageResponse{
		ID:    or.ID,
		Type:  "message",
		Role:  "assistant",
		Model: model,
	}

	if len(or.Choices) == 0 {
		log.Printf("[TRANSFORMER] ❌ OpenAI响应没有choices")
		return nil, fmt.Errorf("openai response has no choices")
	}

	choice := or.Choices[0]
	contentBlocks := t.convertContent(or.ID, choice)
	resp.Content = contentBlocks
	log.Printf("[TRANSFORMER] ✅ 内容转换完成: %d个内容块", len(contentBlocks))

	// 转换停止原因
	stopReason := t.convertFinishReason(choice.FinishReason)
	resp.StopReason = &stopReason
	log.Printf("[TRANSFORMER] 🛑 停止原因: %s → %s", choice.FinishReason, stopReason)

	// 转换用量统计
	if or.Usage != nil {
		resp.Usage = normalizeOpenAIUsage(or.Usage)
		log.Printf("[TRANSFORMER] 📊 用量统计: input=%d, output=%d, cache_read=%d, cache_creation=%d",
			resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.CacheReadInputTokens, resp.Usage.CacheCreationInputTokens)
	}

	log.Printf("[TRANSFORMER] ✅ 响应转换完成")
	return resp, nil
}

// convertContent 将 OpenAI 响应消息转换为 Anthropic ContentBlock 数组。
func (t *ResponseTransformer) convertContent(id string, choice types.ChatChoice) []types.ContentBlock {
	var blocks []types.ContentBlock

	// 1. 文本内容
	if choice.Message.ReasoningContent != nil && *choice.Message.ReasoningContent != "" {
		blocks = append(blocks, types.ContentBlock{
			Type:     "thinking",
			Thinking: *choice.Message.ReasoningContent,
		})
	}

	if choice.Message.Content != nil {
		switch c := choice.Message.Content.(type) {
		case string:
			if c != "" {
				blocks = append(blocks, types.ContentBlock{
					Type: "text",
					Text: c,
				})
			}
		case []interface{}:
			for _, part := range c {
				partMap, ok := part.(map[string]interface{})
				if !ok {
					continue
				}
				if partMap["type"] == "text" {
					if text, ok := partMap["text"].(string); ok {
						blocks = append(blocks, types.ContentBlock{
							Type: "text",
							Text: text,
						})
					}
				}
			}
		}
	}

	// 2. 工具调用 → tool_use 块
	for _, tc := range choice.Message.ToolCalls {
		// 解析 arguments JSON 字符串
		var input interface{}
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
				// 解析失败时保留原始字符串
				input = tc.Function.Arguments
			}
		}

		block := types.ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		}
		blocks = append(blocks, block)
	}

	// 确保至少有一个 text block（Anthropic 要求）
	if len(blocks) == 0 {
		blocks = append(blocks, types.ContentBlock{
			Type: "text",
			Text: "",
		})
	}

	return blocks
}

// convertFinishReason 转换完成原因。
func (t *ResponseTransformer) convertFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "end_turn"
	default:
		return "end_turn"
	}
}

// normalizeOpenAIUsage 兼容不同上游的 usage 字段形态，统一转换为 Anthropic usage。
func normalizeOpenAIUsage(usage *types.ChatUsage) types.Usage {
	if usage == nil {
		return types.Usage{}
	}

	outputTokens := usage.CompletionTokens
	if outputTokens == 0 && usage.TotalTokens >= usage.PromptTokens {
		outputTokens = usage.TotalTokens - usage.PromptTokens
	}

	cacheReadTokens := firstNonZero(
		usage.PromptCacheHitTokens,
		usage.CacheReadInputTokens,
		usage.CachedTokens,
		cachedTokensFromDetails(usage.PromptTokensDetails),
	)
	cacheCreationTokens := firstNonZero(
		usage.PromptCacheMissTokens,
		usage.CacheCreationInputTokens,
	)

	return types.Usage{
		InputTokens:              usage.PromptTokens,
		OutputTokens:             outputTokens,
		CacheReadInputTokens:     cacheReadTokens,
		CacheCreationInputTokens: cacheCreationTokens,
	}
}

func cachedTokensFromDetails(details *types.PromptTokensDetail) int {
	if details == nil {
		return 0
	}
	return details.CachedTokens
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
