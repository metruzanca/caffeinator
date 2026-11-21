package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLimiterAllowsCapacity(t *testing.T) {
	limiter := New(2, time.Minute)

	require.True(t, limiter.Allow("user"))
	require.True(t, limiter.Allow("user"))
	require.False(t, limiter.Allow("user"))
}

func TestLimiterIsPerKey(t *testing.T) {
	limiter := New(1, time.Minute)

	require.True(t, limiter.Allow("alice"))
	require.True(t, limiter.Allow("bob"))
	require.False(t, limiter.Allow("alice"))
}

func TestLimiterRefills(t *testing.T) {
	limiter := New(1, 10*time.Millisecond)

	require.True(t, limiter.Allow("user"))
	require.False(t, limiter.Allow("user"))

	time.Sleep(15 * time.Millisecond)

	require.True(t, limiter.Allow("user"))
}

func TestLimiterRetryAfter(t *testing.T) {
	limiter := New(1, time.Hour)

	require.Equal(t, time.Duration(0), limiter.RetryAfter("user"))

	require.True(t, limiter.Allow("user"))
	wait := limiter.RetryAfter("user")
	require.Greater(t, wait, time.Duration(0))
	require.LessOrEqual(t, wait, time.Hour)
}

func TestLimiterRetryAfterClearsOnRefill(t *testing.T) {
	limiter := New(1, 20*time.Millisecond)

	require.True(t, limiter.Allow("user"))
	require.Greater(t, limiter.RetryAfter("user"), time.Duration(0))

	time.Sleep(25 * time.Millisecond)

	require.Equal(t, time.Duration(0), limiter.RetryAfter("user"))
}
