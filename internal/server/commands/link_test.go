package commands

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteLink(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "x with tracking query",
			in:   "https://x.com/4ster_light/status/2116158570198622382?s=20&t=abc",
			want: "https://fxtwitter.com/4ster_light/status/2116158570198622382",
		},
		{
			name: "twitter",
			in:   "https://twitter.com/example/status/123",
			want: "https://fxtwitter.com/example/status/123",
		},
		{
			name: "mobile x",
			in:   "https://mobile.x.com/example/status/123",
			want: "https://fxtwitter.com/example/status/123",
		},
		{
			name: "www twitter",
			in:   "https://www.twitter.com/example/status/123",
			want: "https://fxtwitter.com/example/status/123",
		},
		{
			name: "instagram post with query",
			in:   "https://www.instagram.com/p/ABC123/?igsh=tracking",
			want: "https://kkinstagram.com/p/ABC123",
		},
		{
			name: "instagram reel with fragment",
			in:   "https://instagram.com/reel/XYZ789/#comments",
			want: "https://kkinstagram.com/reel/XYZ789",
		},
		{
			name: "mobile instagram",
			in:   "https://m.instagram.com/p/ABC123/",
			want: "https://kkinstagram.com/p/ABC123",
		},
		{
			name: "missing scheme",
			in:   "x.com/example/status/123",
			want: "https://fxtwitter.com/example/status/123",
		},
		{
			name: "already fixed",
			in:   "https://fxtwitter.com/example/status/123?t=abc",
			want: "https://fxtwitter.com/example/status/123",
		},
		{
			name: "already fixed with www",
			in:   "https://www.fxtwitter.com/example/status/123",
			want: "https://fxtwitter.com/example/status/123",
		},
		{
			name: "trailing slash",
			in:   "https://x.com/example/status/123/",
			want: "https://fxtwitter.com/example/status/123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RewriteLink(tt.in)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRewriteLinkErrors(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		_, err := RewriteLink("  ")
		require.Error(t, err)
	})

	t.Run("unsupported host", func(t *testing.T) {
		_, err := RewriteLink("https://example.com/post/123")
		require.ErrorIs(t, err, ErrUnsupportedLink)
	})

	t.Run("unsupported scheme", func(t *testing.T) {
		_, err := RewriteLink("ftp://x.com/example/status/123")
		require.Error(t, err)
	})

	t.Run("invalid host", func(t *testing.T) {
		_, err := RewriteLink("https://")
		require.Error(t, err)
	})
}

func TestRewriteLinkUnsupportedHostMessage(t *testing.T) {
	_, err := RewriteLink("https://example.com")
	require.True(t, errors.Is(err, ErrUnsupportedLink))
}
