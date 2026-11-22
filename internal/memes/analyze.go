package memes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/metruzanca/caffeinator/agent"
)

// AnalysisSystemPrompt tells the vision model how to describe a meme.
const AnalysisSystemPrompt = `You describe Reddit posts from programming humour communities so a Discord bot can remember and share them.
Return JSON only with this shape:
{"caption":"one short sentence describing what the image shows","context":"one or two short sentences explaining the joke or reference","tags":["lowercase","search","keywords"],"use_when":["a short phrase describing a conversation where this meme fits"]}
Rules:
- 2 to 6 tags.
- 1 to 3 use_when phrases.
- Describe only what is visible or clearly implied by the post title.
- Do not mention Reddit, upvotes, or the model.
- Do not use markdown.
- Never use em dashes. Use commas or separate sentences.`

// Analysis is the model's understanding of one meme.
type Analysis struct {
	Caption string   `json:"caption"`
	Context string   `json:"context"`
	Tags    []string `json:"tags"`
	UseWhen []string `json:"use_when"`
}

// ChatCaller is the chat surface needed by the analyzer.
type ChatCaller interface {
	ChatWithSystem(ctx context.Context, systemPrompt string, messages []agent.Message) (string, agent.Usage, error)
}

// Analyzer turns a meme image into a caption, context, and retrieval words.
type Analyzer struct {
	client  ChatCaller
	timeout time.Duration
}

// NewAnalyzer creates a meme analyzer.
func NewAnalyzer(client ChatCaller, timeout time.Duration) *Analyzer {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Analyzer{client: client, timeout: timeout}
}

// Analyze describes one meme image.
func (a *Analyzer) Analyze(ctx context.Context, post Post, image Image) (Analysis, error) {
	if a == nil || a.client == nil {
		return Analysis{}, errors.New("meme analyzer is disabled")
	}
	if strings.TrimSpace(image.DataURL()) == "" {
		return Analysis{}, errors.New("meme image is empty")
	}

	prompt := fmt.Sprintf("Subreddit: r/%s\nPost title: %s", post.Subreddit, post.Title)
	messages := []agent.Message{{
		Role:    "user",
		Content: prompt,
		Parts: []agent.ContentPart{
			{Type: "text", Text: "Meme image"},
			{Type: "image_url", ImageURL: &agent.ImageURL{URL: image.DataURL(), Detail: "auto"}},
		},
	}}

	requestCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	content, _, err := a.client.ChatWithSystem(requestCtx, AnalysisSystemPrompt, messages)
	if err != nil {
		return Analysis{}, fmt.Errorf("analyzing meme: %w", err)
	}
	return parseAnalysis(content, post.Title)
}

func parseAnalysis(content, title string) (Analysis, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return Analysis{}, errors.New("meme analysis did not contain JSON")
	}

	var analysis Analysis
	if err := json.Unmarshal([]byte(content[start:end+1]), &analysis); err != nil {
		return Analysis{}, fmt.Errorf("cannot decode meme analysis: %w", err)
	}

	analysis.Caption = truncateMemeText(sanitizeMemeLine(analysis.Caption), 240)
	analysis.Context = truncateMemeText(sanitizeMemeLine(analysis.Context), 480)
	if analysis.Caption == "" {
		analysis.Caption = truncateMemeText(sanitizeMemeLine(title), 240)
	}
	if analysis.Context == "" {
		analysis.Context = analysis.Caption
	}
	analysis.Tags = cleanAnalysisList(analysis.Tags, 8, 32)
	analysis.UseWhen = cleanAnalysisList(analysis.UseWhen, 4, 80)
	return analysis, nil
}

func cleanAnalysisList(values []string, maxItems, maxChars int) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(sanitizeMemeLine(value))
		if value == "" || seen[value] || len(result) >= maxItems {
			continue
		}
		seen[value] = true
		result = append(result, truncateMemeText(value, maxChars))
	}
	return result
}

func sanitizeMemeLine(text string) string {
	text = strings.ReplaceAll(text, "\u2014", ", ")
	text = strings.ReplaceAll(text, "\u2013", ", ")
	return strings.Join(strings.Fields(text), " ")
}

func truncateMemeText(text string, max int) string {
	runes := []rune(text)
	if max <= 0 || len(runes) <= max {
		return text
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
