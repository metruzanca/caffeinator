package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/metruzanca/caffeinator/agent"
	"github.com/metruzanca/caffeinator/internal/server/commands"
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

type learnedCall struct {
	turns         []agent.Turn
	subject       agent.User
	includeServer bool
}

type fakeAgent struct {
	response string
	err      error
	facts    []agent.Fact
	learnErr error

	askCalls int
	got      []agent.Request
	asked    chan agent.Request

	learnCalls int
	learned    chan learnedCall
}

func (f *fakeAgent) Ask(ctx context.Context, req agent.Request) (agent.Response, error) {
	f.askCalls++
	f.got = append(f.got, req)
	if f.asked != nil {
		f.asked <- req
	}
	if f.err != nil {
		return agent.Response{}, f.err
	}
	return agent.Response{
		Text:  f.response,
		Usage: agent.Usage{PromptTokens: 10, CompletionTokens: 5},
		Stats: agent.Stats{Messages: len(req.History) + 1},
	}, nil
}

func (f *fakeAgent) Learn(ctx context.Context, turns []agent.Turn, subject agent.User, includeServer bool) ([]agent.Fact, error) {
	f.learnCalls++
	if f.learned != nil {
		f.learned <- learnedCall{turns: turns, subject: subject, includeServer: includeServer}
	}
	if f.learnErr != nil {
		return nil, f.learnErr
	}
	return f.facts, nil
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

func TestGatherContextWithoutReply(t *testing.T) {
	session := &fakeChatSession{
		history: []*discordgo.Message{
			{ID: "2", Author: &discordgo.User{ID: "bob"}},
			{ID: "1", Author: &discordgo.User{ID: "alice"}},
		},
	}
	chat := newChat(session, &fakeAgent{}, ChatConfig{ContextMessages: 10})

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
	chat := newChat(session, &fakeAgent{}, ChatConfig{ContextMessages: 3})

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
	chat := newChat(session, &fakeAgent{}, ChatConfig{ContextMessages: 5})

	messages := chat.gatherContext(testMessage("3", "channel", "guild", "alice", "<@bot> hi"), nil, 5)

	require.Nil(t, messages)
}

func TestHandleIgnoresBots(t *testing.T) {
	ai := &fakeAgent{asked: make(chan agent.Request, 1)}
	chat := newChat(&fakeChatSession{}, ai, ChatConfig{})

	message := testMessage("1", "channel", "guild", "bot", "<@bot> hi")
	message.Author.Bot = true
	chat.handle(message, "bot", false)

	time.Sleep(20 * time.Millisecond)
	require.Equal(t, 0, ai.askCalls)
}

func TestHandleIgnoresIrrelevantMessages(t *testing.T) {
	ai := &fakeAgent{asked: make(chan agent.Request, 1)}
	chat := newChat(&fakeChatSession{}, ai, ChatConfig{})

	chat.handle(testMessage("1", "channel", "guild", "alice", "just chatting"), "bot", false)

	time.Sleep(20 * time.Millisecond)
	require.Equal(t, 0, ai.askCalls)
}

func TestHandleRespondsToMention(t *testing.T) {
	session := &fakeChatSession{
		history: []*discordgo.Message{
			{ID: "1", Author: &discordgo.User{ID: "carol", Username: "carol"}, Content: "context"},
		},
		sentCh: make(chan *discordgo.MessageSend, 5),
	}
	ai := &fakeAgent{
		response: "the answer",
		asked:    make(chan agent.Request, 1),
	}
	chat := newChat(session, ai, ChatConfig{ContextMessages: 10})

	chat.handle(testMessage("2", "channel", "guild", "alice", "<@bot> what do you think?"), "bot", false)

	var req agent.Request
	select {
	case req = <-ai.asked:
	case <-time.After(time.Second):
		t.Fatal("model was not called")
	}

	require.Equal(t, "alice", req.User.ID)
	require.Equal(t, "what do you think?", req.Prompt)
	require.Len(t, req.History, 1)
	require.Equal(t, "context", req.History[0].Content)

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
	ai := &fakeAgent{
		response: "follow up",
		asked:    make(chan agent.Request, 1),
	}
	chat := newChat(session, ai, ChatConfig{ContextMessages: 10})

	message := testMessage("2", "channel", "guild", "alice", "what about this?")
	message.ReferencedMessage = referenced

	chat.handle(message, "bot", false)

	var req agent.Request
	select {
	case req = <-ai.asked:
	case <-time.After(time.Second):
		t.Fatal("model was not called")
	}

	require.Len(t, req.History, 1)
	require.Equal(t, "bot", req.History[0].Author.ID)
	require.Equal(t, "previous answer", req.History[0].Content)
	require.Equal(t, "what about this?", req.Prompt)

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
	ai := &fakeAgent{
		response: "follow up",
		asked:    make(chan agent.Request, 1),
	}
	chat := newChat(session, ai, ChatConfig{ContextMessages: 10})

	message := testMessage("2", "channel", "guild", "alice", "and then?")
	message.MessageReference = &discordgo.MessageReference{
		MessageID: "ref",
		ChannelID: "channel",
		GuildID:   "guild",
	}

	chat.handle(message, "bot", false)

	select {
	case req := <-ai.asked:
		require.Len(t, req.History, 1)
		require.Equal(t, "bot", req.History[0].Author.ID)
		require.Equal(t, "previous answer", req.History[0].Content)
	case <-time.After(time.Second):
		t.Fatal("model was not called")
	}
}

func TestHandleIgnoresReplyToOtherUserWithoutMention(t *testing.T) {
	ai := &fakeAgent{asked: make(chan agent.Request, 1)}
	chat := newChat(&fakeChatSession{}, ai, ChatConfig{})

	message := testMessage("2", "channel", "guild", "alice", "side conversation")
	message.ReferencedMessage = &discordgo.Message{
		ID:     "ref",
		Author: &discordgo.User{ID: "bob"},
	}

	chat.handle(message, "bot", false)

	time.Sleep(20 * time.Millisecond)
	require.Equal(t, 0, ai.askCalls)
}

func TestSendAllowsUserMentionsAndReferencesMessage(t *testing.T) {
	session := &fakeChatSession{sentCh: make(chan *discordgo.MessageSend, 1)}
	chat := newChat(session, &fakeAgent{}, ChatConfig{})

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
	ai := &fakeAgent{response: "hello", asked: make(chan agent.Request, 1)}
	chat := newChat(&fakeChatSession{}, ai, ChatConfig{ContextMessages: 10})

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

	req := <-ai.asked
	require.True(t, req.UseMemory)
	require.Equal(t, "123", req.User.ID)
	require.Equal(t, "Alice", req.User.Name)
}

func TestAskDisableMemorySkipsMemory(t *testing.T) {
	ai := &fakeAgent{response: "hello", asked: make(chan agent.Request, 1)}
	chat := newChat(&fakeChatSession{}, ai, ChatConfig{ContextMessages: 10})

	_, err := chat.Ask(context.Background(), commands.ChatRequest{
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

	req := <-ai.asked
	require.False(t, req.UseMemory)
}

func TestAskStoresMemoryWhenEnabled(t *testing.T) {
	store, err := agent.NewStore(t.TempDir(), 1000)
	require.NoError(t, err)

	ai := &fakeAgent{
		response: "hello",
		facts:    []agent.Fact{{SubjectID: "123", Text: "Building a Go Discord bot."}},
		learned:  make(chan learnedCall, 1),
	}
	chat := newChat(&fakeChatSession{}, ai, ChatConfig{
		ContextMessages: 10,
		Memory:          store,
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

	select {
	case call := <-ai.learned:
		require.Equal(t, "123", call.subject.ID)
		require.False(t, call.includeServer)
		require.NotEmpty(t, call.turns)
	case <-time.After(time.Second):
		t.Fatal("memory extraction was not called")
	}
}

func TestAskDisableMemorySkipsExtraction(t *testing.T) {
	store, err := agent.NewStore(t.TempDir(), 1000)
	require.NoError(t, err)

	ai := &fakeAgent{response: "hello", learned: make(chan learnedCall, 1)}
	chat := newChat(&fakeChatSession{}, ai, ChatConfig{
		ContextMessages: 10,
		Memory:          store,
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
	require.Equal(t, 0, ai.learnCalls)
}

func TestAdminRememberMentionStoresServerMemory(t *testing.T) {
	store, err := agent.NewStore(t.TempDir(), 1000)
	require.NoError(t, err)

	ai := &fakeAgent{
		response: "Noted.",
		facts:    []agent.Fact{{SubjectID: "server", Text: "The server is for developers."}},
		learned:  make(chan learnedCall, 1),
	}
	session := &fakeChatSession{sentCh: make(chan *discordgo.MessageSend, 1)}
	chat := newChat(session, ai, ChatConfig{
		ContextMessages: 10,
		Memory:          store,
		PassiveMemory:   true,
	})

	chat.handle(testMessage("1", "channel", "guild", "111", "<@bot> remember that this server is for developers"), "bot", true)

	select {
	case call := <-ai.learned:
		require.True(t, call.includeServer)
		require.Equal(t, "111", call.subject.ID)
	case <-time.After(time.Second):
		t.Fatal("memory extraction was not called")
	}
}

func TestNonAdminRememberMentionSkipsServerMemory(t *testing.T) {
	store, err := agent.NewStore(t.TempDir(), 1000)
	require.NoError(t, err)

	ai := &fakeAgent{response: "Noted.", learned: make(chan learnedCall, 1)}
	session := &fakeChatSession{sentCh: make(chan *discordgo.MessageSend, 1)}
	chat := newChat(session, ai, ChatConfig{
		ContextMessages: 10,
		Memory:          store,
		PassiveMemory:   true,
	})

	chat.handle(testMessage("1", "channel", "guild", "111", "<@bot> remember that this server is for developers"), "bot", false)

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 0, ai.learnCalls)
}

func TestAdminMentionWithoutRememberSkipsExtraction(t *testing.T) {
	store, err := agent.NewStore(t.TempDir(), 1000)
	require.NoError(t, err)

	ai := &fakeAgent{response: "It is for developers.", learned: make(chan learnedCall, 1)}
	session := &fakeChatSession{sentCh: make(chan *discordgo.MessageSend, 1)}
	chat := newChat(session, ai, ChatConfig{
		ContextMessages: 10,
		Memory:          store,
		PassiveMemory:   true,
	})

	chat.handle(testMessage("1", "channel", "guild", "111", "<@bot> what is this server about?"), "bot", true)

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 0, ai.learnCalls)
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
