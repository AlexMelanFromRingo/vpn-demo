package session

import (
	"bytes"
	"testing"
	"time"
)

// rekey performs a fresh handshake between the same identities and returns the
// new client/server sessions.
func rekey(t *testing.T, client, server *Identity) (*Session, *Session) {
	t.Helper()
	cs, ss, err := handshake(t, client, server, server.PublicKey())
	if err != nil {
		t.Fatalf("rekey handshake: %v", err)
	}
	return cs, ss
}

func TestChannelBasicRoundTrip(t *testing.T) {
	client, _ := GenerateIdentity()
	server, _ := GenerateIdentity()
	cs, ss := rekey(t, client, server)

	cch := NewChannel(cs, DefaultRekeyGrace)
	sch := NewChannel(ss, DefaultRekeyGrace)

	ct, _ := cch.Encrypt([]byte("hello"))
	got, err := sch.Decrypt(ct)
	if err != nil || string(got) != "hello" {
		t.Fatalf("round trip failed: %v %q", err, got)
	}
}

// After a rotation, a packet sealed under the OLD session must still decrypt
// while the previous session is within its grace window.
func TestChannelGraceDecryptsOldSession(t *testing.T) {
	client, _ := GenerateIdentity()
	server, _ := GenerateIdentity()

	cs1, ss1 := rekey(t, client, server)
	cch := NewChannel(cs1, DefaultRekeyGrace)
	sch := NewChannel(ss1, DefaultRekeyGrace)

	// Client seals a packet under session 1 but it is "in flight"...
	inFlight, _ := cch.Encrypt([]byte("old-keys packet"))

	// ...meanwhile both sides rehandshake and rotate to session 2.
	cs2, ss2 := rekey(t, client, server)
	cch.Rotate(cs2)
	sch.Rotate(ss2)

	// The in-flight packet (old keys) must still decrypt via the previous session.
	got, err := sch.Decrypt(inFlight)
	if err != nil {
		t.Fatalf("grace decrypt failed: %v", err)
	}
	if string(got) != "old-keys packet" {
		t.Fatalf("got %q", got)
	}

	// New traffic flows under session 2.
	ct2, _ := cch.Encrypt([]byte("new-keys packet"))
	got2, err := sch.Decrypt(ct2)
	if err != nil || string(got2) != "new-keys packet" {
		t.Fatalf("post-rotation round trip failed: %v %q", err, got2)
	}
}

// Once the grace window elapses, old-session packets are rejected.
func TestChannelGraceExpires(t *testing.T) {
	client, _ := GenerateIdentity()
	server, _ := GenerateIdentity()

	cs1, ss1 := rekey(t, client, server)
	cch := NewChannel(cs1, DefaultRekeyGrace)
	sch := NewChannel(ss1, DefaultRekeyGrace)

	// Drive a controllable clock.
	base := time.Now()
	cch.now = func() time.Time { return base }
	sch.now = func() time.Time { return base }

	inFlight, _ := cch.Encrypt([]byte("stale"))

	cs2, ss2 := rekey(t, client, server)
	cch.Rotate(cs2)
	sch.Rotate(ss2)

	// Advance past the grace window.
	base = base.Add(DefaultRekeyGrace + time.Second)

	if _, err := sch.Decrypt(inFlight); err == nil {
		t.Fatal("expired old-session packet was accepted")
	}
}

// A genuine replay on the current session is rejected (not retried on previous).
func TestChannelReplayStillRejected(t *testing.T) {
	client, _ := GenerateIdentity()
	server, _ := GenerateIdentity()
	cs, ss := rekey(t, client, server)
	cch := NewChannel(cs, DefaultRekeyGrace)
	sch := NewChannel(ss, DefaultRekeyGrace)

	ct, _ := cch.Encrypt([]byte("once"))
	if _, err := sch.Decrypt(ct); err != nil {
		t.Fatalf("first decrypt: %v", err)
	}
	if _, err := sch.Decrypt(ct); err == nil {
		t.Fatal("replay accepted")
	}
}

func TestChannelMultipleRotations(t *testing.T) {
	client, _ := GenerateIdentity()
	server, _ := GenerateIdentity()
	cs, ss := rekey(t, client, server)
	cch := NewChannel(cs, DefaultRekeyGrace)
	sch := NewChannel(ss, DefaultRekeyGrace)

	for i := 0; i < 5; i++ {
		msg := bytes.Repeat([]byte{byte(i)}, 200)
		ct, _ := cch.Encrypt(msg)
		got, err := sch.Decrypt(ct)
		if err != nil || !bytes.Equal(got, msg) {
			t.Fatalf("round %d failed: %v", i, err)
		}
		ncs, nss := rekey(t, client, server)
		cch.Rotate(ncs)
		sch.Rotate(nss)
	}
}
