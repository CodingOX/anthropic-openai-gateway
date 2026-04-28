// Package transformer 处理 Anthropic ↔ OpenAI 格式转换。
package transformer

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"anthropic-openai-gateway/pkg/types"
)

// RequestTransformer 将 Anthropic 请求转换为 OpenAI 请求。
type RequestTransformer struct{}

// NewRequestTransformer 创建请求转换器。
func NewRequestTransformer() *RequestTransformer {
	return &RequestTransformer{}
}

// TransformRequest 执行 Anthropic → OpenAI 请求转换。
func (t *RequestTransformer) TransformRequest(ar *types.MessageRequest) (*types.ChatCompletionRequest, error) {
	log.Printf("[TRANSFORMER] 🔄 开始转换 Anthropic → OpenAI")
	log.Printf("[TRANSFORMER] 📝 模型: %s, 消息数: %d, 工具数: %d", ar.Model, len(ar.Messages), len(ar.Tools))

	// 转换消息列表
	messages, systemMsg, err := t.convertMessages(ar.Messages, ar.System)
	if err != nil {
		log.Printf("[TRANSFORMER] ❌ 消息转换失败: %v", err)
		return nil, fmt.Errorf("convert messages: %w", err)
	}
	log.Printf("[TRANSFORMER] ✅ 消息转换完成: %d条消息", len(messages))

	req := &types.ChatCompletionRequest{
		Model:         ar.Model, // 直接使用原 model 名（如 gpt-4o）
		Messages:      messages,
		Temperature:   ar.Temperature,
		TopP:          ar.TopP,
		Stop:          t.convertStop(ar.StopSequences),
		Stream:        ar.Stream,
		StreamOptions: t.convertStreamOptions(ar.Stream),
	}

	// 为兼容当前上游实现，保留经典的 max_tokens 字段。
	if ar.MaxTokens > 0 {
		req.MaxTokens = &ar.MaxTokens
		log.Printf("[TRANSFORMER] 📊 max_tokens: %d → max_tokens", ar.MaxTokens)
	}

	// 转换工具定义
	if len(ar.Tools) > 0 {
		req.Tools = t.convertTools(ar.Tools)
		req.ToolChoice = t.convertToolChoice(ar.ToolChoice)
		log.Printf("[TRANSFORMER] 🔧 工具转换完成: %d个工具, tool_choice=%v", len(req.Tools), ar.ToolChoice)
	}

	if isThinkingEnabled(ar.Thinking) {
		applyReasoningPlaceholder(req.Messages)
	}

	// 注入 system prompt 作为第一条 developer 消息。
	// OpenAI o-series 和 gpt-5 系列模型推荐使用 developer role，其余使用 system role。
	if systemMsg != "" {
		systemRole := "system"
		if strings.HasPrefix(ar.Model, "o") || strings.HasPrefix(ar.Model, "gpt-5") {
			systemRole = "developer"
		}
		systemMessage := types.ChatMessage{
			Role:    systemRole,
			Content: systemMsg,
		}
		req.Messages = append([]types.ChatMessage{systemMessage}, req.Messages...)
	}

	return req, nil
}

// convertMessages 转换消息列表，分离 system prompt。
func (t *RequestTransformer) convertMessages(messages []types.Message, system interface{}) ([]types.ChatMessage, string, error) {
	// 处理独立的 system 字段
	var systemText string
	switch s := system.(type) {
	case string:
		systemText = s
	case []interface{}:
		// 从 ContentBlock 数组中提取文本
		for _, block := range s {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}
			if blockMap["type"] == "text" {
				if text, ok := blockMap["text"].(string); ok {
					systemText += text
				}
			}
		}
	}

	var chatMessages []types.ChatMessage
	for _, msg := range messages {
		cms, err := t.convertMessageToMessages(msg)
		if err != nil {
			return nil, "", err
		}
		chatMessages = append(chatMessages, cms...)
	}
	// 过滤空消息
	chatMessages = filterEmptyMessages(chatMessages)
	return chatMessages, systemText, nil
}

func (t *RequestTransformer) convertMessageToMessages(msg types.Message) ([]types.ChatMessage, error) {
	switch content := msg.Content.(type) {
	case string:
		return []types.ChatMessage{{
			Role:    msg.Role,
			Content: content,
		}}, nil
	case []interface{}:
		return t.convertContentBlocksToMessages(msg.Role, content)
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported content type: %T", msg.Content)
	}
}

// convertContentBlocks 转换内容块数组。
func (t *RequestTransformer) convertContentBlocks(role string, blocks []interface{}) ([]types.ChatMessage, error) {
	var textParts []string
	var thinkingParts []string
	var toolCalls []types.ToolCall
	var contentParts []types.ChatContentPart

	for _, block := range blocks {
		// JSON 反序列化后的 map
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue
		}

		blockType, _ := blockMap["type"].(string)
		switch blockType {
		case "text":
			text, _ := blockMap["text"].(string)
			textParts = append(textParts, text)
			contentParts = append(contentParts, types.ChatContentPart{
				Type: "text",
				Text: text,
			})
		case "tool_use":
			tc, err := t.convertToolUse(blockMap)
			if err != nil {
				return nil, err
			}
			toolCalls = append(toolCalls, *tc)
		case "thinking":
			thinking, _ := blockMap["thinking"].(string)
			if thinking != "" {
				thinkingParts = append(thinkingParts, thinking)
			}
		case "image":
			if source, ok := blockMap["source"].(map[string]interface{}); ok {
				mediaType, _ := source["media_type"].(string)
				data, _ := source["data"].(string)
				contentParts = append(contentParts, types.ChatContentPart{
					Type: "image_url",
					ImageURL: &types.ImageURL{
						URL: fmt.Sprintf("data:%s;base64,%s", mediaType, data),
					},
				})
			}
		}
	}

	cm := &types.ChatMessage{Role: role}

	if len(toolCalls) > 0 {
		cm.ToolCalls = toolCalls
		// 如果有 tool_calls 的 assistant 消息在 OpenAI 中 content 可以为 null
		if len(textParts) == 0 {
			cm.Content = nil
		} else {
			cm.Content = strings.Join(textParts, "\n")
		}
	} else if len(contentParts) > 0 {
		if len(contentParts) == 1 && contentParts[0].Type == "text" {
			cm.Content = contentParts[0].Text
		} else {
			cm.Content = contentParts
		}
	}
	if len(thinkingParts) > 0 {
		reasoningContent := strings.Join(thinkingParts, "")
		cm.ReasoningContent = &reasoningContent
	}

	return []types.ChatMessage{*cm}, nil
}

func (t *RequestTransformer) convertContentBlocksToMessages(role string, blocks []interface{}) ([]types.ChatMessage, error) {
	if role == "user" {
		var result []types.ChatMessage
		var textParts []string
		var contentParts []types.ChatContentPart

		for _, block := range blocks {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}

			blockType, _ := blockMap["type"].(string)
			switch blockType {
			case "text":
				text, _ := blockMap["text"].(string)
				textParts = append(textParts, text)
				contentParts = append(contentParts, types.ChatContentPart{
					Type: "text",
					Text: text,
				})
			case "tool_result":
				toolUseID, _ := blockMap["tool_use_id"].(string)
				result = append(result, types.ChatMessage{
					Role:       "tool",
					ToolCallID: toolUseID,
					Content:    extractToolResultText(blockMap["content"]),
				})
			case "image":
				if source, ok := blockMap["source"].(map[string]interface{}); ok {
					mediaType, _ := source["media_type"].(string)
					data, _ := source["data"].(string)
					contentParts = append(contentParts, types.ChatContentPart{
						Type: "image_url",
						ImageURL: &types.ImageURL{
							URL: fmt.Sprintf("data:%s;base64,%s", mediaType, data),
						},
					})
				}
			}
		}

		if len(textParts) > 0 || len(contentParts) > 0 {
			userMsg := types.ChatMessage{Role: "user"}
			if len(contentParts) == 1 && contentParts[0].Type == "text" {
				userMsg.Content = contentParts[0].Text
			} else if len(contentParts) > 0 {
				userMsg.Content = contentParts
			} else {
				userMsg.Content = strings.Join(textParts, "\n")
			}
			result = append(result, userMsg)
		}

		return result, nil
	}

	return t.convertContentBlocks(role, blocks)
}

// convertToolUse 转换 tool_use 块为 OpenAI tool_call。
func (t *RequestTransformer) convertToolUse(blockMap map[string]interface{}) (*types.ToolCall, error) {
	id, _ := blockMap["id"].(string)
	name, _ := blockMap["name"].(string)
	input, _ := blockMap["input"]

	inputJSON := "{}"
	if input != nil {
		inputBytes, err := json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("marshal tool input: %w", err)
		}
		inputJSON = string(inputBytes)
	}

	return &types.ToolCall{
		ID:   id,
		Type: "function",
		Function: types.FunctionCall{
			Name:      name,
			Arguments: inputJSON,
		},
	}, nil
}

// convertTools 转换工具定义。
func (t *RequestTransformer) convertTools(tools []types.Tool) []types.OpenAITool {
	result := make([]types.OpenAITool, len(tools))
	for i, tool := range tools {
		result[i] = types.OpenAITool{
			Type: "function",
			Function: types.OpenAIFunctionDef{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  t.convertJSONSchema(tool.InputSchema),
			},
		}
	}
	return result
}

// convertJSONSchema 将 Anthropic JSONSchema 转为 map。
func (t *RequestTransformer) convertJSONSchema(schema types.JSONSchema) map[string]interface{} {
	result := map[string]interface{}{
		"type": schema.Type,
	}
	if schema.Properties != nil {
		result["properties"] = schema.Properties
	}
	if len(schema.Required) > 0 {
		result["required"] = schema.Required
	}
	if schema.Additional != nil {
		result["additionalProperties"] = schema.Additional
	}
	return result
}

// convertToolChoice 转换工具选择策略。
func (t *RequestTransformer) convertToolChoice(tc interface{}) interface{} {
	if tc == nil {
		return nil
	}
	switch v := tc.(type) {
	case string:
		switch v {
		case "auto":
			return "auto"
		case "any":
			return "required"
		case "none":
			return "none"
		default:
			return map[string]interface{}{
				"type": "function",
				"function": map[string]string{
					"name": v,
				},
			}
		}
	case map[string]interface{}:
		typeVal, _ := v["type"].(string)
		if typeVal == "tool" {
			name, _ := v["name"].(string)
			return map[string]interface{}{
				"type": "function",
				"function": map[string]string{
					"name": name,
				},
			}
		}
	}
	return nil
}

// convertStop 转换停止序列。
func (t *RequestTransformer) convertStop(stopSequences []string) interface{} {
	if len(stopSequences) == 0 {
		return nil
	}
	if len(stopSequences) == 1 {
		return stopSequences[0]
	}
	return stopSequences
}

// convertStreamOptions 设置流选项（OpenAI 需要在流中返回 usage）。
func (t *RequestTransformer) convertStreamOptions(stream *bool) *types.StreamOptions {
	if stream != nil && *stream {
		return &types.StreamOptions{IncludeUsage: true}
	}
	return nil
}

// filterEmptyMessages 过滤空消息。
func filterEmptyMessages(msgs []types.ChatMessage) []types.ChatMessage {
	var result []types.ChatMessage
	for _, m := range msgs {
		if m.Role == "" {
			continue
		}
		// 跳过空消息（除非有 tool_calls）
		if m.Content == nil && len(m.ToolCalls) == 0 {
			continue
		}
		result = append(result, m)
	}
	return result
}

func extractToolResultText(content interface{}) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			block, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if block["type"] == "text" {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

func isThinkingEnabled(thinking interface{}) bool {
	config, ok := thinking.(map[string]interface{})
	if !ok {
		return false
	}
	thinkingType, _ := config["type"].(string)
	return thinkingType == "enabled" || thinkingType == "adaptive"
}

func applyReasoningPlaceholder(messages []types.ChatMessage) {
	for index := range messages {
		message := &messages[index]
		if message.Role != "assistant" || message.ReasoningContent != nil {
			continue
		}
		placeholder := " "
		message.ReasoningContent = &placeholder
	}
}
