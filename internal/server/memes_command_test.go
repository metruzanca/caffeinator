package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/metruzanca/caffeinator/agent"
	"github.com/metruzanca/caffeinator/internal/memes"
	"github.com/stretchr/testify/require"
)

func testMemeCommandService(t *testing.T, store *agent.MemeStore) memeCommandService {
	t.Helper()

	runner := memes.NewRunner(memes.RunnerConfig{
		Store:         store,
		Subreddits:    []string{"ProgrammerHumor"},
		Keep:          50,
		RetentionDays: 30,
		PostsPerRun:   5,
	})
	return memeCommandService{runtime: &memesRuntime{
		Store:       store,
		Runner:      runner,
		Subreddits:  runner.Subreddits(),
		Expression:  "0 15 * * 1,3,5",
		Timezone:    "UTC",
		Keep:        runner.Keep(),
		Retention:   runner.RetentionDays(),
		PostsPerRun: runner.PostsPerRun(),
	}}
}

func TestMemeCommandServiceStatusAndList(t *testing.T) {
	store, err := agent.NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)
	_, err = store.Add(agent.Meme{
		ID:        "abc",
		Subreddit: "ProgrammerHumor",
		Title:     "When the build passes",
		Caption:   "A programmer looks surprised.",
		Context:   "It is a rare clean build.",
		Tags:      []string{"build", "ci"},
		Link:      "https://www.reddit.com/r/ProgrammerHumor/comments/abc",
		AddedAt:   time.Now(),
	})
	require.NoError(t, err)

	service := testMemeCommandService(t, store)

	status := service.StatusText()
	require.Contains(t, status, "Meme memory is enabled.")
	require.Contains(t, status, "Subreddits: programmerhumor")
	require.Contains(t, status, "Schedule: `0 15 * * 1,3,5`")
	require.Contains(t, status, "Stored: 1/50")
	require.Contains(t, status, "Reddit session: missing")

	list := service.ListText(5)
	require.Contains(t, list, "When the build passes")
	require.Contains(t, list, "A programmer looks surprised.")
	require.Contains(t, list, "https://www.reddit.com/r/ProgrammerHumor/comments/abc")
}

func TestMemeCommandServiceRefreshWithoutClients(t *testing.T) {
	store, err := agent.NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)
	service := testMemeCommandService(t, store)

	_, err = service.Refresh(context.Background())
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "reddit is not configured"))
}

func TestMemeCommandServiceListEmpty(t *testing.T) {
	store, err := agent.NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)
	service := testMemeCommandService(t, store)
	require.Equal(t, "No memes stored yet.", service.ListText(5))
}
