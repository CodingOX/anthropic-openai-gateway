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
	if payload["input_tokens"] <= 0 {
		t.Fatalf("input_tokens = %d, want positive", payload["input_tokens"])
	}
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
		OpenAI: config.OpenAIConfig{
			BaseURL:   upstream.URL,
			APIKey:    "test-key",
			TimeoutMS: 1000,
		},
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

func TestHandleMessagesLogsPromptPreviewWhenEnabled(t *testing.T) {
	logBuf, restoreLogs := captureLogs(t)
	defer restoreLogs()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"bad upstream"}`))
	}))
	defer upstream.Close()

	h := NewMessagesHandler(&config.Config{
		ModelsNeedTransformation: []string{"gpt-4o"},
		LogPromptPreviewOnError:  true,
		PromptPreviewMaxChars:    96,
		OpenAI: config.OpenAIConfig{
			BaseURL:   upstream.URL,
			APIKey:    "test-key",
			TimeoutMS: 1000,
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"gpt-4o",
		"system":"System instructions for debugging",
		"max_tokens":16,
		"messages":[{"role":"user","content":"hello from the user side with enough text"}]
	}`))
	rec := httptest.NewRecorder()

	h.HandleMessages(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body=%s", rec.Code, rec.Body.String())
	}

	logs := logBuf.String()
	for _, want := range []string{
		"stage=upstream_request_failed",
		"prompt_preview=",
		"System instructions",
		"hello from the user",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs missing %q; logs=%s", want, logs)
		}
	}
}
