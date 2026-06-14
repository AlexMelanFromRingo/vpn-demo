package crypto

import "testing"

func TestHandshakeRoundTrip(t *testing.T) {
	psk := []byte("correct horse battery staple")
	kp, _ := GenerateKeyPair()
	const now = int64(1_700_000_000)

	payload := BuildHandshake(psk, kp.PublicKey, now)
	if len(payload) != HandshakePayloadSize {
		t.Fatalf("payload size %d != %d", len(payload), HandshakePayloadSize)
	}

	got, err := OpenHandshake(psk, payload, now, DefaultHandshakeSkewSec)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got != kp.PublicKey {
		t.Fatalf("pubkey mismatch")
	}
}

func TestHandshakeWrongPSK(t *testing.T) {
	kp, _ := GenerateKeyPair()
	const now = int64(1_700_000_000)
	payload := BuildHandshake([]byte("secret-A"), kp.PublicKey, now)

	if _, err := OpenHandshake([]byte("secret-B"), payload, now, DefaultHandshakeSkewSec); err == nil {
		t.Fatal("handshake accepted with wrong PSK")
	}
}

// A peer requiring a PSK must reject a peer that sent none (empty PSK), and vice
// versa — the uniform format means mismatched PSK config never interoperates.
func TestHandshakeMissingPSKRejected(t *testing.T) {
	kp, _ := GenerateKeyPair()
	const now = int64(1_700_000_000)

	noPSK := BuildHandshake(nil, kp.PublicKey, now)
	if _, err := OpenHandshake([]byte("required"), noPSK, now, DefaultHandshakeSkewSec); err == nil {
		t.Fatal("server requiring PSK accepted an unauthenticated handshake")
	}

	withPSK := BuildHandshake([]byte("required"), kp.PublicKey, now)
	if _, err := OpenHandshake(nil, withPSK, now, DefaultHandshakeSkewSec); err == nil {
		t.Fatal("server with no PSK accepted a PSK-authenticated handshake from a different config")
	}
}

func TestHandshakeTimestampSkew(t *testing.T) {
	psk := []byte("k")
	kp, _ := GenerateKeyPair()
	const now = int64(1_700_000_000)
	payload := BuildHandshake(psk, kp.PublicKey, now)

	// 61s in the past with a 60s tolerance must be rejected (replay protection).
	if _, err := OpenHandshake(psk, payload, now+61, 60); err == nil {
		t.Fatal("stale handshake accepted")
	}
	// Far future likewise.
	if _, err := OpenHandshake(psk, payload, now-61, 60); err == nil {
		t.Fatal("future handshake accepted")
	}
	// Just within tolerance is fine.
	if _, err := OpenHandshake(psk, payload, now+59, 60); err != nil {
		t.Fatalf("in-tolerance handshake rejected: %v", err)
	}
}

func TestHandshakeTamperedPubKey(t *testing.T) {
	psk := []byte("k")
	kp, _ := GenerateKeyPair()
	const now = int64(1_700_000_000)
	payload := BuildHandshake(psk, kp.PublicKey, now)

	// Flip a bit in the public key; the MAC must no longer verify (anti-MITM).
	payload[0] ^= 0x01
	if _, err := OpenHandshake(psk, payload, now, DefaultHandshakeSkewSec); err == nil {
		t.Fatal("tampered public key accepted")
	}
}

func TestHandshakeWrongSize(t *testing.T) {
	if _, err := OpenHandshake([]byte("k"), make([]byte, 10), 0, 60); err == nil {
		t.Fatal("short payload accepted")
	}
}
