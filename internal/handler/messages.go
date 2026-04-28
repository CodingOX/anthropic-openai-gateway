// Package handler 提供 HTTP 请求处理器。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"anthropic-openai-gateway/internal/client"
	"anthropic-openai-gateway/internal/config"
	"anthropic-openai-gateway/internal/transformer"
	"anthropic-openai-gateway/pkg/types"
)

// MessagesHandler 处理 /v1/messages 端点请求。
type MessagesHandler struct {
	config              *config.Config
	openaiClient        *client.OpenAIClient
	anthropicClient     *client.AnthropicClient
	requestTransformer  *transformer.RequestTransformer
	responseTransformer *transformer.ResponseTransformer
	streamHandler       *transformer.StreamHandler
}

type requestLogContext struct {
	RequestID     string
	Method        string
	Path          string
	Model         string
	Streaming     bool
	MessageCount  int
	ToolCount     int
	HasSystem     bool
	RoleSummary   string
	PromptPreview string
}

var requestIDSeq atomic.Uint64

// HandleCountTokens 提供 Anthropic count_tokens 兼容接口。
// 这里采用轻量估算，满足 Claude Code 对接口形状的依赖；真实计费仍以上游 usage 为准。
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

	inputTokens := estimateInputTokens(anthropicReq)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]int{
		"input_tokens": inputTokens,
	})
}

// NewMessagesHandler 创建消息处理器。
func NewMessagesHandler(cfg *config.Config) *MessagesHandler {
	return &MessagesHandler{
		config:              cfg,
		openaiClient:        client.NewOpenAIClient(cfg),
		anthropicClient:     client.NewAnthropicClient(cfg),
		requestTransformer:  transformer.NewRequestTransformer(),
		responseTransformer: transformer.NewResponseTransformer(),
		streamHandler:       transformer.NewStreamHandler(),
	}
}

// HandleMessages 处理 /v1/messages 请求入口。
func (h *MessagesHandler) HandleMessages(w http.ResponseWriter, r *http.Request) {
	requestLog := newRequestLogContext(r)
	w.Header().Set("X-Request-Id", requestLog.RequestID)

	if r.Method != http.MethodPost {
		h.logError(requestLog, "method_not_allowed",
			fmt.Sprintf("status=%d", http.StatusMethodNotAllowed),
			fmt.Sprintf("error=%q", "method not allowed, use POST"))
		h.sendError(w, http.StatusMethodNotAllowed, "method not allowed, use POST")
		return
	}

	// 预读原始请求体，用于透传模式
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		h.logError(requestLog, "read_request_body",
			fmt.Sprintf("status=%d", http.StatusBadRequest),
			fmt.Sprintf("error=%q", fmt.Sprintf("failed to read request body: %v", err)))
		h.sendError(w, http.StatusBadRequest, fmt.Sprintf("failed to read request body: %v", err))
		return
	}

	// 解析 Anthropic 请求体
	var anthropicReq types.MessageRequest
	if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
		h.logError(requestLog, "decode_request",
			fmt.Sprintf("status=%d", http.StatusBadRequest),
			fmt.Sprintf("error=%q", fmt.Sprintf("invalid request body: %v", err)))
		h.sendError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}
	requestLog = h.enrichRequestLog(requestLog, &anthropicReq)
	updateRequestLogState(r.Context(), requestLog)

	// 参数校验
	if err := anthropicReq.Validate(); err != nil {
		h.logError(requestLog, "validate_request",
			fmt.Sprintf("status=%d", http.StatusBadRequest),
			fmt.Sprintf("error=%q", err.Error()))
		h.sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.logInfo(requestLog, "request_received")

	// 不在转换列表中的模型，透传到 Anthropic API
	if !h.needsTransformation(anthropicReq.Model) {
		h.handlePassThrough(w, r, rawBody, &anthropicReq, requestLog)
		return
	}

	// 路由到流式或非流式处理
	isStreaming := anthropicReq.Stream != nil && *anthropicReq.Stream
	if isStreaming {
		h.handleStreaming(w, r, &anthropicReq, requestLog)
	} else {
		h.handleNonStreaming(w, r, &anthropicReq, requestLog)
	}
}

// needsTransformation 检查 model 是否在转换列表中。
func (h *MessagesHandler) needsTransformation(modelID string) bool {
	for _, m := range h.config.ModelsNeedTransformation {
		if m == modelID {
			return true
		}
	}
	return false
}

// handleStreaming 处理流式请求。
func (h *MessagesHandler) handleStreaming(w http.ResponseWriter, r *http.Request, anthropicReq *types.MessageRequest, requestLog requestLogContext) {
	ctx := r.Context()
	startedAt := time.Now()

	// Anthropic → OpenAI 请求转换
	openaiReq, err := h.requestTransformer.TransformRequest(anthropicReq)
	if err != nil {
		h.logError(requestLog, "transform_request",
			fmt.Sprintf("status=%d", http.StatusInternalServerError),
			fmt.Sprintf("error=%q", err.Error()))
		h.sendError(w, http.StatusInternalServerError,
			fmt.Sprintf("request transform failed: %v", err))
		return
	}

	// 获取 OpenAI 流响应体
	streamBody, err := h.openaiClient.GetStreamingBody(ctx, openaiReq)
	if err != nil {
		h.logUpstreamError(requestLog, err)
		h.sendError(w, http.StatusBadGateway,
			fmt.Sprintf("upstream request failed: %v", err))
		return
	}
	defer streamBody.Close()

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.logError(requestLog, "streaming_not_supported",
			fmt.Sprintf("error=%q", "streaming not supported by response writer"))
		return
	}
	flusher.Flush()

	// 代理并转换流
	h.logInfo(requestLog, "stream_started")
	if err := h.streamHandler.ProxyStream(w, streamBody, anthropicReq.Model, ctx, flusher); err != nil {
		h.logError(requestLog, "stream_proxy",
			fmt.Sprintf("duration_ms=%d", time.Since(startedAt).Milliseconds()),
			fmt.Sprintf("error=%q", err.Error()))
		return
	}
	h.logInfo(requestLog, "stream_completed", fmt.Sprintf("duration_ms=%d", time.Since(startedAt).Milliseconds()))
}

// handleNonStreaming 处理非流式请求。
func (h *MessagesHandler) handleNonStreaming(w http.ResponseWriter, r *http.Request, anthropicReq *types.MessageRequest, requestLog requestLogContext) {
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(),
		time.Duration(h.config.OpenAI.TimeoutMS)*time.Millisecond)
	defer cancel()

	// Anthropic → OpenAI 请求转换
	openaiReq, err := h.requestTransformer.TransformRequest(anthropicReq)
	if err != nil {
		h.logError(requestLog, "transform_request",
			fmt.Sprintf("status=%d", http.StatusInternalServerError),
			fmt.Sprintf("error=%q", err.Error()))
		h.sendError(w, http.StatusInternalServerError,
			fmt.Sprintf("request transform failed: %v", err))
		return
	}

	h.logInfo(requestLog, "upstream_request_started", fmt.Sprintf("upstream_model=%s", openaiReq.Model))

	// 调用 OpenAI API
	openaiResp, err := h.openaiClient.ChatCompletion(ctx, openaiReq)
	if err != nil {
		h.logUpstreamError(requestLog, err)
		h.sendError(w, http.StatusBadGateway,
			fmt.Sprintf("upstream request failed: %v", err))
		return
	}

	// OpenAI → Anthropic 响应转换
	anthropicResp, err := h.responseTransformer.TransformResponse(openaiResp, anthropicReq.Model)
	if err != nil {
		h.logError(requestLog, "transform_response",
			fmt.Sprintf("status=%d", http.StatusInternalServerError),
			fmt.Sprintf("error=%q", err.Error()))
		h.sendError(w, http.StatusInternalServerError,
			fmt.Sprintf("response transform failed: %v", err))
		return
	}

	h.logInfo(requestLog, "request_completed",
		fmt.Sprintf("duration_ms=%d", time.Since(startedAt).Milliseconds()),
		fmt.Sprintf("response_id=%s", anthropicResp.ID),
		fmt.Sprintf("input_tokens=%d", anthropicResp.Usage.InputTokens),
		fmt.Sprintf("output_tokens=%d", anthropicResp.Usage.OutputTokens))

	// 返回 Anthropic 格式响应
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(anthropicResp)
}

// handlePassThrough 将请求原样转发到 Anthropic API，不做格式转换。
func (h *MessagesHandler) handlePassThrough(w http.ResponseWriter, r *http.Request, rawBody []byte, anthropicReq *types.MessageRequest, requestLog requestLogContext) {
	ctx := r.Context()
	startedAt := time.Now()
	isStreaming := anthropicReq.Stream != nil && *anthropicReq.Stream

	h.logInfo(requestLog, "pass_through_started",
		fmt.Sprintf("upstream_model=%s", anthropicReq.Model))

	// 将原始请求体转发到 Anthropic API
	resp, err := h.anthropicClient.ForwardMessage(ctx, bytes.NewReader(rawBody), r.Header)
	if err != nil {
		h.logUpstreamError(requestLog, err)
		h.sendError(w, http.StatusBadGateway,
			fmt.Sprintf("upstream request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	// 复制上游响应头
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	if isStreaming {
		// 流式透传：逐块写入并 flush，确保 SSE 事件不被缓冲
		flusher, ok := w.(http.Flusher)
		if !ok {
			h.logError(requestLog, "pass_through_streaming_not_supported",
				fmt.Sprintf("error=%q", "streaming not supported by response writer"))
			return
		}
		buf := make([]byte, 4096)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					h.logError(requestLog, "pass_through_stream_write",
						fmt.Sprintf("error=%q", writeErr.Error()))
					return
				}
				flusher.Flush()
			}
			if readErr != nil {
				if readErr != io.EOF {
					h.logError(requestLog, "pass_through_stream_read",
						fmt.Sprintf("error=%q", readErr.Error()))
				}
				break
			}
		}
		h.logInfo(requestLog, "pass_through_stream_completed",
			fmt.Sprintf("duration_ms=%d", time.Since(startedAt).Milliseconds()))
	} else {
		// 非流式透传：直接复制响应体
		written, copyErr := io.Copy(w, resp.Body)
		if copyErr != nil {
			h.logError(requestLog, "pass_through_copy_response",
				fmt.Sprintf("error=%q", copyErr.Error()))
			return
		}
		h.logInfo(requestLog, "pass_through_completed",
			fmt.Sprintf("duration_ms=%d", time.Since(startedAt).Milliseconds()),
			fmt.Sprintf("response_bytes=%d", written))
	}
}

// sendError 发送 Anthropic 格式的错误响应。
func (h *MessagesHandler) sendError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(types.ErrorResponse{
		Type: "error",
		Error: types.APIError{
			Type:    "api_error",
			Message: message,
		},
	})
}

func estimateInputTokens(req types.MessageRequest) int {
	totalChars := 0
	switch system := req.System.(type) {
	case string:
		totalChars += utf8.RuneCountInString(system)
	}
	for _, msg := range req.Messages {
		totalChars += utf8.RuneCountInString(msg.Role)
		totalChars += estimateContentChars(msg.Content)
	}
	if totalChars == 0 {
		return 1
	}
	tokens := (totalChars + 3) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func newRequestLogContext(r *http.Request) requestLogContext {
	if state := getRequestLogState(r.Context()); state != nil {
		requestLog := state.requestLog
		if requestLog.RequestID == "" {
			requestLog.RequestID = generateRequestID()
		}
		if requestLog.Method == "" {
			requestLog.Method = r.Method
		}
		if requestLog.Path == "" {
			requestLog.Path = r.URL.Path
		}
		return requestLog
	}

	return requestLogContext{
		RequestID: generateRequestID(),
		Method:    r.Method,
		Path:      r.URL.Path,
	}
}

func generateRequestID() string {
	return fmt.Sprintf("req-%d-%06d", time.Now().UnixMilli(), requestIDSeq.Add(1))
}

func (h *MessagesHandler) enrichRequestLog(requestLog requestLogContext, req *types.MessageRequest) requestLogContext {
	requestLog = requestLog.withAnthropicRequest(req)
	if h.config != nil && h.config.LogPromptPreviewOnError {
		requestLog.PromptPreview = buildPromptPreview(req, normalizePromptPreviewMaxChars(h.config.PromptPreviewMaxChars))
	}
	return requestLog
}

func (ctx requestLogContext) withAnthropicRequest(req *types.MessageRequest) requestLogContext {
	ctx.Model = req.Model
	ctx.Streaming = req.Stream != nil && *req.Stream
	ctx.MessageCount = len(req.Messages)
	ctx.ToolCount = len(req.Tools)
	ctx.HasSystem = hasSystemPrompt(req.System)
	ctx.RoleSummary = summarizeRoles(req.Messages)
	return ctx
}

func (h *MessagesHandler) logInfo(requestLog requestLogContext, stage string, extraFields ...string) {
	h.logWithLevel("info", requestLog, stage, extraFields...)
}

func (h *MessagesHandler) logError(requestLog requestLogContext, stage string, extraFields ...string) {
	h.logWithLevel("error", requestLog, stage, extraFields...)
}

func (h *MessagesHandler) logUpstreamError(requestLog requestLogContext, err error) {
	var upstreamErr *client.UpstreamError
	if errors.As(err, &upstreamErr) {
		extraFields := []string{
			fmt.Sprintf("duration_ms=%d", upstreamErr.Duration.Milliseconds()),
			fmt.Sprintf("upstream_url=%s", upstreamErr.URL),
		}
		if upstreamErr.StatusCode > 0 {
			extraFields = append(extraFields, fmt.Sprintf("upstream_status=%d", upstreamErr.StatusCode))
		}
		if upstreamErr.ResponsePreview != "" {
			extraFields = append(extraFields, fmt.Sprintf("response_preview=%q", upstreamErr.ResponsePreview))
		}
		if upstreamErr.Err != nil {
			extraFields = append(extraFields, fmt.Sprintf("cause=%q", upstreamErr.Err.Error()))
		}
		h.logError(requestLog, "upstream_request_failed", extraFields...)
		return
	}

	h.logError(requestLog, "upstream_request_failed", fmt.Sprintf("error=%q", err.Error()))
}

func (h *MessagesHandler) logWithLevel(level string, requestLog requestLogContext, stage string, extraFields ...string) {
	logRequestEvent(level, requestLog, stage, extraFields...)
}

func logRequestEvent(level string, requestLog requestLogContext, stage string, extraFields ...string) {
	fields := []string{
		fmt.Sprintf("level=%s", level),
		fmt.Sprintf("stage=%s", stage),
		fmt.Sprintf("request_id=%s", requestLog.RequestID),
		fmt.Sprintf("method=%s", requestLog.Method),
		fmt.Sprintf("path=%s", requestLog.Path),
	}
	if requestLog.Model != "" {
		fields = append(fields, fmt.Sprintf("model=%s", requestLog.Model))
	}
	if requestLog.MessageCount > 0 {
		fields = append(fields,
			fmt.Sprintf("stream=%t", requestLog.Streaming),
			fmt.Sprintf("messages=%d", requestLog.MessageCount),
			fmt.Sprintf("tools=%d", requestLog.ToolCount),
			fmt.Sprintf("has_system=%t", requestLog.HasSystem),
		)
	}
	if requestLog.RoleSummary != "" {
		fields = append(fields, fmt.Sprintf("roles=%s", requestLog.RoleSummary))
	}
	if level == "error" && requestLog.PromptPreview != "" {
		fields = append(fields, fmt.Sprintf("prompt_preview=%q", requestLog.PromptPreview))
	}
	fields = append(fields, extraFields...)
	log.Printf(strings.Join(fields, " "))
}

func hasSystemPrompt(system interface{}) bool {
	switch v := system.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case []interface{}:
		return len(v) > 0
	default:
		return false
	}
}

func summarizeRoles(messages []types.Message) string {
	if len(messages) == 0 {
		return ""
	}

	roleCounts := make(map[string]int)
	for _, msg := range messages {
		roleCounts[msg.Role]++
	}

	roles := make([]string, 0, len(roleCounts))
	for role := range roleCounts {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	parts := make([]string, 0, len(roles))
	for _, role := range roles {
		parts = append(parts, fmt.Sprintf("%s:%d", role, roleCounts[role]))
	}
	return strings.Join(parts, ",")
}

func normalizePromptPreviewMaxChars(limit int) int {
	if limit <= 0 {
		return 240
	}
	return limit
}

func buildPromptPreview(req *types.MessageRequest, limit int) string {
	parts := make([]string, 0, len(req.Messages)+1)
	if systemText := extractTextPreview(req.System); systemText != "" {
		parts = append(parts, "system:"+systemText)
	}
	for _, msg := range req.Messages {
		text := extractTextPreview(msg.Content)
		if text == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%s", msg.Role, text))
	}
	return truncateString(strings.Join(parts, " | "), limit)
}

func extractTextPreview(content interface{}) string {
	switch value := content.(type) {
	case string:
		return strings.TrimSpace(value)
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if text := extractTextPreview(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	case map[string]interface{}:
		parts := make([]string, 0, 3)
		for _, key := range []string{"text", "thinking", "content"} {
			if nested, ok := value[key]; ok {
				if text := extractTextPreview(nested); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func truncateString(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || limit <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit]) + "..."
}

func estimateContentChars(content interface{}) int {
	switch v := content.(type) {
	case string:
		return utf8.RuneCountInString(v)
	case []interface{}:
		total := 0
		for _, item := range v {
			block, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			for _, key := range []string{"text", "thinking"} {
				if text, ok := block[key].(string); ok {
					total += utf8.RuneCountInString(text)
				}
			}
			if nested, ok := block["content"]; ok {
				total += estimateContentChars(nested)
			}
		}
		return total
	default:
		return 0
	}
}
