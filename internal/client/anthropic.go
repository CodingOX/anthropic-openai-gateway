// Package client - anthropic.go 提供 Anthropic Messages API 的 HTTP 透传客户端。
package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"anthropic-openai-gateway/internal/config"
)

const anthropicMessagesPath = "/messages"
const defaultAnthropicVersion = "2023-06-01"

// forwardedHeaderPrefixes 定义需要从调用方请求头白名单转发的前缀。
// 仅允许 anthropic- 前缀的 header 透传，避免泄漏敏感信息。
var forwardedHeaderPrefixes = []string{"anthropic-"}

// AnthropicClient 封装对 Anthropic API 的透传 HTTP 请求。
// 与 OpenAIClient 不同，这里不做任何格式转换，只负责原样转发。
type AnthropicClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewAnthropicClient 创建 Anthropic 客户端实例。
func NewAnthropicClient(cfg *config.Config) *AnthropicClient {
	return &AnthropicClient{
		baseURL: cfg.Anthropic.BaseURL,
		apiKey:  cfg.Anthropic.APIKey,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.Anthropic.TimeoutMS) * time.Millisecond,
		},
	}
}

// ForwardMessage 将请求体和 Anthropic 相关请求头原样转发到 Anthropic Messages API。
// body 是已序列化的 Anthropic 格式 JSON 请求体。
// callerHeaders 是调用方原始请求头，其中的 anthropic-* 头会被白名单转发。
// 返回上游的原始 *http.Response，调用方负责关闭 Body。
func (c *AnthropicClient) ForwardMessage(ctx context.Context, body io.Reader, callerHeaders http.Header) (*http.Response, error) {
	endpoint := c.baseURL + anthropicMessagesPath

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)

	// anthropic-version: 优先使用调用方指定的版本，否则使用默认值
	if v := callerHeaders.Get("anthropic-version"); v != "" {
		httpReq.Header.Set("anthropic-version", v)
	} else {
		httpReq.Header.Set("anthropic-version", defaultAnthropicVersion)
	}

	// 白名单转发调用方请求头中 anthropic-* 前缀的 header
	for key, values := range callerHeaders {
		if c.shouldForward(key) {
			for _, v := range values {
				httpReq.Header.Add(key, v)
			}
		}
	}

	return c.httpClient.Do(httpReq)
}

// shouldForward 判断请求头是否应该被转发。
// 只有匹配白名单前缀的 header 才能透传。
func (c *AnthropicClient) shouldForward(headerName string) bool {
	lower := strings.ToLower(headerName)
	for _, prefix := range forwardedHeaderPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}
