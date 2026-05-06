package transformer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"anthropic-openai-gateway/pkg/types"
)

func TestProxyStreamEmitsCompleteTextEventSequence(t *testing.T) {
	var out bytes.Buffer
	body := sseBody(
		`{"id":"chunk_1","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
		`{"id":"chunk_1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`,
		`[DONE]`,
	)

	if err := NewStreamHandler().ProxyStream(&out, body, "gpt-4o", 0, context.Background(), nil); err != nil {
		t.Fatalf("ProxyStream() error = %v", err)
	}

	events := parseEvents(t, out.String())
	want := []string{
		"message_start",
		"content_block_start",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}
	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d: %#v", len(events), len(want), events)
	}
	for i := range want {
		if events[i].Type != want[i] {
			t.Fatalf("event[%d].Type = %q, want %q", i, events[i].Type, want[i])
		}
	}
	if events[2].Delta == nil || events[2].Delta.Text != "hello" {
		t.Fatalf("text delta = %#v, want hello", events[2].Delta)
	}
}

func TestProxyStreamEmitsThinkingThenText(t *testing.T) {
	var out bytes.Buffer
	body := sseBody(
		`{"id":"chunk_1","choices":[{"index":0,"delta":{"reasoning_content":"think"},"finish_reason":null}]}`,
		`{"id":"chunk_1","choices":[{"index":0,"delta":{"content":"answer"},"finish_reason":null}]}`,
		`{"id":"chunk_1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`[DONE]`,
	)

	if err := NewStreamHandler().ProxyStream(&out, body, "gpt-4o", 0, context.Background(), nil); err != nil {
		t.Fatalf("ProxyStream() error = %v", err)
	}

	events := parseEvents(t, out.String())
	want := []string{
		"message_start",
		"content_block_start",
		"content_block_delta",
		"content_block_stop",
		"content_block_start",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}
	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d: %#v", len(events), len(want), events)
	}
	for i := range want {
		if events[i].Type != want[i] {
			t.Fatalf("event[%d].Type = %q, want %q", i, events[i].Type, want[i])
		}
	}
	if events[2].Delta == nil || events[2].Delta.Type != "thinking_delta" || events[2].Delta.Thinking != "think" {
		t.Fatalf("thinking delta = %#v, want thinking", events[2].Delta)
	}
	if events[5].Delta == nil || events[5].Delta.Type != "text_delta" || events[5].Delta.Text != "answer" {
		t.Fatalf("text delta = %#v, want answer", events[5].Delta)
	}
}

func TestProxyStreamEmitsTextFromStructuredContentDelta(t *testing.T) {
	var out bytes.Buffer
	body := sseBody(
		`{"id":"chunk_1","choices":[{"index":0,"delta":{"content":[{"type":"text","text":"hello"},{"type":"text","text":" world"}]},"finish_reason":null}]}`,
		`{"id":"chunk_1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`[DONE]`,
	)

	if err := NewStreamHandler().ProxyStream(&out, body, "gpt-4o", 0, context.Background(), nil); err != nil {
		t.Fatalf("ProxyStream() error = %v", err)
	}

	events := parseEvents(t, out.String())
	if len(events) < 3 {
		t.Fatalf("event count = %d, want at least 3: %#v", len(events), events)
	}
	if events[2].Delta == nil || events[2].Delta.Text != "hello world" {
		t.Fatalf("text delta = %#v, want hello world", events[2].Delta)
	}
}

func TestProxyStreamSplitsUsageBetweenMessageStartAndDelta(t *testing.T) {
	var out bytes.Buffer
	body := sseBody(
		`{"id":"chunk_1","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
		`{"id":"chunk_1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"prompt_cache_hit_tokens":5,"prompt_cache_miss_tokens":6}}`,
		`[DONE]`,
	)

	if err := NewStreamHandler().ProxyStream(&out, body, "gpt-4o", 11, context.Background(), nil); err != nil {
		t.Fatalf("ProxyStream() error = %v", err)
	}

	events := parseEvents(t, out.String())
	rawEvents := parseRawEvents(t, out.String())
	if len(events) < 2 {
		t.Fatalf("event count = %d, want at least 2", len(events))
	}

	messageStart := events[0]
	if messageStart.Type != "message_start" {
		t.Fatalf("first event type = %q, want message_start", messageStart.Type)
	}
	if messageStart.Message == nil {
		t.Fatal("message_start message = nil")
	}
	if messageStart.Message.Usage.InputTokens != 11 {
		t.Fatalf("message_start input_tokens = %d, want 11", messageStart.Message.Usage.InputTokens)
	}
	if messageStart.Message.Usage.OutputTokens != 0 {
		t.Fatalf("message_start output_tokens = %d, want 0", messageStart.Message.Usage.OutputTokens)
	}
	if messageStart.Message.Usage.CacheReadInputTokens != 0 {
		t.Fatalf("message_start cache_read_input_tokens = %d, want 0", messageStart.Message.Usage.CacheReadInputTokens)
	}
	if messageStart.Message.Usage.CacheCreationInputTokens != 0 {
		t.Fatalf("message_start cache_creation_input_tokens = %d, want 0", messageStart.Message.Usage.CacheCreationInputTokens)
	}

	messageDelta := events[len(events)-2]
	if messageDelta.Type != "message_delta" {
		t.Fatalf("final delta type = %q, want message_delta", messageDelta.Type)
	}
	if messageDelta.Usage == nil {
		t.Fatalf("message_delta usage = nil")
	}
	if messageDelta.Usage.InputTokens != 11 {
		t.Fatalf("InputTokens = %d, want 11 (prompt_tokens total, not subtract cache)", messageDelta.Usage.InputTokens)
	}
	if !hasUsageField(rawEvents[len(rawEvents)-2], "input_tokens") {
		t.Fatalf("message_delta usage must include input_tokens even when zero: %s", rawEvents[len(rawEvents)-2])
	}
	if messageDelta.Usage.OutputTokens != 7 {
		t.Fatalf("OutputTokens = %d, want 7", messageDelta.Usage.OutputTokens)
	}
	if messageDelta.Usage.CacheReadInputTokens != 5 {
		t.Fatalf("CacheReadInputTokens = %d, want 5", messageDelta.Usage.CacheReadInputTokens)
	}
	if messageDelta.Usage.CacheCreationInputTokens != 6 {
		t.Fatalf("CacheCreationInputTokens = %d, want 6", messageDelta.Usage.CacheCreationInputTokens)
	}
	if !hasUsageField(rawEvents[len(rawEvents)-2], "cache_read_input_tokens") {
		t.Fatalf("message_delta usage must include cache_read_input_tokens: %s", rawEvents[len(rawEvents)-2])
	}
	if !hasUsageField(rawEvents[len(rawEvents)-2], "cache_creation_input_tokens") {
		t.Fatalf("message_delta usage must include cache_creation_input_tokens: %s", rawEvents[len(rawEvents)-2])
	}
}

func TestProxyStreamNormalizesCacheUsageFallbacks(t *testing.T) {
	var out bytes.Buffer
	body := sseBody(
		`{"id":"chunk_1","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
		`{"id":"chunk_1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":5},"cache_creation_input_tokens":6}}`,
		`[DONE]`,
	)

	if err := NewStreamHandler().ProxyStream(&out, body, "gpt-4o", 0, context.Background(), nil); err != nil {
		t.Fatalf("ProxyStream() error = %v", err)
	}

	events := parseEvents(t, out.String())
	messageDelta := events[len(events)-2]
	if messageDelta.Usage == nil {
		t.Fatal("message_delta usage = nil")
	}
	if messageDelta.Usage.OutputTokens != 7 {
		t.Fatalf("OutputTokens = %d, want 7", messageDelta.Usage.OutputTokens)
	}
	if messageDelta.Usage.CacheReadInputTokens != 5 {
		t.Fatalf("CacheReadInputTokens = %d, want 5", messageDelta.Usage.CacheReadInputTokens)
	}
	if messageDelta.Usage.CacheCreationInputTokens != 6 {
		t.Fatalf("CacheCreationInputTokens = %d, want 6", messageDelta.Usage.CacheCreationInputTokens)
	}
}

func TestProxyStreamStopsToolBlockWhenFinishReasonArrives(t *testing.T) {
	var out bytes.Buffer
	idx := 0
	body := sseBody(
		`{"id":"chunk_1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"q\""}}]},"finish_reason":null}]}`,
		`{"id":"chunk_1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"docs\"}"}}]},"finish_reason":null}]}`,
		`{"id":"chunk_1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`[DONE]`,
	)
	_ = idx

	if err := NewStreamHandler().ProxyStream(&out, body, "gpt-4o", 0, context.Background(), nil); err != nil {
		t.Fatalf("ProxyStream() error = %v", err)
	}

	events := parseEvents(t, out.String())
	var sawToolStart, sawToolStop bool
	for _, event := range events {
		if event.Type == "content_block_start" &&
			event.ContentBlock != nil &&
			event.ContentBlock.Type == "tool_use" {
			sawToolStart = true
		}
		if event.Type == "content_block_stop" {
			sawToolStop = true
		}
	}
	if !sawToolStart {
		t.Fatalf("missing tool content_block_start: %#v", events)
	}
	if !sawToolStop {
		t.Fatalf("missing tool content_block_stop: %#v", events)
	}
}

func sseBody(lines ...string) io.ReadCloser {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("data: ")
		b.WriteString(line)
		b.WriteString("\n\n")
	}
	return io.NopCloser(strings.NewReader(b.String()))
}

func parseEvents(t *testing.T, raw string) []types.StreamEvent {
	t.Helper()
	var events []types.StreamEvent
	for _, data := range parseRawEvents(t, raw) {
		var event types.StreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatalf("unmarshal event: %v; line=%s", err, data)
		}
		events = append(events, event)
	}
	return events
}

func parseRawEvents(t *testing.T, raw string) []string {
	t.Helper()
	var events []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		events = append(events, strings.TrimPrefix(line, "data: "))
	}
	return events
}

func hasUsageField(rawEvent, field string) bool {
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(rawEvent), &event); err != nil {
		panic(fmt.Sprintf("unmarshal event: %v", err))
	}
	usage, ok := event["usage"].(map[string]interface{})
	if !ok {
		return false
	}
	_, ok = usage[field]
	return ok
}
