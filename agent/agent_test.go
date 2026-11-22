package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeCompleter struct {
	response     string
	err          error
	calls        int
	got          []Message
	systemPrompt string
}

func (f *fakeCompleter) Chat(ctx context.Context, messages []Message) (string, Usage, error) {
	return f.ChatWithSystem(ctx, "", messages)
}

func (f *fakeCompleter) ChatWithSystem(ctx context.Context, systemPrompt string, messages []Message) (string, Usage, error) {
	f.calls++
	f.got = messages
	f.systemPrompt = systemPrompt
	if f.err != nil {
		return "", Usage{}, f.err
	}
	return f.response, Usage{PromptTokens: 10, CompletionTokens: 5}, nil
}

func testTurn(id, authorID, content string) Turn {
	return Turn{
		ID:      id,
		Author:  User{ID: authorID, Name: authorID},
		Content: content,
	}
}

func TestBuildMessagesRolesAndFormatting(t *testing.T) {
	a := New(Config{
		Client:             &fakeCompleter{},
		MaxCharsPerMessage: 100,
		MaxContextChars:    1000,
	})
	req := Request{
		Self:   User{ID: "bot"},
		User:   User{ID: "bob", Name: "bob"},
		Prompt: "what now",
		History: []Turn{
			testTurn("1", "alice", "hello"),
			testTurn("2", "bot", "hi alice"),
		},
	}

	messages := a.buildMessages(req)

	require.Len(t, messages, 3)
	require.Equal(t, "user", messages[0].Role)
	require.Contains(t, messages[0].Content, "alice: hello")
	require.Equal(t, "assistant", messages[1].Role)
	require.Equal(t, "hi alice", messages[1].Content)
	require.Equal(t, "user", messages[2].Role)
	require.Contains(t, messages[2].Content, "bob: what now")
	for _, message := range messages {
		require.NotEqual(t, "system", message.Role)
	}
}

func TestBuildMessagesDropsOldestContext(t *testing.T) {
	a := New(Config{
		Client:             &fakeCompleter{},
		MaxCharsPerMessage: 1000,
		MaxContextChars:    60,
	})
	req := Request{
		Self:   User{ID: "bot"},
		User:   User{ID: "dave", Name: "dave"},
		Prompt: "latest",
		History: []Turn{
			testTurn("1", "alice", "first message is very long"),
			testTurn("2", "bob", "second message is very long"),
			testTurn("3", "carol", "third message is very long"),
		},
	}

	messages := a.buildMessages(req)

	require.NotEmpty(t, messages)
	require.Contains(t, messages[len(messages)-1].Content, "dave: latest")

	total := 0
	for _, message := range messages {
		total += len([]rune(message.Content))
	}
	require.LessOrEqual(t, total, 60)
}

func TestBuildMessagesIncludesTimestamps(t *testing.T) {
	a := New(Config{
		Client:             &fakeCompleter{},
		MaxCharsPerMessage: 200,
		MaxContextChars:    1000,
	})
	req := Request{
		Self:      User{ID: "bot"},
		User:      User{ID: "bob", Name: "bob"},
		Prompt:    "what now",
		Timestamp: time.Date(2026, 11, 21, 2, 25, 0, 0, time.UTC),
		History: []Turn{
			{
				ID:        "1",
				Author:    User{ID: "alice", Name: "alice"},
				Content:   "hello",
				Timestamp: time.Date(2026, 11, 21, 2, 24, 48, 0, time.UTC),
			},
			{
				ID:        "2",
				Author:    User{ID: "bot", Name: "bot"},
				Content:   "hi",
				Timestamp: time.Date(2026, 11, 21, 2, 24, 52, 0, time.UTC),
			},
		},
	}

	messages := a.buildMessages(req)

	require.Len(t, messages, 3)
	require.Contains(t, messages[0].Content, "alice [2026-11-21T02:24:48Z]: hello")
	require.Equal(t, "[2026-11-21T02:24:52Z]: hi", messages[1].Content)
	require.Contains(t, messages[2].Content, "bob [2026-11-21T02:25:00Z]: what now")
}

func TestAskAddsContextInstructionWhenHistoryExists(t *testing.T) {
	client := &fakeCompleter{response: "answer"}
	a := New(Config{
		Client:       client,
		SystemPrompt: "base prompt",
	})

	_, err := a.Ask(context.Background(), Request{
		Self:    User{ID: "bot"},
		User:    User{ID: "123", Name: "Alice"},
		Prompt:  "what now?",
		History: []Turn{testTurn("1", "carol", "old context")},
	})
	require.NoError(t, err)
	require.Contains(t, client.systemPrompt, "context only")
	require.Contains(t, client.systemPrompt, "Current Unix time:")
}

func TestAskSkipsContextInstructionWithoutHistory(t *testing.T) {
	client := &fakeCompleter{response: "answer"}
	a := New(Config{
		Client:       client,
		SystemPrompt: "base prompt",
	})

	_, err := a.Ask(context.Background(), Request{
		User:   User{ID: "123", Name: "Alice"},
		Prompt: "what now?",
	})
	require.NoError(t, err)
	require.NotContains(t, client.systemPrompt, "context only")
}

func TestAskInjectsMemory(t *testing.T) {
	store, err := NewStore(t.TempDir(), 1000)
	require.NoError(t, err)
	require.NoError(t, store.AppendFacts([]Fact{
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
	}))

	client := &fakeCompleter{response: "hello"}
	a := New(Config{
		Client:       client,
		SystemPrompt: "base prompt",
		Memory:       store,
	})

	_, err = a.Ask(context.Background(), Request{
		User:      User{ID: "123", Name: "Alice"},
		Prompt:    "what am I working on?",
		UseMemory: true,
	})
	require.NoError(t, err)
	require.Contains(t, client.systemPrompt, "base prompt")
	require.Contains(t, client.systemPrompt, "Building a Go Discord bot.")
	require.Contains(t, client.systemPrompt, "The server is for developers.")
}

func TestAskDisableMemorySkipsMemory(t *testing.T) {
	store, err := NewStore(t.TempDir(), 1000)
	require.NoError(t, err)
	require.NoError(t, store.AppendFacts([]Fact{{
		SubjectID:   "123",
		SubjectName: "Alice",
		Text:        "Building a Go Discord bot.",
		Category:    "work",
		Confidence:  0.9,
	}}))

	client := &fakeCompleter{response: "hello"}
	a := New(Config{
		Client:       client,
		SystemPrompt: "base prompt",
		Memory:       store,
	})

	_, err = a.Ask(context.Background(), Request{
		User:      User{ID: "123", Name: "Alice"},
		Prompt:    "what am I working on?",
		UseMemory: false,
	})
	require.NoError(t, err)
	require.NotContains(t, client.systemPrompt, "Building a Go Discord bot.")
}

func TestAskAttachesImages(t *testing.T) {
	client := &fakeCompleter{response: "looks good"}
	a := New(Config{Client: client})

	_, err := a.Ask(context.Background(), Request{
		User:   User{ID: "123", Name: "Alice"},
		Prompt: "what is this?",
		Images: []Image{{
			Label:  "Current message image: cat.png",
			URL:    "https://cdn.discordapp.com/cat.png",
			Detail: "auto",
		}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, client.got)
	last := client.got[len(client.got)-1]
	require.Len(t, last.Parts, 2)
	require.Equal(t, "text", last.Parts[0].Type)
	require.Contains(t, last.Parts[0].Text, "cat.png")
	require.Equal(t, "image_url", last.Parts[1].Type)
	require.Equal(t, "https://cdn.discordapp.com/cat.png", last.Parts[1].ImageURL.URL)
}

func TestCollectSubjectsIncludesHistoryAuthors(t *testing.T) {
	a := New(Config{Client: &fakeCompleter{}})
	req := Request{
		Self:     User{ID: "bot"},
		User:     User{ID: "111", Name: "alice"},
		Mentions: []User{{ID: "456", Name: "bob"}},
		History: []Turn{
			testTurn("1", "222", "hello"),
			{ID: "2", Author: User{ID: "999", Name: "other", Bot: true}},
		},
	}

	subjects := a.collectSubjects(req)

	require.Contains(t, subjects, "server")
	require.Contains(t, subjects, "111")
	require.Contains(t, subjects, "222")
	require.Contains(t, subjects, "456")
	require.NotContains(t, subjects, "999")
}

func TestLearnStoresFactsForSubject(t *testing.T) {
	store, err := NewStore(t.TempDir(), 1000)
	require.NoError(t, err)
	client := &fakeExtractionClient{response: `{
		"facts": [
			{"subject_id": "111", "fact": "I work on a Go bot.", "category": "work", "confidence": 0.9},
			{"subject_id": "222", "fact": "I am an AI doomer.", "category": "identity", "confidence": 0.9}
		]
	}`}
	a := New(Config{
		Client:    &fakeCompleter{},
		Memory:    store,
		Extractor: NewExtractor(client, 0.7),
	})

	turns := []Turn{
		testTurn("1", "111", "I work on a Go bot."),
		testTurn("2", "222", "I am an AI doomer."),
	}
	facts, err := a.Learn(context.Background(), turns, User{ID: "111", Name: "alice"}, false)
	require.NoError(t, err)
	require.Len(t, facts, 1)

	text, err := store.Get("111")
	require.NoError(t, err)
	require.Contains(t, text, "I work on a Go bot.")
	require.NotContains(t, client.got, "I am an AI doomer")
}

func TestLearnIncludesServerWhenAllowed(t *testing.T) {
	store, err := NewStore(t.TempDir(), 1000)
	require.NoError(t, err)
	client := &fakeExtractionClient{response: `{
		"facts": [
			{"subject_id": "server", "fact": "The server is for developers.", "category": "server", "confidence": 0.9}
		]
	}`}
	a := New(Config{
		Client:    &fakeCompleter{},
		Memory:    store,
		Extractor: NewExtractor(client, 0.7),
	})

	facts, err := a.Learn(
		context.Background(),
		[]Turn{testTurn("1", "111", "remember that this server is for developers")},
		User{ID: "111", Name: "alice"},
		true,
	)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Contains(t, client.got, "server: this Discord server")

	text, err := store.Get("server")
	require.NoError(t, err)
	require.Contains(t, text, "The server is for developers.")
}

func TestLearnSkipsBotsAndNonNumericSubjects(t *testing.T) {
	store, err := NewStore(t.TempDir(), 1000)
	require.NoError(t, err)
	client := &fakeExtractionClient{}
	a := New(Config{
		Client:    &fakeCompleter{},
		Memory:    store,
		Extractor: NewExtractor(client, 0.7),
	})

	facts, err := a.Learn(context.Background(), []Turn{testTurn("1", "alice", "hello")}, User{ID: "alice", Name: "alice"}, false)
	require.NoError(t, err)
	require.Empty(t, facts)
	require.False(t, strings.Contains(client.got, "hello"))
}

func TestAskInjectsRelevantMemesAndStripsMarker(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	store, err := NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)
	store.now = func() time.Time { return now }

	added, err := store.Add(Meme{
		ID:        "abc123",
		Title:     "When the build passes",
		Caption:   "A programmer looks surprised at a green build.",
		Context:   "It jokes about clean first builds.",
		Tags:      []string{"build", "ci"},
		UseWhen:   []string{"someone celebrates a passing build"},
		AddedAt:   now,
		ImageFile: "abc123.png",
	})
	require.NoError(t, err)
	require.True(t, added)

	client := &fakeCompleter{response: "That is a classic. [meme:abc123]"}
	a := New(Config{
		Client:       client,
		SystemPrompt: "base prompt",
		Memes:        store,
		MemeLimit:    3,
	})

	response, err := a.Ask(context.Background(), Request{
		Self:      User{ID: "bot"},
		User:      User{ID: "123", Name: "Alice"},
		Prompt:    "my build keeps failing",
		UseMemory: true,
	})
	require.NoError(t, err)
	require.Equal(t, "That is a classic.", response.Text)
	require.Len(t, response.Memes, 1)
	require.Equal(t, "abc123", response.Memes[0].ID)
	require.Equal(t, 1, response.Stats.MemeCandidates)
	require.Contains(t, client.systemPrompt, "Meme memory")
	require.Contains(t, client.systemPrompt, "abc123")

	memes := store.List()
	require.Len(t, memes, 1)
	require.Equal(t, 1, memes[0].Uses)
}

func TestAskSkipsMemesWhenMemoryDisabled(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	store, err := NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)
	store.now = func() time.Time { return now }
	_, err = store.Add(Meme{
		ID:      "abc123",
		Title:   "Build meme",
		Caption: "A build meme.",
		Tags:    []string{"build"},
		AddedAt: now,
	})
	require.NoError(t, err)

	client := &fakeCompleter{response: "plain answer"}
	a := New(Config{
		Client:       client,
		SystemPrompt: "base prompt",
		Memes:        store,
	})

	response, err := a.Ask(context.Background(), Request{
		Self:      User{ID: "bot"},
		User:      User{ID: "123", Name: "Alice"},
		Prompt:    "my build keeps failing",
		UseMemory: false,
	})
	require.NoError(t, err)
	require.Equal(t, "plain answer", response.Text)
	require.Empty(t, response.Memes)
	require.NotContains(t, client.systemPrompt, "Meme memory")
}

func TestAskStripsUnknownMemeMarkers(t *testing.T) {
	client := &fakeCompleter{response: "hello [meme:unknown]"}
	a := New(Config{
		Client:       client,
		SystemPrompt: "base prompt",
	})

	response, err := a.Ask(context.Background(), Request{
		Self:      User{ID: "bot"},
		User:      User{ID: "123", Name: "Alice"},
		Prompt:    "what now",
		UseMemory: true,
	})
	require.NoError(t, err)
	require.Equal(t, "hello", response.Text)
	require.Empty(t, response.Memes)
}
