package reminders

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchedulerFiresOneShot(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	reminder := &Reminder{
		Kind:      KindReminder,
		CreatorID: "111",
		ChannelID: "channel",
		Timezone:  "UTC",
		StartAt:   now.Add(-time.Minute),
		NextRunAt: now.Add(-time.Minute),
		Active:    true,
	}
	require.NoError(t, store.Create(reminder))

	calls := 0
	scheduler := NewScheduler(store, func(ctx context.Context, reminder *Reminder) error {
		calls++
		return nil
	}, time.Minute, time.Hour)

	scheduler.tick(now)
	require.Equal(t, 1, calls)

	stored, ok := store.Get(reminder.ID)
	require.True(t, ok)
	require.False(t, stored.Active)
}

func TestSchedulerAdvancesRecurring(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	start := now.Add(-time.Minute)
	reminder := &Reminder{
		Kind:       KindReminder,
		CreatorID:  "111",
		ChannelID:  "channel",
		Timezone:   "UTC",
		StartAt:    start,
		NextRunAt:  start,
		Recurrence: &Recurrence{Freq: "daily", Interval: 1},
		Active:     true,
	}
	require.NoError(t, store.Create(reminder))

	calls := 0
	scheduler := NewScheduler(store, func(ctx context.Context, reminder *Reminder) error {
		calls++
		return nil
	}, time.Minute, time.Hour)

	scheduler.tick(now)
	require.Equal(t, 1, calls)

	stored, ok := store.Get(reminder.ID)
	require.True(t, ok)
	require.True(t, stored.Active)
	require.True(t, stored.NextRunAt.After(now))
}

func TestSchedulerSkipsMissedReminder(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	reminder := &Reminder{
		Kind:      KindReminder,
		CreatorID: "111",
		ChannelID: "channel",
		Timezone:  "UTC",
		StartAt:   now.Add(-2 * time.Hour),
		NextRunAt: now.Add(-2 * time.Hour),
		Active:    true,
	}
	require.NoError(t, store.Create(reminder))

	calls := 0
	scheduler := NewScheduler(store, func(ctx context.Context, reminder *Reminder) error {
		calls++
		return nil
	}, time.Minute, time.Hour)

	scheduler.tick(now)
	require.Equal(t, 0, calls)

	stored, ok := store.Get(reminder.ID)
	require.True(t, ok)
	require.False(t, stored.Active)
}
