package memory

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStoreAppendAndRead(t *testing.T) {
	store, err := NewStore(t.TempDir(), 1000)
	require.NoError(t, err)

	err = store.AppendFacts([]Fact{
		{
			SubjectID:   "123",
			SubjectName: "Alice",
			Text:        "Building a Go Discord bot.",
			Category:    "work",
			Confidence:  0.9,
		},
		{
			SubjectID:   "server",
			SubjectName: "server",
			Text:        "The server is for developers.",
			Category:    "server",
			Confidence:  0.9,
		},
	})
	require.NoError(t, err)

	userText, err := store.Get("123")
	require.NoError(t, err)
	require.Contains(t, userText, "# Alice (123)")
	require.Contains(t, userText, "Building a Go Discord bot.")

	serverText, err := store.Get("server")
	require.NoError(t, err)
	require.Contains(t, serverText, "# Server")
	require.Contains(t, serverText, "The server is for developers.")

	combined := store.ForSubjects(map[string]string{
		"server": "server",
		"123":    "Alice",
	})
	require.Contains(t, combined, "Server memory:")
	require.Contains(t, combined, "Memory for Alice (123):")
	require.Contains(t, combined, "Building a Go Discord bot.")

	users, err := store.ListUsers()
	require.NoError(t, err)
	require.Equal(t, []string{"123"}, users)
}

func TestStoreDeduplicatesFacts(t *testing.T) {
	store, err := NewStore(t.TempDir(), 1000)
	require.NoError(t, err)

	fact := Fact{
		SubjectID:   "123",
		SubjectName: "Alice",
		Text:        "Building a Go Discord bot.",
		Category:    "work",
		Confidence:  0.9,
	}
	require.NoError(t, store.AppendFacts([]Fact{fact}))
	require.NoError(t, store.AppendFacts([]Fact{fact}))

	text, err := store.Get("123")
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(text, "Building a Go Discord bot."))
}

func TestStoreForget(t *testing.T) {
	store, err := NewStore(t.TempDir(), 1000)
	require.NoError(t, err)
	require.NoError(t, store.AppendFacts([]Fact{{
		SubjectID:   "123",
		SubjectName: "Alice",
		Text:        "Building a Go Discord bot.",
		Category:    "work",
		Confidence:  0.9,
	}}))

	require.NoError(t, store.Forget("123"))

	text, err := store.Get("123")
	require.NoError(t, err)
	require.Empty(t, text)

	users, err := store.ListUsers()
	require.NoError(t, err)
	require.Empty(t, users)
}
