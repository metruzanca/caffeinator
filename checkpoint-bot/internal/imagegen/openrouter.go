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
	"strings"
)

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	defaultModel   = "bytedance-seed/seedream-5-0-flash"
	maxBodyBytes   = 64 << 20
)

// Request describes one image generation request.
type Request struct {
	Prompt           string
	Model            string
	Resolution       string
	AspectRatio      string
	OutputFormat     string
	ReferenceDataURL string
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
	httpReq.Header.Set("HTTP-Referer", "https://github.com/metruzanca/checkpoint-bot")
	httpReq.Header.Set("X-Title", "Caffeinator")

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
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != nil && apiErr.Error.Message != "" {
			return Result{}, fmt.Errorf("openrouter image: %s (status %d)", apiErr.Error.Message, resp.StatusCode)
		}
		return Result{}, fmt.Errorf("openrouter image: status %d: %s", resp.StatusCode, truncate(string(data), 300))
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

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
