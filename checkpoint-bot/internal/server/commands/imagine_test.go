package commands

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/metruzanca/checkpoint-bot/internal/imagegen"
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
	sexual := &imagegen.ContentBlockedError{
		Reason:   "flagged for sexual or adult content",
		Category: imagegen.BlockedSexual,
	}
	require.Equal(t, "That's a bit too lewd for my liking. This is a Christian server.", imageGenerationErrorMessage(sexual, false))
	require.Contains(t, imageGenerationErrorMessage(sexual, true), "flagged for sexual or adult content")

	other := &imagegen.ContentBlockedError{
		Reason:   "violated the content policy",
		Category: imagegen.BlockedOther,
	}
	require.Equal(t, "The image provider blocked that prompt.", imageGenerationErrorMessage(other, false))
	require.Contains(t, imageGenerationErrorMessage(other, true), "violated the content policy")

	provider := &imagegen.ProviderError{Reason: "submit returned 422", StatusCode: 422}
	require.Equal(t, "Image generation failed. Try again in a bit.", imageGenerationErrorMessage(provider, false))
	require.Contains(t, imageGenerationErrorMessage(provider, true), "submit returned 422")

	require.Equal(t, "Image generation failed. Try again in a bit.", imageGenerationErrorMessage(errors.New("boom"), false))
}

func TestResolveImageSafetyTolerance(t *testing.T) {
	requested := 5

	value, err := resolveImageSafetyTolerance(false, nil, 2, 5, 4)
	require.NoError(t, err)
	require.Equal(t, 2, value)

	value, err = resolveImageSafetyTolerance(true, nil, 2, 5, 4)
	require.NoError(t, err)
	require.Equal(t, 4, value)

	value, err = resolveImageSafetyTolerance(true, &requested, 2, 5, 4)
	require.NoError(t, err)
	require.Equal(t, 4, value)

	_, err = resolveImageSafetyTolerance(false, &requested, 2, 5, 4)
	require.Error(t, err)

	tooHigh := 9
	value, err = resolveImageSafetyTolerance(true, &tooHigh, 2, 5, 4)
	require.NoError(t, err)
	require.Equal(t, 4, value)

	tooLow := -1
	value, err = resolveImageSafetyTolerance(true, &tooLow, 2, 5, 4)
	require.NoError(t, err)
	require.Equal(t, 0, value)

	value, err = resolveImageSafetyTolerance(true, &requested, 2, 5, 5)
	require.NoError(t, err)
	require.Equal(t, 5, value)
}

func TestMaxSafetyToleranceForModel(t *testing.T) {
	require.Equal(t, 4, maxSafetyToleranceForModel("black-forest-labs/flux-3-image-klein"))
	require.Equal(t, 4, maxSafetyToleranceForModel("black-forest-labs/flux-3-image"))
	require.Equal(t, 5, maxSafetyToleranceForModel("black-forest-labs/flux.2-klein-4b"))
	require.Equal(t, 5, maxSafetyToleranceForModel("some-other-model"))
}

func TestModelSupportsResolution(t *testing.T) {
	require.True(t, modelSupportsResolution("black-forest-labs/flux-3-image-klein"))
	require.False(t, modelSupportsResolution("black-forest-labs/flux.2-klein-4b"))
	require.True(t, modelSupportsResolution("bytedance-seed/seedream-5-0-flash"))
	require.False(t, modelSupportsResolution("some-other-model"))
}

func TestImageExtension(t *testing.T) {
	require.Equal(t, ".jpg", imageExtension("image/jpeg"))
	require.Equal(t, ".webp", imageExtension("image/webp"))
	require.Equal(t, ".gif", imageExtension("image/gif"))
	require.Equal(t, ".svg", imageExtension("image/svg+xml"))
	require.Equal(t, ".png", imageExtension("image/png"))
	require.Equal(t, ".png", imageExtension(""))
}
