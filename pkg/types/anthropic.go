// Package types 定义 Anthropic 和 OpenAI API 的数据类型。
package types

import "fmt"

// ContentBlock 表示 Anthropic 消息中的内容块。
// 支持 text、tool_use、tool_result、image 等类型。
type ContentBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text,omitempty"`
	ID           string        `json:"id,omitempty"`
	Name         string        `json:"name,omitempty"`
	Input        interface{}   `json:"input,omitempty"`
	ToolUseID    string        `json:"tool_use_id,omitempty"`
	Content      interface{}   `json:"content,omitempty"`
	IsError      *bool         `json:"is_error,omitempty"`
	Thinking     string        `json:"thinking,omitempty"`
	Signature    string        `json:"signature,omitempty"`
	Source       *ImageSource  `json:"source,omitempty"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// ImageSource 图片源信息。
type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// CacheControl 缓存控制（Anthropic 特有功能）。
type CacheControl struct {
	Type string `json:"type"`
}

// Message 单条消息。
type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string 或 []ContentBlock
}

// Tool 工具定义。
type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	InputSchema JSONSchema `json:"input_schema"`
}

// JSONSchema 简化的 JSON Schema 类型。
type JSONSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
	Additional interface{}            `json:"additionalProperties,omitempty"`
}

// MessageRequest Anthropic /v1/messages 请求体。
type MessageRequest struct {
	Model         string                 `json:"model"`
	Messages      []Message              `json:"messages"`
	System        interface{}            `json:"system,omitempty"` // string 或 []ContentBlock
	MaxTokens     int                    `json:"max_tokens"`
	Temperature   *float64               `json:"temperature,omitempty"`
	TopP          *float64               `json:"top_p,omitempty"`
	TopK          *int                   `json:"top_k,omitempty"`
	Stream        *bool                  `json:"stream,omitempty"`
	StopSequences []string               `json:"stop_sequences,omitempty"`
	Tools         []Tool                 `json:"tools,omitempty"`
	ToolChoice    interface{}            `json:"tool_choice,omitempty"`
	Thinking      interface{}            `json:"thinking,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// Validate 验证请求参数的合法性。
func (r *MessageRequest) Validate() error {
	if r.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("messages is required")
	}
	if r.MaxTokens < 0 {
		return fmt.Errorf("max_tokens must be non-negative")
	}
	return nil
}

// MessageResponse Anthropic /v1/messages 响应体（非流式）。
type MessageResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   *string        `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
}

// Usage token 用量统计。
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// StreamUsage 表示 Anthropic 流式 delta 中的增量用量。
type StreamUsage struct {
	OutputTokens int `json:"output_tokens"`
}

// StreamEvent SSE 流事件（Anthropic 格式）。
type StreamEvent struct {
	Type         string           `json:"type"`
	Index        *int             `json:"index,omitempty"`
	Delta        *DeltaContent    `json:"delta,omitempty"`
	Message      *MessageResponse `json:"message,omitempty"`
	ContentBlock *ContentBlock    `json:"content_block,omitempty"`
	Usage        *StreamUsage     `json:"usage,omitempty"`
	Error        *APIError        `json:"error,omitempty"`
}

// DeltaContent 增量内容（流式）。
type DeltaContent struct {
	Type         string  `json:"type,omitempty"`
	Text         string  `json:"text,omitempty"`
	Thinking     string  `json:"thinking,omitempty"`
	PartialJSON  string  `json:"partial_json,omitempty"`
	StopReason   *string `json:"stop_reason,omitempty"`
	StopSequence *string `json:"stop_sequence,omitempty"`
}

// APIError API 错误。
type APIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ErrorResponse Anthropic 错误响应体。
type ErrorResponse struct {
	Type  string   `json:"type"`
	Error APIError `json:"error"`
}
