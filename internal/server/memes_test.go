package server

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/metruzanca/caffeinator/agent"
	"github.com/stretchr/testify/require"
)

var testMemePNG, _ = base64.StdEncoding.DecodeString(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lK3Q6wAAAABJRU5ErkJggg==",
)

func TestSendAttachesMemeAndRecordsMessage(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "abc.png"), testMemePNG, 0600))

	store, err := agent.NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)
	_, err = store.Add(agent.Meme{ID: "abc", ImageFile: "abc.png", AddedAt: time.Now()})
	require.NoError(t, err)

	session := &fakeChatSession{}
	chat := newChat(session, &fakeAgent{}, ChatConfig{Memes: store, MemeDir: dir})
	chat.send(testMessage("42", "channel", "guild", "alice", "<@bot> hi"), "look [meme:abc]", []agent.Meme{
		{ID: "abc", ImageFile: "abc.png"},
	})

	require.Len(t, session.sent, 1)
	require.Len(t, session.sent[0].Files, 1)
	require.Equal(t, "abc.png", session.sent[0].Files[0].Name)

	meme, ok := store.FindByMessage("sent-1")
	require.True(t, ok)
	require.Equal(t, "abc", meme.ID)
}

func TestSendFallsBackToTextWhenMemeImageIsMissing(t *testing.T) {
	store, err := agent.NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)

	session := &fakeChatSession{}
	chat := newChat(session, &fakeAgent{}, ChatConfig{Memes: store, MemeDir: t.TempDir()})
	chat.send(testMessage("42", "channel", "guild", "alice", "<@bot> hi"), "text only", []agent.Meme{
		{ID: "gone", ImageFile: "gone.png"},
	})

	require.Len(t, session.sent, 1)
	require.Empty(t, session.sent[0].Files)
	require.Equal(t, "text only", session.sent[0].Content)
}

func TestSendAllowsImageOnlyReply(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "abc.png"), testMemePNG, 0600))

	session := &fakeChatSession{}
	chat := newChat(session, &fakeAgent{}, ChatConfig{MemeDir: dir})
	chat.send(testMessage("42", "channel", "guild", "alice", "<@bot> hi"), "", []agent.Meme{
		{ID: "abc", ImageFile: "abc.png"},
	})

	require.Len(t, session.sent, 1)
	require.Empty(t, session.sent[0].Content)
	require.Len(t, session.sent[0].Files, 1)
}

func TestMemeReactionCountsPositiveFromUniqueHumans(t *testing.T) {
	store, err := agent.NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)
	_, err = store.Add(agent.Meme{ID: "abc", ImageFile: "abc.png", AddedAt: time.Now()})
	require.NoError(t, err)
	require.NoError(t, store.MarkShared("abc", "channel", "message-1"))

	bot := &Bot{memes: &memesRuntime{
		Store:    store,
		Positive: parseReactionSet("🔥,❤️"),
	}}
	session := &discordgo.Session{State: discordgo.NewState()}
	session.State.User = &discordgo.User{ID: "bot"}

	reaction := &discordgo.MessageReactionAdd{MessageReaction: &discordgo.MessageReaction{
		MessageID: "message-1",
		ChannelID: "channel",
		GuildID:   "guild",
		UserID:    "user-1",
		Emoji:     discordgo.Emoji{Name: "🔥"},
	}}

	bot.onMemeReaction(session, reaction)
	bot.onMemeReaction(session, reaction)

	meme, ok := store.FindByMessage("message-1")
	require.True(t, ok)
	require.Equal(t, 1, meme.Reactions)

	reaction.UserID = "user-2"
	reaction.Emoji.Name = "❤️"
	bot.onMemeReaction(session, reaction)

	meme, _ = store.FindByMessage("message-1")
	require.Equal(t, 2, meme.Reactions)
}

func TestMemeReactionIgnoresNegativeAndBot(t *testing.T) {
	store, err := agent.NewMemeStore(filepath.Join(t.TempDir(), "memes.json"))
	require.NoError(t, err)
	_, err = store.Add(agent.Meme{ID: "abc", ImageFile: "abc.png", AddedAt: time.Now()})
	require.NoError(t, err)
	require.NoError(t, store.MarkShared("abc", "channel", "message-1"))

	bot := &Bot{memes: &memesRuntime{
		Store:    store,
		Positive: parseReactionSet("🔥"),
	}}
	session := &discordgo.Session{State: discordgo.NewState()}
	session.State.User = &discordgo.User{ID: "bot"}

	bot.onMemeReaction(session, &discordgo.MessageReactionAdd{MessageReaction: &discordgo.MessageReaction{
		MessageID: "message-1",
		ChannelID: "channel",
		GuildID:   "guild",
		UserID:    "user-1",
		Emoji:     discordgo.Emoji{Name: "👎"},
	}})
	bot.onMemeReaction(session, &discordgo.MessageReactionAdd{MessageReaction: &discordgo.MessageReaction{
		MessageID: "message-1",
		ChannelID: "channel",
		GuildID:   "guild",
		UserID:    "bot",
		Emoji:     discordgo.Emoji{Name: "🔥"},
	}})

	meme, _ := store.FindByMessage("message-1")
	require.Equal(t, 0, meme.Reactions)
}

func TestNormalizeReactionEmoji(t *testing.T) {
	require.Equal(t, "❤", normalizeReactionEmoji("❤️"))
	require.Equal(t, "👍", normalizeReactionEmoji("👍🏽"))
	require.Equal(t, "🔥", normalizeReactionEmoji("🔥"))
}
