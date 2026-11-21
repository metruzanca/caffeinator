package imagegen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGenerateBuildsRequest(t *testing.T) {
	var got imageRequest
	var gotPath, gotAuth string
	imageBytes := []byte("fake-image-bytes")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")

		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("cannot decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"b64_json":   base64.StdEncoding.EncodeToString(imageBytes),
				"media_type": "image/png",
			}},
			"usage": map[string]any{"cost": 0.018},
		})
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "test-key",
		Model:      "bytedance-seed/seedream-5-0-flash",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	result, err := client.Generate(context.Background(), Request{
		Prompt:           "turn this into watercolor",
		Resolution:       "1K",
		AspectRatio:      "1:1",
		ReferenceDataURL: "data:image/png;base64,abc",
	})
	require.NoError(t, err)
	require.Equal(t, 0.018, result.CostUSD)
	require.Len(t, result.Images, 1)
	require.Equal(t, imageBytes, result.Images[0].Data)
	require.Equal(t, "image/png", result.Images[0].MediaType)

	require.Equal(t, "/images", gotPath)
	require.Equal(t, "Bearer test-key", gotAuth)
	require.Equal(t, "bytedance-seed/seedream-5-0-flash", got.Model)
	require.Equal(t, "turn this into watercolor", got.Prompt)
	require.Equal(t, "1K", got.Resolution)
	require.Equal(t, "1:1", got.AspectRatio)
	require.Equal(t, "b64_json", got.ResponseFormat)
	require.Len(t, got.InputReferences, 1)
	require.Equal(t, "image_url", got.InputReferences[0].Type)
	require.Equal(t, "data:image/png;base64,abc", got.InputReferences[0].ImageURL.URL)
}

func TestGenerateAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad prompt"}}`))
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "test-key",
		Model:      "model",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	_, err := client.Generate(context.Background(), Request{Prompt: "hello"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad prompt")
}

func TestGenerateRequiresPrompt(t *testing.T) {
	client := &Client{APIKey: "test-key", Model: "model"}
	_, err := client.Generate(context.Background(), Request{Prompt: "  "})
	require.Error(t, err)
	require.Contains(t, err.Error(), "prompt")
}

func TestGenerateNoImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "test-key",
		Model:      "model",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	_, err := client.Generate(context.Background(), Request{Prompt: "hello"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no images")
}

func TestGenerateRespectsContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	client := &Client{
		APIKey:     "test-key",
		Model:      "model",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	_, err := client.Generate(ctx, Request{Prompt: "hello"})
	require.Error(t, err)
}
