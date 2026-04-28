// Package transformer - stream.go 处理 SSE 流式响应的实时转换。
package transformer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"anthropic-openai-gateway/pkg/types"
)

// StreamHandler 处理 OpenAI SSE 流 → Anthropic SSE 流的实时转换。
type StreamHandler struct{}

// NewStreamHandler 创建流处理器。
func NewStreamHandler() *StreamHandler {
	return &StreamHandler{}
}

// ProxyStream 读取 OpenAI SSE 流，转换后写入 writer。
func (h *StreamHandler) ProxyStream(w io.Writer, body io.ReadCloser, model string, ctx context.Context, flusher ...http.Flusher) error {
	log.Printf("[STREAM] 🎬 开始流式传输: model=%s", model)
	startTime := time.Now()
	eventCount := 0

	scanner := bufio.NewScanner(body)
	// 增大 buffer 以处理大行
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// 状态追踪
	state := newStreamState(model)
	toolUseState := make(map[int]*toolUseBuilder) // index → builder
	messageID := ""
	var accumulatedUsage *types.Usage
	stopReason := "end_turn"
	finishSeen := false

	h.writeEvent(w, types.StreamEvent{
		Type: "message_start",
		Message: &types.MessageResponse{
			ID:      state.messageID,
			Type:    "message",
			Role:    "assistant",
			Content: []types.ContentBlock{},
			Model:   model,
		},
	}, flusher...)
	eventCount++

	for scanner.Scan() {
		// 检查上下文取消
		select {
		case <-ctx.Done():
			log.Printf("[STREAM] ⚠️  流式传输被取消")
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := line[6:] // 去掉 "data: "
		if data == "[DONE]" {
			log.Printf("[STREAM] 🏁 收到 [DONE] 信号")
			if !finishSeen {
				for _, event := range state.closeOpenBlock() {
					h.writeEvent(w, event, flusher...)
					eventCount++
				}
			}
			h.writeDone(w, messageID, model, &stopReason, accumulatedUsage, toolUseState, flusher...)
			eventCount += 2 // message_delta + message_stop
			continue
		}

		var chunk types.ChatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			log.Printf("[STREAM] ❌ 解析SSE数据块失败: %v", err)
			continue
		}

		if messageID == "" {
			messageID = chunk.ID
		}

		// 处理每个 choice
		for _, choice := range chunk.Choices {
			delta := choice.Delta

			// 处理文本增量
			if delta.Content != "" {
				events := state.textDelta(delta.Content)
				for _, event := range events {
					h.writeEvent(w, event, flusher...)
					eventCount++
				}
			}

			if delta.ReasoningContent != nil && *delta.ReasoningContent != "" {
				events := state.thinkingDelta(*delta.ReasoningContent)
				for _, event := range events {
					h.writeEvent(w, event, flusher...)
				}
			}

			// 处理工具调用增量
			if len(delta.ToolCalls) > 0 {
				for _, tc := range delta.ToolCalls {
					events := state.toolDelta(tc, toolUseState)
					for _, ev := range events {
						h.writeEvent(w, ev, flusher...)
					}
				}
			}

			// 处理 finish_reason → message_stop
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finishSeen = true
				for _, event := range state.closeOpenBlock() {
					h.writeEvent(w, event, flusher...)
				}
				for _, event := range closeToolBlocks(toolUseState) {
					h.writeEvent(w, event, flusher...)
				}
				stopReason = h.convertStreamFinishReason(*choice.FinishReason)
			}
		}

		// 收集 usage（通常只在最后一个 chunk 出现）
		if chunk.Usage != nil {
			accumulatedUsage = &types.Usage{
				InputTokens:              chunk.Usage.PromptTokens,
				OutputTokens:             chunk.Usage.CompletionTokens,
				CacheReadInputTokens:     chunk.Usage.PromptCacheHitTokens,
				CacheCreationInputTokens: chunk.Usage.PromptCacheMissTokens,
			}
		}
	}

	duration := time.Since(startTime)
	log.Printf("[STREAM] ✅ 流式传输完成: 共%d个事件, 耗时%s", eventCount, duration)
	return scanner.Err()
}

// toolUseBuilder 构建 tool_use 增量。
type toolUseBuilder struct {
	id    string
	name  string
	args  strings.Builder
	index int
	fired bool // 是否已发送 content_block_start
}

// writeEvent 写入 SSE 事件。
func (h *StreamHandler) writeEvent(w io.Writer, event types.StreamEvent, flusher ...http.Flusher) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal stream event: %v", err)
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data))
	if len(flusher) > 0 && flusher[0] != nil {
		flusher[0].Flush()
	}
}

// writeDone 发送流结束事件。
func (h *StreamHandler) writeDone(w io.Writer, messageID, model string, stopReason *string, usage *types.Usage, state map[int]*toolUseBuilder, flusher ...http.Flusher) {
	// 发送所有 content_block_stop
	for _, event := range closeToolBlocks(state) {
		h.writeEvent(w, event, flusher...)
	}

	// 发送 message_delta（含 stop_reason 和 usage）
	delta := &types.DeltaContent{
		StopReason: stopReason,
	}
	messageDelta := types.StreamEvent{
		Type:  "message_delta",
		Delta: delta,
	}
	if usage != nil {
		messageDelta.Usage = usage
	}
	h.writeEvent(w, messageDelta, flusher...)

	// 发送 message_stop
	stopEvent := types.StreamEvent{
		Type: "message_stop",
	}
	h.writeEvent(w, stopEvent, flusher...)
}

type streamState struct {
	messageID string
	nextIndex int
	openType  string
	openIndex int
}

func newStreamState(_ string) *streamState {
	return &streamState{
		messageID: fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		openIndex: -1,
	}
}

func (s *streamState) textDelta(text string) []types.StreamEvent {
	events := s.ensureBlock("text")
	events = append(events, types.StreamEvent{
		Type:  "content_block_delta",
		Index: intPtr(s.openIndex),
		Delta: &types.DeltaContent{
			Type: "text_delta",
			Text: text,
		},
	})
	return events
}

func (s *streamState) thinkingDelta(thinking string) []types.StreamEvent {
	events := s.ensureBlock("thinking")
	events = append(events, types.StreamEvent{
		Type:  "content_block_delta",
		Index: intPtr(s.openIndex),
		Delta: &types.DeltaContent{
			Type:     "thinking_delta",
			Thinking: thinking,
		},
	})
	return events
}

func (s *streamState) toolDelta(tc types.ToolCall, builders map[int]*toolUseBuilder) []types.StreamEvent {
	events := s.closeOpenBlock()
	idx := 0
	if tc.Index != nil {
		idx = *tc.Index
	}

	builder, exists := builders[idx]
	if !exists {
		builder = &toolUseBuilder{index: s.nextIndex}
		builders[idx] = builder
		s.nextIndex++
	}
	if tc.ID != "" && builder.id == "" {
		builder.id = tc.ID
	}
	if tc.Function.Name != "" && builder.name == "" {
		builder.name = tc.Function.Name
	}
	if tc.Function.Arguments != "" {
		builder.args.WriteString(tc.Function.Arguments)
	}
	if !builder.fired && builder.id != "" && builder.name != "" {
		builder.fired = true
		events = append(events, types.StreamEvent{
			Type:  "content_block_start",
			Index: intPtr(builder.index),
			ContentBlock: &types.ContentBlock{
				Type:  "tool_use",
				ID:    builder.id,
				Name:  builder.name,
				Input: json.RawMessage("{}"),
			},
		})
	}
	if tc.Function.Arguments != "" {
		events = append(events, types.StreamEvent{
			Type:  "content_block_delta",
			Index: intPtr(builder.index),
			Delta: &types.DeltaContent{
				Type:        "input_json_delta",
				PartialJSON: tc.Function.Arguments,
			},
		})
	}
	return events
}

func (s *streamState) ensureBlock(blockType string) []types.StreamEvent {
	var events []types.StreamEvent
	if s.openType == blockType {
		return events
	}
	events = append(events, s.closeOpenBlock()...)
	s.openType = blockType
	s.openIndex = s.nextIndex
	s.nextIndex++
	events = append(events, types.StreamEvent{
		Type:  "content_block_start",
		Index: intPtr(s.openIndex),
		ContentBlock: &types.ContentBlock{
			Type: blockType,
		},
	})
	return events
}

func (s *streamState) closeOpenBlock() []types.StreamEvent {
	if s.openType == "" || s.openIndex < 0 {
		return nil
	}
	event := types.StreamEvent{
		Type:  "content_block_stop",
		Index: intPtr(s.openIndex),
	}
	s.openType = ""
	s.openIndex = -1
	return []types.StreamEvent{event}
}

func closeToolBlocks(state map[int]*toolUseBuilder) []types.StreamEvent {
	var events []types.StreamEvent
	for _, builder := range state {
		if !builder.fired {
			continue
		}
		events = append(events, types.StreamEvent{
			Type:  "content_block_stop",
			Index: intPtr(builder.index),
		})
		builder.fired = false
	}
	return events
}

// convertStreamFinishReason 转换流式中的完成原因。
func (h *StreamHandler) convertStreamFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "end_turn"
	default:
		return "end_turn"
	}
}

// intPtr 返回 int 指针。
func intPtr(i int) *int {
	return &i
}
