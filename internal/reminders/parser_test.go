package reminders

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/metruzanca/caffeinator/internal/llm"
	"github.com/stretchr/testify/require"
)

type fakeParserClient struct {
	response string
	err      error
}

func (f *fakeParserClient) Chat(ctx context.Context, messages []llm.Message) (string, llm.Usage, error) {
	if f.err != nil {
		return "", llm.Usage{}, f.err
	}
	return f.response, llm.Usage{}, nil
}

func TestParserParseCreate(t *testing.T) {
	parser := NewParser(&fakeParserClient{response: `{
		"action": "create",
		"kind": "reminder",
		"text": "standup",
		"start": "2026-11-22T16:00:00-05:00",
		"recurrence": null,
		"confidence": 0.95
	}`})

	result, err := parser.Parse(context.Background(), "remind me tomorrow at 4pm", "America/New_York", time.Now())
	require.NoError(t, err)
	require.Equal(t, "create", result.Action)
	require.Equal(t, "reminder", result.Kind)
	require.Equal(t, "standup", result.Text)
	require.Equal(t, "2026-11-22T16:00:00-05:00", result.Start)
	require.Nil(t, result.Recurrence)
	require.Equal(t, 0.95, result.Confidence)
}

func TestParserParseRecurring(t *testing.T) {
	parser := NewParser(&fakeParserClient{response: `{
		"action": "create",
		"kind": "event",
		"text": "coffee",
		"start": "2026-11-23T09:00:00-05:00",
		"recurrence": {"freq":"weekly","interval":1,"weekdays":[1]},
		"confidence": 0.9
	}`})

	result, err := parser.Parse(context.Background(), "every monday at 9am", "America/New_York", time.Now())
	require.NoError(t, err)
	require.Equal(t, "event", result.Kind)
	require.NotNil(t, result.Recurrence)
	require.Equal(t, "weekly", result.Recurrence.Freq)
	require.Equal(t, []int{1}, result.Recurrence.Weekdays)
}

func TestParserParseListAndNone(t *testing.T) {
	parser := NewParser(&fakeParserClient{response: `{"action":"list","kind":"reminder","text":"","start":"","recurrence":null,"confidence":0.9}`})
	result, err := parser.Parse(context.Background(), "what reminders do I have", "UTC", time.Now())
	require.NoError(t, err)
	require.Equal(t, "list", result.Action)

	parser = NewParser(&fakeParserClient{response: `{"action":"none","kind":"reminder","text":"","start":"","recurrence":null,"confidence":0.9}`})
	result, err = parser.Parse(context.Background(), "what is the deploy command", "UTC", time.Now())
	require.NoError(t, err)
	require.Equal(t, "none", result.Action)
}

func TestParserRejectsInvalidJSON(t *testing.T) {
	parser := NewParser(&fakeParserClient{response: "not json"})
	_, err := parser.Parse(context.Background(), "remind me tomorrow", "UTC", time.Now())
	require.Error(t, err)
}

func TestParserReturnsClientError(t *testing.T) {
	parser := NewParser(&fakeParserClient{err: errors.New("model down")})
	_, err := parser.Parse(context.Background(), "remind me tomorrow", "UTC", time.Now())
	require.Error(t, err)
}
