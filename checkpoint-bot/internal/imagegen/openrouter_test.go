package imagegen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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

	tolerance := 5
	client := &Client{
		APIKey:     "test-key",
		Model:      "black-forest-labs/flux-3-image-klein",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	result, err := client.Generate(context.Background(), Request{
		Prompt:           "turn this into watercolor",
		Resolution:       "1K",
		AspectRatio:      "1:1",
		ReferenceDataURL: "data:image/png;base64,abc",
		SafetyTolerance:  &tolerance,
	})
	require.NoError(t, err)
	require.Equal(t, 0.018, result.CostUSD)
	require.Len(t, result.Images, 1)
	require.Equal(t, imageBytes, result.Images[0].Data)
	require.Equal(t, "image/png", result.Images[0].MediaType)

	require.Equal(t, "/images", gotPath)
	require.Equal(t, "Bearer test-key", gotAuth)
	require.Equal(t, "black-forest-labs/flux-3-image-klein", got.Model)
	require.Equal(t, "turn this into watercolor", got.Prompt)
	require.Equal(t, "1K", got.Resolution)
	require.Equal(t, "1:1", got.AspectRatio)
	require.Equal(t, "b64_json", got.ResponseFormat)
	require.Len(t, got.InputReferences, 1)
	require.Equal(t, "image_url", got.InputReferences[0].Type)
	require.Equal(t, "data:image/png;base64,abc", got.InputReferences[0].ImageURL.URL)
	require.NotNil(t, got.Provider)
	require.Equal(t, float64(4), got.Provider.Options["black-forest-labs"]["safety_tolerance"])
}

func TestGenerateFlux2AllowsTolerance5(t *testing.T) {
	var got imageRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("cannot decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("image")) + `"}]}`))
	}))
	defer server.Close()

	tolerance := 5
	client := &Client{
		APIKey:     "test-key",
		Model:      "black-forest-labs/flux.2-klein-4b",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	_, err := client.Generate(context.Background(), Request{
		Prompt:          "hello",
		SafetyTolerance: &tolerance,
	})
	require.NoError(t, err)
	require.NotNil(t, got.Provider)
	require.Equal(t, float64(5), got.Provider.Options["black-forest-labs"]["safety_tolerance"])
}

func TestGenerateOmitsProviderOptionsForOtherModels(t *testing.T) {
	var got imageRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("cannot decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("image")) + `"}]}`))
	}))
	defer server.Close()

	tolerance := 5
	client := &Client{
		APIKey:     "test-key",
		Model:      "bytedance-seed/seedream-5-0-flash",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	_, err := client.Generate(context.Background(), Request{
		Prompt:          "hello",
		SafetyTolerance: &tolerance,
	})
	require.NoError(t, err)
	require.Nil(t, got.Provider)
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

func TestGenerateContentBlockedSexual(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Black Forest Labs blocked this request: it was flagged for sexual or adult content."}}`))
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "test-key",
		Model:      "black-forest-labs/flux-3-image-klein",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	_, err := client.Generate(context.Background(), Request{Prompt: "hello"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrContentBlocked)

	var blocked *ContentBlockedError
	require.True(t, errors.As(err, &blocked))
	require.Equal(t, BlockedSexual, blocked.Category)
	require.Equal(t, http.StatusBadRequest, blocked.StatusCode)
}

func TestGenerateContentBlockedOther(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"The request violated the content policy."}}`))
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "test-key",
		Model:      "black-forest-labs/flux-3-image-klein",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	_, err := client.Generate(context.Background(), Request{Prompt: "hello"})
	require.Error(t, err)

	var blocked *ContentBlockedError
	require.True(t, errors.As(err, &blocked))
	require.Equal(t, BlockedOther, blocked.Category)
}

func TestGenerateNonModerationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid image size"}}`))
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "test-key",
		Model:      "black-forest-labs/flux-3-image-klein",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	_, err := client.Generate(context.Background(), Request{Prompt: "hello"})
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrContentBlocked)

	var blocked *ContentBlockedError
	require.False(t, errors.As(err, &blocked))
}

func TestGenerateRateLimitFromStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "45")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"Black Forest Labs submit returned 429: You have too many active tasks."}}`))
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "test-key",
		Model:      "black-forest-labs/flux.2-klein-4b",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	_, err := client.Generate(context.Background(), Request{Prompt: "hello"})
	require.Error(t, err)

	var rateErr *RateLimitError
	require.True(t, errors.As(err, &rateErr))
	require.Equal(t, http.StatusTooManyRequests, rateErr.StatusCode)
	require.InDelta(t, 45, rateErr.RetryAfter.Seconds(), 1)
}

func TestGenerateRateLimitParsesMessageDuration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"Too many requests. Please try again in 2 minutes."}}`))
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "test-key",
		Model:      "black-forest-labs/flux.2-klein-4b",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	_, err := client.Generate(context.Background(), Request{Prompt: "hello"})
	require.Error(t, err)

	var rateErr *RateLimitError
	require.True(t, errors.As(err, &rateErr))
	require.InDelta(t, (2 * time.Minute).Seconds(), rateErr.RetryAfter.Seconds(), 1)
}

func TestGenerateRateLimitWithoutHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"Black Forest Labs submit returned 429: You have too many active tasks."}}`))
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "test-key",
		Model:      "black-forest-labs/flux.2-klein-4b",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	_, err := client.Generate(context.Background(), Request{Prompt: "hello"})
	require.Error(t, err)

	var rateErr *RateLimitError
	require.True(t, errors.As(err, &rateErr))
	require.Zero(t, rateErr.RetryAfter)
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
