package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/metruzanca/checkpoint-bot/internal/llm"
	"github.com/stretchr/testify/require"
)

type fakeExtractionClient struct {
	response string
	err      error
}

func (f *fakeExtractionClient) Chat(ctx context.Context, messages []llm.Message) (string, llm.Usage, error) {
	if f.err != nil {
		return "", llm.Usage{}, f.err
	}
	return f.response, llm.Usage{}, nil
}

func TestExtractorParsesFacts(t *testing.T) {
	client := &fakeExtractionClient{response: "```json\n" + `{
		"facts": [
			{"subject_id": "123", "fact": "Building a Go Discord bot.", "category": "work", "confidence": 0.95},
			{"subject_id": "server", "fact": "The server is for developers.", "category": "server", "confidence": 0.9}
		]
	}` + "\n```"}
	extractor := NewExtractor(client, 0.7)

	facts, err := extractor.Extract(context.Background(), "alice (123): I am building a Go bot.", map[string]string{
		"server": "server",
		"123":    "Alice",
	})
	require.NoError(t, err)
	require.Len(t, facts, 2)

	require.Equal(t, "123", facts[0].SubjectID)
	require.Equal(t, "Alice", facts[0].SubjectName)
	require.Equal(t, "work", facts[0].Category)
	require.Equal(t, "server", facts[1].SubjectID)
}

func TestExtractorFiltersFacts(t *testing.T) {
	client := &fakeExtractionClient{response: `{
		"facts": [
			{"subject_id": "123", "fact": "Low confidence.", "category": "work", "confidence": 0.2},
			{"subject_id": "999", "fact": "Unknown subject.", "category": "work", "confidence": 0.9},
			{"subject_id": "123", "fact": "Valid fact.", "category": "work", "confidence": 0.9}
		]
	}`}
	extractor := NewExtractor(client, 0.7)

	facts, err := extractor.Extract(context.Background(), "alice (123): hello", map[string]string{
		"123": "Alice",
	})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Equal(t, "Valid fact.", facts[0].Text)
}

func TestExtractorRejectsServerWhenNotAllowed(t *testing.T) {
	client := &fakeExtractionClient{response: `{
		"facts": [
			{"subject_id": "server", "fact": "The server is for developers.", "category": "server", "confidence": 0.9}
		]
	}`}
	extractor := NewExtractor(client, 0.7)

	facts, err := extractor.Extract(context.Background(), "alice (123): hello", map[string]string{
		"123": "Alice",
	})
	require.NoError(t, err)
	require.Empty(t, facts)
}

func TestExtractorRejectsInvalidJSON(t *testing.T) {
	client := &fakeExtractionClient{response: "not json"}
	extractor := NewExtractor(client, 0.7)

	facts, err := extractor.Extract(context.Background(), "alice (123): hello", map[string]string{
		"123": "Alice",
	})
	require.Error(t, err)
	require.Empty(t, facts)
}

func TestExtractorReturnsClientError(t *testing.T) {
	client := &fakeExtractionClient{err: errors.New("model down")}
	extractor := NewExtractor(client, 0.7)

	_, err := extractor.Extract(context.Background(), "alice (123): hello", map[string]string{
		"123": "Alice",
	})
	require.Error(t, err)
}
