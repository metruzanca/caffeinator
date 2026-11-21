package reminders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/metruzanca/checkpoint-bot/internal/llm"
)

// ParserSystemPrompt is the system prompt for reminder intent parsing.
const ParserSystemPrompt = `You parse Discord reminder requests. Return JSON only:
{"action":"create|list|none","kind":"reminder|event","text":"...","start":"RFC3339 or empty","recurrence":{"freq":"daily|weekly|monthly","interval":1,"weekdays":[0,1,2,3,4,5,6],"month_day":1}|null,"confidence":0.0}
Use the provided current time and user timezone. Only create when a future time or recurrence is present. If the user asks for a reminder but gives no time, use action create with an empty start. If the user asks what reminders they have, use action list. If the message is not a reminder request, use action none. Use kind event only when the user says event or public. Weekdays use 0 for Sunday through 6 for Saturday. Do not invent times. Do not use em dashes.`

// ChatClient is the small LLM surface needed by the parser.
type ChatClient interface {
	Chat(ctx context.Context, messages []llm.Message) (string, llm.Usage, error)
}

// ParseResult is the structured reminder intent.
type ParseResult struct {
	Action     string      `json:"action"`
	Kind       string      `json:"kind"`
	Text       string      `json:"text"`
	Start      string      `json:"start"`
	Recurrence *Recurrence `json:"recurrence"`
	Confidence float64     `json:"confidence"`
}

// Parser turns natural language into reminder actions.
type Parser struct {
	client ChatClient
}

// NewParser creates a reminder parser.
func NewParser(client ChatClient) *Parser {
	return &Parser{client: client}
}

// Parse asks the model for a reminder action.
func (p *Parser) Parse(ctx context.Context, message, timezone string, now time.Time) (ParseResult, error) {
	if p == nil || p.client == nil {
		return ParseResult{}, errors.New("reminder parser is disabled")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return ParseResult{Action: "none"}, nil
	}
	if strings.TrimSpace(timezone) == "" {
		timezone = "unknown"
	}

	content, _, err := p.client.Chat(ctx, []llm.Message{{
		Role: "user",
		Content: fmt.Sprintf(
			"Current time: %s\nUser timezone: %s\nMessage: %s",
			now.Format(time.RFC3339),
			timezone,
			message,
		),
	}})
	if err != nil {
		return ParseResult{}, err
	}

	return parseResult(content)
}

func parseResult(content string) (ParseResult, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return ParseResult{}, errors.New("reminder parser response did not contain JSON")
	}

	var result ParseResult
	if err := json.Unmarshal([]byte(content[start:end+1]), &result); err != nil {
		return ParseResult{}, fmt.Errorf("cannot decode reminder parser response: %w", err)
	}

	result.Action = strings.ToLower(strings.TrimSpace(result.Action))
	switch result.Action {
	case "create", "list", "none":
	case "":
		result.Action = "none"
	default:
		return ParseResult{}, fmt.Errorf("unknown reminder action %q", result.Action)
	}

	result.Kind = strings.ToLower(strings.TrimSpace(result.Kind))
	if result.Kind == "" {
		result.Kind = string(KindReminder)
	}
	if result.Kind != string(KindReminder) && result.Kind != string(KindEvent) {
		return ParseResult{}, fmt.Errorf("unknown reminder kind %q", result.Kind)
	}

	if result.Confidence <= 0 {
		result.Confidence = 0.8
	}
	if result.Recurrence != nil {
		result.Recurrence.Freq = strings.ToLower(strings.TrimSpace(result.Recurrence.Freq))
		if result.Recurrence.Interval < 1 {
			result.Recurrence.Interval = 1
		}
	}
	result.Text = strings.TrimSpace(result.Text)
	result.Start = strings.TrimSpace(result.Start)
	return result, nil
}
