// Package llm provides a minimal OpenRouter chat client.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/metruzanca/caffeinator/internal/config"
)

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	maxBodyBytes   = 1 << 20
)

// Message is a single chat message in an OpenAI-compatible request.
type Message struct {
	Role    string        `json:"role"`
	Content string        `json:"-"`
	Parts   []ContentPart `json:"-"`
}

// ContentPart is a text or image part in a multimodal message.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL points at an image or a base64 data URL.
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// MarshalJSON keeps text-only messages as a string and switches to content
// parts when images are present.
func (m Message) MarshalJSON() ([]byte, error) {
	type wireMessage struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}

	if len(m.Parts) == 0 {
		return json.Marshal(wireMessage{Role: m.Role, Content: m.Content})
	}

	parts := make([]ContentPart, 0, len(m.Parts)+1)
	if strings.TrimSpace(m.Content) != "" {
		parts = append(parts, ContentPart{Type: "text", Text: m.Content})
	}
	parts = append(parts, m.Parts...)

	return json.Marshal(wireMessage{Role: m.Role, Content: parts})
}

// UnmarshalJSON accepts either a string or an array of content parts.
func (m *Message) UnmarshalJSON(data []byte) error {
	var wire struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	m.Role = wire.Role
	m.Content = ""
	m.Parts = nil

	if len(wire.Content) == 0 || string(wire.Content) == "null" {
		return nil
	}

	var text string
	if err := json.Unmarshal(wire.Content, &text); err == nil {
		m.Content = text
		return nil
	}

	var parts []ContentPart
	if err := json.Unmarshal(wire.Content, &parts); err != nil {
		return err
	}
	for _, part := range parts {
		if part.Type == "text" {
			if m.Content != "" {
				m.Content += "\n"
			}
			m.Content += part.Text
			continue
		}
		m.Parts = append(m.Parts, part)
	}
	return nil
}

// Usage reports token usage for a request.
type Usage struct {
	PromptTokens         int `json:"prompt_tokens"`
	CompletionTokens     int `json:"completion_tokens"`
	TotalTokens          int `json:"total_tokens"`
	ServerToolUseDetails struct {
		WebSearchRequests int `json:"web_search_requests"`
	} `json:"server_tool_use_details"`
}

// Tool is an OpenRouter server tool entry.
type Tool struct {
	Type       string               `json:"type"`
	Parameters *WebSearchParameters `json:"parameters,omitempty"`
}

// WebSearchParameters configures the OpenRouter web search server tool.
type WebSearchParameters struct {
	MaxResults int `json:"max_results,omitempty"`
	MaxUses    int `json:"max_uses,omitempty"`
}

// ResponseFormat requests a specific response shape.
type ResponseFormat struct {
	Type string `json:"type"`
}

// Client calls OpenRouter's chat completions endpoint.
type Client struct {
	APIKey              string
	Model               string
	BaseURL             string
	MaxTokens           int
	SystemPrompt        string
	ResponseFormat      *ResponseFormat
	WebSearch           bool
	WebSearchMaxResults int
	WebSearchMaxUses    int
	RetryDelay          time.Duration
	HTTPClient          *http.Client
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	Tools          []Tool          `json:"tools,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	} `json:"error"`
}

// Chat sends the conversation to OpenRouter using the client's system prompt.
func (c *Client) Chat(ctx context.Context, messages []Message) (string, Usage, error) {
	return c.ChatWithSystem(ctx, c.SystemPrompt, messages)
}

// ChatWithSystem sends the conversation with the given system prompt.
func (c *Client) ChatWithSystem(ctx context.Context, systemPrompt string, messages []Message) (string, Usage, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return "", Usage{}, errors.New("openrouter API key is empty")
	}
	if strings.TrimSpace(c.Model) == "" {
		return "", Usage{}, errors.New("openrouter model is empty")
	}
	if len(messages) == 0 {
		return "", Usage{}, errors.New("no messages to send")
	}

	if systemPrompt = strings.TrimSpace(systemPrompt); systemPrompt != "" {
		messages = append([]Message{{
			Role:    "system",
			Content: systemPrompt,
		}}, messages...)
	}

	request := chatRequest{
		Model:          c.Model,
		Messages:       messages,
		MaxTokens:      c.MaxTokens,
		ResponseFormat: c.ResponseFormat,
	}
	if c.WebSearch {
		request.Tools = []Tool{{
			Type: "openrouter:web_search",
			Parameters: &WebSearchParameters{
				MaxResults: c.WebSearchMaxResults,
				MaxUses:    c.WebSearchMaxUses,
			},
		}}
	}

	body, err := json.Marshal(request)
	if err != nil {
		return "", Usage{}, fmt.Errorf("cannot encode request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			delay := c.RetryDelay
			if delay <= 0 {
				delay = 750 * time.Millisecond
			}
			select {
			case <-ctx.Done():
				return "", Usage{}, ctx.Err()
			case <-time.After(delay):
			}
		}

		content, usage, retry, err := c.do(ctx, body)
		if err == nil {
			return content, usage, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}

	return "", Usage{}, lastErr
}

func (c *Client) do(ctx context.Context, body []byte) (string, Usage, bool, error) {
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, false, fmt.Errorf("cannot create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", config.AppURL())
	req.Header.Set("X-Title", config.BotName())

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", Usage{}, true, fmt.Errorf("openrouter request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", Usage{}, true, fmt.Errorf("cannot read openrouter response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError

		var apiErr chatResponse
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != nil {
			return "", Usage{}, retry, fmt.Errorf("openrouter: %s (status %d)", apiErr.Error.Message, resp.StatusCode)
		}

		return "", Usage{}, retry, fmt.Errorf("openrouter: status %d: %s", resp.StatusCode, truncate(string(data), 300))
	}

	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", Usage{}, true, fmt.Errorf("cannot decode openrouter response: %w", err)
	}
	if parsed.Error != nil {
		return "", parsed.Usage, false, fmt.Errorf("openrouter: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", parsed.Usage, true, errors.New("openrouter returned no choices")
	}

	choice := parsed.Choices[0]
	content := strings.TrimSpace(choice.Message.Content)
	if content == "" {
		finishReason := choice.FinishReason
		if finishReason == "" {
			finishReason = "unknown"
		}

		if reasoning := strings.TrimSpace(choice.Message.Reasoning); reasoning != "" {
			return "", parsed.Usage, true, fmt.Errorf(
				"openrouter returned reasoning without content (finish_reason=%s, reasoning_chars=%d)",
				finishReason,
				len([]rune(reasoning)),
			)
		}

		return "", parsed.Usage, true, fmt.Errorf("openrouter returned an empty response (finish_reason=%s)", finishReason)
	}

	return content, parsed.Usage, false, nil
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
