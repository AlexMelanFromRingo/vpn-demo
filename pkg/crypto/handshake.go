package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
)

const (
	// HandshakePayloadSize is the fixed size of an authenticated handshake
	// payload: pubkey(32) || timestamp(8) || HMAC-SHA256(32).
	HandshakePayloadSize = 32 + 8 + 32
	// DefaultHandshakeSkewSec bounds how far a handshake timestamp may deviate
	// from local time; it limits replay of a captured handshake.
	DefaultHandshakeSkewSec = 60
)

// handshakeMAC computes HMAC-SHA256(psk, pubKey || timestamp).
func handshakeMAC(psk []byte, pubKey [32]byte, ts int64) []byte {
	var tsb [8]byte
	binary.BigEndian.PutUint64(tsb[:], uint64(ts))
	m := hmac.New(sha256.New, psk)
	m.Write(pubKey[:])
	m.Write(tsb[:])
	return m.Sum(nil)
}

// BuildHandshake builds an authenticated handshake payload binding the sender's
// public key and a timestamp under a pre-shared key:
//
//	pubkey(32) || timestamp(8, unix seconds) || HMAC-SHA256(psk, pubkey||ts)(32)
//
// When psk is empty the MAC carries no security (anyone can compute it), but the
// wire format stays uniform so a peer that *does* require a PSK will reject an
// empty-PSK peer (and vice versa) because the MACs will not match.
func BuildHandshake(psk []byte, pubKey [32]byte, nowUnix int64) []byte {
	out := make([]byte, HandshakePayloadSize)
	copy(out[0:32], pubKey[:])
	binary.BigEndian.PutUint64(out[32:40], uint64(nowUnix))
	copy(out[40:72], handshakeMAC(psk, pubKey, nowUnix))
	return out
}

// OpenHandshake verifies an authenticated handshake payload and returns the
// peer's public key. It verifies the PSK-keyed MAC in constant time and checks
// that the embedded timestamp is within maxSkewSec of nowUnix.
func OpenHandshake(psk, payload []byte, nowUnix, maxSkewSec int64) ([32]byte, error) {
	var pub [32]byte
	if len(payload) != HandshakePayloadSize {
		return pub, fmt.Errorf("invalid handshake size: %d (want %d)", len(payload), HandshakePayloadSize)
	}

	copy(pub[:], payload[0:32])
	ts := int64(binary.BigEndian.Uint64(payload[32:40]))
	mac := payload[40:72]

	expected := handshakeMAC(psk, pub, ts)
	if subtle.ConstantTimeCompare(mac, expected) != 1 {
		return pub, fmt.Errorf("handshake authentication failed (wrong or missing PSK)")
	}

	skew := nowUnix - ts
	if skew < 0 {
		skew = -skew
	}
	if skew > maxSkewSec {
		return pub, fmt.Errorf("handshake timestamp out of range (skew %ds > %ds)", skew, maxSkewSec)
	}

	return pub, nil
}
