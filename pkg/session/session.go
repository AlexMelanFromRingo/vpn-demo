// Package session implements the VPN's authenticated key agreement and
// transport encryption on top of the Noise Protocol Framework (the same
// framework WireGuard uses).
//
// Handshake: Noise_IK_25519_ChaChaPoly_BLAKE2s. Each peer owns a long-term
// static Curve25519 identity ("certificate"). The initiator (client) knows the
// responder's (server's) static public key in advance and pins it, which
// authenticates the server and defeats a man-in-the-middle. The initiator's
// static key is delivered to the responder inside the encrypted handshake, so
// the server learns and can authorise the client's identity (an allowlist of
// public keys replaces the shared PSK).
//
// Transport: the Noise handshake yields two symmetric CipherStates (one per
// direction). Because the underlying UDP transport reorders and drops packets,
// each data frame carries an explicit 64-bit counter that is used as the cipher
// nonce, and the receiver runs a sliding-window replay filter — exactly the
// approach WireGuard takes.
package session

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/flynn/noise"
	"golang.org/x/crypto/curve25519"
)

// cipherSuite is WireGuard's primitive set: Curve25519 + ChaCha20-Poly1305 + BLAKE2s.
var cipherSuite = noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashBLAKE2s)

// handshakePattern is Noise_IK: the initiator knows the responder's static key.
var handshakePattern = noise.HandshakeIK

const (
	// KeySize is the size of a Curve25519 key.
	KeySize = 32
	// seqSize is the size of the per-frame counter prepended to each ciphertext.
	seqSize = 8
	// tagSize is the AEAD authentication tag size.
	tagSize = 16
)

// ErrReplay is returned when a frame's counter indicates a replay or a packet
// that has fallen behind the anti-replay window.
var ErrReplay = errors.New("replayed or stale packet rejected")

// Identity is a long-term static Curve25519 keypair — a peer's cryptographic
// identity.
type Identity struct {
	kp noise.DHKey
}

// GenerateIdentity creates a fresh random static identity.
func GenerateIdentity() (*Identity, error) {
	kp, err := noise.DH25519.GenerateKeypair(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate identity: %w", err)
	}
	return &Identity{kp: kp}, nil
}

// LoadIdentity reconstructs an identity from a base64-encoded 32-byte private
// key, recomputing the matching public key.
func LoadIdentity(privB64 string) (*Identity, error) {
	priv, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privB64))
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	if len(priv) != KeySize {
		return nil, fmt.Errorf("invalid private key length: %d", len(priv))
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("derive public key: %w", err)
	}
	return &Identity{kp: noise.DHKey{Private: priv, Public: pub}}, nil
}

// PublicKey returns the raw 32-byte static public key.
func (id *Identity) PublicKey() []byte { return id.kp.Public }

// PublicKeyBase64 returns the static public key in base64.
func (id *Identity) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(id.kp.Public)
}

// PrivateKeyBase64 returns the static private key in base64 (for persistence).
func (id *Identity) PrivateKeyBase64() string {
	return base64.StdEncoding.EncodeToString(id.kp.Private)
}

// ParsePublicKey decodes a base64-encoded 32-byte public key.
func ParsePublicKey(b64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	if len(key) != KeySize {
		return nil, fmt.Errorf("invalid public key length: %d", len(key))
	}
	return key, nil
}

// Initiator drives the client side of the Noise_IK handshake.
type Initiator struct {
	hs *noise.HandshakeState
}

// NewInitiator starts a handshake toward a server whose static public key is
// known and pinned.
func NewInitiator(local *Identity, serverStatic []byte) (*Initiator, error) {
	if len(serverStatic) != KeySize {
		return nil, fmt.Errorf("invalid server static key length: %d", len(serverStatic))
	}
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   cipherSuite,
		Random:        rand.Reader,
		Pattern:       handshakePattern,
		Initiator:     true,
		StaticKeypair: local.kp,
		PeerStatic:    serverStatic,
	})
	if err != nil {
		return nil, fmt.Errorf("init handshake: %w", err)
	}
	return &Initiator{hs: hs}, nil
}

// WriteMsg1 produces the first handshake message to send to the server.
func (i *Initiator) WriteMsg1() ([]byte, error) {
	msg, _, _, err := i.hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("write msg1: %w", err)
	}
	return msg, nil
}

// ReadMsg2 consumes the server's response and, on success, returns the
// established transport Session.
func (i *Initiator) ReadMsg2(msg2 []byte) (*Session, error) {
	_, cs1, cs2, err := i.hs.ReadMessage(nil, msg2)
	if err != nil {
		return nil, fmt.Errorf("read msg2: %w", err)
	}
	if cs1 == nil || cs2 == nil {
		return nil, errors.New("handshake did not complete on msg2")
	}
	// Initiator sends with the first CipherState, receives with the second.
	return newSession(cs1, cs2), nil
}

// Responder drives the server side of the Noise_IK handshake.
type Responder struct {
	hs *noise.HandshakeState
}

// NewResponder starts a responder handshake. The peer's static key is learned
// from the first message.
func NewResponder(local *Identity) (*Responder, error) {
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   cipherSuite,
		Random:        rand.Reader,
		Pattern:       handshakePattern,
		Initiator:     false,
		StaticKeypair: local.kp,
	})
	if err != nil {
		return nil, fmt.Errorf("init handshake: %w", err)
	}
	return &Responder{hs: hs}, nil
}

// ReadMsg1 consumes the client's first message and returns the client's static
// public key (so the caller can check it against an allowlist).
func (r *Responder) ReadMsg1(msg1 []byte) (peerStatic []byte, err error) {
	if _, _, _, err := r.hs.ReadMessage(nil, msg1); err != nil {
		return nil, fmt.Errorf("read msg1: %w", err)
	}
	ps := r.hs.PeerStatic()
	if len(ps) != KeySize {
		return nil, errors.New("client did not present a static key")
	}
	return ps, nil
}

// WriteMsg2 produces the response message and, on success, the transport Session.
func (r *Responder) WriteMsg2() ([]byte, *Session, error) {
	msg, cs1, cs2, err := r.hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("write msg2: %w", err)
	}
	if cs1 == nil || cs2 == nil {
		return nil, nil, errors.New("handshake did not complete on msg2")
	}
	// Responder sends with the second CipherState, receives with the first.
	return msg, newSession(cs2, cs1), nil
}

// Session is an established, authenticated transport channel. It is safe for
// one sender goroutine and one receiver goroutine to use concurrently.
type Session struct {
	sendMu sync.Mutex
	send   *noise.CipherState

	recvMu sync.Mutex
	recv   *noise.CipherState

	replay replayWindow
}

func newSession(send, recv *noise.CipherState) *Session {
	return &Session{send: send, recv: recv}
}

// Encrypt seals plaintext into a frame: [counter(8) | ciphertext | tag]. The
// counter is the cipher nonce, guaranteeing nonce uniqueness within the session.
func (s *Session) Encrypt(plaintext []byte) ([]byte, error) {
	s.sendMu.Lock()
	seq := s.send.Nonce()
	ct, err := s.send.Encrypt(nil, nil, plaintext)
	s.sendMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}

	out := make([]byte, seqSize+len(ct))
	binary.BigEndian.PutUint64(out[:seqSize], seq)
	copy(out[seqSize:], ct)
	return out, nil
}

// Decrypt opens a frame produced by Encrypt and rejects replayed/stale frames.
func (s *Session) Decrypt(data []byte) ([]byte, error) {
	if len(data) < seqSize+tagSize {
		return nil, fmt.Errorf("frame too short: %d", len(data))
	}
	seq := binary.BigEndian.Uint64(data[:seqSize])

	s.recvMu.Lock()
	s.recv.SetNonce(seq)
	pt, err := s.recv.Decrypt(nil, nil, data[seqSize:])
	s.recvMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	// Only update the replay window after authentication, so a forged counter
	// cannot advance it.
	if !s.replay.accept(seq) {
		return nil, ErrReplay
	}
	return pt, nil
}
