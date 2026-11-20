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
