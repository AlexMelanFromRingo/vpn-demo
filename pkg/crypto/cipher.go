package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	// KeyRotationInterval is how often we rotate session keys (5 minutes).
	KeyRotationInterval = 5 * time.Minute
	// NonceSize is the size of the GCM nonce.
	NonceSize = 12
	// gcmTagSize is the size of the GCM authentication tag.
	gcmTagSize = 16
	// headerSize is the size of the per-message header: epoch(8) + nonce(12).
	headerSize = 8 + NonceSize
	// epochWindow is how many epochs away from "now" we are willing to cache a
	// derived key for. Packets in flight across an epoch boundary, plus a small
	// amount of clock skew between peers, must still decrypt — so we accept the
	// current epoch and its immediate neighbours.
	epochWindow = 1
)

// SessionCipher manages AES-256-GCM encryption with time-based key rotation.
//
// Both peers derive each session key independently from the shared secret and a
// time epoch (sessionKey = SHA256(sharedSecret || epoch)); the epoch is carried
// in every message so the receiver always knows which key to use without any
// clock synchronisation. Derived AEADs are cached per epoch so that the hot path
// never re-derives a key (the previous implementation ran SHA-256 + AES key
// schedule on *every* decrypted packet).
type SessionCipher struct {
	baseSecret [32]byte

	mu    sync.Mutex
	cache map[uint64]cipher.AEAD // epoch -> AEAD
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
// Output layout: [epoch(8) | nonce(12) | ciphertext | tag(16)].
func (sc *SessionCipher) Encrypt(plaintext []byte) ([]byte, error) {
	epoch := currentEpoch()
	aead, err := sc.aeadForEpoch(epoch, true)
	if err != nil {
		return nil, err
	}

	// Generate a random nonce. With a 96-bit random nonce and per-epoch keys the
	// number of messages under one key stays far below the GCM birthday bound.
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := aead.Seal(nil, nonce, plaintext, nil)

	result := make([]byte, headerSize+len(ciphertext))
	binary.BigEndian.PutUint64(result[0:8], epoch)
	copy(result[8:headerSize], nonce)
	copy(result[headerSize:], ciphertext)

	return result, nil
}

// Decrypt decrypts ciphertext produced by Encrypt, deriving the session key from
// the epoch embedded in the message.
func (sc *SessionCipher) Decrypt(data []byte) ([]byte, error) {
	if len(data) < headerSize+gcmTagSize {
		return nil, fmt.Errorf("ciphertext too short: %d", len(data))
	}

	epoch := binary.BigEndian.Uint64(data[0:8])

	// Only cache keys for epochs near the current time; far-away epochs (clock
	// skew beyond tolerance, or hostile packets) are still attempted but never
	// cached, bounding memory use.
	allowCache := epochDistance(epoch, currentEpoch()) <= epochWindow

	aead, err := sc.aeadForEpoch(epoch, allowCache)
	if err != nil {
		return nil, err
	}

	nonce := data[8:headerSize]
	ciphertext := data[headerSize:]

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// GetCurrentEpoch returns the current key epoch (for logging/debugging).
func (sc *SessionCipher) GetCurrentEpoch() uint64 {
	return currentEpoch()
}
