// Package client 提供 OpenAI API 的 HTTP 客户端封装。
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"anthropic-openai-gateway/internal/config"
	"anthropic-openai-gateway/pkg/types"
)

// OpenAIClient 封装对 OpenAI API 的 HTTP 请求。
type OpenAIClient struct {
	baseURL          string
	apiKey           string
	httpClient       *http.Client
	streamHTTPClient *http.Client
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
	transport := http.DefaultTransport.(*http.Transport).Clone()

	return &OpenAIClient{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout:   time.Duration(cfg.NonStreamingTimeoutMS) * time.Millisecond,
			Transport: transport,
		},
		streamHTTPClient: &http.Client{
			Transport: transport,
		},
	}
}

// ChatCompletion 发送非流式聊天完成请求。
func (c *OpenAIClient) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	endpoint := c.baseURL + "/chat/completions"
	startedAt := time.Now()

	// 记录请求信息
	log.Printf("[OpenAI] 📤 发送非流式请求: model=%s, messages=%d, max_tokens=%d",
		req.Model, len(req.Messages), tokenLogValue(req))

	body, err := json.Marshal(req)
	if err != nil {
		log.Printf("[OpenAI] ❌ 序列化请求失败: %v", err)
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.doRequestWithRetry(ctx, endpoint, body, false)
	if err != nil {
		duration := time.Since(startedAt)
		log.Printf("[OpenAI] ❌ 请求失败 (%s): %v", duration, err)
		return nil, c.wrapTransportError("chat_completion", endpoint, startedAt, err)
	}
	defer resp.Body.Close()

	duration := time.Since(startedAt)
	log.Printf("[OpenAI] 📥 收到响应: status=%d, duration=%s", resp.StatusCode, duration)

	if resp.StatusCode != http.StatusOK {
		log.Printf("[OpenAI] ❌ 上游返回非200状态: %d", resp.StatusCode)
		return nil, c.parseError("chat_completion", endpoint, startedAt, resp)
	}

	var result types.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[OpenAI] ❌ 解析响应失败: %v", err)
		return nil, fmt.Errorf("decode response: %w", err)
	}

	log.Printf("[OpenAI] ✅ 请求成功: id=%s, finish_reason=%s, prompt_tokens=%d, completion_tokens=%d",
		result.ID, result.Choices[0].FinishReason, result.Usage.PromptTokens, result.Usage.CompletionTokens)
	return &result, nil
}

// GetStreamingBody 发送流式请求，返回响应体供逐行读取 SSE。
func (c *OpenAIClient) GetStreamingBody(ctx context.Context, req *types.ChatCompletionRequest) (io.ReadCloser, error) {
	endpoint := c.baseURL + "/chat/completions"
	startedAt := time.Now()

	log.Printf("[OpenAI] 📤 发送流式请求: model=%s, messages=%d, max_tokens=%d",
		req.Model, len(req.Messages), tokenLogValue(req))

	req.Stream = boolPtr(true)
	body, err := json.Marshal(req)
	if err != nil {
		log.Printf("[OpenAI] ❌ 序列化流式请求失败: %v", err)
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.doRequestWithRetry(ctx, endpoint, body, true)
	if err != nil {
		duration := time.Since(startedAt)
		log.Printf("[OpenAI] ❌ 流式请求失败 (%s): %v", duration, err)
		return nil, c.wrapTransportError("stream_chat_completion", endpoint, startedAt, err)
	}

	duration := time.Since(startedAt)
	log.Printf("[OpenAI] 📥 流式连接已建立: status=%d, duration=%s", resp.StatusCode, duration)

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		log.Printf("[OpenAI] ❌ 流式请求返回非200: %d", resp.StatusCode)
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

func (c *OpenAIClient) doRequestWithRetry(ctx context.Context, endpoint string, body []byte, streaming bool) (*http.Response, error) {
	attempts := 1
	if !streaming {
		attempts = 2
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			if streaming {
				log.Printf("[OpenAI] ❌ 创建流式HTTP请求失败: %v", err)
			} else {
				log.Printf("[OpenAI] ❌ 创建HTTP请求失败: %v", err)
			}
			return nil, fmt.Errorf("create request: %w", err)
		}
		c.setHeaders(httpReq)

		httpClient := c.httpClient
		if streaming && c.streamHTTPClient != nil {
			httpClient = c.streamHTTPClient
		}

		resp, err := httpClient.Do(httpReq)
		if err == nil {
			return resp, nil
		}
		if attempt == attempts || !shouldRetryEOF(err, ctx) {
			return nil, err
		}

		log.Printf("[OpenAI] ⚠️  上游连接出现瞬时EOF，准备重试: attempt=%d endpoint=%s", attempt+1, endpoint)
	}

	return nil, fmt.Errorf("request failed without response")
}

func shouldRetryEOF(err error, ctx context.Context) bool {
	if err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
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

func tokenLogValue(req *types.ChatCompletionRequest) int {
	if req == nil {
		return 0
	}
	if req.MaxTokens != nil {
		return *req.MaxTokens
	}
	if req.MaxCompletionTokens != nil {
		return *req.MaxCompletionTokens
	}
	return 0
}

func boolPtr(b bool) *bool {
	return &b
}
