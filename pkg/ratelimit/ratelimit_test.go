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
