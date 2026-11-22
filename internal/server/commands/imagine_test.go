package commands

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/metruzanca/caffeinator/agent"
	"github.com/stretchr/testify/require"
)

func TestDownloadReferenceImage(t *testing.T) {
	data := []byte("fake-png-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
	}))
	defer server.Close()

	attachment := &discordgo.MessageAttachment{
		URL:         server.URL,
		ContentType: "image/png",
		Size:        len(data),
	}

	got, err := downloadReferenceImage(context.Background(), attachment, 1024)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(got, "data:image/png;base64,"))

	encoded := strings.TrimPrefix(got, "data:image/png;base64,")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	require.Equal(t, data, decoded)
}

func TestDownloadReferenceImageRejectsLargeAttachment(t *testing.T) {
	attachment := &discordgo.MessageAttachment{
		URL:         "https://example.com/image.png",
		ContentType: "image/png",
		Size:        2048,
	}

	_, err := downloadReferenceImage(context.Background(), attachment, 1024)
	require.Error(t, err)
	require.Contains(t, err.Error(), "too large")
}

func TestDownloadReferenceImageRejectsUnsupportedType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	attachment := &discordgo.MessageAttachment{
		URL:         server.URL,
		ContentType: "text/plain",
		Size:        5,
	}

	_, err := downloadReferenceImage(context.Background(), attachment, 1024)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported")
}

func TestDownloadReferenceImageWithoutAttachment(t *testing.T) {
	got, err := downloadReferenceImage(context.Background(), nil, 1024)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestImageCaption(t *testing.T) {
	require.Equal(t, "a cat in space", imageCaption("  a cat in space  "))
	require.Equal(t, "a cat in space", imageCaption("a cat in space"))
	require.Empty(t, imageCaption("   "))
}

func TestImageGenerationErrorMessage(t *testing.T) {
	sexual := &agent.ContentBlockedError{
		Reason:   "flagged for sexual or adult content",
		Category: agent.BlockedSexual,
	}
	require.Equal(t, "Blocked by the image provider: flagged for sexual or adult content", imageGenerationErrorMessage(sexual))

	other := &agent.ContentBlockedError{
		Reason:   "violated the content policy",
		Category: agent.BlockedOther,
	}
	require.Equal(t, "Blocked by the image provider: violated the content policy", imageGenerationErrorMessage(other))

	provider := &agent.ProviderError{Reason: "submit returned 422", StatusCode: 422}
	require.Equal(t, "The image provider rejected the request: submit returned 422", imageGenerationErrorMessage(provider))

	require.Equal(t, "Image generation failed. Try again in a bit.", imageGenerationErrorMessage(errors.New("boom")))
}

func TestImageGenerationRateLimitMessage(t *testing.T) {
	rateErr := &agent.RateLimitError{
		Reason:     "You have too many active tasks.",
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: 45 * time.Second,
	}

	message := imageGenerationErrorMessage(rateErr)
	require.Contains(t, message, "rate limited")
	require.Contains(t, message, "<t:")
	require.Contains(t, message, "too many active tasks")
}

func TestImageGenerationRateLimitMessageWithoutHint(t *testing.T) {
	rateErr := &agent.RateLimitError{
		Reason:     "You have too many active tasks.",
		StatusCode: http.StatusTooManyRequests,
	}

	require.Contains(t, imageGenerationErrorMessage(rateErr), "<t:")
}

func TestWithImagePrompt(t *testing.T) {
	got := withImagePrompt("it broke", "a duck")
	require.Equal(t, "it broke\n\nTry again with:\n/imagine prompt:a duck", got)

	got = withImagePrompt("it broke", "line one\nline two")
	require.Contains(t, got, "/imagine prompt:line one line two")

	require.Equal(t, "it broke", withImagePrompt("it broke", "   "))
}

func TestDiscordRetryTimestamp(t *testing.T) {
	timestamp := discordRetryTimestamp(0)
	require.Regexp(t, `^<t:\d+:R>$`, timestamp)
}

func TestResolveImageSafetyTolerance(t *testing.T) {
	require.Equal(t, 2, resolveImageSafetyTolerance(nil, 2))

	requested := 4
	require.Equal(t, 4, resolveImageSafetyTolerance(&requested, 2))

	tooHigh := 9
	require.Equal(t, 4, resolveImageSafetyTolerance(&tooHigh, 2))

	tooLow := -1
	require.Equal(t, 0, resolveImageSafetyTolerance(&tooLow, 2))

	defaultTooHigh := 9
	require.Equal(t, 4, resolveImageSafetyTolerance(nil, defaultTooHigh))
}

func TestImageExtension(t *testing.T) {
	require.Equal(t, ".jpg", imageExtension("image/jpeg"))
	require.Equal(t, ".webp", imageExtension("image/webp"))
	require.Equal(t, ".gif", imageExtension("image/gif"))
	require.Equal(t, ".svg", imageExtension("image/svg+xml"))
	require.Equal(t, ".png", imageExtension("image/png"))
	require.Equal(t, ".png", imageExtension(""))
}
