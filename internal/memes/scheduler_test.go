package memes

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchedulerNextRun(t *testing.T) {
	scheduler, err := NewScheduler(nil, "0 15 * * 1,3,5", "UTC", time.Minute)
	require.NoError(t, err)

	next := scheduler.Next()
	require.False(t, next.IsZero())
	require.True(t, next.After(time.Now()))
	require.Equal(t, 15, next.Hour())
	require.Equal(t, 0, next.Minute())
	switch next.Weekday() {
	case time.Monday, time.Wednesday, time.Friday:
	default:
		t.Fatalf("unexpected weekday: %s", next.Weekday())
	}
}

func TestSchedulerTimezone(t *testing.T) {
	scheduler, err := NewScheduler(nil, "0 9 * * *", "Europe/Lisbon", time.Minute)
	require.NoError(t, err)
	require.Equal(t, "Europe/Lisbon", scheduler.Timezone())
	require.Equal(t, 9, scheduler.Next().Hour())
}

func TestSchedulerEmptyExpression(t *testing.T) {
	scheduler, err := NewScheduler(nil, "", "UTC", time.Minute)
	require.NoError(t, err)
	require.True(t, scheduler.Next().IsZero())
	require.Equal(t, "", scheduler.Expression())
}

func TestSchedulerRejectsInvalidExpression(t *testing.T) {
	_, err := NewScheduler(nil, "@every 1h", "UTC", time.Minute)
	require.Error(t, err)
}

func TestSchedulerRejectsInvalidTimezone(t *testing.T) {
	_, err := NewScheduler(nil, "0 15 * * *", "Mars/Olympus", time.Minute)
	require.Error(t, err)
}

func TestSchedulerStartStop(t *testing.T) {
	scheduler, err := NewScheduler(nil, "0 15 * * *", "UTC", time.Minute)
	require.NoError(t, err)
	scheduler.Start()
	scheduler.Start()
	scheduler.Stop()
	scheduler.Stop()
}
