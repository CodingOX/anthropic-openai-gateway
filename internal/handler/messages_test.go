package handler

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"anthropic-openai-gateway/internal/config"
)

func captureLogs(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()

	buf := &bytes.Buffer{}
	originalWriter := log.Writer()
	originalFlags := log.Flags()

	log.SetOutput(buf)
	log.SetFlags(0)

	return buf, func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	}
}

func TestHandleCountTokensReturnsAnthropicShape(t *testing.T) {
	h := NewMessagesHandler(&config.Config{})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{
		"model":"gpt-4o",
		"messages":[{"role":"user","content":"hello world"}]
	}`))
	rec := httptest.NewRecorder()

	h.HandleCountTokens(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; body=%s", err, rec.Body.String())
	}
	// "hello world" = 2 tokens in o200k_base, + role + overhead
	// 精确值不重要，但必须 > 字符估算的差异阈值
	if payload["input_tokens"] < 2 {
		t.Fatalf("input_tokens = %d, want >= 2", payload["input_tokens"])
	}
	t.Logf("input_tokens = %d", payload["input_tokens"])
}

func TestHandleMessagesLogsDecodeFailuresWithRequestContext(t *testing.T) {
	logBuf, restoreLogs := captureLogs(t)
	defer restoreLogs()

	h := NewMessagesHandler(&config.Config{})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":`))
	rec := httptest.NewRecorder()

	h.HandleMessages(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}

	logs := logBuf.String()
	for _, want := range []string{
		"request_id=",
		"stage=decode_request",
		"status=400",
		"invalid request body",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs missing %q; logs=%s", want, logs)
		}
	}
}

func TestHandleMessagesLogsUpstreamFailuresWithContext(t *testing.T) {
	logBuf, restoreLogs := captureLogs(t)
	defer restoreLogs()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream exploded","trace":"abc123"}`))
	}))
	defer upstream.Close()

	h := NewMessagesHandler(&config.Config{
		ModelsNeedTransformation: []string{"gpt-4o"},
		BaseURL:                  upstream.URL,
		APIKey:                   "test-key",
		NonStreamingTimeoutMS:    1000,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"gpt-4o",
		"max_tokens":16,
		"messages":[{"role":"user","content":"hello"}]
	}`))
	rec := httptest.NewRecorder()

	h.HandleMessages(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body=%s", rec.Code, rec.Body.String())
	}

	logs := logBuf.String()
	for _, want := range []string{
		"request_id=",
		"stage=upstream_request_failed",
		"model=gpt-4o",
		"upstream_status=502",
		"response_preview=",
		"upstream exploded",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs missing %q; logs=%s", want, logs)
		}
	}
}

func TestHandleMessagesLogsTransformRequestSummary(t *testing.T) {
	logBuf, restoreLogs := captureLogs(t)
	defer restoreLogs()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_123",
			"object":"chat.completion",
			"created":1710000000,
			"model":"deepseek-v4-flash",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":42,"completion_tokens":7,"total_tokens":49}
		}`))
	}))
	defer upstream.Close()

	h := NewMessagesHandler(&config.Config{
		ModelsNeedTransformation: []string{"deepseek-v4-flash"},
		BaseURL:                  upstream.URL,
		APIKey:                   "test-key",
		NonStreamingTimeoutMS:    1000,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"deepseek-v4-flash",
		"max_tokens":16,
		"system":[{"type":"text","text":"system prefix","cache_control":{"type":"ephemeral"}}],
		"messages":[
			{"role":"assistant","content":"prior answer"},
			{"role":"user","content":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral"}}]}
		]
	}`))
	rec := httptest.NewRecorder()

	h.HandleMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	logs := logBuf.String()
	for _, want := range []string{
		"stage=transform_request_summary",
		"anthropic_input_tokens=",
		"openai_input_tokens=",
		"anthropic_request_bytes=",
		"openai_request_bytes=",
		"reasoning_placeholders=",
		"cache_control_blocks=",
		"system_role=system",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs missing %q; logs=%s", want, logs)
		}
	}
}

func TestHandleMessagesPassThroughNonStreaming(t *testing.T) {
	// 模拟 Anthropic 上游，验证透传请求和响应
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求头
		if got := r.Header.Get("x-api-key"); got != "sk-ant-test" {
			t.Errorf("x-api-key = %q, want sk-ant-test", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q, want 2023-06-01", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		// 验证 anthropic-beta 白名单透传
		if got := r.Header.Get("anthropic-beta"); got != "extended-thinking-2025-04-22" {
			t.Errorf("anthropic-beta = %q, want extended-thinking-2025-04-22", got)
		}
		if got := r.URL.Path; got != "/messages" {
			t.Errorf("path = %q, want /messages", got)
		}
		// 返回 Anthropic 格式响应
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom-Header", "test-value")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"msg_001","type":"message","role":"assistant","content":[{"type":"text","text":"hello from claude"}]}`))
	}))
	defer upstream.Close()

	h := NewMessagesHandler(&config.Config{
		BaseURL:               upstream.URL,
		APIKey:                "sk-ant-test",
		NonStreamingTimeoutMS: 5000,
	})
	// claude-sonnet-4-20250514 不在默认转换列表中 → 透传
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-sonnet-4-20250514",
		"max_tokens":128,
		"messages":[{"role":"user","content":"hello"}]
	}`))
	req.Header.Set("Anthropic-Beta", "extended-thinking-2025-04-22")
	rec := httptest.NewRecorder()

	h.HandleMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Custom-Header"); got != "test-value" {
		t.Fatalf("X-Custom-Header = %q, want test-value", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; body=%s", err, rec.Body.String())
	}
	if resp["id"] != "msg_001" {
		t.Fatalf("id = %v, want msg_001", resp["id"])
	}
}

func TestHandleMessagesPassThroughUpstreamErrorStatus(t *testing.T) {
	// 上游返回错误状态码时，透传模式也原样返回
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad request"}}`))
	}))
	defer upstream.Close()

	h := NewMessagesHandler(&config.Config{
		BaseURL:               upstream.URL,
		APIKey:                "sk-ant-test",
		NonStreamingTimeoutMS: 5000,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-opus-4-20250514",
		"max_tokens":128,
		"messages":[{"role":"user","content":"hello"}]
	}`))
	rec := httptest.NewRecorder()

	h.HandleMessages(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_request_error") {
		t.Fatalf("body = %s, want containing invalid_request_error", rec.Body.String())
	}
}

func TestHandleMessagesPassThroughLogsEvents(t *testing.T) {
	logBuf, restoreLogs := captureLogs(t)
	defer restoreLogs()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"msg_001","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}]}`))
	}))
	defer upstream.Close()

	h := NewMessagesHandler(&config.Config{
		BaseURL:               upstream.URL,
		APIKey:                "sk-ant-test",
		NonStreamingTimeoutMS: 5000,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-sonnet-4-20250514",
		"max_tokens":128,
		"messages":[{"role":"user","content":"hello"}]
	}`))
	rec := httptest.NewRecorder()

	h.HandleMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	logs := logBuf.String()
	for _, want := range []string{
		"stage=request_received",
		"stage=pass_through_started",
		"upstream_model=claude-sonnet-4-20250514",
		"stage=pass_through_completed",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs missing %q; logs=%s", want, logs)
		}
	}
}

func TestHandleMessagesPassThroughStreamingPreservesSSEHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		// 模拟 SSE 事件流
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("upstream response writer does not support flush")
		}
		w.Write([]byte("event: message_start\n"))
		flusher.Flush()
		w.Write([]byte("data: {\"type\":\"message\",\"message\":{\"id\":\"msg_001\",\"role\":\"assistant\"}}\n\n"))
		flusher.Flush()
	}))
	defer upstream.Close()

	h := NewMessagesHandler(&config.Config{
		BaseURL:               upstream.URL,
		APIKey:                "sk-ant-test",
		NonStreamingTimeoutMS: 5000,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-sonnet-4-20250514",
		"max_tokens":128,
		"stream":true,
		"messages":[{"role":"user","content":"hello"}]
	}`))
	rec := httptest.NewRecorder()

	h.HandleMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if !strings.Contains(rec.Body.String(), "message_start") {
		t.Fatalf("body missing SSE event: %s", rec.Body.String())
	}
}

func TestHandleMessagesPassThroughUnreachableUpstream(t *testing.T) {
	logBuf, restoreLogs := captureLogs(t)
	defer restoreLogs()

	h := NewMessagesHandler(&config.Config{
		BaseURL:               "http://127.0.0.1:1",
		APIKey:                "sk-ant-test",
		NonStreamingTimeoutMS: 50,
	})
	// claude-sonnet-4-20250514 不在默认转换列表中 → 透传
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-sonnet-4-20250514",
		"max_tokens":128,
		"messages":[{"role":"user","content":"hello"}]
	}`))
	rec := httptest.NewRecorder()

	h.HandleMessages(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body=%s", rec.Code, rec.Body.String())
	}

	logs := logBuf.String()
	for _, want := range []string{
		"stage=pass_through_started",
		"stage=upstream_request_failed",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs missing %q; logs=%s", want, logs)
		}
	}
}
