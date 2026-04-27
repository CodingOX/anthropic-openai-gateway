package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"

	"anthropic-openai-gateway/pkg/types"
)

type requestLogStateKey struct{}

type requestLogState struct {
	requestLog requestLogContext
}

const maxPanicStackChars = 2000

func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, state := ensureRequestLogState(r)
		writer := &responseCaptureWriter{ResponseWriter: w}
		writer.Header().Set("X-Request-Id", state.requestLog.RequestID)

		defer func() {
			if recovered := recover(); recovered != nil {
				requestLog := newRequestLogContext(r)
				logRequestEvent("error", requestLog, "panic_recovered",
					fmt.Sprintf("panic=%q", fmt.Sprint(recovered)),
					fmt.Sprintf("stack=%q", truncateString(string(debug.Stack()), maxPanicStackChars)))

				if writer.wroteHeader {
					return
				}

				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(writer).Encode(types.ErrorResponse{
					Type: "error",
					Error: types.APIError{
						Type:    "api_error",
						Message: "internal server error",
					},
				})
			}
		}()

		next.ServeHTTP(writer, r)
	})
}

func ensureRequestLogState(r *http.Request) (*http.Request, *requestLogState) {
	if state := getRequestLogState(r.Context()); state != nil {
		if state.requestLog.RequestID == "" {
			state.requestLog.RequestID = generateRequestID()
		}
		if state.requestLog.Method == "" {
			state.requestLog.Method = r.Method
		}
		if state.requestLog.Path == "" {
			state.requestLog.Path = r.URL.Path
		}
		return r, state
	}

	state := &requestLogState{
		requestLog: requestLogContext{
			RequestID: generateRequestID(),
			Method:    r.Method,
			Path:      r.URL.Path,
		},
	}
	ctx := context.WithValue(r.Context(), requestLogStateKey{}, state)
	return r.WithContext(ctx), state
}

func getRequestLogState(ctx context.Context) *requestLogState {
	state, _ := ctx.Value(requestLogStateKey{}).(*requestLogState)
	return state
}

func updateRequestLogState(ctx context.Context, requestLog requestLogContext) {
	if state := getRequestLogState(ctx); state != nil {
		state.requestLog = requestLog
	}
}

type responseCaptureWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *responseCaptureWriter) WriteHeader(statusCode int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseCaptureWriter) Write(body []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(body)
}

func (w *responseCaptureWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
