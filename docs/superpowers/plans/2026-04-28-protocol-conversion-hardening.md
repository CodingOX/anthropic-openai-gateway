# Protocol Conversion Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the Anthropic/OpenAI protocol conversion layer by improving streaming content compatibility, propagating top-level thinking intent, and normalizing cache usage extraction across upstream variants.

**Architecture:** Keep the existing handler and transformer boundaries intact. Add narrow compatibility helpers inside transformer and DTO layers so request/response behavior changes remain local, testable, and low-risk.

**Tech Stack:** Go, standard testing package, existing transformer/types packages

---

### Task 1: Streaming Structured Content Compatibility

**Files:**
- Modify: `pkg/types/openai.go`
- Modify: `internal/transformer/stream.go`
- Test: `internal/transformer/stream_test.go`

- [ ] **Step 1: Write the failing test**

Add a stream test covering an OpenAI chunk whose `delta.content` is a structured array containing text parts.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transformer -run TestProxyStreamEmitsTextFromStructuredContentDelta`

Expected: FAIL because `ChatDelta.Content` only accepts string content.

- [ ] **Step 3: Write minimal implementation**

Change the streaming delta DTO to accept either string or array content, add a helper that extracts text parts from the delta payload, and reuse the existing Anthropic SSE text emission path.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/transformer -run TestProxyStreamEmitsTextFromStructuredContentDelta`

Expected: PASS.

### Task 2: Top-Level Thinking Intent Propagation

**Files:**
- Modify: `internal/transformer/request.go`
- Test: `internal/transformer/request_test.go`

- [ ] **Step 1: Write the failing test**

Add a request transformer test covering top-level `thinking` enabled on an assistant message that lacks explicit thinking blocks.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transformer -run TestTransformRequestAddsReasoningPlaceholderWhenTopLevelThinkingEnabled`

Expected: FAIL because the request currently ignores top-level thinking intent.

- [ ] **Step 3: Write minimal implementation**

Detect top-level thinking enablement and add a minimal `reasoning_content` placeholder to assistant messages that do not already carry reasoning content.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/transformer -run TestTransformRequestAddsReasoningPlaceholderWhenTopLevelThinkingEnabled`

Expected: PASS.

### Task 3: Usage And Cache Normalization

**Files:**
- Modify: `pkg/types/openai.go`
- Modify: `internal/transformer/response.go`
- Modify: `internal/transformer/stream.go`
- Test: `internal/transformer/response_test.go`
- Test: `internal/transformer/stream_test.go`

- [ ] **Step 1: Write the failing tests**

Add response and stream tests covering cache usage coming from fallback shapes such as `prompt_tokens_details.cached_tokens` and `cache_read_input_tokens`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/transformer -run 'TestTransformResponseNormalizesCacheUsageFallbacks|TestProxyStreamNormalizesCacheUsageFallbacks'`

Expected: FAIL because only direct `prompt_cache_hit_tokens` and `prompt_cache_miss_tokens` are consumed.

- [ ] **Step 3: Write minimal implementation**

Extend the upstream usage DTO with compatible cache fields and add one normalization helper reused by both non-streaming and streaming conversion paths.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/transformer -run 'TestTransformResponseNormalizesCacheUsageFallbacks|TestProxyStreamNormalizesCacheUsageFallbacks'`

Expected: PASS.

### Task 4: Final Verification

**Files:**
- Modify: `internal/transformer/request_test.go`
- Modify: `internal/transformer/response_test.go`
- Modify: `internal/transformer/stream_test.go`
- Modify: `internal/transformer/request.go`
- Modify: `internal/transformer/response.go`
- Modify: `internal/transformer/stream.go`
- Modify: `pkg/types/openai.go`

- [ ] **Step 1: Run focused transformer tests**

Run: `go test ./internal/transformer`

Expected: PASS.

- [ ] **Step 2: Run repository test suite**

Run: `go test ./...`

Expected: PASS.