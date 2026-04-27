// Package client 提供 OpenAI API 的 HTTP 客户端封装。
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"anthropic-openai-gateway/internal/config"
	"anthropic-openai-gateway/pkg/types"
)

// OpenAIClient 封装对 OpenAI API 的 HTTP 请求。
type OpenAIClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// UpstreamError 保留上游请求失败时的关键调试上下文。
type UpstreamError struct {
	Operation       string
	URL             string
	StatusCode      int
	Duration        time.Duration
	ResponsePreview string
	Err             error
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "<nil>"
	}

	base := fmt.Sprintf("%s %s failed", e.Operation, e.URL)
	if e.StatusCode > 0 {
		base = fmt.Sprintf("%s with status=%d", base, e.StatusCode)
	}
	if e.Duration > 0 {
		base = fmt.Sprintf("%s duration=%s", base, e.Duration.Round(time.Millisecond))
	}
	if e.ResponsePreview != "" {
		base = fmt.Sprintf("%s body=%s", base, e.ResponsePreview)
	}
	if e.Err != nil {
		base = fmt.Sprintf("%s: %v", base, e.Err)
	}
	return base
}

func (e *UpstreamError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

const maxErrorPreviewChars = 400

// NewOpenAIClient 创建 OpenAI 客户端实例。
func NewOpenAIClient(cfg *config.Config) *OpenAIClient {
	return &OpenAIClient{
		baseURL: cfg.OpenAI.BaseURL,
		apiKey:  cfg.OpenAI.APIKey,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.OpenAI.TimeoutMS) * time.Millisecond,
		},
	}
}

// ChatCompletion 发送非流式聊天完成请求。
func (c *OpenAIClient) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	endpoint := c.baseURL + "/chat/completions"
	startedAt := time.Now()

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, c.wrapTransportError("chat_completion", endpoint, startedAt, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError("chat_completion", endpoint, startedAt, resp)
	}

	var result types.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// GetStreamingBody 发送流式请求，返回响应体供逐行读取 SSE。
func (c *OpenAIClient) GetStreamingBody(ctx context.Context, req *types.ChatCompletionRequest) (io.ReadCloser, error) {
	endpoint := c.baseURL + "/chat/completions"
	startedAt := time.Now()

	req.Stream = boolPtr(true)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, c.wrapTransportError("stream_chat_completion", endpoint, startedAt, err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, c.parseError("stream_chat_completion", endpoint, startedAt, resp)
	}
	return resp.Body, nil
}

// setHeaders 设置公共请求头。
func (c *OpenAIClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

// parseError 解析 OpenAI 非 200 响应，并保留可用于排障的错误上下文。
func (c *OpenAIClient) parseError(operation string, endpoint string, startedAt time.Time, resp *http.Response) error {
	respBody, _ := io.ReadAll(resp.Body)
	return &UpstreamError{
		Operation:       operation,
		URL:             endpoint,
		StatusCode:      resp.StatusCode,
		Duration:        time.Since(startedAt),
		ResponsePreview: truncateForLog(string(respBody), maxErrorPreviewChars),
	}
}

func (c *OpenAIClient) wrapTransportError(operation string, endpoint string, startedAt time.Time, err error) error {
	return &UpstreamError{
		Operation: operation,
		URL:       endpoint,
		Duration:  time.Since(startedAt),
		Err:       err,
	}
}

func truncateForLog(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || limit <= 0 {
		return trimmed
	}

	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit]) + "..."
}

func boolPtr(b bool) *bool {
	return &b
}
