package memes

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/metruzanca/caffeinator/agent"
	"github.com/stretchr/testify/require"
)

var onePixelPNG, _ = base64.StdEncoding.DecodeString(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lK3Q6wAAAABJRU5ErkJggg==",
)

func TestImageFetcherDownloadsImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(onePixelPNG)
	}))
	defer server.Close()

	fetcher := NewImageFetcher("test-agent", 1)
	fetcher.allowURL = func(string) bool { return true }

	image, err := fetcher.Fetch(context.Background(), server.URL+"/meme.png")
	require.NoError(t, err)
	require.Equal(t, "image/png", image.MediaType)
	require.Equal(t, ".png", image.Ext)
	require.Equal(t, onePixelPNG, image.Data)
	require.True(t, strings.HasPrefix(image.DataURL(), "data:image/png;base64,"))
}

func TestImageFetcherRejectsOversizedImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(onePixelPNG)
	}))
	defer server.Close()

	fetcher := NewImageFetcher("test-agent", 1)
	fetcher.allowURL = func(string) bool { return true }
	fetcher.maxBytes = 10

	_, err := fetcher.Fetch(context.Background(), server.URL+"/meme.png")
	require.Error(t, err)
}

func TestImageFetcherRejectsHTMLBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("<html><body>blocked</body></html>"))
	}))
	defer server.Close()

	fetcher := NewImageFetcher("test-agent", 1)
	fetcher.allowURL = func(string) bool { return true }

	_, err := fetcher.Fetch(context.Background(), server.URL+"/meme.png")
	require.Error(t, err)
}

func TestImageFetcherRejectsUnknownHost(t *testing.T) {
	fetcher := NewImageFetcher("test-agent", 1)
	_, err := fetcher.Fetch(context.Background(), "https://example.com/meme.png")
	require.Error(t, err)
}

func TestSaveAndLoadImage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "images")
	image := Image{Data: onePixelPNG, MediaType: "image/png", Ext: ".png"}

	name, err := SaveImage(dir, "abc123", image)
	require.NoError(t, err)
	require.Equal(t, "abc123.png", name)

	data, mediaType, err := LoadImageFile(dir, name)
	require.NoError(t, err)
	require.Equal(t, "image/png", mediaType)
	require.Equal(t, onePixelPNG, data)

	require.NoError(t, DeleteImage(dir, name))
	_, _, err = LoadImageFile(dir, name)
	require.Error(t, err)
}

func TestSaveImageRejectsUnsafeName(t *testing.T) {
	_, err := SaveImage(t.TempDir(), "../escape", Image{Data: onePixelPNG, MediaType: "image/png", Ext: ".png"})
	require.Error(t, err)
}

type fakeMemeChat struct {
	response string
	err      error
	system   string
	messages []agent.Message
}

func (f *fakeMemeChat) ChatWithSystem(ctx context.Context, systemPrompt string, messages []agent.Message) (string, agent.Usage, error) {
	f.system = systemPrompt
	f.messages = messages
	return f.response, agent.Usage{}, f.err
}

func TestAnalyzerParsesStructuredResponse(t *testing.T) {
	client := &fakeMemeChat{response: "```json\n{\"caption\":\"A green build.\",\"context\":\"It is a rare clean build.\",\"tags\":[\"Build\",\"CI\"],\"use_when\":[\"someone celebrates a passing build\"]}\n```"}
	analyzer := NewAnalyzer(client, time.Second)

	post := Post{Subreddit: "ProgrammerHumor", Title: "When it works", ImageURL: "https://i.redd.it/x.png"}
	image := Image{Data: onePixelPNG, MediaType: "image/png", Ext: ".png"}

	analysis, err := analyzer.Analyze(context.Background(), post, image)
	require.NoError(t, err)
	require.Equal(t, "A green build.", analysis.Caption)
	require.Equal(t, "It is a rare clean build.", analysis.Context)
	require.Equal(t, []string{"build", "ci"}, analysis.Tags)
	require.Equal(t, []string{"someone celebrates a passing build"}, analysis.UseWhen)

	require.Len(t, client.messages, 1)
	require.Len(t, client.messages[0].Parts, 2)
	require.Equal(t, "image_url", client.messages[0].Parts[1].Type)
	require.True(t, strings.HasPrefix(client.messages[0].Parts[1].ImageURL.URL, "data:image/png;base64,"))
	require.Contains(t, client.messages[0].Content, "When it works")
}

func TestAnalyzerFallsBackToTitle(t *testing.T) {
	client := &fakeMemeChat{response: `{"caption":"","context":"","tags":[],"use_when":[]}`}
	analyzer := NewAnalyzer(client, time.Second)

	analysis, err := analyzer.Analyze(context.Background(), Post{Title: "Fallback title"}, Image{
		Data:      onePixelPNG,
		MediaType: "image/png",
		Ext:       ".png",
	})
	require.NoError(t, err)
	require.Equal(t, "Fallback title", analysis.Caption)
	require.Equal(t, "Fallback title", analysis.Context)
}

func TestAnalyzerRejectsInvalidJSON(t *testing.T) {
	client := &fakeMemeChat{response: "not json"}
	analyzer := NewAnalyzer(client, time.Second)

	_, err := analyzer.Analyze(context.Background(), Post{Title: "Nope"}, Image{
		Data:      onePixelPNG,
		MediaType: "image/png",
		Ext:       ".png",
	})
	require.Error(t, err)
}
