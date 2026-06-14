// Package ratelimit provides a small, dependency-free token-bucket rate limiter.
package ratelimit

import (
	"sync"
	"time"
)

// TokenBucket is a classic token-bucket rate limiter. It refills at a fixed rate
// up to a burst capacity and is safe for concurrent use.
type TokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	capacity float64
	rate     float64 // tokens added per second
	last     time.Time
	now      func() time.Time // injectable clock for tests
}

// NewTokenBucket creates a limiter that allows up to ratePerSec events per
// second on average, with a burst of up to capacity events.
func NewTokenBucket(ratePerSec, capacity float64) *TokenBucket {
	return &TokenBucket{
		tokens:   capacity,
		capacity: capacity,
		rate:     ratePerSec,
		last:     time.Now(),
		now:      time.Now,
	}
}

// Allow reports whether one event may proceed now, consuming a token if so.
func (b *TokenBucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.rate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.last = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}
