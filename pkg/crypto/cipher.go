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
	// KeyRotationInterval is how often we rotate session keys (5 minutes)
	KeyRotationInterval = 5 * time.Minute
	// NonceSize is the size of GCM nonce
	NonceSize = 12
)

// SessionCipher manages AES-256-GCM encryption with key rotation
type SessionCipher struct {
	baseSecret     [32]byte
	currentKey     []byte
	currentAEAD    cipher.AEAD
	keyEpoch       uint64
	mu             sync.RWMutex
	lastRotation   time.Time
}

// NewSessionCipher creates a new cipher with key rotation support
func NewSessionCipher(sharedSecret [32]byte) (*SessionCipher, error) {
	sc := &SessionCipher{
		baseSecret:   sharedSecret,
		lastRotation: time.Now(),
	}

	// Initialize with first key
	if err := sc.rotateKey(); err != nil {
		return nil, err
	}

	return sc, nil
}

// rotateKey generates a new session key based on time epoch
func (sc *SessionCipher) rotateKey() error {
	// Calculate current epoch (time-based, similar to TOTP but for keys)
	currentEpoch := uint64(time.Now().Unix()) / uint64(KeyRotationInterval.Seconds())

	if currentEpoch == sc.keyEpoch {
		return nil // Key is still valid
	}

	// Derive new key from base secret + epoch
	// This allows both sides to independently derive the same key at the same time
	h := sha256.New()
	h.Write(sc.baseSecret[:])
	binary.Write(h, binary.BigEndian, currentEpoch)
	sessionKey := h.Sum(nil) // 32 bytes for AES-256

	// Create new AES-GCM cipher
	block, err := aes.NewCipher(sessionKey)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	sc.mu.Lock()
	sc.currentKey = sessionKey
	sc.currentAEAD = aead
	sc.keyEpoch = currentEpoch
	sc.lastRotation = time.Now()
	sc.mu.Unlock()

	return nil
}

// Encrypt encrypts plaintext using AES-256-GCM with automatic key rotation
func (sc *SessionCipher) Encrypt(plaintext []byte) ([]byte, error) {
	// Check if we need to rotate key
	if time.Since(sc.lastRotation) > KeyRotationInterval {
		if err := sc.rotateKey(); err != nil {
			return nil, err
		}
	}

	sc.mu.RLock()
	aead := sc.currentAEAD
	epoch := sc.keyEpoch
	sc.mu.RUnlock()

	// Generate random nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt: [epoch(8) | nonce(12) | ciphertext | tag(16)]
	ciphertext := aead.Seal(nil, nonce, plaintext, nil)

	// Prepend epoch and nonce
	result := make([]byte, 8+NonceSize+len(ciphertext))
	binary.BigEndian.PutUint64(result[0:8], epoch)
	copy(result[8:8+NonceSize], nonce)
	copy(result[8+NonceSize:], ciphertext)

	return result, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM with automatic key rotation
func (sc *SessionCipher) Decrypt(data []byte) ([]byte, error) {
	if len(data) < 8+NonceSize+aes.BlockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract epoch
	epoch := binary.BigEndian.Uint64(data[0:8])

	// Derive key for this epoch
	h := sha256.New()
	h.Write(sc.baseSecret[:])
	binary.Write(h, binary.BigEndian, epoch)
	sessionKey := h.Sum(nil)

	block, err := aes.NewCipher(sessionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Extract nonce and ciphertext
	nonce := data[8 : 8+NonceSize]
	ciphertext := data[8+NonceSize:]

	// Decrypt
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// GetCurrentEpoch returns the current key epoch for debugging
func (sc *SessionCipher) GetCurrentEpoch() uint64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.keyEpoch
}
