// Package client 提供 OpenAI API 的 HTTP 客户端封装。
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result types.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// GetStreamingBody 发送流式请求，返回响应体供逐行读取 SSE。
func (c *OpenAIClient) GetStreamingBody(ctx context.Context, req *types.ChatCompletionRequest) (io.ReadCloser, error) {
	req.Stream = boolPtr(true)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, c.parseError(resp)
	}
	return resp.Body, nil
}

// setHeaders 设置公共请求头。
func (c *OpenAIClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

// parseError 解析 OpenAI 非 200 响应。
func (c *OpenAIClient) parseError(resp *http.Response) error {
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("openai status %d: %s", resp.StatusCode, string(respBody))
}

func boolPtr(b bool) *bool {
	return &b
}
