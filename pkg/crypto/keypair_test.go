package crypto

import (
	"bytes"
	"testing"
)

// TestECDHAgreement verifies that two independently generated key pairs derive
// an identical shared secret — the core property the handshake relies on.
func TestECDHAgreement(t *testing.T) {
	alice, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("alice keygen: %v", err)
	}
	bob, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("bob keygen: %v", err)
	}

	aliceShared, err := alice.ComputeSharedSecret(bob.PublicKey)
	if err != nil {
		t.Fatalf("alice ECDH: %v", err)
	}
	bobShared, err := bob.ComputeSharedSecret(alice.PublicKey)
	if err != nil {
		t.Fatalf("bob ECDH: %v", err)
	}

	if !bytes.Equal(aliceShared[:], bobShared[:]) {
		t.Fatalf("shared secrets differ:\n alice=%x\n bob  =%x", aliceShared, bobShared)
	}

	var zero [32]byte
	if bytes.Equal(aliceShared[:], zero[:]) {
		t.Fatalf("shared secret is all-zero (degenerate)")
	}
}

// TestDifferentPairsDifferentSecret ensures distinct peers yield distinct secrets.
func TestDifferentPairsDifferentSecret(t *testing.T) {
	a, _ := GenerateKeyPair()
	b, _ := GenerateKeyPair()
	c, _ := GenerateKeyPair()

	ab, _ := a.ComputeSharedSecret(b.PublicKey)
	ac, _ := a.ComputeSharedSecret(c.PublicKey)

	if bytes.Equal(ab[:], ac[:]) {
		t.Fatalf("secret with different peers collided")
	}
}

// TestPublicKeyStringRoundTrip checks base64 encode/decode of public keys.
func TestPublicKeyStringRoundTrip(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	s := kp.PublicKeyToString()
	parsed, err := PublicKeyFromString(s)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed != kp.PublicKey {
		t.Fatalf("round-trip mismatch:\n want=%x\n got =%x", kp.PublicKey, parsed)
	}
}

func TestPublicKeyFromStringErrors(t *testing.T) {
	cases := map[string]string{
		"not base64":   "!!!!not-base64!!!!",
		"wrong length": "YWJj", // "abc" -> 3 bytes, not 32
		"empty":        "",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := PublicKeyFromString(in); err == nil {
				t.Fatalf("expected error for %q", in)
			}
		})
	}
}

func TestKeyPairUniqueness(t *testing.T) {
	seen := make(map[[32]byte]bool)
	for i := 0; i < 100; i++ {
		kp, err := GenerateKeyPair()
		if err != nil {
			t.Fatalf("keygen: %v", err)
		}
		if seen[kp.PublicKey] {
			t.Fatalf("duplicate public key generated at iteration %d", i)
		}
		seen[kp.PublicKey] = true
	}
}
