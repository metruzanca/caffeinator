package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/metruzanca/checkpoint-bot/internal/llm"
	"github.com/metruzanca/checkpoint-bot/internal/memory"
	"github.com/metruzanca/checkpoint-bot/internal/server/commands"
	"github.com/stretchr/testify/require"
)

type fakeChatSession struct {
	messages   map[string]*discordgo.Message
	history    []*discordgo.Message
	historyErr error

	lastChannel string
	lastLimit   int
	lastBefore  string

	sent   []*discordgo.MessageSend
	sentCh chan *discordgo.MessageSend
}

func (f *fakeChatSession) ChannelMessage(channelID, messageID string, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	if message, ok := f.messages[messageID]; ok {
		return message, nil
	}
	return nil, errors.New("message not found")
}

func (f *fakeChatSession) ChannelMessages(channelID string, limit int, beforeID, afterID, aroundID string, _ ...discordgo.RequestOption) ([]*discordgo.Message, error) {
	f.lastChannel = channelID
	f.lastLimit = limit
	f.lastBefore = beforeID
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	return f.history, nil
}

func (f *fakeChatSession) ChannelTyping(channelID string, _ ...discordgo.RequestOption) error {
	return nil
}

func (f *fakeChatSession) ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.sent = append(f.sent, data)
	if f.sentCh != nil {
		f.sentCh <- data
	}
	return &discordgo.Message{}, nil
}

type fakeExtractor struct {
	calls       int
	facts       []memory.Fact
	err         error
	gotTxt      string
	gotTexts    []string
	gotSubjects []map[string]string
}

func (f *fakeExtractor) Extract(ctx context.Context, transcript string, subjects map[string]string) ([]memory.Fact, error) {
	f.calls++
	f.gotTxt = transcript
	f.gotTexts = append(f.gotTexts, transcript)
	f.gotSubjects = append(f.gotSubjects, subjects)
	return f.facts, f.err
}

type fakeChatClient struct {
	response     string
	systemPrompt string
	err          error
	calls        int
	got          []llm.Message
	called       chan []llm.Message
}

func (f *fakeChatClient) Chat(ctx context.Context, messages []llm.Message) (string, llm.Usage, error) {
	return f.ChatWithSystem(ctx, "", messages)
}

func (f *fakeChatClient) ChatWithSystem(ctx context.Context, systemPrompt string, messages []llm.Message) (string, llm.Usage, error) {
	f.calls++
	f.got = messages
	f.systemPrompt = systemPrompt
	if f.called != nil {
		f.called <- messages
	}
	if f.err != nil {
		return "", llm.Usage{}, f.err
	}
	return f.response, llm.Usage{PromptTokens: 10, CompletionTokens: 5}, nil
}

func testMessage(id, channel, guild, authorID, content string) *discordgo.MessageCreate {
	return &discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        id,
		ChannelID: channel,
		GuildID:   guild,
		Author:    &discordgo.User{ID: authorID, Username: authorID},
		Content:   content,
	}}
}

func TestBuildMessagesRolesAndFormatting(t *testing.T) {
	history := []*discordgo.Message{
		{ID: "1", Author: &discordgo.User{ID: "alice", Username: "alice"}, Content: "hello"},
		{ID: "2", Author: &discordgo.User{ID: "bot", Username: "bot"}, Content: "hi alice"},
	}
	current := testMessage("3", "channel", "guild", "bob", "<@bot> what now")

	messages := buildMessages(history, current, "what now", "bot", ChatConfig{
		MaxCharsPerMessage: 100,
		MaxContextChars:    1000,
	})

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
	history := []*discordgo.Message{
		{ID: "1", Author: &discordgo.User{ID: "alice", Username: "alice"}, Content: "first message is very long"},
		{ID: "2", Author: &discordgo.User{ID: "bob", Username: "bob"}, Content: "second message is very long"},
		{ID: "3", Author: &discordgo.User{ID: "carol", Username: "carol"}, Content: "third message is very long"},
	}
	current := testMessage("4", "channel", "guild", "dave", "<@bot> latest")

	messages := buildMessages(history, current, "latest", "bot", ChatConfig{
		MaxCharsPerMessage: 1000,
		MaxContextChars:    60,
	})

	require.NotEmpty(t, messages)
	require.Contains(t, messages[len(messages)-1].Content, "dave: latest")

	total := 0
	for _, message := range messages {
		total += len([]rune(message.Content))
	}
	require.LessOrEqual(t, total, 60)
}

func TestGatherContextWithoutReply(t *testing.T) {
	session := &fakeChatSession{
		history: []*discordgo.Message{
			{ID: "2", Author: &discordgo.User{ID: "bob"}},
			{ID: "1", Author: &discordgo.User{ID: "alice"}},
		},
	}
	chat := newChat(session, &fakeChatClient{}, ChatConfig{ContextMessages: 10})

	messages := chat.gatherContext(testMessage("3", "channel", "guild", "alice", "<@bot> hi"), nil, 10)

	require.Len(t, messages, 2)
	require.Equal(t, "1", messages[0].ID)
	require.Equal(t, "2", messages[1].ID)
	require.Equal(t, "channel", session.lastChannel)
	require.Equal(t, 10, session.lastLimit)
	require.Equal(t, "3", session.lastBefore)
}

func TestGatherContextUsesReplyAnchor(t *testing.T) {
	referenced := &discordgo.Message{
		ID:        "ref",
		ChannelID: "channel",
		Author:    &discordgo.User{ID: "bob"},
		Content:   "the original question",
	}
	session := &fakeChatSession{
		history: []*discordgo.Message{
			{ID: "p2", Author: &discordgo.User{ID: "carol"}},
			{ID: "p1", Author: &discordgo.User{ID: "alice"}},
		},
	}
	chat := newChat(session, &fakeChatClient{}, ChatConfig{ContextMessages: 3})

	messages := chat.gatherContext(testMessage("3", "channel", "guild", "alice", "<@bot> hi"), referenced, 3)

	require.Len(t, messages, 3)
	require.Equal(t, "p1", messages[0].ID)
	require.Equal(t, "p2", messages[1].ID)
	require.Equal(t, "ref", messages[2].ID)
	require.Equal(t, 2, session.lastLimit)
	require.Equal(t, "ref", session.lastBefore)
}

func TestGatherContextFallsBackWhenHistoryFails(t *testing.T) {
	session := &fakeChatSession{historyErr: errors.New("no access")}
	chat := newChat(session, &fakeChatClient{}, ChatConfig{ContextMessages: 5})

	messages := chat.gatherContext(testMessage("3", "channel", "guild", "alice", "<@bot> hi"), nil, 5)

	require.Nil(t, messages)
}

func TestHandleIgnoresBots(t *testing.T) {
	client := &fakeChatClient{called: make(chan []llm.Message, 1)}
	chat := newChat(&fakeChatSession{}, client, ChatConfig{})

	message := testMessage("1", "channel", "guild", "bot", "<@bot> hi")
	message.Author.Bot = true
	chat.handle(message, "bot", false)

	time.Sleep(20 * time.Millisecond)
	require.Equal(t, 0, client.calls)
}

func TestHandleIgnoresIrrelevantMessages(t *testing.T) {
	client := &fakeChatClient{called: make(chan []llm.Message, 1)}
	chat := newChat(&fakeChatSession{}, client, ChatConfig{})

	chat.handle(testMessage("1", "channel", "guild", "alice", "just chatting"), "bot", false)

	time.Sleep(20 * time.Millisecond)
	require.Equal(t, 0, client.calls)
}

func TestHandleRespondsToMention(t *testing.T) {
	session := &fakeChatSession{
		history: []*discordgo.Message{
			{ID: "1", Author: &discordgo.User{ID: "carol", Username: "carol"}, Content: "context"},
		},
		sentCh: make(chan *discordgo.MessageSend, 5),
	}
	client := &fakeChatClient{
		response: "the answer",
		called:   make(chan []llm.Message, 1),
	}
	chat := newChat(session, client, ChatConfig{ContextMessages: 10})

	chat.handle(testMessage("2", "channel", "guild", "alice", "<@bot> what do you think?"), "bot", false)

	var sentMessages []llm.Message
	select {
	case sentMessages = <-client.called:
	case <-time.After(time.Second):
		t.Fatal("model was not called")
	}

	require.NotEmpty(t, sentMessages)
	last := sentMessages[len(sentMessages)-1]
	require.Equal(t, "user", last.Role)
	require.Contains(t, last.Content, "alice: what do you think?")
	require.NotContains(t, last.Content, "<@bot>")

	select {
	case sent := <-session.sentCh:
		require.Equal(t, "the answer", sent.Content)
		require.NotNil(t, sent.Reference)
		require.Equal(t, "2", sent.Reference.MessageID)
		require.Contains(t, sent.AllowedMentions.Parse, discordgo.AllowedMentionTypeUsers)
		require.NotContains(t, sent.AllowedMentions.Parse, discordgo.AllowedMentionTypeEveryone)
		require.NotContains(t, sent.AllowedMentions.Parse, discordgo.AllowedMentionTypeRoles)
		require.False(t, sent.AllowedMentions.RepliedUser)
	case <-time.After(time.Second):
		t.Fatal("response was not sent")
	}
}

func TestHandleRespondsToReplyWithoutMention(t *testing.T) {
	referenced := &discordgo.Message{
		ID:        "ref",
		ChannelID: "channel",
		GuildID:   "guild",
		Author:    &discordgo.User{ID: "bot", Username: "bot"},
		Content:   "previous answer",
	}
	session := &fakeChatSession{
		sentCh: make(chan *discordgo.MessageSend, 5),
	}
	client := &fakeChatClient{
		response: "follow up",
		called:   make(chan []llm.Message, 1),
	}
	chat := newChat(session, client, ChatConfig{ContextMessages: 10})

	message := testMessage("2", "channel", "guild", "alice", "what about this?")
	message.ReferencedMessage = referenced

	chat.handle(message, "bot", false)

	var sentMessages []llm.Message
	select {
	case sentMessages = <-client.called:
	case <-time.After(time.Second):
		t.Fatal("model was not called")
	}

	require.Len(t, sentMessages, 2)
	require.Equal(t, "assistant", sentMessages[0].Role)
	require.Equal(t, "previous answer", sentMessages[0].Content)
	require.Equal(t, "user", sentMessages[1].Role)
	require.Contains(t, sentMessages[1].Content, "alice: what about this?")

	select {
	case sent := <-session.sentCh:
		require.Equal(t, "follow up", sent.Content)
	case <-time.After(time.Second):
		t.Fatal("response was not sent")
	}
}

func TestHandleFetchesReferencedMessage(t *testing.T) {
	session := &fakeChatSession{
		messages: map[string]*discordgo.Message{
			"ref": {
				ID:        "ref",
				ChannelID: "channel",
				GuildID:   "guild",
				Author:    &discordgo.User{ID: "bot", Username: "bot"},
				Content:   "previous answer",
			},
		},
		sentCh: make(chan *discordgo.MessageSend, 5),
	}
	client := &fakeChatClient{
		response: "follow up",
		called:   make(chan []llm.Message, 1),
	}
	chat := newChat(session, client, ChatConfig{ContextMessages: 10})

	message := testMessage("2", "channel", "guild", "alice", "and then?")
	message.MessageReference = &discordgo.MessageReference{
		MessageID: "ref",
		ChannelID: "channel",
		GuildID:   "guild",
	}

	chat.handle(message, "bot", false)

	select {
	case sentMessages := <-client.called:
		require.Len(t, sentMessages, 2)
		require.Equal(t, "assistant", sentMessages[0].Role)
		require.Equal(t, "previous answer", sentMessages[0].Content)
	case <-time.After(time.Second):
		t.Fatal("model was not called")
	}
}

func TestHandleIgnoresReplyToOtherUserWithoutMention(t *testing.T) {
	client := &fakeChatClient{called: make(chan []llm.Message, 1)}
	chat := newChat(&fakeChatSession{}, client, ChatConfig{})

	message := testMessage("2", "channel", "guild", "alice", "side conversation")
	message.ReferencedMessage = &discordgo.Message{
		ID:     "ref",
		Author: &discordgo.User{ID: "bob"},
	}

	chat.handle(message, "bot", false)

	time.Sleep(20 * time.Millisecond)
	require.Equal(t, 0, client.calls)
}

func TestSendAllowsUserMentionsAndReferencesMessage(t *testing.T) {
	session := &fakeChatSession{sentCh: make(chan *discordgo.MessageSend, 1)}
	chat := newChat(session, &fakeChatClient{}, ChatConfig{})

	chat.send(testMessage("42", "channel", "guild", "alice", "<@bot> hi"), "@everyone hello")

	select {
	case sent := <-session.sentCh:
		require.Equal(t, "@everyone hello", sent.Content)
		require.NotNil(t, sent.Reference)
		require.Equal(t, "42", sent.Reference.MessageID)
		require.Contains(t, sent.AllowedMentions.Parse, discordgo.AllowedMentionTypeUsers)
		require.NotContains(t, sent.AllowedMentions.Parse, discordgo.AllowedMentionTypeEveryone)
		require.NotContains(t, sent.AllowedMentions.Parse, discordgo.AllowedMentionTypeRoles)
		require.False(t, sent.AllowedMentions.RepliedUser)
	case <-time.After(time.Second):
		t.Fatal("message was not sent")
	}
}

func TestAskUsesMemory(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), 1000)
	require.NoError(t, err)
	require.NoError(t, store.AppendFacts([]memory.Fact{
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

	client := &fakeChatClient{response: "hello"}
	chat := newChat(&fakeChatSession{}, client, ChatConfig{
		ContextMessages: 10,
		SystemPrompt:    "base prompt",
		Memory:          store,
	})

	content, err := chat.Ask(context.Background(), commands.ChatRequest{
		GuildID:       "guild",
		ChannelID:     "channel",
		UserID:        "123",
		DisplayName:   "Alice",
		BotID:         "bot",
		Text:          "what am I working on?",
		HistoryLength: 10,
	})
	require.NoError(t, err)
	require.Equal(t, "hello", content)
	require.Contains(t, client.systemPrompt, "base prompt")
	require.Contains(t, client.systemPrompt, "Building a Go Discord bot.")
	require.Contains(t, client.systemPrompt, "The server is for developers.")
	require.Contains(t, client.systemPrompt, "Current Unix time:")
}

func TestAskDisableMemorySkipsMemory(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), 1000)
	require.NoError(t, err)
	require.NoError(t, store.AppendFacts([]memory.Fact{{
		SubjectID:   "123",
		SubjectName: "Alice",
		Text:        "Building a Go Discord bot.",
		Category:    "work",
		Confidence:  0.9,
	}}))

	client := &fakeChatClient{response: "hello"}
	chat := newChat(&fakeChatSession{}, client, ChatConfig{
		ContextMessages: 10,
		SystemPrompt:    "base prompt",
		Memory:          store,
	})

	_, err = chat.Ask(context.Background(), commands.ChatRequest{
		GuildID:       "guild",
		ChannelID:     "channel",
		UserID:        "123",
		DisplayName:   "Alice",
		BotID:         "bot",
		Text:          "what am I working on?",
		HistoryLength: 10,
		DisableMemory: true,
	})
	require.NoError(t, err)
	require.Contains(t, client.systemPrompt, "base prompt")
	require.NotContains(t, client.systemPrompt, "Building a Go Discord bot.")
}

func TestAskStoresMemoryWhenEnabled(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), 1000)
	require.NoError(t, err)

	extractor := &fakeExtractor{facts: []memory.Fact{{
		SubjectID:   "123",
		SubjectName: "Alice",
		Text:        "Building a Go Discord bot.",
		Category:    "work",
		Confidence:  0.9,
	}}}

	client := &fakeChatClient{response: "hello"}
	chat := newChat(&fakeChatSession{}, client, ChatConfig{
		ContextMessages: 10,
		SystemPrompt:    "base prompt",
		Memory:          store,
		Extractor:       extractor,
	})

	_, err = chat.Ask(context.Background(), commands.ChatRequest{
		GuildID:       "guild",
		ChannelID:     "channel",
		UserID:        "123",
		DisplayName:   "Alice",
		BotID:         "bot",
		Text:          "I am building a Go bot.",
		HistoryLength: 10,
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		text, err := store.Get("123")
		return err == nil && strings.Contains(text, "Building a Go Discord bot.")
	}, time.Second, 10*time.Millisecond)
}

func TestAskDisableMemorySkipsExtraction(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), 1000)
	require.NoError(t, err)

	extractor := &fakeExtractor{facts: []memory.Fact{{
		SubjectID: "123",
		Text:      "Should not be stored.",
	}}}

	client := &fakeChatClient{response: "hello"}
	chat := newChat(&fakeChatSession{}, client, ChatConfig{
		ContextMessages: 10,
		SystemPrompt:    "base prompt",
		Memory:          store,
		Extractor:       extractor,
	})

	_, err = chat.Ask(context.Background(), commands.ChatRequest{
		GuildID:       "guild",
		ChannelID:     "channel",
		UserID:        "123",
		DisplayName:   "Alice",
		BotID:         "bot",
		Text:          "Do not remember this.",
		HistoryLength: 10,
		DisableMemory: true,
	})
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 0, extractor.calls)

	text, err := store.Get("123")
	require.NoError(t, err)
	require.Empty(t, text)
}

func TestCollectSubjectsIncludesHistoryAuthors(t *testing.T) {
	message := testMessage("1", "channel", "guild", "111", "hello <@456>")
	message.Author.Username = "alice"
	history := []*discordgo.Message{
		{Author: &discordgo.User{ID: "222", Username: "bob"}},
		{Author: &discordgo.User{ID: "999", Username: "bot", Bot: true}},
	}

	subjects := collectSubjects(message, nil, "hello <@456>", "bot", history)

	require.Contains(t, subjects, "server")
	require.Contains(t, subjects, "111")
	require.Contains(t, subjects, "222")
	require.Contains(t, subjects, "456")
	require.NotContains(t, subjects, "999")
}

func TestExtractionSubjectsOnlySpeaker(t *testing.T) {
	message := testMessage("1", "channel", "guild", "111", "remember that <@222> is an AI doomer")
	message.Author.Username = "alice"

	subjects := extractionSubjects(message, "bot", false)

	require.Len(t, subjects, 1)
	require.Contains(t, subjects, "111")
	require.NotContains(t, subjects, "222")
	require.NotContains(t, subjects, "server")
}

func TestExtractionSubjectsIncludesServerForAdmins(t *testing.T) {
	message := testMessage("1", "channel", "guild", "111", "remember that this server is for developers")
	message.Author.Username = "alice"

	subjects := extractionSubjects(message, "bot", true)

	require.Len(t, subjects, 2)
	require.Contains(t, subjects, "111")
	require.Contains(t, subjects, "server")
}

func TestAdminRememberMentionStoresServerMemory(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), 1000)
	require.NoError(t, err)

	extractor := &fakeExtractor{facts: []memory.Fact{{
		SubjectID:   "server",
		SubjectName: "server",
		Text:        "The server is for developers.",
		Category:    "server",
		Confidence:  0.9,
	}}}

	session := &fakeChatSession{sentCh: make(chan *discordgo.MessageSend, 1)}
	chat := newChat(session, &fakeChatClient{response: "Noted."}, ChatConfig{
		ContextMessages: 10,
		Memory:          store,
		Extractor:       extractor,
		PassiveMemory:   true,
	})

	chat.handle(testMessage("1", "channel", "guild", "111", "<@bot> remember that this server is for developers"), "bot", true)

	require.Eventually(t, func() bool {
		text, err := store.Get("server")
		return err == nil && strings.Contains(text, "The server is for developers.")
	}, time.Second, 10*time.Millisecond)
}

func TestNonAdminRememberMentionSkipsServerMemory(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), 1000)
	require.NoError(t, err)

	extractor := &fakeExtractor{facts: []memory.Fact{{
		SubjectID:   "server",
		SubjectName: "server",
		Text:        "The server is for developers.",
		Category:    "server",
		Confidence:  0.9,
	}}}

	session := &fakeChatSession{sentCh: make(chan *discordgo.MessageSend, 1)}
	chat := newChat(session, &fakeChatClient{response: "Noted."}, ChatConfig{
		ContextMessages: 10,
		Memory:          store,
		Extractor:       extractor,
		PassiveMemory:   true,
	})

	chat.handle(testMessage("1", "channel", "guild", "111", "<@bot> remember that this server is for developers"), "bot", false)

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 0, extractor.calls)

	text, err := store.Get("server")
	require.NoError(t, err)
	require.Empty(t, text)
}

func TestAdminMentionWithoutRememberSkipsExtraction(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), 1000)
	require.NoError(t, err)

	extractor := &fakeExtractor{facts: []memory.Fact{{
		SubjectID: "server",
		Text:      "The server is for developers.",
	}}}

	session := &fakeChatSession{sentCh: make(chan *discordgo.MessageSend, 1)}
	chat := newChat(session, &fakeChatClient{response: "It is for developers."}, ChatConfig{
		ContextMessages: 10,
		Memory:          store,
		Extractor:       extractor,
		PassiveMemory:   true,
	})

	chat.handle(testMessage("1", "channel", "guild", "111", "<@bot> what is this server about?"), "bot", true)

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 0, extractor.calls)
}

func TestIsGuildAdmin(t *testing.T) {
	session, err := discordgo.New("Bot test")
	require.NoError(t, err)

	require.NoError(t, session.State.GuildAdd(&discordgo.Guild{
		ID:      "guild",
		OwnerID: "owner",
		Roles: []*discordgo.Role{
			{ID: "guild"},
			{ID: "admin-role", Permissions: discordgo.PermissionAdministrator},
		},
	}))
	require.NoError(t, session.State.ChannelAdd(&discordgo.Channel{
		ID:      "channel",
		GuildID: "guild",
	}))

	message := &discordgo.Message{
		ID:        "1",
		ChannelID: "channel",
		GuildID:   "guild",
		Author:    &discordgo.User{ID: "111"},
		Member:    &discordgo.Member{Roles: []string{"admin-role"}},
	}
	require.True(t, isGuildAdmin(session, message))

	message.Author.ID = "222"
	message.Member.Roles = nil
	require.False(t, isGuildAdmin(session, message))

	message.Author.ID = "owner"
	require.True(t, isGuildAdmin(session, message))
}

func TestWantsMemoryUpdate(t *testing.T) {
	require.True(t, wantsMemoryUpdate("remember that this server is for developers"))
	require.True(t, wantsMemoryUpdate("Please MEMORIZE this"))
	require.False(t, wantsMemoryUpdate("what is this server about?"))
}

func TestBuildSelfTranscriptExcludesOtherUsers(t *testing.T) {
	message := testMessage("3", "channel", "guild", "111", "remember that <@222> is an AI doomer")
	message.Author.Username = "alice"
	history := []*discordgo.Message{
		{ID: "1", Author: &discordgo.User{ID: "111", Username: "alice"}, Content: "I work on a Go bot."},
		{ID: "2", Author: &discordgo.User{ID: "222", Username: "bob"}, Content: "I am an AI doomer."},
	}

	transcript := buildSelfTranscript(history, message, message.Content)

	require.Contains(t, transcript, "I work on a Go bot.")
	require.NotContains(t, transcript, "I am an AI doomer.")
	require.Contains(t, transcript, "remember that <@222> is an AI doomer")
}

func TestMentionsUser(t *testing.T) {
	require.True(t, mentionsUser(testMessage("1", "c", "g", "alice", "<@123> hi"), "123"))
	require.True(t, mentionsUser(testMessage("1", "c", "g", "alice", "<@!123> hi"), "123"))
	require.True(t, mentionsUser(&discordgo.MessageCreate{Message: &discordgo.Message{
		Mentions: []*discordgo.User{{ID: "123"}},
	}}, "123"))
	require.False(t, mentionsUser(testMessage("1", "c", "g", "alice", "hi"), "123"))
}

func TestCleanContentStripsMention(t *testing.T) {
	message := testMessage("1", "c", "g", "alice", "<@123> hello <@!123> there")
	require.Equal(t, "hello  there", cleanContent(message, "123", true))
	require.Equal(t, "<@123> hello <@!123> there", cleanContent(message, "123", false))
}

func TestSplitMessage(t *testing.T) {
	t.Run("single short message", func(t *testing.T) {
		require.Equal(t, []string{"hello"}, splitMessage("hello", 100, 3))
	})

	t.Run("splits on newlines", func(t *testing.T) {
		content := strings.Repeat("line\n", 300)
		chunks := splitMessage(content, 100, 3)
		require.NotEmpty(t, chunks)
		require.LessOrEqual(t, len(chunks), 3)
		for _, chunk := range chunks {
			require.LessOrEqual(t, len([]rune(chunk)), 100)
		}
	})

	t.Run("hard splits a long line", func(t *testing.T) {
		chunks := splitMessage(strings.Repeat("a", 5000), 1000, 3)
		require.Len(t, chunks, 3)
		for _, chunk := range chunks {
			require.LessOrEqual(t, len([]rune(chunk)), 1000)
		}
	})
}
