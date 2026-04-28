package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"anthropic-openai-gateway/internal/config"
)

func TestAnthropicClientForwardsRequestAsIs(t *testing.T) {
	var gotBody, gotMethod, gotPath string
	var gotHeaders http.Header

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}]}`))
	}))
	defer upstream.Close()

	c := NewAnthropicClient(&config.Config{
		Anthropic: config.AnthropicConfig{
			BaseURL:   upstream.URL,
			APIKey:    "sk-ant-test123",
			TimeoutMS: 5000,
		},
	})

	// 模拟调用方请求头，包含 anthropic-beta 等 Anthropic 扩展头
	callerHeaders := http.Header{
		"Anthropic-Beta":           {"extended-thinking-2025-04-22"},
		"Anthropic-Version":        {"2024-02-15"},
		"X-Custom-Private":         {"should-not-leak"},
		"Authorization":            {"Bearer should-not-leak"},
	}

	reqBody := `{"model":"claude-sonnet-4-20250514","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`
	resp, err := c.ForwardMessage(context.Background(), strings.NewReader(reqBody), callerHeaders)
	if err != nil {
		t.Fatalf("ForwardMessage() error = %v", err)
	}
	defer resp.Body.Close()

	// 验证方法、路径
	if gotMethod != http.MethodPost {
		t.Fatalf("upstream method = %q, want POST", gotMethod)
	}
	if gotPath != "/messages" {
		t.Fatalf("upstream path = %q, want /messages", gotPath)
	}

	// 验证认证头（始终由网关注入，不来自调用方）
	if got := gotHeaders.Get("x-api-key"); got != "sk-ant-test123" {
		t.Fatalf("x-api-key = %q, want sk-ant-test123", got)
	}

	// 验证 anthropic-version 使用调用方指定的版本
	if got := gotHeaders.Get("anthropic-version"); got != "2024-02-15" {
		t.Fatalf("anthropic-version = %q, want 2024-02-15 (caller override)", got)
	}

	// 验证 anthropic-beta 白名单转发
	if got := gotHeaders.Get("anthropic-beta"); got != "extended-thinking-2025-04-22" {
		t.Fatalf("anthropic-beta = %q, want extended-thinking-2025-04-22", got)
	}

	// 验证不应泄漏的 header
	if got := gotHeaders.Get("x-custom-private"); got != "" {
		t.Fatalf("x-custom-private = %q, should not be forwarded", got)
	}
	if got := gotHeaders.Get("authorization"); got != "" {
		t.Fatalf("authorization = %q, should not be forwarded", got)
	}

	// 验证请求体原样转发
	if strings.TrimSpace(gotBody) != reqBody {
		t.Fatalf("forwarded body mismatch:\ngot:  %s\nwant: %s", gotBody, reqBody)
	}

	// 验证响应
	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), `"ok"`) {
		t.Fatalf("response body = %s, want containing ok", respBody)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestAnthropicClientUsesDefaultAnthropicVersionWhenCallerHeadersNil(t *testing.T) {
	var gotHeaders http.Header

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	c := NewAnthropicClient(&config.Config{
		Anthropic: config.AnthropicConfig{
			BaseURL:   upstream.URL,
			APIKey:    "sk-ant-test",
			TimeoutMS: 5000,
		},
	})

	resp, err := c.ForwardMessage(context.Background(), strings.NewReader("{}"), nil)
	if err != nil {
		t.Fatalf("ForwardMessage() error = %v", err)
	}
	resp.Body.Close()

	if got := gotHeaders.Get("anthropic-version"); got != defaultAnthropicVersion {
		t.Fatalf("anthropic-version = %q, want %s (default)", got, defaultAnthropicVersion)
	}
}

func TestAnthropicClientPassesUpstreamErrorStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad request"}}`))
	}))
	defer upstream.Close()

	c := NewAnthropicClient(&config.Config{
		Anthropic: config.AnthropicConfig{
			BaseURL:   upstream.URL,
			APIKey:    "sk-ant-test123",
			TimeoutMS: 5000,
		},
	})

	resp, err := c.ForwardMessage(context.Background(), strings.NewReader("{}"), nil)
	if err != nil {
		t.Fatalf("ForwardMessage() error = %v, want nil (should not error on HTTP-level failures)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "invalid_request_error") {
		t.Fatalf("body = %s, want containing error details", body)
	}
}
