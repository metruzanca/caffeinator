package agent

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestMemeStore(t *testing.T, now time.Time) *MemeStore {
	t.Helper()

	store, err := NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)
	store.now = func() time.Time { return now }
	return store
}

func testMeme(id string, added time.Time) Meme {
	return Meme{
		ID:        id,
		Subreddit: "ProgrammerHumor",
		Title:     "Build passed",
		Caption:   "A programmer looks surprised at a green build.",
		Context:   "It jokes about clean first builds.",
		Tags:      []string{"build", "ci", "go"},
		UseWhen:   []string{"someone celebrates a passing build"},
		Link:      "https://www.reddit.com/r/ProgrammerHumor/comments/" + id,
		ImageFile: id + ".jpg",
		AddedAt:   added,
	}
}

func TestMemeStoreAddAndReload(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "memes.json")

	store, err := NewMemeStore(path)
	require.NoError(t, err)
	store.now = func() time.Time { return now }

	added, err := store.Add(testMeme("abc", now))
	require.NoError(t, err)
	require.True(t, added)

	added, err = store.Add(testMeme("abc", now))
	require.NoError(t, err)
	require.False(t, added)

	reloaded, err := NewMemeStore(path)
	require.NoError(t, err)
	require.Len(t, reloaded.List(), 1)
	require.Equal(t, "Build passed", reloaded.List()[0].Title)
}

func TestMemeStoreAddAllDeduplicates(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	store := newTestMemeStore(t, now)

	added, err := store.AddAll([]Meme{
		testMeme("one", now),
		testMeme("one", now),
		testMeme("two", now),
	})
	require.NoError(t, err)
	require.Equal(t, 2, added)
	require.Len(t, store.List(), 2)
}

func TestMemeStoreRelevant(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	store := newTestMemeStore(t, now)

	build := testMeme("build", now)
	build.Tags = []string{"build", "ci"}
	build.UseWhen = []string{"someone talks about a failing pipeline"}

	database := testMeme("db", now.Add(-48*time.Hour))
	database.Title = "SQL joins"
	database.Caption = "A developer joins three tables and regrets it."
	database.Tags = []string{"sql", "database"}
	database.UseWhen = []string{"someone mentions database queries"}

	cpp := testMeme("cpp", now)
	cpp.Title = "C++ templates"
	cpp.Caption = "A developer stares at a template error."
	cpp.Tags = []string{"c++", "templates"}
	cpp.UseWhen = []string{"someone complains about C++ templates"}

	_, err := store.AddAll([]Meme{build, database, cpp})
	require.NoError(t, err)

	results := store.Relevant("my CI build keeps failing in the pipeline", 3)
	require.NotEmpty(t, results)
	require.Equal(t, "build", results[0].ID)

	results = store.Relevant("how do I write a SQL join", 3)
	require.NotEmpty(t, results)
	require.Equal(t, "db", results[0].ID)

	results = store.Relevant("why are c++ templates so hard", 3)
	require.NotEmpty(t, results)
	require.Equal(t, "cpp", results[0].ID)

	require.Empty(t, store.Relevant("the weather is nice today", 3))
}

func TestMemeStoreReactedMemeGetsRelevanceBoost(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	store := newTestMemeStore(t, now)

	plain := testMeme("plain", now.Add(-24*time.Hour))
	plain.Tags = []string{"build"}
	plain.UseWhen = nil

	liked := testMeme("liked", now.Add(-24*time.Hour))
	liked.Tags = []string{"build"}
	liked.UseWhen = nil
	liked.ReactorIDs = []string{"1", "2", "3"}
	liked.Reactions = 3

	_, err := store.AddAll([]Meme{plain, liked})
	require.NoError(t, err)

	results := store.Relevant("build failed", 2)
	require.Len(t, results, 2)
	require.Equal(t, "liked", results[0].ID)
}

func TestMemeStoreExtractMemeMarkers(t *testing.T) {
	selected := []Meme{{ID: "abc123"}, {ID: "def456"}}

	text, used := ExtractMemeMarkers("That is true. [meme:abc123]", selected)
	require.Equal(t, "That is true.", text)
	require.Len(t, used, 1)
	require.Equal(t, "abc123", used[0].ID)

	text, used = ExtractMemeMarkers("[meme:unknown] still here", selected)
	require.Equal(t, "still here", text)
	require.Empty(t, used)

	text, used = ExtractMemeMarkers("[meme:<id>] placeholder", selected)
	require.Equal(t, "placeholder", text)
	require.Empty(t, used)

	text, used = ExtractMemeMarkers("[meme:abc123] [meme:def456]", selected)
	require.Equal(t, "", text)
	require.Len(t, used, 1)
	require.Equal(t, "abc123", used[0].ID)
}

func TestMemeStoreSharesAndReactions(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	store := newTestMemeStore(t, now)

	_, err := store.Add(testMeme("abc", now))
	require.NoError(t, err)

	require.NoError(t, store.MarkShared("abc", "channel", "message-1"))

	meme, ok := store.FindByMessage("message-1")
	require.True(t, ok)
	require.Equal(t, "abc", meme.ID)

	meme, counted, err := store.AddReaction("abc", "user-1")
	require.NoError(t, err)
	require.True(t, counted)
	require.Equal(t, 1, meme.Reactions)

	_, counted, err = store.AddReaction("abc", "user-1")
	require.NoError(t, err)
	require.False(t, counted)

	meme, counted, err = store.AddReaction("abc", "user-2")
	require.NoError(t, err)
	require.True(t, counted)
	require.Equal(t, 2, meme.Reactions)

	require.NoError(t, store.MarkUsed([]string{"abc"}))
	meme, ok = store.FindByMessage("message-1")
	require.True(t, ok)
	require.Equal(t, 1, meme.Uses)
	require.Equal(t, now, meme.LastUsed)
}

func TestMemeStorePruneRetiresUnreactedOldMemes(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	store := newTestMemeStore(t, now)

	old := testMeme("old", now.Add(-40*24*time.Hour))
	oldReaction := testMeme("old-liked", now.Add(-40*24*time.Hour))
	oldReaction.ReactorIDs = []string{"1"}
	oldReaction.Reactions = 1
	oldUsed := testMeme("old-used", now.Add(-40*24*time.Hour))
	oldUsed.Uses = 1
	fresh := testMeme("fresh", now.Add(-2*24*time.Hour))

	_, err := store.AddAll([]Meme{old, oldReaction, oldUsed, fresh})
	require.NoError(t, err)

	removed, err := store.Prune(30, 50)
	require.NoError(t, err)
	require.Len(t, removed, 1)
	require.Equal(t, "old", removed[0].ID)

	memes := store.List()
	require.Len(t, memes, 3)
	require.True(t, store.Contains("old-liked"))
	require.True(t, store.Contains("old-used"))
	require.True(t, store.Contains("fresh"))
}

func TestMemeStorePruneEvictsUnreactedFirst(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	store := newTestMemeStore(t, now)

	plain := testMeme("plain", now.Add(-3*time.Hour))
	liked := testMeme("liked", now.Add(-48*time.Hour))
	liked.ReactorIDs = []string{"1"}
	liked.Reactions = 1

	_, err := store.AddAll([]Meme{plain, liked})
	require.NoError(t, err)

	removed, err := store.Prune(30, 1)
	require.NoError(t, err)
	require.Len(t, removed, 1)
	require.Equal(t, "plain", removed[0].ID)
	require.True(t, store.Contains("liked"))
}

func TestMemeStoreRecordRun(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	store := newTestMemeStore(t, now)

	require.NoError(t, store.RecordRun("reddit is down"))
	status := store.Status()
	require.Equal(t, now, status.LastRun)
	require.Equal(t, "reddit is down", status.LastError)
}
