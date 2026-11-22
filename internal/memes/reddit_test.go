package memes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const listingFixture = `<!doctype html>
<html><body>
<div class="thing link" data-fullname="t3_abc123" data-permalink="/r/ProgrammerHumor/comments/abc123/build_passed/" data-url="https://i.redd.it/abc123.jpg" data-author="alice" data-timestamp="1763700000">
  <p class="title"><a class="title may-blank" href="/r/ProgrammerHumor/comments/abc123/build_passed/">When the build passes</a></p>
  <div class="score unvoted" title="1,234">1,234</div>
  <time datetime="2026-11-20T10:00:00+00:00"></time>
</div>
<div class="thing link" data-fullname="t3_gallery1" data-permalink="/r/ProgrammerHumor/comments/gallery1/two_panel/" data-url="https://www.reddit.com/gallery/gallery1">
  <p class="title"><a class="title" href="/r/ProgrammerHumor/comments/gallery1/two_panel/">Two panel meme</a></p>
  <a class="thumbnail" href="https://www.reddit.com/gallery/gallery1"><img src="https://preview.redd.it/gallery1.jpg?width=140&amp;crop=1:1"></a>
  <div class="score unvoted" title="99">99</div>
</div>
<div class="thing self" data-fullname="t3_self1" data-permalink="/r/ProgrammerHumor/comments/self1/text/" data-url="https://www.reddit.com/r/ProgrammerHumor/comments/self1/text/">
  <p class="title"><a class="title" href="/r/ProgrammerHumor/comments/self1/text/">A text post</a></p>
</div>
<div class="thing link" data-promoted="true" data-fullname="t3_ad1" data-url="https://i.redd.it/ad1.jpg">
  <p class="title"><a class="title" href="/ad">An advertisement</a></p>
</div>
<div class="thing link over18" data-fullname="t3_nsfw1" data-nsfw="true" data-permalink="/r/ProgrammerHumor/comments/nsfw1/nsfw/" data-url="https://i.redd.it/nsfw1.jpg">
  <p class="title"><a class="title" href="/r/ProgrammerHumor/comments/nsfw1/nsfw/">An NSFW post</a></p>
</div>
<div class="thing link" data-fullname="t3_link1" data-permalink="/r/ProgrammerHumor/comments/link1/article/" data-url="https://example.com/article">
  <p class="title"><a class="title" href="/r/ProgrammerHumor/comments/link1/article/">A link post</a></p>
</div>
</body></html>`

func TestParseTopListing(t *testing.T) {
	posts, err := parseTopListing([]byte(listingFixture), 10, false)
	require.NoError(t, err)
	require.Len(t, posts, 2)

	first := posts[0]
	require.Equal(t, "abc123", first.ID)
	require.Equal(t, "When the build passes", first.Title)
	require.Equal(t, "alice", first.Author)
	require.Equal(t, "https://www.reddit.com/r/ProgrammerHumor/comments/abc123/build_passed/", first.Permalink)
	require.Equal(t, "https://i.redd.it/abc123.jpg", first.ImageURL)
	require.Equal(t, 1234, first.Score)
	require.Equal(t, time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC), first.CreatedAt)

	second := posts[1]
	require.Equal(t, "gallery1", second.ID)
	require.Equal(t, "https://i.redd.it/gallery1.jpg", second.ImageURL)
	require.Equal(t, 99, second.Score)
}

func TestParseTopListingIncludesNSFWWhenEnabled(t *testing.T) {
	posts, err := parseTopListing([]byte(listingFixture), 10, true)
	require.NoError(t, err)
	require.Len(t, posts, 3)
	require.Equal(t, "nsfw1", posts[2].ID)
	require.True(t, posts[2].NSFW)
}

func TestParseTopListingHonorsLimit(t *testing.T) {
	posts, err := parseTopListing([]byte(listingFixture), 1, false)
	require.NoError(t, err)
	require.Len(t, posts, 1)
}

func TestRedditTopFetchesWithSession(t *testing.T) {
	var gotPath string
	var gotCookie string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotCookie = r.Header.Get("Cookie")
		_, _ = w.Write([]byte(listingFixture))
	}))
	defer server.Close()

	client := NewRedditClient(RedditConfig{
		BaseURL:   server.URL,
		Session:   "secret",
		UserAgent: "test-agent",
		MinGap:    0,
	})

	posts, err := client.Top(context.Background(), "ProgrammerHumor", "week", 5)
	require.NoError(t, err)
	require.Len(t, posts, 2)
	require.Equal(t, "/r/programmerhumor/top/?t=week&limit=5", gotPath)
	require.Contains(t, gotCookie, "reddit_session=secret")
	require.Equal(t, "programmerhumor", posts[0].Subreddit)
}

func TestRedditTopRequiresSession(t *testing.T) {
	client := NewRedditClient(RedditConfig{MinGap: 0})
	_, err := client.Top(context.Background(), "ProgrammerHumor", "week", 5)
	require.ErrorIs(t, err, ErrRedditSession)
}

func TestRedditTopDetectsLoginRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/login") {
			_, _ = w.Write([]byte(`<html><body><form id="login-form"></form></body></html>`))
			return
		}
		http.Redirect(w, r, "/login/?reason=lor2", http.StatusFound)
	}))
	defer server.Close()

	client := NewRedditClient(RedditConfig{
		BaseURL: server.URL,
		Session: "secret",
		MinGap:  0,
	})

	_, err := client.Top(context.Background(), "ProgrammerHumor", "week", 5)
	require.ErrorIs(t, err, ErrRedditSession)
}

func TestRedditTopHandlesRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewRedditClient(RedditConfig{
		BaseURL: server.URL,
		Session: "secret",
		MinGap:  0,
	})

	_, err := client.Top(context.Background(), "ProgrammerHumor", "week", 5)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRedditSession)
}

func TestRedditCookieAcceptsFullHeader(t *testing.T) {
	client := NewRedditClient(RedditConfig{Session: "reddit_session=abc; other=1", IncludeNSFW: true})
	require.Contains(t, client.cookie(), "reddit_session=abc")
	require.Contains(t, client.cookie(), "over18=1")
}
