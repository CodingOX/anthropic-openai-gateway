// Package handler 提供 HTTP 请求处理器。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	requestTransformer  *transformer.RequestTransformer
	responseTransformer *transformer.ResponseTransformer
	streamHandler       *transformer.StreamHandler
}

// HandleCountTokens 提供 Anthropic count_tokens 兼容接口。
// 这里采用轻量估算，满足 Claude Code 对接口形状的依赖；真实计费仍以上游 usage 为准。
func (h *MessagesHandler) HandleCountTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, http.StatusMethodNotAllowed, "method not allowed, use POST")
		return
	}

	var anthropicReq types.MessageRequest
	if err := json.NewDecoder(r.Body).Decode(&anthropicReq); err != nil {
		h.sendError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

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
		requestTransformer:  transformer.NewRequestTransformer(),
		responseTransformer: transformer.NewResponseTransformer(),
		streamHandler:       transformer.NewStreamHandler(),
	}
}

// HandleMessages 处理 /v1/messages 请求入口。
func (h *MessagesHandler) HandleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, http.StatusMethodNotAllowed, "method not allowed, use POST")
		return
	}

	// 解析 Anthropic 请求体
	var anthropicReq types.MessageRequest
	if err := json.NewDecoder(r.Body).Decode(&anthropicReq); err != nil {
		h.sendError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	// 参数校验
	if err := anthropicReq.Validate(); err != nil {
		h.sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	log.Printf("[REQ] model=%s stream=%v messages=%d tools=%d",
		anthropicReq.Model,
		anthropicReq.Stream != nil && *anthropicReq.Stream,
		len(anthropicReq.Messages),
		len(anthropicReq.Tools),
	)

	// 判断是否需要转换
	if !h.needsTransformation(anthropicReq.Model) {
		log.Printf("[PASS] model=%s not in transform list, pass-through", anthropicReq.Model)
		h.sendError(w, http.StatusNotImplemented,
			"pass-through mode not implemented for non-transform models")
		return
	}

	// 路由到流式或非流式处理
	isStreaming := anthropicReq.Stream != nil && *anthropicReq.Stream
	if isStreaming {
		h.handleStreaming(w, r, &anthropicReq)
	} else {
		h.handleNonStreaming(w, r, &anthropicReq)
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
func (h *MessagesHandler) handleStreaming(w http.ResponseWriter, r *http.Request, anthropicReq *types.MessageRequest) {
	ctx := r.Context()

	// Anthropic → OpenAI 请求转换
	openaiReq, err := h.requestTransformer.TransformRequest(anthropicReq)
	if err != nil {
		log.Printf("[ERR] request transform: %v", err)
		h.sendError(w, http.StatusInternalServerError,
			fmt.Sprintf("request transform failed: %v", err))
		return
	}

	// 获取 OpenAI 流响应体
	streamBody, err := h.openaiClient.GetStreamingBody(ctx, openaiReq)
	if err != nil {
		log.Printf("[ERR] upstream stream: %v", err)
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
		log.Printf("[ERR] streaming not supported")
		return
	}
	flusher.Flush()

	// 代理并转换流
	log.Printf("[STREAM] starting proxy for model=%s", anthropicReq.Model)
	if err := h.streamHandler.ProxyStream(w, streamBody, anthropicReq.Model, ctx, flusher); err != nil {
		log.Printf("[ERR] stream proxy: %v", err)
	}
	log.Printf("[STREAM] ended for model=%s", anthropicReq.Model)
}

// handleNonStreaming 处理非流式请求。
func (h *MessagesHandler) handleNonStreaming(w http.ResponseWriter, r *http.Request, anthropicReq *types.MessageRequest) {
	ctx, cancel := context.WithTimeout(r.Context(),
		time.Duration(h.config.OpenAI.TimeoutMS)*time.Millisecond)
	defer cancel()

	// Anthropic → OpenAI 请求转换
	openaiReq, err := h.requestTransformer.TransformRequest(anthropicReq)
	if err != nil {
		log.Printf("[ERR] request transform: %v", err)
		h.sendError(w, http.StatusInternalServerError,
			fmt.Sprintf("request transform failed: %v", err))
		return
	}

	log.Printf("[API] calling OpenAI model=%s", openaiReq.Model)

	// 调用 OpenAI API
	openaiResp, err := h.openaiClient.ChatCompletion(ctx, openaiReq)
	if err != nil {
		log.Printf("[ERR] upstream API: %v", err)
		h.sendError(w, http.StatusBadGateway,
			fmt.Sprintf("upstream request failed: %v", err))
		return
	}

	// OpenAI → Anthropic 响应转换
	anthropicResp, err := h.responseTransformer.TransformResponse(openaiResp, anthropicReq.Model)
	if err != nil {
		log.Printf("[ERR] response transform: %v", err)
		h.sendError(w, http.StatusInternalServerError,
			fmt.Sprintf("response transform failed: %v", err))
		return
	}

	log.Printf("[API] success: id=%s, tokens(in=%d out=%d)",
		anthropicResp.ID, anthropicResp.Usage.InputTokens, anthropicResp.Usage.OutputTokens)

	// 返回 Anthropic 格式响应
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(anthropicResp)
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
