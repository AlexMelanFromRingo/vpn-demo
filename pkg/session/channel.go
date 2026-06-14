package session

import (
	"errors"
	"sync"
	"time"
)

// DefaultRekeyGrace is how long the previous session stays usable for DECRYPTION
// after a rehandshake, so packets already in flight under the old keys are not
// dropped while both peers switch over.
const DefaultRekeyGrace = 30 * time.Second

// Channel wraps the "current" transport Session plus, briefly after a
// rehandshake, the "previous" one. New traffic is always sent and received under
// the current session; the previous session is consulted only to decrypt
// in-flight packets during the grace window after a key rotation. This mirrors
// WireGuard keeping the prior keypair valid for a short period across a rekey.
type Channel struct {
	mu        sync.RWMutex
	current   *Session
	previous  *Session
	prevUntil time.Time
	grace     time.Duration
	now       func() time.Time
}

// NewChannel creates a Channel around an established session.
func NewChannel(initial *Session, grace time.Duration) *Channel {
	if grace <= 0 {
		grace = DefaultRekeyGrace
	}
	return &Channel{current: initial, grace: grace, now: time.Now}
}

// Encrypt seals a packet under the current session.
func (c *Channel) Encrypt(plaintext []byte) ([]byte, error) {
	c.mu.RLock()
	cur := c.current
	c.mu.RUnlock()
	return cur.Encrypt(plaintext)
}

// Decrypt opens a packet. It tries the current session first; if that fails to
// authenticate (e.g. the packet was sealed under the prior keys just before a
// rotation) and the previous session is still within its grace window, it falls
// back to the previous session. A genuine replay on the current session is
// reported as such and never retried against the previous one.
func (c *Channel) Decrypt(data []byte) ([]byte, error) {
	c.mu.RLock()
	cur := c.current
	prev := c.previous
	prevValid := prev != nil && c.now().Before(c.prevUntil)
	c.mu.RUnlock()

	pt, err := cur.Decrypt(data)
	if err == nil {
		return pt, nil
	}
	if errors.Is(err, ErrReplay) {
		return nil, err
	}
	if prevValid {
		if pt2, err2 := prev.Decrypt(data); err2 == nil {
			return pt2, nil
		}
	}
	return nil, err
}

// Rotate installs a freshly negotiated session as current and keeps the old one
// as previous for the grace window.
func (c *Channel) Rotate(next *Session) {
	c.mu.Lock()
	c.previous = c.current
	c.prevUntil = c.now().Add(c.grace)
	c.current = next
	c.mu.Unlock()
}
