package reminders

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "reminders.json"))
	require.NoError(t, err)
	return store
}

func TestStoreCreateAndList(t *testing.T) {
	store := testStore(t)

	require.NoError(t, store.SetTimezone("111", "America/New_York"))
	require.Equal(t, "America/New_York", store.Timezone("111"))

	now := time.Now().UTC().Add(time.Hour)
	require.NoError(t, store.Create(&Reminder{
		Kind:      KindReminder,
		CreatorID: "111",
		ChannelID: "channel",
		Text:      "standup",
		Timezone:  "America/New_York",
		StartAt:   now,
		NextRunAt: now,
		Active:    true,
	}))
	require.NoError(t, store.Create(&Reminder{
		Kind:      KindEvent,
		CreatorID: "222",
		ChannelID: "channel",
		Text:      "coffee",
		Timezone:  "America/New_York",
		StartAt:   now,
		NextRunAt: now,
		Active:    true,
	}))

	items := store.ListForUser("111")
	require.Len(t, items, 2)

	items = store.ListForUser("333")
	require.Len(t, items, 1)
	require.Equal(t, KindEvent, items[0].Kind)
}

func TestStoreParticipants(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC().Add(time.Hour)
	reminder := &Reminder{
		Kind:      KindReminder,
		CreatorID: "111",
		ChannelID: "channel",
		Timezone:  "UTC",
		StartAt:   now,
		NextRunAt: now,
		Active:    true,
	}
	require.NoError(t, store.Create(reminder))

	_, added, err := store.AddParticipant(reminder.ID, "222")
	require.NoError(t, err)
	require.True(t, added)

	_, added, err = store.AddParticipant(reminder.ID, "222")
	require.NoError(t, err)
	require.False(t, added)

	_, removed, err := store.RemoveParticipant(reminder.ID, "222")
	require.NoError(t, err)
	require.True(t, removed)

	_, removed, err = store.RemoveParticipant(reminder.ID, "333")
	require.NoError(t, err)
	require.False(t, removed)
}

func TestStoreFindByMessageAndPrefix(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC().Add(time.Hour)
	reminder := &Reminder{
		Kind:                  KindReminder,
		CreatorID:             "111",
		ChannelID:             "channel",
		AnnouncementMessageID: "announcement",
		LastTriggerMessageID:  "trigger",
		Timezone:              "UTC",
		StartAt:               now,
		NextRunAt:             now,
		Active:                true,
	}
	require.NoError(t, store.Create(reminder))

	found, ok := store.FindByMessageID("announcement")
	require.True(t, ok)
	require.Equal(t, reminder.ID, found.ID)

	found, ok = store.FindByMessageID("trigger")
	require.True(t, ok)
	require.Equal(t, reminder.ID, found.ID)

	found, ok = store.FindByIDPrefix(reminder.ID[:6])
	require.True(t, ok)
	require.Equal(t, reminder.ID, found.ID)
}

func TestStoreDueAndCount(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	require.NoError(t, store.Create(&Reminder{
		Kind:      KindReminder,
		CreatorID: "111",
		ChannelID: "channel",
		Timezone:  "UTC",
		StartAt:   now.Add(-time.Minute),
		NextRunAt: now.Add(-time.Minute),
		Active:    true,
	}))
	require.NoError(t, store.Create(&Reminder{
		Kind:      KindReminder,
		CreatorID: "111",
		ChannelID: "channel",
		Timezone:  "UTC",
		StartAt:   now.Add(time.Hour),
		NextRunAt: now.Add(time.Hour),
		Active:    true,
	}))

	require.Len(t, store.Due(now), 1)
	require.Equal(t, 2, store.CountCreatedBy("111"))
}
