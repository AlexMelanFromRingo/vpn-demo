package session

import (
	"sync"
	"testing"
)

func TestReplayWindowInOrder(t *testing.T) {
	var w replayWindow
	for seq := uint64(1); seq <= 5000; seq++ {
		if !w.accept(seq) {
			t.Fatalf("in-order seq %d rejected", seq)
		}
	}
}

func TestReplayWindowDuplicate(t *testing.T) {
	var w replayWindow
	if !w.accept(10) {
		t.Fatal("first accept failed")
	}
	if w.accept(10) {
		t.Fatal("duplicate accepted")
	}
}

func TestReplayWindowReorderWithinWindow(t *testing.T) {
	var w replayWindow
	// Establish a high-water mark, then deliver earlier (but in-window) seqs.
	if !w.accept(1000) {
		t.Fatal("seed accept failed")
	}
	for _, seq := range []uint64{995, 990, 999, 500, 1001} {
		if !w.accept(seq) {
			t.Fatalf("in-window seq %d rejected", seq)
		}
		if w.accept(seq) {
			t.Fatalf("duplicate of %d accepted", seq)
		}
	}
}

func TestReplayWindowTooOld(t *testing.T) {
	var w replayWindow
	if !w.accept(replayWindowBits + 100) {
		t.Fatal("seed accept failed")
	}
	// Anything more than the window behind the high-water mark is stale.
	if w.accept(50) {
		t.Fatal("stale packet accepted")
	}
}

func TestReplayWindowLargeJump(t *testing.T) {
	var w replayWindow
	if !w.accept(1) {
		t.Fatal("seed accept failed")
	}
	// Jump far ahead, clearing the whole window.
	big := uint64(10 * replayWindowBits)
	if !w.accept(big) {
		t.Fatal("forward jump rejected")
	}
	if w.accept(big) {
		t.Fatal("duplicate after jump accepted")
	}
	// The old low seqs are now far behind the window and must be stale.
	if w.accept(2) {
		t.Fatal("ancient packet accepted after jump")
	}
}

func TestReplayWindowConcurrent(t *testing.T) {
	var w replayWindow
	const n = 4000
	results := make([]bool, n+1)
	var wg sync.WaitGroup
	// Each unique seq accepted by exactly one caller; run concurrently for -race.
	for seq := 1; seq <= n; seq++ {
		wg.Add(1)
		go func(s uint64) {
			defer wg.Done()
			results[s] = w.accept(s)
		}(uint64(seq))
	}
	wg.Wait()

	// Every seq within the final window must have been accepted exactly once;
	// seqs that ended up far behind the high-water mark may be stale. We only
	// assert no panics/races and that re-accepting the high-water mark fails.
	if w.accept(w.last) {
		t.Fatal("high-water mark re-accepted (replay)")
	}
}
