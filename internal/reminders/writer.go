package reminders

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/metruzanca/caffeinator/agent"
)

// WriterSystemPrompt is the system prompt for reminder message text.
const WriterSystemPrompt = `Write one short Discord reminder sentence. Do not include @mentions. Do not use em dashes. Keep it under 200 characters. If reminder text is provided, include it naturally. If it is empty, write a simple reminder.`

// Writer generates the body of a reminder message.
type Writer struct {
	client ChatClient
}

// NewWriter creates a reminder message writer.
func NewWriter(client ChatClient) *Writer {
	return &Writer{client: client}
}

// Write returns a short reminder body.
func (w *Writer) Write(ctx context.Context, text string, at time.Time) (string, error) {
	if w == nil || w.client == nil {
		return "", errors.New("reminder writer is disabled")
	}

	content, _, err := w.client.Chat(ctx, []agent.Message{{
		Role: "user",
		Content: fmt.Sprintf(
			"Reminder text: %s\nScheduled time: %s",
			strings.TrimSpace(text),
			at.Format(time.RFC3339),
		),
	}})
	if err != nil {
		return "", err
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return "", errors.New("reminder writer returned an empty message")
	}
	return content, nil
}
