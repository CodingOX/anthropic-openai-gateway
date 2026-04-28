package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"anthropic-openai-gateway/internal/config"
	"anthropic-openai-gateway/pkg/types"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestChatCompletionRetriesOnceOnEOF(t *testing.T) {
	attempts := 0
	client := &OpenAIClient{
		baseURL: "https://example.com/v1",
		apiKey:  "test-key",
		httpClient: &http.Client{
			Timeout: time.Second,
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				if attempts == 1 {
					return nil, io.EOF
				}

				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("io.ReadAll(req.Body) error = %v", err)
				}
				if !strings.Contains(string(body), `"max_tokens":50`) {
					t.Fatalf("request body = %s, want containing max_tokens", body)
				}

				payload := types.ChatCompletionResponse{
					ID:      "chatcmpl_123",
					Object:  "chat.completion",
					Created: 1,
					Model:   "deepseek-v4-pro",
					Choices: []types.ChatChoice{{
						Index: 0,
						Message: types.ChatMessage{
							Role:    "assistant",
							Content: "hello",
						},
						FinishReason: "stop",
					}},
					Usage: &types.ChatUsage{PromptTokens: 10, CompletionTokens: 5},
				}
				respBody, err := json.Marshal(payload)
				if err != nil {
					t.Fatalf("json.Marshal() error = %v", err)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(string(respBody))),
				}, nil
			}),
		},
	}

	resp, err := client.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model:     "deepseek-v4-pro",
		Messages:  []types.ChatMessage{{Role: "user", Content: "hi"}},
		MaxTokens: intPtr(50),
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if resp.ID != "chatcmpl_123" {
		t.Fatalf("resp.ID = %q, want chatcmpl_123", resp.ID)
	}
}

func TestNewOpenAIClientDoesNotSetStreamClientTimeout(t *testing.T) {
	client := NewOpenAIClient(&config.Config{
		BaseURL:               "https://example.com/v1",
		APIKey:                "test-key",
		NonStreamingTimeoutMS: 120000,
	})

	if client.httpClient.Timeout != 120*time.Second {
		t.Fatalf("httpClient.Timeout = %s, want 120s", client.httpClient.Timeout)
	}
	if client.streamHTTPClient == nil {
		t.Fatal("streamHTTPClient = nil, want non-nil")
	}
	if client.streamHTTPClient.Timeout != 0 {
		t.Fatalf("streamHTTPClient.Timeout = %s, want 0", client.streamHTTPClient.Timeout)
	}
}

func TestGetStreamingBodyDoesNotRetryEOF(t *testing.T) {
	attempts := 0
	client := &OpenAIClient{
		baseURL: "https://example.com/v1",
		apiKey:  "test-key",
		httpClient: &http.Client{
			Timeout: time.Second,
		},
		streamHTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				return nil, io.EOF
			}),
		},
	}

	_, err := client.GetStreamingBody(context.Background(), &types.ChatCompletionRequest{
		Model:    "deepseek-v4-pro",
		Messages: []types.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if !errors.Is(err, io.EOF) {
		t.Fatalf("GetStreamingBody() error = %v, want EOF", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func intPtr(value int) *int {
	return &value
}
