package session

import "sync"

// replayWindowBits is the size of the anti-replay sliding window, in packets.
// A packet whose sequence number is more than this many positions behind the
// highest accepted sequence number is rejected as too old.
const replayWindowBits = 1024

// replayWindow implements an IPsec/WireGuard-style sliding-window replay filter
// (RFC 6479). It tracks the highest accepted sequence number plus a bitmap of
// recently seen lower sequence numbers, so it rejects both duplicates and
// packets that fall behind the window, while still accepting in-order and
// modestly reordered traffic.
type replayWindow struct {
	mu      sync.Mutex
	last    uint64
	bitmap  [replayWindowBits / 64]uint64
	started bool
}

func (w *replayWindow) bit(seq uint64) (word int, mask uint64) {
	pos := seq % replayWindowBits
	return int(pos / 64), uint64(1) << (pos % 64)
}

func (w *replayWindow) set(seq uint64)   { i, m := w.bit(seq); w.bitmap[i] |= m }
func (w *replayWindow) clear(seq uint64) { i, m := w.bit(seq); w.bitmap[i] &^= m }
func (w *replayWindow) get(seq uint64) bool {
	i, m := w.bit(seq)
	return w.bitmap[i]&m != 0
}

// accept reports whether seq is fresh (neither a replay nor too old) and, if so,
// records it as seen. It must only be called for packets that have already been
// authenticated, so that a forged sequence number cannot advance the window.
func (w *replayWindow) accept(seq uint64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	// First authenticated packet establishes the window.
	if !w.started {
		w.started = true
		w.last = seq
		w.set(seq)
		return true
	}

	if seq > w.last {
		// Advance the window: clear the positions we slide over so they start
		// fresh, then record the new high-water mark.
		diff := seq - w.last
		if diff >= replayWindowBits {
			for i := range w.bitmap {
				w.bitmap[i] = 0
			}
		} else {
			for s := w.last + 1; s < seq; s++ {
				w.clear(s)
			}
		}
		w.last = seq
		w.set(seq)
		return true
	}

	// seq <= last: within or behind the window.
	if w.last-seq >= replayWindowBits {
		return false // too old
	}
	if w.get(seq) {
		return false // duplicate / replay
	}
	w.set(seq)
	return true
}
