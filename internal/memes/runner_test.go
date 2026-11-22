package memes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/metruzanca/caffeinator/agent"
	"github.com/stretchr/testify/require"
)

type fakePostSource struct {
	posts map[string][]Post
	err   error
}

func (f *fakePostSource) Top(ctx context.Context, subreddit, period string, limit int) ([]Post, error) {
	if f.err != nil {
		return nil, f.err
	}
	posts := f.posts[subreddit]
	if len(posts) > limit {
		posts = posts[:limit]
	}
	return posts, nil
}

type fakeImageSource struct {
	image Image
	err   error
	calls int
}

func (f *fakeImageSource) Fetch(ctx context.Context, rawURL string) (Image, error) {
	f.calls++
	if f.err != nil {
		return Image{}, f.err
	}
	return f.image, nil
}

type fakeAnalyzer struct {
	analysis Analysis
	err      error
	calls    int
}

func (f *fakeAnalyzer) Analyze(ctx context.Context, post Post, image Image) (Analysis, error) {
	f.calls++
	if f.err != nil {
		return Analysis{}, f.err
	}
	return f.analysis, nil
}

func newTestRunner(t *testing.T, now time.Time, posts []Post, store *agent.MemeStore, dir string) (*Runner, *fakeImageSource, *fakeAnalyzer) {
	t.Helper()

	images := &fakeImageSource{image: Image{Data: onePixelPNG, MediaType: "image/png", Ext: ".png"}}
	analyzer := &fakeAnalyzer{analysis: Analysis{
		Caption: "A green build.",
		Context: "It is a rare clean build.",
		Tags:    []string{"build", "ci"},
		UseWhen: []string{"someone celebrates a passing build"},
	}}
	runner := NewRunner(RunnerConfig{
		Reddit:        &fakePostSource{posts: map[string][]Post{"programmerhumor": posts}},
		Images:        images,
		Analyzer:      analyzer,
		Store:         store,
		Dir:           dir,
		Subreddits:    []string{"ProgrammerHumor"},
		Period:        "week",
		ScanLimit:     10,
		PostsPerRun:   5,
		Keep:          10,
		RetentionDays: 30,
	})
	runner.now = func() time.Time { return now }
	return runner, images, analyzer
}

func TestRunnerStoresNewMemesAndSkipsKnown(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	store, err := agent.NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)

	added, err := store.Add(agent.Meme{
		ID:        "known",
		Title:     "Known meme",
		Caption:   "Known",
		ImageFile: "known.jpg",
		AddedAt:   now,
	})
	require.NoError(t, err)
	require.True(t, added)

	dir := filepath.Join(t.TempDir(), "images")
	posts := []Post{
		{ID: "known", Subreddit: "programmerhumor", Title: "Known", ImageURL: "https://i.redd.it/known.jpg"},
		{ID: "fresh", Subreddit: "programmerhumor", Title: "Fresh", ImageURL: "https://i.redd.it/fresh.png", Score: 42},
	}
	runner, _, _ := newTestRunner(t, now, posts, store, dir)

	result, err := runner.Refresh(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, result.Added)
	require.Equal(t, 1, result.Skipped)
	require.Equal(t, 0, result.Failed)

	require.True(t, store.Contains("fresh"))
	memes := store.List()
	var fresh agent.Meme
	for _, meme := range memes {
		if meme.ID == "fresh" {
			fresh = meme
		}
	}
	require.Equal(t, "A green build.", fresh.Caption)
	require.Equal(t, "fresh.png", fresh.ImageFile)
	require.Equal(t, 42, fresh.Score)

	_, err = os.Stat(filepath.Join(dir, "fresh.png"))
	require.NoError(t, err)

	status := store.Status()
	require.False(t, status.LastRun.IsZero())
	require.Empty(t, status.LastError)
}

func TestRunnerRecordsFailures(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	store, err := agent.NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)

	posts := []Post{{ID: "fresh", Title: "Fresh", ImageURL: "https://i.redd.it/fresh.png"}}
	runner, images, _ := newTestRunner(t, now, posts, store, filepath.Join(t.TempDir(), "images"))
	images.err = errors.New("image is gone")

	result, err := runner.Refresh(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, result.Failed)

	status := store.Status()
	require.Contains(t, status.LastError, "image is gone")
}

func TestRunnerBusy(t *testing.T) {
	now := time.Now()
	store, err := agent.NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)
	runner, _, _ := newTestRunner(t, now, nil, store, t.TempDir())

	runner.mu.Lock()
	_, err = runner.Refresh(context.Background())
	runner.mu.Unlock()
	require.ErrorIs(t, err, ErrBusy)
}

func TestRunnerPrunesOldMemesAndImages(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	store, err := agent.NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "images")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "old.png"), onePixelPNG, 0600))

	added, err := store.Add(agent.Meme{
		ID:        "old",
		Title:     "Old meme",
		Caption:   "Old",
		ImageFile: "old.png",
		AddedAt:   now.Add(-40 * 24 * time.Hour),
	})
	require.NoError(t, err)
	require.True(t, added)

	runner, _, _ := newTestRunner(t, now, nil, store, dir)
	result, err := runner.Refresh(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, result.Pruned)
	require.False(t, store.Contains("old"))

	_, err = os.Stat(filepath.Join(dir, "old.png"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestFormatResult(t *testing.T) {
	require.Equal(t, "Added 1 new meme.", FormatResult(Result{Added: 1}))
	require.Equal(t, "Added 2 new memes. Retired 1 old meme.", FormatResult(Result{Added: 2, Pruned: 1}))
}

func TestRunnerRequiresSubreddits(t *testing.T) {
	store, err := agent.NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)
	runner := NewRunner(RunnerConfig{Store: store})
	_, err = runner.Refresh(context.Background())
	require.Error(t, err)
}
