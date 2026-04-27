package types

// ChatMessage OpenAI 消息体。
type ChatMessage struct {
	Role             string      `json:"role"`
	Content          interface{} `json:"content"` // string 或 []ChatContentPart
	ReasoningContent *string     `json:"reasoning_content,omitempty"`
	Name             string      `json:"name,omitempty"`
	ToolCalls        []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID       string      `json:"tool_call_id,omitempty"`
}

// ChatContentPart 多模态内容部分（OpenAI 风格）。
type ChatContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL 图片 URL。
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// ToolCall OpenAI 工具调用。
type ToolCall struct {
	Index    *int         `json:"index,omitempty"`
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall 函数调用细节。
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

// OpenAITool OpenAI 工具定义。
type OpenAITool struct {
	Type     string            `json:"type"` // "function"
	Function OpenAIFunctionDef `json:"function"`
}

// OpenAIFunctionDef 函数定义。
type OpenAIFunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// ChatCompletionRequest OpenAI /v1/chat/completions 请求体。
type ChatCompletionRequest struct {
	Model               string                 `json:"model"`
	Messages            []ChatMessage          `json:"messages"`
	Temperature         *float64               `json:"temperature,omitempty"`
	TopP                *float64               `json:"top_p,omitempty"`
	N                   *int                   `json:"n,omitempty"`
	Stream              *bool                  `json:"stream,omitempty"`
	Stop                interface{}            `json:"stop,omitempty"` // string 或 []string
	MaxTokens           *int                   `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                   `json:"max_completion_tokens,omitempty"`
	PresencePenalty     *float64               `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64               `json:"frequency_penalty,omitempty"`
	LogitBias           map[string]int         `json:"logit_bias,omitempty"`
	User                string                 `json:"user,omitempty"`
	Tools               []OpenAITool           `json:"tools,omitempty"`
	ToolChoice          interface{}            `json:"tool_choice,omitempty"`
	ResponseFormat      map[string]interface{} `json:"response_format,omitempty"`
	Seed                *int                   `json:"seed,omitempty"`
	StreamOptions       *StreamOptions         `json:"stream_options,omitempty"`
}

// StreamOptions 流选项。
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// ChatCompletionResponse OpenAI 非流式响应体。
type ChatCompletionResponse struct {
	ID                string       `json:"id"`
	Object            string       `json:"object"`
	Created           int64        `json:"created"`
	Model             string       `json:"model"`
	Choices           []ChatChoice `json:"choices"`
	Usage             *ChatUsage   `json:"usage,omitempty"`
	SystemFingerprint string       `json:"system_fingerprint,omitempty"`
}

// ChatChoice 响应的选择项。
type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
	LogProbs     interface{} `json:"logprobs,omitempty"`
}

// ChatUsage token 使用统计。
type ChatUsage struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	TotalTokens           int `json:"total_tokens"`
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens,omitempty"`
}

// ChatCompletionChunk 流式响应的 chunk。
type ChatCompletionChunk struct {
	ID                string        `json:"id"`
	Object            string        `json:"object"`
	Created           int64         `json:"created"`
	Model             string        `json:"model"`
	SystemFingerprint string        `json:"system_fingerprint,omitempty"`
	Choices           []ChunkChoice `json:"choices"`
	Usage             *ChatUsage    `json:"usage,omitempty"`
}

// ChunkChoice 流式 chunk 的选择项。
type ChunkChoice struct {
	Index        int         `json:"index"`
	Delta        ChatDelta   `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
	LogProbs     interface{} `json:"logprobs,omitempty"`
}

// ChatDelta 增量消息。
type ChatDelta struct {
	Role             string     `json:"role,omitempty"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent *string    `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}
