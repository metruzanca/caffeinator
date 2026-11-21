package server

import (
	"testing"
	"time"

	"github.com/metruzanca/checkpoint-bot/internal/reminders"
	"github.com/stretchr/testify/require"
)

func TestFormatReminderList(t *testing.T) {
	require.Equal(t, "No reminders or events.", formatReminderList(nil))

	now := time.Now().UTC()
	items := []*reminders.Reminder{
		{
			ID:        "abcdef123456",
			Kind:      reminders.KindReminder,
			Text:      "standup",
			NextRunAt: now,
			Active:    true,
		},
		{
			ID:        "123456abcdef",
			Kind:      reminders.KindEvent,
			Text:      "coffee",
			NextRunAt: now,
			Active:    true,
		},
	}

	content := formatReminderList(items)
	require.Contains(t, content, "Your reminders and events:")
	require.Contains(t, content, "`abcdef12` Reminder")
	require.Contains(t, content, "`123456ab` Event")
	require.Contains(t, content, "standup")
	require.Contains(t, content, "coffee")
}

func TestParseStart(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	parsed, err := parseStart("2026-11-22T16:00:00", loc)
	require.NoError(t, err)
	require.Equal(t, 16, parsed.Hour())
	require.Equal(t, time.November, parsed.Month())
	require.Equal(t, 22, parsed.Day())

	parsed, err = parseStart("2026-11-22T21:00:00Z", loc)
	require.NoError(t, err)
	require.Equal(t, 16, parsed.In(loc).Hour())

	_, err = parseStart("tomorrow at 4", loc)
	require.Error(t, err)
}

func TestLooksLikeReminderRequest(t *testing.T) {
	require.True(t, looksLikeReminderRequest("remind me tomorrow at 4pm"))
	require.True(t, looksLikeReminderRequest("what reminders do I have"))
	require.True(t, looksLikeReminderRequest("create an event for Friday"))
	require.False(t, looksLikeReminderRequest("what is the deploy command"))
}

func TestJoinAndLeavePatterns(t *testing.T) {
	require.True(t, joinPattern.MatchString("me too"))
	require.True(t, joinPattern.MatchString("count me in"))
	require.True(t, joinPattern.MatchString("+1"))
	require.True(t, leavePattern.MatchString("leave"))
	require.True(t, leavePattern.MatchString("remove me"))
	require.False(t, joinPattern.MatchString("what time is it"))
	require.False(t, joinPattern.MatchString("I can't join, I have another meeting"))
}

func TestTriggerMessageAllowedMentions(t *testing.T) {
	reminder := &reminders.Reminder{
		AnnouncementMessageID: "announcement",
		ChannelID:             "channel",
		GuildID:               "guild",
	}

	data := triggerMessageData(reminder, "<@111> reminder", []string{"111"})

	require.NotNil(t, data.AllowedMentions)
	require.Empty(t, data.AllowedMentions.Parse)
	require.Equal(t, []string{"111"}, data.AllowedMentions.Users)
	require.False(t, data.AllowedMentions.RepliedUser)
	require.NotNil(t, data.Reference)
	require.Equal(t, "announcement", data.Reference.MessageID)
}

func TestMentionIDs(t *testing.T) {
	reminder := &reminders.Reminder{
		CreatorID:    "111",
		Participants: []string{"222", "111", "333"},
	}

	ids := reminder.MentionIDs()
	require.Equal(t, []string{"111", "222", "333"}, ids)
}
