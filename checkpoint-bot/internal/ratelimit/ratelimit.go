// Package ratelimit provides a small in-memory token bucket limiter.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a token bucket rate limiter keyed by an arbitrary string.
// It is safe for concurrent use.
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	capacity int
	refill   time.Duration
	window   time.Duration
}

type bucket struct {
	tokens     int
	lastRefill time.Time
	lastReset  time.Time
}

// New creates a limiter that allows capacity requests per window.
func New(capacity int, window time.Duration) *Limiter {
	if capacity < 1 {
		capacity = 1
	}
	if window <= 0 {
		window = time.Minute
	}

	return &Limiter{
		buckets:  make(map[string]*bucket),
		capacity: capacity,
		refill:   window / time.Duration(capacity),
		window:   window,
	}
}

// Allow reports whether the key may make a request now.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, exists := l.buckets[key]
	if !exists {
		l.buckets[key] = &bucket{
			tokens:     l.capacity - 1,
			lastRefill: now,
			lastReset:  now,
		}
		return true
	}

	// Drop idle buckets so the map does not grow forever.
	if now.Sub(b.lastReset) > l.window*2 {
		delete(l.buckets, key)
		l.buckets[key] = &bucket{
			tokens:     l.capacity - 1,
			lastRefill: now,
			lastReset:  now,
		}
		return true
	}

	if elapsed := now.Sub(b.lastRefill); elapsed >= l.refill {
		tokensToAdd := int(elapsed / l.refill)
		b.tokens = min(l.capacity, b.tokens+tokensToAdd)
		b.lastRefill = now
	}

	if b.tokens > 0 {
		b.tokens--
		return true
	}

	return false
}

// RetryAfter reports how long until key may make another request.
// It returns zero when the key can request now.
func (l *Limiter) RetryAfter(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, exists := l.buckets[key]
	if !exists {
		return 0
	}

	elapsed := time.Since(b.lastRefill)
	tokens := b.tokens
	if elapsed >= l.refill {
		tokens = min(l.capacity, tokens+int(elapsed/l.refill))
	}
	if tokens > 0 {
		return 0
	}

	wait := l.refill - elapsed%l.refill
	if wait <= 0 || wait > l.refill {
		wait = l.refill
	}
	return wait
}
