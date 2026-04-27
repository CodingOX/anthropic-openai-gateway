package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"anthropic-openai-gateway/internal/config"
)

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
