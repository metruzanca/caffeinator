package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChatSendsMessagesWithoutSystemPrompt(t *testing.T) {
	var got chatRequest
	var gotPath, gotAuth, gotContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("cannot decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "hello there"}},
			},
			"usage": map[string]int{
				"prompt_tokens":     12,
				"completion_tokens": 3,
				"total_tokens":      15,
			},
		}); err != nil {
			t.Errorf("cannot encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "test-key",
		Model:      "deepseek/deepseek-v4.1-flash-1017",
		BaseURL:    server.URL,
		MaxTokens:  256,
		RetryDelay: time.Millisecond,
	}

	content, usage, err := client.Chat(context.Background(), []Message{
		{Role: "user", Content: "hello"},
	})
	require.NoError(t, err)
	require.Equal(t, "hello there", content)
	require.Equal(t, 12, usage.PromptTokens)
	require.Equal(t, 3, usage.CompletionTokens)
	require.Equal(t, "/chat/completions", gotPath)
	require.Equal(t, "Bearer test-key", gotAuth)
	require.Equal(t, "application/json", gotContentType)
	require.Equal(t, "deepseek/deepseek-v4.1-flash-1017", got.Model)
	require.Equal(t, 256, got.MaxTokens)
	require.Len(t, got.Messages, 1)
	require.Equal(t, "user", got.Messages[0].Role)
	for _, message := range got.Messages {
		require.NotEqual(t, "system", message.Role)
	}
}

func TestChatReturnsAPIErrorWithoutRetry(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid model","code":400}}`))
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "test-key",
		Model:      "bad-model",
		BaseURL:    server.URL,
		RetryDelay: time.Millisecond,
	}

	_, _, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid model")
	require.Equal(t, int32(1), calls.Load())
}

func TestChatRetriesServerError(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"temporary"}}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"recovered"}}]}`))
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "test-key",
		Model:      "model",
		BaseURL:    server.URL,
		RetryDelay: time.Millisecond,
	}

	content, _, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	require.NoError(t, err)
	require.Equal(t, "recovered", content)
	require.Equal(t, int32(2), calls.Load())
}

func TestChatRejectsEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "test-key",
		Model:      "model",
		BaseURL:    server.URL,
		RetryDelay: time.Millisecond,
	}

	_, _, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "no choices"))
}

func TestChatRequiresAPIKey(t *testing.T) {
	client := &Client{Model: "model"}
	_, _, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "API key")
}
