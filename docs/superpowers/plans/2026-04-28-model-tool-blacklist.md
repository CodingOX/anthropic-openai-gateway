# Model Tool Blacklist Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a small model-scoped tool blacklist so known incompatible tools such as `web_search` can be filtered before OpenAI-format upstream requests are sent.

**Architecture:** Keep the policy simple and opt-in: exact model name plus exact tool name. Parse one environment variable into config, inject it into the request transformer, filter matching `tools[]`, and fail fast only when `tool_choice` forces a filtered tool. Do not introduce provider capability matrices, wildcard matching, or schema rewriting in this phase.

**Tech Stack:** Go standard library, existing config/handler/transformer packages, standard `testing` package.

---

## Context

The observed upstream error is:

```text
Invalid schema for function 'web_search': "" is not valid under any of the schemas listed in the 'anyOf' keyword
```

Current transformer behavior converts every Anthropic tool directly into OpenAI `tools[].function.parameters`. That is correct for normal tools, but it means a provider-specific schema incompatibility becomes an upstream 400.

The intended first blacklist entry is:

```dotenv
DISABLED_TOOLS_BY_MODEL=deepseek-v4-flash:web_search,deepseek-v4-pro:web_search
```

Parsing rule:

- Comma separates entries.
- Each entry is `model:tool`.
- Model and tool names are exact-match after trimming spaces.
- Duplicate entries are harmless.
- Blank env value means no blacklist.
- Malformed non-blank entries should make startup fail with a clear config error.

## File Structure

- Modify `internal/config/config.go`
  - Add `DisabledToolsByModel map[string]map[string]bool`.
  - Parse `DISABLED_TOOLS_BY_MODEL`.
  - Log blacklist entry count, not every rule unless the list is small.

- Modify `internal/config/config_test.go`
  - Cover valid parsing, blank value, duplicate entries, and malformed entries.

- Modify `internal/handler/messages.go`
  - Construct request transformer with config blacklist.
  - Log transformed tool counts if filtering happens.

- Modify `internal/transformer/request.go`
  - Store blacklist policy on `RequestTransformer`.
  - Filter tools before converting them.
  - Convert `tool_choice` after filtering.
  - Return a client-facing transform error when `tool_choice` forces a disabled tool.

- Modify `internal/transformer/request_test.go`
  - Cover normal tools unchanged.
  - Cover blacklisted `web_search` removed for `deepseek-v4-flash`.
  - Cover another model still keeps `web_search`.
  - Cover forced `tool_choice` to filtered tool returns error.

- Modify `.env.example`
  - Add commented blacklist example.

- Modify `README.md`
  - Add configuration documentation and one short troubleshooting note.

## Non-Goals

- Do not build a whitelist.
- Do not add wildcard model matching.
- Do not rewrite JSON Schema.
- Do not infer provider capabilities from model names.
- Do not suppress all tools for DeepSeek.

---

### Task 1: Config Parsing

**Files:**

- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write failing config tests**

Add tests for:

```go
func TestLoadParsesDisabledToolsByModel(t *testing.T) {
    runWithoutDotEnv(t)

    t.Setenv("DISABLED_TOOLS_BY_MODEL", "deepseek-v4-flash:web_search, deepseek-v4-pro:web_search")

    cfg, err := Load()
    if err != nil {
        t.Fatalf("Load() error = %v, want nil", err)
    }

    if !cfg.DisabledToolsByModel["deepseek-v4-flash"]["web_search"] {
        t.Fatalf("deepseek-v4-flash web_search blacklist missing: %#v", cfg.DisabledToolsByModel)
    }
    if !cfg.DisabledToolsByModel["deepseek-v4-pro"]["web_search"] {
        t.Fatalf("deepseek-v4-pro web_search blacklist missing: %#v", cfg.DisabledToolsByModel)
    }
}
```

Also add malformed config coverage:

```go
func TestLoadRejectsMalformedDisabledToolsByModel(t *testing.T) {
    runWithoutDotEnv(t)

    t.Setenv("DISABLED_TOOLS_BY_MODEL", "deepseek-v4-flash")

    _, err := Load()
    if err == nil {
        t.Fatal("Load() error = nil, want malformed blacklist error")
    }
}
```

- [ ] **Step 2: Run config tests and verify failure**

Run:

```bash
timeout 60s go test ./internal/config -run 'TestLoadParsesDisabledToolsByModel|TestLoadRejectsMalformedDisabledToolsByModel' -count=1
```

Expected:

```text
FAIL
```

- [ ] **Step 3: Implement parsing**

Implementation shape:

```go
type Config struct {
    ListenPort               int
    APIKey                   string
    BaseURL                  string
    NonStreamingTimeoutMS    int
    ModelsNeedTransformation []string
    DisabledToolsByModel     map[string]map[string]bool
}
```

Add a small parser:

```go
func parseDisabledToolsByModel(value string) (map[string]map[string]bool, error) {
    result := make(map[string]map[string]bool)
    if strings.TrimSpace(value) == "" {
        return result, nil
    }

    for _, entry := range strings.Split(value, ",") {
        entry = strings.TrimSpace(entry)
        if entry == "" {
            continue
        }
        parts := strings.Split(entry, ":")
        if len(parts) != 2 {
            return nil, fmt.Errorf("invalid DISABLED_TOOLS_BY_MODEL entry %q, want model:tool", entry)
        }
        model := strings.TrimSpace(parts[0])
        tool := strings.TrimSpace(parts[1])
        if model == "" || tool == "" {
            return nil, fmt.Errorf("invalid DISABLED_TOOLS_BY_MODEL entry %q, model and tool are required", entry)
        }
        if result[model] == nil {
            result[model] = make(map[string]bool)
        }
        result[model][tool] = true
    }
    return result, nil
}
```

Update `Load()` / `overrideFromEnv()` so parse errors are returned.

- [ ] **Step 4: Run config tests and verify pass**

Run:

```bash
timeout 60s go test ./internal/config -count=1
```

Expected:

```text
ok  	anthropic-openai-gateway/internal/config
```

---

### Task 2: Transformer Filtering

**Files:**

- Modify: `internal/transformer/request.go`
- Test: `internal/transformer/request_test.go`

- [ ] **Step 1: Write failing transformer tests**

Add test for filtering:

```go
func TestTransformRequestFiltersDisabledToolsByModel(t *testing.T) {
    req := &types.MessageRequest{
        Model:     "deepseek-v4-flash",
        MaxTokens: 128,
        Messages:  []types.Message{{Role: "user", Content: "hi"}},
        Tools: []types.Tool{
            {Name: "web_search", InputSchema: types.JSONSchema{Type: "object"}},
            {Name: "lookup", InputSchema: types.JSONSchema{Type: "object"}},
        },
    }

    transformer := NewRequestTransformerWithDisabledTools(map[string]map[string]bool{
        "deepseek-v4-flash": {"web_search": true},
    })

    got, err := transformer.TransformRequest(req)
    if err != nil {
        t.Fatalf("TransformRequest() error = %v", err)
    }
    if len(got.Tools) != 1 || got.Tools[0].Function.Name != "lookup" {
        t.Fatalf("Tools = %#v, want only lookup", got.Tools)
    }
}
```

Add test for forced tool choice:

```go
func TestTransformRequestRejectsToolChoiceForDisabledTool(t *testing.T) {
    req := &types.MessageRequest{
        Model:     "deepseek-v4-flash",
        MaxTokens: 128,
        Messages:  []types.Message{{Role: "user", Content: "hi"}},
        Tools: []types.Tool{
            {Name: "web_search", InputSchema: types.JSONSchema{Type: "object"}},
        },
        ToolChoice: map[string]interface{}{
            "type": "tool",
            "name": "web_search",
        },
    }

    transformer := NewRequestTransformerWithDisabledTools(map[string]map[string]bool{
        "deepseek-v4-flash": {"web_search": true},
    })

    _, err := transformer.TransformRequest(req)
    if err == nil {
        t.Fatal("TransformRequest() error = nil, want disabled tool_choice error")
    }
}
```

- [ ] **Step 2: Run transformer tests and verify failure**

Run:

```bash
timeout 60s go test ./internal/transformer -run 'TestTransformRequestFiltersDisabledToolsByModel|TestTransformRequestRejectsToolChoiceForDisabledTool' -count=1
```

Expected:

```text
FAIL
```

- [ ] **Step 3: Implement transformer policy**

Keep the old constructor for existing tests:

```go
type RequestTransformer struct {
    disabledToolsByModel map[string]map[string]bool
}

func NewRequestTransformer() *RequestTransformer {
    return NewRequestTransformerWithDisabledTools(nil)
}

func NewRequestTransformerWithDisabledTools(disabled map[string]map[string]bool) *RequestTransformer {
    return &RequestTransformer{disabledToolsByModel: disabled}
}
```

Filter before conversion:

```go
filteredTools, removedTools := t.filterDisabledTools(ar.Model, ar.Tools)
if len(filteredTools) > 0 {
    req.Tools = t.convertTools(filteredTools)
}
if len(removedTools) > 0 && t.toolChoiceForcesDisabledTool(ar.ToolChoice, removedTools) {
    return nil, fmt.Errorf("tool_choice references disabled tool for model %s", ar.Model)
}
if len(req.Tools) > 0 {
    req.ToolChoice = t.convertToolChoice(ar.ToolChoice)
}
```

Add a core Chinese comment near the filtering block:

```go
// 按模型过滤已知不兼容工具，避免供应商因工具 schema 差异直接返回 400。
```

- [ ] **Step 4: Run transformer tests and verify pass**

Run:

```bash
timeout 60s go test ./internal/transformer -count=1
```

Expected:

```text
ok  	anthropic-openai-gateway/internal/transformer
```

---

### Task 3: Handler Wiring

**Files:**

- Modify: `internal/handler/messages.go`
- Existing tests: `internal/handler/messages_test.go`

- [ ] **Step 1: Wire config into transformer**

Change handler construction:

```go
requestTransformer: transformer.NewRequestTransformerWithDisabledTools(cfg.DisabledToolsByModel),
```

- [ ] **Step 2: Confirm transform errors are client errors**

Current transform errors are returned as 500. For a forced disabled `tool_choice`, this is a bad client request, not a gateway crash.

Implement one of these small options:

- Preferable: introduce a typed transformer error with `StatusCode() int`.
- Simpler: return `400` for request transform errors whose message contains `"disabled tool"`.

Recommended minimal typed shape:

```go
type ClientRequestError struct {
    Message string
}

func (e *ClientRequestError) Error() string {
    return e.Message
}
```

Handler:

```go
var clientErr *transformer.ClientRequestError
if errors.As(err, &clientErr) {
    h.sendError(w, http.StatusBadRequest, clientErr.Error())
    return
}
```

- [ ] **Step 3: Run handler tests**

Run:

```bash
timeout 60s go test ./internal/handler -count=1
```

Expected:

```text
ok  	anthropic-openai-gateway/internal/handler
```

---

### Task 4: Docs and Example Env

**Files:**

- Modify: `.env.example`
- Modify: `README.md`

- [ ] **Step 1: Update `.env.example`**

Add:

```dotenv
# 按模型禁用已知不兼容工具，格式：model:tool,model:tool
# 例如 DeepSeek 当前不接受 Claude Code web_search 的部分 schema。
DISABLED_TOOLS_BY_MODEL=deepseek-v4-flash:web_search,deepseek-v4-pro:web_search
```

- [ ] **Step 2: Update README config table**

Add:

```text
DISABLED_TOOLS_BY_MODEL
按模型禁用工具，格式为 model:tool,model:tool。
用于处理特定上游不兼容某个工具 schema 的情况。
```

Add troubleshooting note:

```text
如果上游返回 Invalid schema for function 'web_search'，可以先把对应模型加入
DISABLED_TOOLS_BY_MODEL，而不是全局禁用所有 tools。
```

- [ ] **Step 3: Run docs grep sanity check**

Run:

```bash
rg -n "DISABLED_TOOLS_BY_MODEL|web_search" .env.example README.md
```

Expected:

```text
.env.example:...
README.md:...
```

---

### Task 5: Full Verification

**Files:**

- No new code edits unless tests fail.

- [ ] **Step 1: Run formatting**

Run:

```bash
gofmt -w internal/config/config.go internal/config/config_test.go internal/handler/messages.go internal/transformer/request.go internal/transformer/request_test.go
```

- [ ] **Step 2: Run full test suite**

Run:

```bash
timeout 60s go test ./...
```

Expected:

```text
PASS
ok  	anthropic-openai-gateway/...
```

- [ ] **Step 3: Manual request-shape check**

Use a local unit test or temporary debug output only if needed. Confirm:

- `deepseek-v4-flash + web_search` sends no `web_search` upstream.
- `deepseek-v4-flash + lookup` still sends `lookup`.
- `gpt-4o + web_search` still sends `web_search`.
- `deepseek-v4-flash + tool_choice web_search` returns 400 before upstream call.

- [ ] **Step 4: Final review**

Check:

```bash
git diff -- internal/config/config.go internal/config/config_test.go internal/handler/messages.go internal/transformer/request.go internal/transformer/request_test.go .env.example README.md
```

Verify there is no schema sanitizer, no whitelist, no wildcard matching, and no unrelated tokenizer changes.

---

## Rollout

Start with:

```dotenv
DISABLED_TOOLS_BY_MODEL=deepseek-v4-flash:web_search,deepseek-v4-pro:web_search
```

If another model/tool pair fails later, append one entry:

```dotenv
DISABLED_TOOLS_BY_MODEL=deepseek-v4-flash:web_search,deepseek-v4-pro:web_search,kimi-k2.6:web_search
```

This keeps the policy incident-driven and easy to audit.
