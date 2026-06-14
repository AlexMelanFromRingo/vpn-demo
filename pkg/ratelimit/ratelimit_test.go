package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestTokenBucketBurst(t *testing.T) {
	b := NewTokenBucket(1, 5)
	// Freeze time so no refill happens during the burst.
	frozen := time.Now()
	b.now = func() time.Time { return frozen }

	allowed := 0
	for i := 0; i < 10; i++ {
		if b.Allow() {
			allowed++
		}
	}
	if allowed != 5 {
		t.Fatalf("burst allowed %d, want 5", allowed)
	}
}

func TestTokenBucketRefill(t *testing.T) {
	b := NewTokenBucket(10, 1) // 10/sec, burst 1
	now := time.Now()
	b.now = func() time.Time { return now }

	if !b.Allow() {
		t.Fatal("first event should be allowed")
	}
	if b.Allow() {
		t.Fatal("second immediate event should be denied")
	}

	// Advance 200ms => 2 tokens refilled (capped at burst=1).
	now = now.Add(200 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("event after refill should be allowed")
	}
}

func TestIPLimiterIsolatesSources(t *testing.T) {
	l := NewIPLimiter(1, 3, 100)
	frozen := time.Now()
	l.now = func() time.Time { return frozen }

	// IP "A" burns its whole burst...
	aAllowed := 0
	for i := 0; i < 10; i++ {
		if l.Allow("A") {
			aAllowed++
		}
	}
	if aAllowed != 3 {
		t.Fatalf("A allowed %d, want 3", aAllowed)
	}
	// ...but a different source "B" is unaffected.
	if !l.Allow("B") {
		t.Fatal("B should be allowed despite A being throttled")
	}
}

func TestIPLimiterEvictsWhenFull(t *testing.T) {
	l := NewIPLimiter(1, 1, 2) // track at most 2 sources
	frozen := time.Now()
	l.now = func() time.Time { return frozen }

	l.Allow("A")
	frozen = frozen.Add(time.Millisecond)
	l.Allow("B")
	frozen = frozen.Add(time.Millisecond)
	l.Allow("C") // should evict the oldest (A)

	l.mu.Lock()
	n := len(l.buckets)
	_, hasA := l.buckets["A"]
	l.mu.Unlock()
	if n != 2 {
		t.Fatalf("tracked %d sources, want 2", n)
	}
	if hasA {
		t.Fatal("oldest source A should have been evicted")
	}
}

func TestIPLimiterConcurrent(t *testing.T) {
	l := NewIPLimiter(1000, 100, 1000)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := string(rune('A' + i%5))
			for j := 0; j < 100; j++ {
				l.Allow(key)
			}
		}(i)
	}
	wg.Wait() // -race + no panic
}

func TestTokenBucketConcurrent(t *testing.T) {
	b := NewTokenBucket(1000, 100)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				b.Allow()
			}
		}()
	}
	wg.Wait() // race detector + no panic is the assertion
}
