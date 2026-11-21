// Package imagegen provides a minimal OpenRouter image generation client.
package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/metruzanca/caffeinator/internal/config"
)

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	defaultModel   = "black-forest-labs/flux-3-image-klein"
	maxBodyBytes   = 64 << 20
)

// ErrContentBlocked indicates the image provider rejected the prompt or output.
var ErrContentBlocked = errors.New("image content blocked")

// BlockedCategory classifies a content block.
type BlockedCategory string

const (
	BlockedSexual BlockedCategory = "sexual"
	BlockedOther  BlockedCategory = "other"
)

// ContentBlockedError describes a provider moderation block.
type ContentBlockedError struct {
	Reason     string
	Category   BlockedCategory
	StatusCode int
}

func (e *ContentBlockedError) Error() string {
	if strings.TrimSpace(e.Reason) == "" {
		return "image content blocked"
	}
	return e.Reason
}

// Unwrap allows errors.Is(err, ErrContentBlocked).
func (e *ContentBlockedError) Unwrap() error {
	return ErrContentBlocked
}

// ProviderError describes a non-moderation provider rejection.
type ProviderError struct {
	Reason     string
	StatusCode int
}

func (e *ProviderError) Error() string {
	if strings.TrimSpace(e.Reason) == "" {
		return fmt.Sprintf("image provider returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("openrouter image: %s (status %d)", e.Reason, e.StatusCode)
}

// RateLimitError describes a provider throttle or busy response.
type RateLimitError struct {
	Reason     string
	StatusCode int
	// RetryAfter is zero when the provider gave no hint.
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if strings.TrimSpace(e.Reason) == "" {
		return fmt.Sprintf("image provider rate limited (status %d)", e.StatusCode)
	}
	return fmt.Sprintf("openrouter image: %s (status %d)", e.Reason, e.StatusCode)
}

// Request describes one image generation request.
type Request struct {
	Prompt           string
	Model            string
	Resolution       string
	AspectRatio      string
	OutputFormat     string
	ReferenceDataURL string
	SafetyTolerance  *int
}

// Output is a generated image.
type Output struct {
	Data      []byte
	MediaType string
}

// Result is a completed image generation response.
type Result struct {
	Images  []Output
	CostUSD float64
}

// Client calls OpenRouter's image generation endpoint.
type Client struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

type imageRequest struct {
	Model           string           `json:"model"`
	Prompt          string           `json:"prompt"`
	Resolution      string           `json:"resolution,omitempty"`
	AspectRatio     string           `json:"aspect_ratio,omitempty"`
	OutputFormat    string           `json:"output_format,omitempty"`
	ResponseFormat  string           `json:"response_format"`
	InputReferences []inputReference `json:"input_references,omitempty"`
	Provider        *providerOptions `json:"provider,omitempty"`
}

type providerOptions struct {
	Options map[string]map[string]any `json:"options,omitempty"`
}

type inputReference struct {
	Type     string `json:"type"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

type imageResponse struct {
	Data []struct {
		B64JSON   string `json:"b64_json"`
		URL       string `json:"url"`
		MediaType string `json:"media_type"`
	} `json:"data"`
	Usage struct {
		Cost float64 `json:"cost"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Generate creates one or more images.
func (c *Client) Generate(ctx context.Context, req Request) (Result, error) {
	if c == nil || strings.TrimSpace(c.APIKey) == "" {
		return Result{}, errors.New("openrouter API key is empty")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return Result{}, errors.New("image prompt is empty")
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = c.Model
	}
	if model == "" {
		model = defaultModel
	}

	body := imageRequest{
		Model:          model,
		Prompt:         strings.TrimSpace(req.Prompt),
		Resolution:     strings.TrimSpace(req.Resolution),
		AspectRatio:    strings.TrimSpace(req.AspectRatio),
		OutputFormat:   strings.TrimSpace(req.OutputFormat),
		ResponseFormat: "b64_json",
	}
	if ref := strings.TrimSpace(req.ReferenceDataURL); ref != "" {
		reference := inputReference{Type: "image_url"}
		reference.ImageURL.URL = ref
		body.InputReferences = []inputReference{reference}
	}
	if req.SafetyTolerance != nil && strings.HasPrefix(strings.ToLower(model), "black-forest-labs/") {
		tolerance := *req.SafetyTolerance
		if tolerance < 0 {
			tolerance = 0
		}
		if tolerance > 4 {
			tolerance = 4
		}
		body.Provider = &providerOptions{
			Options: map[string]map[string]any{
				"black-forest-labs": {
					"safety_tolerance": tolerance,
				},
			},
		}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return Result{}, fmt.Errorf("cannot encode image request: %w", err)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/images", bytes.NewReader(payload))
	if err != nil {
		return Result{}, fmt.Errorf("cannot create image request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", config.AppURL())
	httpReq.Header.Set("X-Title", config.BotName())

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("openrouter image request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return Result{}, fmt.Errorf("cannot read image response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		var apiErr imageResponse
		message := ""
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != nil {
			message = strings.TrimSpace(apiErr.Error.Message)
		}
		if message == "" {
			message = truncate(string(data), 300)
		}

		if resp.StatusCode == http.StatusBadRequest && isModerationMessage(message) {
			return Result{}, &ContentBlockedError{
				Reason:     message,
				Category:   classifyBlocked(message),
				StatusCode: resp.StatusCode,
			}
		}

		if resp.StatusCode == http.StatusTooManyRequests || isRateLimitMessage(message) {
			return Result{}, &RateLimitError{
				Reason:     message,
				StatusCode: resp.StatusCode,
				RetryAfter: parseRetryAfter(resp.Header, message),
			}
		}

		return Result{}, &ProviderError{
			Reason:     message,
			StatusCode: resp.StatusCode,
		}
	}

	var parsed imageResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Result{}, fmt.Errorf("cannot decode image response: %w", err)
	}
	if parsed.Error != nil {
		return Result{}, fmt.Errorf("openrouter image: %s", parsed.Error.Message)
	}
	if len(parsed.Data) == 0 {
		return Result{}, errors.New("openrouter returned no images")
	}

	result := Result{CostUSD: parsed.Usage.Cost}
	for _, item := range parsed.Data {
		if strings.TrimSpace(item.B64JSON) == "" {
			if strings.TrimSpace(item.URL) != "" {
				return Result{}, errors.New("openrouter returned a URL instead of image bytes")
			}
			continue
		}
		encoded := item.B64JSON
		if strings.HasPrefix(encoded, "data:") {
			if comma := strings.Index(encoded, ","); comma >= 0 {
				encoded = encoded[comma+1:]
			}
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return Result{}, fmt.Errorf("cannot decode image bytes: %w", err)
		}
		mediaType := strings.TrimSpace(item.MediaType)
		if mediaType == "" {
			mediaType = "image/png"
		}
		result.Images = append(result.Images, Output{
			Data:      decoded,
			MediaType: mediaType,
		})
	}

	if len(result.Images) == 0 {
		return Result{}, errors.New("openrouter returned no decodable images")
	}
	return result, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func isRateLimitMessage(message string) bool {
	message = strings.ToLower(message)
	phrases := []string{
		"too many active tasks",
		"too many requests",
		"rate limit",
		"rate-limit",
		"ratelimit",
		"too quickly",
		"please wait for some to complete",
	}
	for _, phrase := range phrases {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

var retryDurationPattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(hours?|hrs?|h|minutes?|mins?|m|seconds?|secs?|s)\b`)

// parseRetryAfter reads provider headers first, then falls back to the message.
// It returns zero when no hint is available.
func parseRetryAfter(header http.Header, message string) time.Duration {
	for _, name := range []string{"Retry-After", "X-RateLimit-Reset-After"} {
		raw := strings.TrimSpace(header.Get(name))
		if raw == "" {
			continue
		}
		if seconds, err := strconv.ParseFloat(raw, 64); err == nil && seconds >= 0 {
			return time.Duration(seconds * float64(time.Second))
		}
		if when, err := http.ParseTime(raw); err == nil {
			if wait := time.Until(when); wait > 0 {
				return wait
			}
		}
	}

	if raw := strings.TrimSpace(header.Get("X-RateLimit-Reset")); raw != "" {
		if epoch, err := strconv.ParseInt(raw, 10, 64); err == nil {
			if wait := time.Until(time.Unix(epoch, 0)); wait > 0 {
				return wait
			}
		}
	}

	return parseMessageDuration(message)
}

func parseMessageDuration(message string) time.Duration {
	match := retryDurationPattern.FindStringSubmatch(message)
	if len(match) != 3 {
		return 0
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil || value < 0 {
		return 0
	}

	unit := strings.ToLower(match[2])
	multiplier := time.Second
	switch {
	case strings.HasPrefix(unit, "h"):
		multiplier = time.Hour
	case strings.HasPrefix(unit, "m"):
		multiplier = time.Minute
	}
	return time.Duration(value * float64(multiplier))
}

func isModerationMessage(message string) bool {
	message = strings.ToLower(message)
	phrases := []string{
		"blocked this request",
		"flagged for sexual",
		"adult content",
		"content policy",
		"moderation",
		"nudity",
		"nsfw",
		"safety filter",
		"sensitive content",
	}
	for _, phrase := range phrases {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

func classifyBlocked(message string) BlockedCategory {
	message = strings.ToLower(message)
	sexualPhrases := []string{"sexual", "adult content", "nsfw", "nudity", "explicit"}
	for _, phrase := range sexualPhrases {
		if strings.Contains(message, phrase) {
			return BlockedSexual
		}
	}
	return BlockedOther
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
