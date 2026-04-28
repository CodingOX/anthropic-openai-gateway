# Pass-Through + GPT-5 Developer Role Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable pass-through forwarding for non-transformed models to Anthropic API, plus extend developer role to gpt-5 series.

**Architecture:**
1. **Pass-through**: New `AnthropicClient` (symmetrical to existing `OpenAIClient`) forwards raw Anthropic-format requests as-is to the upstream Anthropic API without any transformation. The handler checks the model list and routes to pass-through when the model is not found.
2. **GPT-5 developer role**: Simple prefix check addition in `request.go` — `strings.HasPrefix(model, "gpt-5")` alongside existing `"o"` prefix.

**Tech Stack:** Go 1.23+ standard library only (no external deps)

---

## File Structure

### Files to Create
| File | Responsibility |
|------|---------------|
| `internal/client/anthropic.go` | Anthropic API HTTP client — forward requests as-is, no transformation |

### Files to Modify
| File | Change |
|------|--------|
| `internal/config/config.go` | Add `AnthropicConfig` struct + env var overrides |
| `configs/config.example.json` | Add `anthropic` config section |
| `internal/handler/messages.go` | Read raw body first; add `handlePassThrough` routing (streaming + non-streaming) |
| `internal/transformer/request.go` | Add gpt-5 prefix to developer role check |
| `README.md` | Update known limitations |

### Files to Test
| File | What to Test |
|------|-------------|
| `internal/client/anthropic_test.go` | Anthropic client request/response forwarding |
| `internal/handler/messages_test.go` | Pass-through routing for non-transform models |
| `internal/transformer/request_test.go` | gpt-5 models use developer role |

---

## Task 1: Config — Add Anthropic backend config

**Files:**
- Modify: `internal/config/config.go` — add `AnthropicConfig` struct + env overrides
- Modify: `configs/config.example.json` — add `anthropic` section

- [ ] **Step 1: Add `AnthropicConfig` struct and `Anthropic` field to `Config`**

```go
// After OpenAIConfig
type AnthropicConfig struct {
    BaseURL   string `json:"base_url"`
    APIKey    string `json:"api_key"`
    TimeoutMS int    `json:"timeout_ms"`
}
```

Add to `Config` struct:
```go
Anthropic AnthropicConfig `json:"anthropic"`
```

Set defaults in `Load()`:
```go
cfg.Anthropic = AnthropicConfig{
    BaseURL:   "https://api.anthropic.com/v1",
    TimeoutMS: 120000,
}
```

- [ ] **Step 2: Add env var overrides to `overrideFromEnv`**

```go
if v := os.Getenv("ANTHROPIC_BASE_URL"); v != "" {
    cfg.Anthropic.BaseURL = v
}
if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
    cfg.Anthropic.APIKey = v
}
if v := os.Getenv("ANTHROPIC_TIMEOUT_MS"); v != "" {
    if t, err := strconv.Atoi(v); err == nil {
        cfg.Anthropic.TimeoutMS = t
    }
}
```

- [ ] **Step 3: Update `configs/config.example.json`**

Add after the `openai` block:
```json
"anthropic": {
    "base_url": "https://api.anthropic.com/v1",
    "api_key": "${ANTHROPIC_API_KEY}",
    "timeout_ms": 120000
}
```

- [ ] **Step 4: Verify it compiles**

Run: `cd /Users/alistar/code-all/ai/anthropic-openai-gateway && go build ./internal/config/`
Expected: no error

---

## Task 2: Anthropic Client — Forward requests as-is

**Files:**
- Create: `internal/client/anthropic.go`
- Test: `internal/client/anthropic_test.go`

- [ ] **Step 1: Write the failing tests** (`internal/client/anthropic_test.go`)

```go
package client

import (
    "context"
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestAnthropicClientNonStreaming(t *testing.T) {
    var gotBody string
    var gotHeaders http.Header

    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        gotHeaders = r.Header
        body, _ := io.ReadAll(r.Body)
        gotBody = string(body)
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"type":"message","content":[{"type":"text","text":"ok"}]}`))
    }))
    defer upstream.Close()

    c := NewAnthropicClient(&config.Config{
        Anthropic: config.AnthropicConfig{
            BaseURL: upstream.URL,
            APIKey:  "sk-ant-test123",
            TimeoutMS: 5000,
        },
    })

    reqBody := `{"model":"claude-sonnet-4-20250514","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`
    resp, err := c.ForwardMessage(context.Background(), strings.NewReader(reqBody), false)
    if err != nil {
        t.Fatalf("ForwardMessage() error = %v", err)
    }
    defer resp.Body.Close()

    respBody, _ := io.ReadAll(resp.Body)
    if got := string(respBody); !strings.Contains(got, `"ok"`) {
        t.Fatalf("body = %s, want ok", got)
    }

    // Verify auth header was set correctly
    if gotHeaders.Get("x-api-key") != "sk-ant-test123" {
        t.Fatalf("x-api-key = %q, want sk-ant-test123", gotHeaders.Get("x-api-key"))
    }
    if gotHeaders.Get("anthropic-version") != "2023-06-01" {
        t.Fatalf("anthropic-version = %q, want 2023-06-01", gotHeaders.Get("anthropic-version"))
    }

    // Verify original body was forwarded
    if strings.TrimSpace(gotBody) != reqBody {
        t.Fatalf("forwarded body mismatch:\ngot:  %s\nwant: %s", gotBody, reqBody)
    }
}

func TestAnthropicClientStreaming(t *testing.T) {
    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/event-stream")
        w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n"))
        w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
    }))
    defer upstream.Close()

    c := NewAnthropicClient(&config.Config{
        Anthropic: config.AnthropicConfig{
            BaseURL: upstream.URL,
            APIKey:  "sk-ant-test123",
            TimeoutMS: 5000,
        },
    })

    reqBody := `{"model":"claude-sonnet-4-20250514","stream":true,"max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`
    resp, err := c.ForwardMessage(context.Background(), strings.NewReader(reqBody), true)
    if err != nil {
        t.Fatalf("ForwardMessage() streaming error = %v", err)
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    if !strings.Contains(string(body), "message_start") {
        t.Fatalf("streaming body missing message_start: %s", body)
    }
}

func TestAnthropicClientError(t *testing.T) {
    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusBadRequest)
        w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad request"}}`))
    }))
    defer upstream.Close()

    c := NewAnthropicClient(&config.Config{
        Anthropic: config.AnthropicConfig{
            BaseURL: upstream.URL,
            APIKey:  "sk-ant-test123",
            TimeoutMS: 5000,
        },
    })

    _, err := c.ForwardMessage(context.Background(), strings.NewReader("{}"), false)
    if err == nil {
        t.Fatal("expected error, got nil")
    }
}
```

Wait — `ForwardMessage` returns `(*http.Response, error)`. For errors, we should return the upstream response body. Let me reconsider the API. Actually, for pass-through, the simplest approach is to return the raw `*http.Response` so the handler can copy headers and body directly. For error cases, the handler should also pass through the error status code and body.

Let me revise — the client should return `(*http.Response, error)` where on error (transport/network), we return a proper error, but HTTP-level errors (4xx, 5xx) should still be returned as `*http.Response` because the handler needs to proxy them back to the caller. Actually no, 4xx/5xx errors ARE part of the response — the upstream communicates errors via the response body. So:

```go
func (c *AnthropicClient) ForwardMessage(ctx context.Context, body io.Reader, stream bool) (*http.Response, error)
```

This returns the raw response. Non-2xx responses are still returned (not wrapped as errors), so the handler can proxy them back. The error is only for transport-level failures.

Wait, but the existing `OpenAIClient` does return errors for non-200. Let me keep it consistent with the existing pattern but simpler — since we're forwarding, just pass through the raw response, errors included.

Actually, for KISS and to make it easy to test, let me make the client super simple:
- Send the request
- Return the response (whatever status code)
- Error only for transport failures

Let me rewrite the test:

```go
func TestAnthropicClient(t *testing.T) {
    ...
    resp, err := c.ForwardMessage(ctx, body, false)
    if err != nil {
        t.Fatalf("...")
    }
    defer resp.Body.Close()
    // resp.StatusCode should be the upstream status
    // resp.Body should be the upstream body
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/alistar/code-all/ai/anthropic-openai-gateway && go test ./internal/client/ -run TestAnthropicClient -v`
Expected: FAIL — package not found or not compiled

- [ ] **Step 3: Write minimal implementation** (`internal/client/anthropic.go`)

```go
package client

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "time"

    "anthropic-openai-gateway/internal/config"
)

const anthropicAPIBase = "/messages"
const defaultAnthropicVersion = "2023-06-01"

type AnthropicClient struct {
    baseURL    string
    apiKey     string
    httpClient *http.Client
}

func NewAnthropicClient(cfg *config.Config) *AnthropicClient {
    return &AnthropicClient{
        baseURL: cfg.Anthropic.BaseURL,
        apiKey:  cfg.Anthropic.APIKey,
        httpClient: &http.Client{
            Timeout: time.Duration(cfg.Anthropic.TimeoutMS) * time.Millisecond,
        },
    }
}

// ForwardMessage forwards the request body to Anthropic Messages API as-is.
// stream=true sets stream: true in the request and returns a streaming response.
func (c *AnthropicClient) ForwardMessage(ctx context.Context, body io.Reader, stream bool) (*http.Response, error) {
    endpoint := c.baseURL + anthropicAPIBase

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }

    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("x-api-key", c.apiKey)
    httpReq.Header.Set("anthropic-version", defaultAnthropicVersion)

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("upstream request failed: %w", err)
    }

    return resp, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/alistar/code-all/ai/anthropic-openai-gateway && go test ./internal/client/ -run TestAnthropicClient -v`
Expected: PASS

---

## Task 3: GPT-5 Developer Role — Extend prefix check

**Files:**
- Modify: `internal/transformer/request.go:52-54`
- Test: `internal/transformer/request_test.go`

- [ ] **Step 1: Write the failing tests** (`internal/transformer/request_test.go`)

```go
func TestTransformRequestUsesDeveloperRoleForGPT5(t *testing.T) {
    tests := []struct {
        name  string
        model string
        want  string
    }{
        {"gpt-5 uses developer", "gpt-5", "developer"},
        {"gpt-5.4 uses developer", "gpt-5.4", "developer"},
        {"gpt-5-mini uses developer", "gpt-5-mini", "developer"},
        {"gpt-4o still uses system", "gpt-4o", "system"},
        {"o3 uses developer", "o3", "developer"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := &types.MessageRequest{
                Model:     tt.model,
                System:    "You are helpful.",
                MaxTokens: 128,
                Messages:  []types.Message{{Role: "user", Content: "hi"}},
            }
            got, err := NewRequestTransformer().TransformRequest(req)
            if err != nil {
                t.Fatalf("TransformRequest() error = %v", err)
            }
            if got.Messages[0].Role != tt.want {
                t.Fatalf("role = %q, want %q for model %s", got.Messages[0].Role, tt.want, tt.model)
            }
        })
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/alistar/code-all/ai/anthropic-openai-gateway && go test ./internal/transformer/ -run TestTransformRequestUsesDeveloperRoleForGPT5 -v`
Expected: FAIL — gpt-5 and gpt-5.4 lines fail

- [ ] **Step 3: Modify prefix check** (`internal/transformer/request.go:52-54`)

Change:
```go
if strings.HasPrefix(ar.Model, "o") {
    systemRole = "developer"
}
```

To:
```go
if strings.HasPrefix(ar.Model, "o") || strings.HasPrefix(ar.Model, "gpt-5") {
    systemRole = "developer"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/alistar/code-all/ai/anthropic-openai-gateway && go test ./internal/transformer/ -run TestTransformRequestUsesDeveloperRoleForGPT5 -v`
Expected: PASS

- [ ] **Step 5: Run all transformer tests**

Run: `cd /Users/alistar/code-all/ai/anthropic-openai-gateway && go test ./internal/transformer/ -v`
Expected: all pass

---

## Task 4: Handler — Implement pass-through routing

**Files:**
- Modify: `internal/handler/messages.go` — add `handlePassThrough` + raw body pre-read
- Test: `internal/handler/messages_test.go` — add pass-through tests

- [ ] **Step 1: Write the failing tests** (`internal/handler/messages_test.go`)

```go
func TestHandleMessagesPassThroughNonStreaming(t *testing.T) {
    var capturedBody string
    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        capturedBody = string(body)
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hello from claude"}],"model":"claude-sonnet-4-20250514","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`))
    }))
    defer upstream.Close()

    h := NewMessagesHandler(&config.Config{
        ModelsNeedTransformation: []string{"gpt-4o"},
        Anthropic: config.AnthropicConfig{
            BaseURL:   upstream.URL,
            APIKey:    "sk-ant-test",
            TimeoutMS: 5000,
        },
    })

    req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
        "model":"claude-sonnet-4-20250514",
        "max_tokens":100,
        "messages":[{"role":"user","content":"hello"}]
    }`))
    rec := httptest.NewRecorder()

    h.HandleMessages(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
    }

    // Verify the response is the original Anthropic body
    var resp map[string]interface{}
    if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
        t.Fatalf("json.Unmarshal() error = %v; body=%s", err, rec.Body.String())
    }

    if resp["id"] != "msg_1" {
        t.Fatalf("response id = %v, want msg_1", resp["id"])
    }

    // Verify the request was forwarded as-is
    if !strings.Contains(capturedBody, `"model":"claude-sonnet-4-20250514"`) {
        t.Fatalf("forwarded body missing original model: %s", capturedBody)
    }
}

func TestHandleMessagesPassThroughReturnsUpstreamError(t *testing.T) {
    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusBadRequest)
        w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad request"}}`))
    }))
    defer upstream.Close()

    h := NewMessagesHandler(&config.Config{
        ModelsNeedTransformation: []string{"gpt-4o"},
        Anthropic: config.AnthropicConfig{
            BaseURL:   upstream.URL,
            APIKey:    "sk-ant-test",
            TimeoutMS: 5000,
        },
    })

    req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
        "model":"claude-sonnet-4-20250514",
        "max_tokens":100,
        "messages":[{"role":"user","content":"hello"}]
    }`))
    rec := httptest.NewRecorder()

    h.HandleMessages(rec, req)

    if rec.Code != http.StatusBadRequest {
        t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
    }
}
```

- [ ] **Step 2: Add `NewAnthropicClient` dependency to `MessagesHandler`** (`internal/handler/messages.go`)

Add field to struct:
```go
anthropicClient *client.AnthropicClient
```

Initialize in `NewMessagesHandler`:
```go
anthropicClient: client.NewAnthropicClient(cfg),
```

- [ ] **Step 3: Change request body reading to pre-read raw bytes** (`internal/handler/messages.go`)

In `HandleMessages`, replace `json.NewDecoder(r.Body).Decode(&anthropicReq)` with:
```go
rawBody, err := io.ReadAll(r.Body)
if err != nil {
    // ...
}

var anthropicReq types.MessageRequest
if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
    // ...
}
```

- [ ] **Step 4: Add pass-through handlers** (`internal/handler/messages.go`)

After `handleNonStreaming`:

```go
func (h *MessagesHandler) handlePassThrough(w http.ResponseWriter, r *http.Request, anthropicReq *types.MessageRequest, rawBody []byte, requestLog requestLogContext) {
    startedAt := time.Now()

    isStreaming := anthropicReq.Stream != nil && *anthropicReq.Stream
    ctx := r.Context()

    // Forward as-is to Anthropic API
    resp, err := h.anthropicClient.ForwardMessage(ctx, bytes.NewReader(rawBody), isStreaming)
    if err != nil {
        h.logError(requestLog, "pass_through_upstream_error",
            fmt.Sprintf("duration_ms=%d", time.Since(startedAt).Milliseconds()),
            fmt.Sprintf("error=%q", err.Error()))
        h.sendError(w, http.StatusBadGateway, fmt.Sprintf("upstream request failed: %v", err))
        return
    }
    defer resp.Body.Close()

    // Pass through status code and content type
    for key, values := range resp.Header {
        for _, v := range values {
            w.Header().Add(key, v)
        }
    }
    w.WriteHeader(resp.StatusCode)

    // Copy body
    copied, _ := io.Copy(w, resp.Body)
    _ = copied

    h.logInfo(requestLog, "pass_through_completed",
        fmt.Sprintf("duration_ms=%d", time.Since(startedAt).Milliseconds()),
        fmt.Sprintf("upstream_status=%d", resp.StatusCode))
}
```

Modify `HandleMessages` to route non-transform models:
```go
if !h.needsTransformation(anthropicReq.Model) {
    h.handlePassThrough(w, r, &anthropicReq, rawBody, requestLog)
    return
}
```

- [ ] **Step 5: Add missing imports**

Ensure `bytes`, `io`, and `time` are imported in `internal/handler/messages.go`.

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/alistar/code-all/ai/anthropic-openai-gateway && go test ./internal/handler/ -run TestHandleMessagesPassThrough -v`
Expected: PASS

- [ ] **Step 7: Run ALL tests**

Run: `cd /Users/alistar/code-all/ai/anthropic-openai-gateway && go test ./...`
Expected: all pass

---

## Task 5: Verify build and update README

**Files:**
- Modify: `README.md` — update known limitations

- [ ] **Step 1: Full build check**

Run: `cd /Users/alistar/code-all/ai/anthropic-openai-gateway && go build ./cmd/gateway/`
Expected: no error

- [ ] **Step 2: Update README known limitations**

Update section:

```markdown
## 已知限制

- **o-series/gpt-5 模型**：自动使用 `developer` role 替代 `system`（符合 OpenAI 官方推荐）
- **thinking/推理**：Anthropic 的 thinking 内容已支持（↔ OpenAI reasoning_content），但 `thinking` 配置参数（如 `budget_tokens`）未做特殊处理
- **缓存控制**：Anthropic 的 `cache_control` 参数被忽略（OpenAI 无等价机制，自动管理缓存）
```

Remove the "透传模式未实现" line since it's now implemented.

- [ ] **Step 3: Verify go vet**

Run: `cd /Users/alistar/code-all/ai/anthropic-openai-gateway && go vet ./...`
Expected: no warnings
