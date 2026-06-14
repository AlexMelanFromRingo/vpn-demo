package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// KeyRotationInterval is how often we rotate session keys (5 minutes).
	KeyRotationInterval = 5 * time.Minute
	// NonceSize is the size of the GCM nonce.
	NonceSize = 12
	// gcmTagSize is the size of the GCM authentication tag.
	gcmTagSize = 16
	// seqSize is the size of the per-message sequence number.
	seqSize = 8
	// headerSize is the size of the per-message header: epoch(8) + seq(8).
	headerSize = 8 + seqSize
	// epochWindow is how many epochs away from "now" we are willing to cache a
	// derived key for. Packets in flight across an epoch boundary, plus a small
	// amount of clock skew between peers, must still decrypt — so we accept the
	// current epoch and its immediate neighbours.
	epochWindow = 1
)

// ErrReplay is returned by Decrypt when a packet's sequence number indicates a
// replay or a packet that has fallen behind the anti-replay window.
var ErrReplay = errors.New("replayed or stale packet rejected")

// SessionCipher manages AES-256-GCM encryption with time-based key rotation,
// deterministic (counter) nonces, and sliding-window replay protection.
//
// Both peers derive each session key independently from the shared secret and a
// time epoch (sessionKey = SHA256(sharedSecret || epoch)); the epoch is carried
// in every message so the receiver always knows which key to use without any
// clock synchronisation. Each message also carries a monotonic sequence number
// which (a) doubles as the GCM nonce — guaranteeing nonce uniqueness per key and
// sidestepping the random-nonce birthday bound — and (b) feeds the receiver's
// replay filter.
type SessionCipher struct {
	baseSecret [32]byte

	sendSeq uint64 // atomic; next sequence number is sendSeq+1

	mu    sync.Mutex
	cache map[uint64]cipher.AEAD // epoch -> AEAD

	replay replayWindow
}

// NewSessionCipher creates a new cipher with key rotation support.
func NewSessionCipher(sharedSecret [32]byte) (*SessionCipher, error) {
	sc := &SessionCipher{
		baseSecret: sharedSecret,
		cache:      make(map[uint64]cipher.AEAD),
	}

	// Pre-warm the current epoch so configuration errors surface immediately.
	if _, err := sc.aeadForEpoch(currentEpoch(), true); err != nil {
		return nil, err
	}

	return sc, nil
}

// epochSeconds is the rotation interval expressed in whole seconds.
func epochSeconds() uint64 {
	return uint64(KeyRotationInterval.Seconds())
}

// currentEpoch returns the key epoch for the current wall-clock time.
func currentEpoch() uint64 {
	return uint64(time.Now().Unix()) / epochSeconds()
}

// epochDistance returns the absolute difference between two epochs.
func epochDistance(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return b - a
}

// nonceForSeq builds the deterministic 96-bit GCM nonce for a sequence number.
// Within a single epoch key, sequence numbers are unique and monotonic, so the
// resulting nonces never repeat under the same key.
func nonceForSeq(seq uint64) []byte {
	nonce := make([]byte, NonceSize)
	binary.BigEndian.PutUint64(nonce[NonceSize-seqSize:], seq)
	return nonce
}

// deriveAEAD builds a fresh AES-256-GCM AEAD for the given epoch.
func (sc *SessionCipher) deriveAEAD(epoch uint64) (cipher.AEAD, error) {
	h := sha256.New()
	h.Write(sc.baseSecret[:])
	_ = binary.Write(h, binary.BigEndian, epoch)
	sessionKey := h.Sum(nil) // 32 bytes for AES-256

	block, err := aes.NewCipher(sessionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return aead, nil
}

// aeadForEpoch returns the AEAD for an epoch, deriving and (optionally) caching
// it. allowCache is false for epochs far from "now" so that an attacker who
// sprays packets with arbitrary epoch values cannot grow the cache without
// bound — those packets are still processed, just never cached.
func (sc *SessionCipher) aeadForEpoch(epoch uint64, allowCache bool) (cipher.AEAD, error) {
	sc.mu.Lock()
	if aead, ok := sc.cache[epoch]; ok {
		sc.mu.Unlock()
		return aead, nil
	}
	sc.mu.Unlock()

	aead, err := sc.deriveAEAD(epoch)
	if err != nil {
		return nil, err
	}

	if allowCache {
		sc.mu.Lock()
		sc.cache[epoch] = aead
		// Evict any cached epochs that have fallen outside the window.
		for e := range sc.cache {
			if epochDistance(e, epoch) > epochWindow {
				delete(sc.cache, e)
			}
		}
		sc.mu.Unlock()
	}

	return aead, nil
}

// Encrypt encrypts plaintext using AES-256-GCM with automatic key rotation.
// Output layout: [epoch(8) | seq(8) | ciphertext | tag(16)].
func (sc *SessionCipher) Encrypt(plaintext []byte) ([]byte, error) {
	epoch := currentEpoch()
	aead, err := sc.aeadForEpoch(epoch, true)
	if err != nil {
		return nil, err
	}

	seq := atomic.AddUint64(&sc.sendSeq, 1)
	nonce := nonceForSeq(seq)

	ciphertext := aead.Seal(nil, nonce, plaintext, nil)

	result := make([]byte, headerSize+len(ciphertext))
	binary.BigEndian.PutUint64(result[0:8], epoch)
	binary.BigEndian.PutUint64(result[8:headerSize], seq)
	copy(result[headerSize:], ciphertext)

	return result, nil
}

// Decrypt decrypts ciphertext produced by Encrypt, deriving the session key from
// the epoch embedded in the message and rejecting replayed/stale packets.
func (sc *SessionCipher) Decrypt(data []byte) ([]byte, error) {
	if len(data) < headerSize+gcmTagSize {
		return nil, fmt.Errorf("ciphertext too short: %d", len(data))
	}

	epoch := binary.BigEndian.Uint64(data[0:8])
	seq := binary.BigEndian.Uint64(data[8:headerSize])

	// Only cache keys for epochs near the current time; far-away epochs (clock
	// skew beyond tolerance, or hostile packets) are still attempted but never
	// cached, bounding memory use.
	allowCache := epochDistance(epoch, currentEpoch()) <= epochWindow

	aead, err := sc.aeadForEpoch(epoch, allowCache)
	if err != nil {
		return nil, err
	}

	nonce := nonceForSeq(seq)
	ciphertext := data[headerSize:]

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	// Only after the packet is authenticated do we touch the replay window, so a
	// forged sequence number can never advance it.
	if !sc.replay.accept(seq) {
		return nil, ErrReplay
	}

	return plaintext, nil
}

// GetCurrentEpoch returns the current key epoch (for logging/debugging).
func (sc *SessionCipher) GetCurrentEpoch() uint64 {
	return currentEpoch()
}
