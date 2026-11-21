package commands

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
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

func TestResolveImageSafetyTolerance(t *testing.T) {
	requested := 5

	value, err := resolveImageSafetyTolerance(false, nil, 2, 5)
	require.NoError(t, err)
	require.Equal(t, 2, value)

	value, err = resolveImageSafetyTolerance(true, nil, 2, 5)
	require.NoError(t, err)
	require.Equal(t, 5, value)

	value, err = resolveImageSafetyTolerance(true, &requested, 2, 5)
	require.NoError(t, err)
	require.Equal(t, 5, value)

	_, err = resolveImageSafetyTolerance(false, &requested, 2, 5)
	require.Error(t, err)

	tooHigh := 9
	value, err = resolveImageSafetyTolerance(true, &tooHigh, 2, 5)
	require.NoError(t, err)
	require.Equal(t, 5, value)

	tooLow := -1
	value, err = resolveImageSafetyTolerance(true, &tooLow, 2, 5)
	require.NoError(t, err)
	require.Equal(t, 0, value)
}

func TestImageExtension(t *testing.T) {
	require.Equal(t, ".jpg", imageExtension("image/jpeg"))
	require.Equal(t, ".webp", imageExtension("image/webp"))
	require.Equal(t, ".gif", imageExtension("image/gif"))
	require.Equal(t, ".svg", imageExtension("image/svg+xml"))
	require.Equal(t, ".png", imageExtension("image/png"))
	require.Equal(t, ".png", imageExtension(""))
}
