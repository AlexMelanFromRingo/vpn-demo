package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/curve25519"
)

// KeyPair represents a Curve25519 key pair for ECDH
type KeyPair struct {
	PrivateKey [32]byte
	PublicKey  [32]byte
}

// GenerateKeyPair creates a new Curve25519 key pair
func GenerateKeyPair() (*KeyPair, error) {
	kp := &KeyPair{}

	// Generate random private key
	if _, err := io.ReadFull(rand.Reader, kp.PrivateKey[:]); err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Compute public key from private key
	curve25519.ScalarBaseMult(&kp.PublicKey, &kp.PrivateKey)

	return kp, nil
}

// ComputeSharedSecret performs ECDH to compute shared secret
func (kp *KeyPair) ComputeSharedSecret(peerPublicKey [32]byte) ([32]byte, error) {
	var sharedSecret [32]byte
	curve25519.ScalarMult(&sharedSecret, &kp.PrivateKey, &peerPublicKey)
	return sharedSecret, nil
}

// PublicKeyToString converts public key to base64 string
func (kp *KeyPair) PublicKeyToString() string {
	return base64.StdEncoding.EncodeToString(kp.PublicKey[:])
}

// PublicKeyFromString parses base64 public key
func PublicKeyFromString(s string) ([32]byte, error) {
	var key [32]byte
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return key, fmt.Errorf("failed to decode public key: %w", err)
	}
	if len(decoded) != 32 {
		return key, fmt.Errorf("invalid public key length: %d", len(decoded))
	}
	copy(key[:], decoded)
	return key, nil
}
