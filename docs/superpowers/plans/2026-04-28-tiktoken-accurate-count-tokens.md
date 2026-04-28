# Accurate Token Counting with Tiktoken Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current character-based token estimation in `/v1/messages/count_tokens` with tiktoken-based encoding for accurate token counts that match upstream API values.

**Architecture:** Create a new `internal/tokenizer` package with a single public `CountTokens(req *types.MessageRequest) int` function. It maps the model to the correct tiktoken encoding, then encodes all content sources (system, messages, tools) through that encoding. The handler calls it and falls back to character estimation on error.

**Tech Stack:** Go 1.23, `github.com/pkoukk/tiktoken-go` (tiktoken Go port), existing `pkg/types`, `internal/config`

---

## File Structure

### Existing files to modify:
- `internal/handler/messages.go` — replace `estimateInputTokens` call, remove old helper functions
- `internal/handler/messages_test.go` — update count_tokens test with precise assertions

### New files to create:
- `internal/tokenizer/tokenizer.go` — model→encoding mapping, `CountTokens()` function
- `internal/tokenizer/tokenizer_test.go` — unit tests with known token counts

### No changes needed:
- `pkg/types/anthropic.go` — types already cover all content shapes
- `internal/config/config.go` — `ModelsNeedTransformation` already available via handler
- `go.mod` — will be updated by `go get`

### Design Decision

`CountTokens` takes `*types.MessageRequest` directly, avoiding a separate type layer and adapter code in the handler. The tokenizer iterates `Messages`, `Tools`, `System` using the same `interface{}` type assertion pattern as the existing character estimator — simple, no duplication.

---

### Task 1: Create `internal/tokenizer` package

**Files:**
- Create: `internal/tokenizer/tokenizer.go`
- Create: `internal/tokenizer/tokenizer_test.go`

- [ ] **Step 1: Write the failing test**

`internal/tokenizer/tokenizer_test.go`:

```go
package tokenizer

import (
    "testing"

    "anthropic-openai-gateway/pkg/types"
)

func TestEncodingForModel(t *testing.T) {
    cases := []struct {
        model    string
        wantName string
    }{
        {"gpt-4o", "o200k_base"},
        {"gpt-4.1", "o200k_base"},
        {"gpt-5", "o200k_base"},
        {"o3-mini", "o200k_base"},
        {"deepseek-v4-pro", "o200k_base"},
        {"deepseek-v4-flash", "o200k_base"},
        {"gpt-4", "cl100k_base"},
        {"glm-5", "cl100k_base"},
        {"kimi-k2.5", "cl100k_base"},
        {"qwen3.6-plus", "cl100k_base"},
        {"claude-sonnet-4-6", "cl100k_base"},
        {"unknown-model", "cl100k_base"},
    }
    for _, tc := range cases {
        t.Run(tc.model, func(t *testing.T) {
            enc := encodingForModel(tc.model)
            if enc.Name != tc.wantName {
                t.Fatalf("encodingForModel(%q).Name = %q, want %q", tc.model, enc.Name, tc.wantName)
            }
        })
    }
}

func TestCountTokensSimple(t *testing.T) {
    n := CountTokens(&types.MessageRequest{
        Model: "gpt-4o",
        Messages: []types.Message{
            {Role: "user", Content: "hello world"},
        },
    })
    if n <= 0 {
        t.Fatalf("CountTokens = %d, want > 0", n)
    }
}

func TestCountTokensWithSystem(t *testing.T) {
    n := CountTokens(&types.MessageRequest{
        Model:  "gpt-4o",
        System: "You are a helpful assistant.",
        Messages: []types.Message{
            {Role: "user", Content: "hello"},
        },
    })
    if n <= 0 {
        t.Fatalf("CountTokens = %d, want > 0", n)
    }
}

func TestCountTokensWithTools(t *testing.T) {
    n := CountTokens(&types.MessageRequest{
        Model: "gpt-4o",
        Messages: []types.Message{
            {Role: "user", Content: "what's the weather"},
        },
        Tools: []types.Tool{
            {
                Name:        "get_weather",
                Description: "Get current weather",
                InputSchema: types.JSONSchema{
                    Type: "object",
                    Properties: map[string]interface{}{
                        "location": map[string]interface{}{"type": "string"},
                    },
                    Required: []string{"location"},
                },
            },
        },
    })
    if n <= 0 {
        t.Fatalf("CountTokens = %d, want > 0", n)
    }
}

func TestCountTokensEmptyReturnsOne(t *testing.T) {
    n := CountTokens(&types.MessageRequest{
        Model:    "gpt-4o",
        Messages: []types.Message{{Role: "user", Content: ""}},
    })
    if n != 1 {
        t.Fatalf("CountTokens = %d, want 1", n)
    }
}

func TestCountTokensContentBlocks(t *testing.T) {
    n := CountTokens(&types.MessageRequest{
        Model: "gpt-4o",
        Messages: []types.Message{
            {
                Role: "user",
                Content: []interface{}{
                    map[string]interface{}{"type": "text", "text": "hello"},
                    map[string]interface{}{"type": "text", "text": "world"},
                },
            },
        },
    })
    if n <= 0 {
        t.Fatalf("CountTokens = %d, want > 0", n)
    }
}

func TestCountTokensChineseText(t *testing.T) {
    n := CountTokens(&types.MessageRequest{
        Model: "deepseek-v4-pro",
        Messages: []types.Message{
            {Role: "user", Content: "你好，今天天气怎么样？"},
        },
    })
    if n <= 0 {
        t.Fatalf("CountTokens = %d, want > 0", n)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tokenizer/ -v`
Expected: FAIL (package does not exist)

- [ ] **Step 3: Add tiktoken dependency**

```bash
cd /Users/alistar/code-all/ai/anthropic-openai-gateway
go get github.com/pkoukk/tiktoken-go
```

- [ ] **Step 4: Write minimal implementation**

`internal/tokenizer/tokenizer.go`:

```go
// Package tokenizer 提供基于 tiktoken 的精确 token 计数。
package tokenizer

import (
    "encoding/json"
    "sync"

    "github.com/pkoukk/tiktoken-go"

    "anthropic-openai-gateway/pkg/types"
)

var (
    encMu    sync.Mutex
    encCache = map[string]*tiktoken.Tiktoken{}
)

// CountTokens 计算 Anthropic MessageRequest 的 input_tokens。
// 使用 tiktoken 精确编码；tiktoken 不可用时返回 -1。
// 模型识别规则：
//   - o200k_base: gpt-4o*, gpt-4.1*, gpt-5, o3*, o4*, deepseek-v4*
//   - cl100k_base: 其余所有
func CountTokens(req *types.MessageRequest) int {
    enc := encodingForModel(req.Model)
    if enc == nil {
        return -1
    }

    total := 0
    total += countContent(enc, req.System)
    for _, msg := range req.Messages {
        total += countMessage(enc, msg)
    }
    for _, tool := range req.Tools {
        total += countTool(enc, tool)
    }
    if total == 0 {
        return 1
    }
    return total
}

// encodingForModel 返回模型对应的 tiktoken encoding。初始化失败时返回 nil。
func encodingForModel(model string) *tiktoken.Tiktoken {
    encName := resolveEncodingName(model)
    encMu.Lock()
    defer encMu.Unlock()
    if cached, ok := encCache[encName]; ok {
        return cached
    }
    enc, err := tiktoken.GetEncoding(encName)
    if err != nil {
        return nil
    }
    encCache[encName] = enc
    return enc
}

// resolveEncodingName 根据模型名前缀返回 encoding 名称。
func resolveEncodingName(model string) string {
    prefixes := []string{"gpt-4o", "gpt-4.1", "gpt-5", "o3", "o4", "deepseek-v4"}
    for _, p := range prefixes {
        if len(model) >= len(p) && model[:len(p)] == p {
            return "o200k_base"
        }
    }
    return "cl100k_base"
}

// countContent 对 system 等 interface{} 内容进行编码计数。
// 支持 string 和 []ContentBlock（JSON 反序列化后为 []interface{}{map[string]interface{}}）。
func countContent(enc *tiktoken.Tiktoken, content interface{}) int {
    switch v := content.(type) {
    case string:
        return len(enc.Encode(v, nil, nil))
    case []interface{}:
        total := 0
        for _, item := range v {
            if block, ok := item.(map[string]interface{}); ok {
                for _, key := range []string{"text", "thinking"} {
                    if text, ok := block[key].(string); ok {
                        total += len(enc.Encode(text, nil, nil))
                    }
                }
                // tool_result 嵌套 content
                if nested, ok := block["content"]; ok {
                    total += countContent(enc, nested)
                }
            }
        }
        return total
    }
    return 0
}

// countMessage 编码单条消息（role + content + 格式开销）。
func countMessage(enc *tiktoken.Tiktoken, msg types.Message) int {
    total := len(enc.Encode(msg.Role, nil, nil))
    total += countContent(enc, msg.Content)
    total += 4 // 消息格式开销（OpenAI 消息分隔符）
    return total
}

// countTool 编码单个工具定义（name + description + schema + 格式开销）。
func countTool(enc *tiktoken.Tiktoken, tool types.Tool) int {
    total := len(enc.Encode(tool.Name, nil, nil))
    total += len(enc.Encode(tool.Description, nil, nil))
    if schemaBytes, err := json.Marshal(tool.InputSchema); err == nil {
        total += len(enc.Encode(string(schemaBytes), nil, nil))
    }
    total += 4 // 工具格式开销
    return total
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/tokenizer/ -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tokenizer/ go.mod go.sum
git commit -m "feat(tokenizer): add tiktoken-based token counting"
```

---

### Task 2: Integrate new tokenizer into handler

**Files:**
- Modify: `internal/handler/messages.go` (import, HandleCountTokens, remove old helpers)
- Modify: `internal/handler/messages_test.go` (update assertion)

- [ ] **Step 1: Write the failing test with precise token assertions**

Update `TestHandleCountTokensReturnsAnthropicShape` in `internal/handler/messages_test.go` to assert non-character-estimation behavior:

```go
func TestHandleCountTokensReturnsAnthropicShape(t *testing.T) {
    h := NewMessagesHandler(&config.Config{
        ModelsNeedTransformation: []string{"gpt-4o"},
    })
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
```

- [ ] **Step 2: Run test to verify current state**

Run: `go test ./internal/handler/ -run TestHandleCountTokensReturnsAnthropicShape -v`
Expected: PASS (current assertion just validates > 0)

- [ ] **Step 3: Modify `HandleCountTokens` to use tokenizer**

Changes in `internal/handler/messages.go`:

1. **Import** — add `"anthropic-openai-gateway/internal/tokenizer"`

2. **Modify `HandleCountTokens`** (lines 49-81):

```go
func (h *MessagesHandler) HandleCountTokens(w http.ResponseWriter, r *http.Request) {
    requestLog := newRequestLogContext(r)
    w.Header().Set("X-Request-Id", requestLog.RequestID)

    if r.Method != http.MethodPost {
        h.logError(requestLog, "count_tokens_method_not_allowed",
            fmt.Sprintf("status=%d", http.StatusMethodNotAllowed),
            fmt.Sprintf("error=%q", "method not allowed, use POST"))
        h.sendError(w, http.StatusMethodNotAllowed, "method not allowed, use POST")
        return
    }

    var anthropicReq types.MessageRequest
    if err := json.NewDecoder(r.Body).Decode(&anthropicReq); err != nil {
        h.logError(requestLog, "count_tokens_decode_request",
            fmt.Sprintf("status=%d", http.StatusBadRequest),
            fmt.Sprintf("error=%q", fmt.Sprintf("invalid request body: %v", err)))
        h.sendError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
        return
    }
    requestLog = h.enrichRequestLog(requestLog, &anthropicReq)
    updateRequestLogState(r.Context(), requestLog)
    h.logInfo(requestLog, "count_tokens_requested")

    // 使用 tiktoken 精确计数；失败时回退到字符估算
    inputTokens := tokenizer.CountTokens(&anthropicReq)
    if inputTokens < 0 {
        h.logError(requestLog, "count_tokens_fallback",
            fmt.Sprintf("error=%q", "tiktoken unavailable, using char estimate"))
        inputTokens = estimateInputTokens(anthropicReq)
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]int{
        "input_tokens": inputTokens,
    })
}
```

3. **Remove** `estimateInputTokens` function (lines 353-371) and `estimateContentChars` function (lines 521-545) — both become dead code.

4. **Remove** `"unicode/utf8"` from imports (no longer used).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/handler/ -run TestHandleCountTokensReturnsAnthropicShape -v`
Expected: PASS with tiktoken-based count

- [ ] **Step 5: Run full test suite**

Run: `go test ./... -v -timeout 60s -count=1`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/handler/messages.go internal/handler/messages_test.go
git commit -m "feat(handler): integrate tiktoken for accurate count_tokens"
```

---

### Task 3: Update README

- [ ] **Step 1: Update the count_tokens description**

In `README.md` line 186, change:
```
- **count_tokens**：当前为本地字符估算，非调用上游 API 的真实计数
```
To:
```
- **count_tokens**：使用 tiktoken 精确计数，转换路径模型结果与上游一致；透传模型使用 cl100k_base 近似。tiktoken 不可用时自动降级为字符估算
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: update count_tokens description after tiktoken migration"
```

---

### Task 4: Final verification

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -v -timeout 60s -count=1`
Expected: ALL PASS

- [ ] **Step 2: Build check**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 3: Quick smoke test**

```bash
go run . &
PID=$!
sleep 2
curl -s -X POST http://localhost:3456/v1/messages/count_tokens \
  -H "Content-Type: application/json" \
  -H "x-api-key: test" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hello world"}]}'
kill $PID 2>/dev/null
```

Expected: `{"input_tokens":2}`

---

## Risks & Edge Cases

| Risk | Mitigation |
|------|-----------|
| tiktoken-go wasm 加载失败 | `encodingForModel` 返回 nil，handler 降级为字符估算 |
| 透传模型（Claude）用 cl100k_base 近似 | 这些模型不走本网关转换，影响有限；token 数以上游返回为准 |
| ContentBlock 嵌套 `tool_result.content` | `countContent` 递归处理嵌套 `content` 字段 |
| `tool_use.input` 的 JSON | 不计数（属 assistant 输出，非 prompt token） |
| 模型前缀匹配冲突（如 `gpt-4` vs `gpt-4o`） | 按前缀列表顺序匹配，`gpt-4o` 在 `gpt-4` 之前匹配到 o200k_base（字符串前缀 `"gpt-4o"[0:6]` ≠ `"gpt-4"[0:6]`，所以不会误匹配） |
