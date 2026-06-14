package crypto

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
)

func newCipherPair(t *testing.T) (*SessionCipher, *SessionCipher) {
	t.Helper()
	alice, _ := GenerateKeyPair()
	bob, _ := GenerateKeyPair()
	as, _ := alice.ComputeSharedSecret(bob.PublicKey)
	bs, _ := bob.ComputeSharedSecret(alice.PublicKey)

	ac, err := NewSessionCipher(as)
	if err != nil {
		t.Fatalf("alice cipher: %v", err)
	}
	bc, err := NewSessionCipher(bs)
	if err != nil {
		t.Fatalf("bob cipher: %v", err)
	}
	return ac, bc
}

// TestEncryptDecryptInterop models the real client/server: one side encrypts,
// the other decrypts using its independently derived key.
func TestEncryptDecryptInterop(t *testing.T) {
	client, server := newCipherPair(t)

	for _, size := range []int{0, 1, 20, 64, 1420, 1500, 4096} {
		plain := bytes.Repeat([]byte{0xAB}, size)
		ct, err := client.Encrypt(plain)
		if err != nil {
			t.Fatalf("encrypt size=%d: %v", size, err)
		}
		got, err := server.Decrypt(ct)
		if err != nil {
			t.Fatalf("decrypt size=%d: %v", size, err)
		}
		if !bytes.Equal(got, plain) {
			t.Fatalf("round-trip mismatch size=%d", size)
		}
	}
}

// TestEncryptProducesDistinctCiphertexts ensures the random nonce makes repeated
// encryptions of the same plaintext differ (no deterministic leakage).
func TestEncryptProducesDistinctCiphertexts(t *testing.T) {
	c, _ := newCipherPair(t)
	plain := []byte("the same message every time")
	a, _ := c.Encrypt(plain)
	b, _ := c.Encrypt(plain)
	if bytes.Equal(a, b) {
		t.Fatalf("two encryptions of identical plaintext were identical (nonce reuse?)")
	}
}

// TestEncryptHeaderHasEpoch checks the wire layout: first 8 bytes are the epoch.
func TestEncryptHeaderHasEpoch(t *testing.T) {
	c, _ := newCipherPair(t)
	ct, err := c.Encrypt([]byte("hi"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if len(ct) < headerSize+gcmTagSize {
		t.Fatalf("ciphertext too short: %d", len(ct))
	}
	gotEpoch := binary.BigEndian.Uint64(ct[0:8])
	if gotEpoch != currentEpoch() {
		t.Fatalf("epoch in header = %d, want %d", gotEpoch, currentEpoch())
	}
}

// TestTamperDetected verifies GCM authentication rejects modified ciphertext.
func TestTamperDetected(t *testing.T) {
	client, server := newCipherPair(t)
	ct, _ := client.Encrypt([]byte("authenticate me"))

	// Flip a bit in the ciphertext body (after the epoch+nonce header).
	tampered := append([]byte(nil), ct...)
	tampered[len(tampered)-1] ^= 0x01
	if _, err := server.Decrypt(tampered); err == nil {
		t.Fatalf("tampered ciphertext was accepted")
	}

	// Flipping the nonce must also fail authentication.
	tampered2 := append([]byte(nil), ct...)
	tampered2[10] ^= 0xFF
	if _, err := server.Decrypt(tampered2); err == nil {
		t.Fatalf("nonce-tampered ciphertext was accepted")
	}
}

// TestWrongKeyFails ensures a cipher with a different secret cannot decrypt.
func TestWrongKeyFails(t *testing.T) {
	client, _ := newCipherPair(t)
	var bogusSecret [32]byte
	bogusSecret[0] = 0xFF
	stranger, _ := NewSessionCipher(bogusSecret)

	ct, _ := client.Encrypt([]byte("secret"))
	if _, err := stranger.Decrypt(ct); err == nil {
		t.Fatalf("decryption with wrong key succeeded")
	}
}

func TestDecryptShortInput(t *testing.T) {
	c, _ := newCipherPair(t)
	for _, n := range []int{0, 1, headerSize, headerSize + gcmTagSize - 1} {
		if _, err := c.Decrypt(make([]byte, n)); err == nil {
			t.Fatalf("expected error for short input len=%d", n)
		}
	}
}

// TestEpochKeyDerivationDeterministic verifies both peers derive the same key
// for a given epoch (so a packet stamped with epoch E always decrypts).
func TestEpochKeyDerivationDeterministic(t *testing.T) {
	var secret [32]byte
	secret[1] = 0x42
	a, _ := NewSessionCipher(secret)
	b, _ := NewSessionCipher(secret)

	const epoch = uint64(12345)
	aeadA, err := a.deriveAEAD(epoch)
	if err != nil {
		t.Fatal(err)
	}
	aeadB, err := b.deriveAEAD(epoch)
	if err != nil {
		t.Fatal(err)
	}

	nonce := make([]byte, NonceSize)
	sealed := aeadA.Seal(nil, nonce, []byte("payload"), nil)
	opened, err := aeadB.Open(nil, nonce, sealed, nil)
	if err != nil {
		t.Fatalf("cross-derived key failed to decrypt: %v", err)
	}
	if string(opened) != "payload" {
		t.Fatalf("got %q", opened)
	}
}

// TestAEADCacheBounded ensures decrypting far-future/forged epochs does not grow
// the cache without bound (DoS resistance).
func TestAEADCacheBounded(t *testing.T) {
	var secret [32]byte
	c, _ := NewSessionCipher(secret)

	// Forge ciphertexts stamped with wildly different epochs. They will fail to
	// authenticate, but the point is that out-of-window epochs are not cached.
	for i := uint64(0); i < 1000; i++ {
		data := make([]byte, headerSize+gcmTagSize)
		binary.BigEndian.PutUint64(data[0:8], 1_000_000+i*1000)
		_, _ = c.Decrypt(data) // expected to fail auth; we only care about caching
	}

	c.mu.Lock()
	n := len(c.cache)
	c.mu.Unlock()
	if n > 2*epochWindow+2 {
		t.Fatalf("cache grew unbounded: %d entries", n)
	}
}

// TestConcurrentEncryptDecrypt exercises the cipher from many goroutines to
// surface data races under `go test -race`.
func TestConcurrentEncryptDecrypt(t *testing.T) {
	client, server := newCipherPair(t)
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			msg := bytes.Repeat([]byte{byte(g)}, 100)
			for i := 0; i < 200; i++ {
				ct, err := client.Encrypt(msg)
				if err != nil {
					t.Errorf("encrypt: %v", err)
					return
				}
				got, err := server.Decrypt(ct)
				if err != nil {
					t.Errorf("decrypt: %v", err)
					return
				}
				if !bytes.Equal(got, msg) {
					t.Errorf("mismatch in goroutine %d", g)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

func BenchmarkEncrypt(b *testing.B) {
	var secret [32]byte
	c, _ := NewSessionCipher(secret)
	plain := bytes.Repeat([]byte{0xCD}, 1400)
	b.SetBytes(1400)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Encrypt(plain); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecrypt(b *testing.B) {
	var secret [32]byte
	c, _ := NewSessionCipher(secret)
	plain := bytes.Repeat([]byte{0xCD}, 1400)
	ct, _ := c.Encrypt(plain)
	b.SetBytes(1400)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Decrypt(ct); err != nil {
			b.Fatal(err)
		}
	}
}
