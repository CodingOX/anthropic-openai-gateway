package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"anthropic-openai-gateway/pkg/types"
)

func TestRecoverMiddlewareLogsPanicWithRequestContext(t *testing.T) {
	logBuf, restoreLogs := captureLogs(t)
	defer restoreLogs()

	handler := RecoverMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestLog := newRequestLogContext(r)
		requestLog = requestLog.withAnthropicRequest(&types.MessageRequest{
			Model:     "gpt-4o",
			MaxTokens: 32,
			Messages: []types.Message{
				{Role: "user", Content: "panic case"},
			},
		})
		updateRequestLogState(r.Context(), requestLog)
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"gpt-4o"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Request-Id"); got == "" {
		t.Fatalf("X-Request-Id = empty, want value")
	}

	logs := logBuf.String()
	for _, want := range []string{
		"stage=panic_recovered",
		"request_id=",
		"model=gpt-4o",
		"messages=1",
		"panic=\"boom\"",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs missing %q; logs=%s", want, logs)
		}
	}
}
