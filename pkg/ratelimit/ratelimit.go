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

// IPLimiter applies an independent token bucket per source key (typically a
// source IP), so a single noisy source cannot exhaust the budget of others. The
// number of tracked sources is bounded to prevent a spoofed-source flood from
// growing memory without limit: when the table is full, the least-recently-seen
// entry is evicted.
type IPLimiter struct {
	mu      sync.Mutex
	buckets map[string]*ipEntry
	rate    float64
	burst   float64
	maxKeys int
	now     func() time.Time
}

type ipEntry struct {
	bucket   *TokenBucket
	lastSeen time.Time
}

// NewIPLimiter creates a per-source limiter allowing ratePerSec/burst per key,
// tracking at most maxKeys distinct sources.
func NewIPLimiter(ratePerSec, burst float64, maxKeys int) *IPLimiter {
	if maxKeys < 1 {
		maxKeys = 1
	}
	return &IPLimiter{
		buckets: make(map[string]*ipEntry),
		rate:    ratePerSec,
		burst:   burst,
		maxKeys: maxKeys,
		now:     time.Now,
	}
}

// Allow reports whether an event from the given source key may proceed.
func (l *IPLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	e, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.maxKeys {
			l.evictOldestLocked()
		}
		b := NewTokenBucket(l.rate, l.burst)
		b.now = l.now
		e = &ipEntry{bucket: b}
		l.buckets[key] = e
	}
	e.lastSeen = now
	return e.bucket.Allow()
}

// evictOldestLocked removes the least-recently-seen entry. Caller holds l.mu.
func (l *IPLimiter) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	first := true
	for k, e := range l.buckets {
		if first || e.lastSeen.Before(oldest) {
			oldest, oldestKey, first = e.lastSeen, k, false
		}
	}
	if !first {
		delete(l.buckets, oldestKey)
	}
}
