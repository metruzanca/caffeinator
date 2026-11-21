package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/metruzanca/checkpoint-bot/internal/llm"
)

// ExtractionSystemPrompt is the system prompt used to extract durable facts.
const ExtractionSystemPrompt = `You extract durable facts from messages a Discord user wrote.
Return JSON only, with this shape:
{"facts":[{"subject_id":"...","fact":"...","category":"identity|work|preference|project|server|other","confidence":0.0}]}
Use only the subject IDs listed as allowed subjects.
Extract first-person facts that the speaker states about themselves.
When "server" is an allowed subject, also extract clear facts the speaker states about the server itself, using subject_id "server".
Only store server facts that describe what the server is, who it is for, or how it is run.
Ignore statements about other people, even if the speaker mentions them.
Do not extract opinions, jokes, complaints, or guesses about the server.
Do not extract facts from other users' messages or from the bot.
Do not extract secrets, credentials, tokens, addresses, health data, or other sensitive personal data.
Do not extract transient plans or small talk.
If nothing is worth remembering, return {"facts":[]}.`

// ChatClient is the small LLM surface needed by the extractor.
type ChatClient interface {
	Chat(ctx context.Context, messages []llm.Message) (string, llm.Usage, error)
}

// Extractor turns a transcript into durable facts.
type Extractor struct {
	client        ChatClient
	minConfidence float64
	maxFacts      int
}

// NewExtractor creates an extractor.
func NewExtractor(client ChatClient, minConfidence float64) *Extractor {
	if minConfidence <= 0 || minConfidence > 1 {
		minConfidence = 0.7
	}
	return &Extractor{
		client:        client,
		minConfidence: minConfidence,
		maxFacts:      8,
	}
}

type extractionResponse struct {
	Facts []struct {
		SubjectID  string  `json:"subject_id"`
		Fact       string  `json:"fact"`
		Category   string  `json:"category"`
		Confidence float64 `json:"confidence"`
	} `json:"facts"`
}

// Extract asks the model for facts and filters the result.
func (e *Extractor) Extract(ctx context.Context, transcript string, subjects map[string]string) ([]Fact, error) {
	if e == nil || e.client == nil {
		return nil, errors.New("memory extractor is disabled")
	}
	if strings.TrimSpace(transcript) == "" {
		return nil, nil
	}

	var builder strings.Builder
	builder.WriteString("Allowed subjects:\n")
	if _, ok := subjects["server"]; ok {
		builder.WriteString("server: this Discord server\n")
	}

	ids := make([]string, 0, len(subjects))
	for id := range subjects {
		if id == "server" {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		name := strings.TrimSpace(subjects[id])
		if name == "" {
			name = id
		}
		fmt.Fprintf(&builder, "%s: %s\n", id, name)
	}

	builder.WriteString("\nTranscript:\n")
	builder.WriteString(transcript)

	content, _, err := e.client.Chat(ctx, []llm.Message{{
		Role:    "user",
		Content: builder.String(),
	}})
	if err != nil {
		return nil, err
	}
	return e.parse(content, subjects)
}

func (e *Extractor) parse(content string, subjects map[string]string) ([]Fact, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, errors.New("memory extraction response did not contain JSON")
	}

	var response extractionResponse
	if err := json.Unmarshal([]byte(content[start:end+1]), &response); err != nil {
		return nil, fmt.Errorf("cannot decode memory extraction response: %w", err)
	}

	facts := make([]Fact, 0, len(response.Facts))
	for _, item := range response.Facts {
		if len(facts) >= e.maxFacts {
			break
		}

		subjectID := strings.TrimSpace(item.SubjectID)
		if _, ok := subjects[subjectID]; !ok {
			continue
		}
		if item.Confidence < e.minConfidence {
			continue
		}

		text := cleanFact(item.Fact, defaultMaxFactChars)
		if text == "" {
			continue
		}

		name := "server"
		if subjectID != "server" {
			name = strings.TrimSpace(subjects[subjectID])
			if name == "" {
				name = subjectID
			}
		}

		facts = append(facts, Fact{
			SubjectID:   subjectID,
			SubjectName: name,
			Text:        text,
			Category:    cleanCategory(item.Category),
			Confidence:  item.Confidence,
		})
	}
	return facts, nil
}
