package reminders

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNextRunOneShot(t *testing.T) {
	now := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	start := time.Date(2026, 11, 22, 16, 0, 0, 0, time.UTC)

	next, ok := NextRun(now, start, nil, time.UTC)
	require.True(t, ok)
	require.Equal(t, start, next)

	_, ok = NextRun(now, start.Add(-48*time.Hour), nil, time.UTC)
	require.False(t, ok)
}

func TestNextRunDaily(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	start := time.Date(2026, 11, 21, 9, 0, 0, 0, loc)
	now := time.Date(2026, 11, 21, 10, 0, 0, 0, loc)

	next, ok := NextRun(now, start, &Recurrence{Freq: "daily", Interval: 1}, loc)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 11, 22, 9, 0, 0, 0, loc), next.In(loc))
}

func TestNextRunWeekly(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	start := time.Date(2026, 11, 16, 9, 0, 0, 0, loc) // Monday
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, loc)

	next, ok := NextRun(now, start, &Recurrence{Freq: "weekly", Interval: 1, Weekdays: []int{1}}, loc)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 11, 23, 9, 0, 0, 0, loc), next.In(loc))
}

func TestNextRunEveryOtherWeek(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	start := time.Date(2026, 11, 16, 9, 0, 0, 0, loc) // Monday
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, loc)

	next, ok := NextRun(now, start, &Recurrence{Freq: "weekly", Interval: 2, Weekdays: []int{1}}, loc)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 11, 30, 9, 0, 0, 0, loc), next.In(loc))
}

func TestNextRunWeekdays(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	start := time.Date(2026, 11, 16, 9, 0, 0, 0, loc) // Monday
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, loc)

	next, ok := NextRun(now, start, &Recurrence{Freq: "weekly", Interval: 1, Weekdays: []int{1, 3}}, loc)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 11, 18, 9, 0, 0, 0, loc), next.In(loc))
}

func TestNextRunMonthly(t *testing.T) {
	loc := time.UTC
	start := time.Date(2026, 1, 15, 9, 0, 0, 0, loc)
	now := time.Date(2026, 2, 1, 10, 0, 0, 0, loc)

	next, ok := NextRun(now, start, &Recurrence{Freq: "monthly", Interval: 1, MonthDay: 15}, loc)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 2, 15, 9, 0, 0, 0, loc), next.In(loc))
}

func TestNextRunKeepsLocalTimeAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	start := time.Date(2026, 3, 1, 9, 0, 0, 0, loc)
	now := time.Date(2026, 3, 7, 10, 0, 0, 0, loc)

	next, ok := NextRun(now, start, &Recurrence{Freq: "daily", Interval: 1}, loc)
	require.True(t, ok)
	require.Equal(t, 9, next.In(loc).Hour())
	require.Equal(t, 2026, next.In(loc).Year())
	require.Equal(t, time.March, next.In(loc).Month())
	require.Equal(t, 8, next.In(loc).Day())
}

func TestValidateRecurrence(t *testing.T) {
	require.NoError(t, ValidateRecurrence(nil))
	require.NoError(t, ValidateRecurrence(&Recurrence{Freq: "daily", Interval: 1}))
	require.Error(t, ValidateRecurrence(&Recurrence{Freq: "yearly", Interval: 1}))
	require.Error(t, ValidateRecurrence(&Recurrence{Freq: "weekly", Interval: 0}))
	require.Error(t, ValidateRecurrence(&Recurrence{Freq: "weekly", Interval: 1, Weekdays: []int{7}}))
}
