package session

import (
	"bytes"
	"errors"
	"testing"
)

// handshake runs a full in-memory Noise_IK handshake and returns both sessions.
func handshake(t *testing.T, client, server *Identity, pinnedServerKey []byte) (*Session, *Session, error) {
	t.Helper()

	ini, err := NewInitiator(client, pinnedServerKey)
	if err != nil {
		return nil, nil, err
	}
	resp, err := NewResponder(server)
	if err != nil {
		return nil, nil, err
	}

	msg1, err := ini.WriteMsg1()
	if err != nil {
		return nil, nil, err
	}
	peer, err := resp.ReadMsg1(msg1)
	if err != nil {
		return nil, nil, err
	}
	if !bytes.Equal(peer, client.PublicKey()) {
		t.Fatalf("server learned wrong client key")
	}
	msg2, serverSess, err := resp.WriteMsg2()
	if err != nil {
		return nil, nil, err
	}
	clientSess, err := ini.ReadMsg2(msg2)
	if err != nil {
		return nil, nil, err
	}
	return clientSess, serverSess, nil
}

func TestHandshakeAndTransport(t *testing.T) {
	client, _ := GenerateIdentity()
	server, _ := GenerateIdentity()

	cs, ss, err := handshake(t, client, server, server.PublicKey())
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}

	// Both directions, various sizes.
	for _, size := range []int{0, 1, 64, 1420, 4096} {
		msg := bytes.Repeat([]byte{0xA5}, size)

		ct, err := cs.Encrypt(msg)
		if err != nil {
			t.Fatalf("client encrypt size=%d: %v", size, err)
		}
		got, err := ss.Decrypt(ct)
		if err != nil {
			t.Fatalf("server decrypt size=%d: %v", size, err)
		}
		if !bytes.Equal(got, msg) {
			t.Fatalf("c->s mismatch size=%d", size)
		}

		ct2, err := ss.Encrypt(msg)
		if err != nil {
			t.Fatalf("server encrypt: %v", err)
		}
		got2, err := cs.Decrypt(ct2)
		if err != nil {
			t.Fatalf("client decrypt: %v", err)
		}
		if !bytes.Equal(got2, msg) {
			t.Fatalf("s->c mismatch size=%d", size)
		}
	}
}

func TestHandshakeWrongServerKey(t *testing.T) {
	client, _ := GenerateIdentity()
	server, _ := GenerateIdentity()
	imposter, _ := GenerateIdentity()

	// Client pins the imposter's key but talks to the real server: must fail.
	_, _, err := handshake(t, client, server, imposter.PublicKey())
	if err == nil {
		t.Fatal("handshake succeeded against a server with a different key (MITM not prevented)")
	}
}

func TestTransportReplayRejected(t *testing.T) {
	client, _ := GenerateIdentity()
	server, _ := GenerateIdentity()
	cs, ss, err := handshake(t, client, server, server.PublicKey())
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}

	ct, _ := cs.Encrypt([]byte("hello"))
	if _, err := ss.Decrypt(ct); err != nil {
		t.Fatalf("first decrypt: %v", err)
	}
	if _, err := ss.Decrypt(ct); !errors.Is(err, ErrReplay) {
		t.Fatalf("replay not rejected: %v", err)
	}
}

func TestTransportTamperRejected(t *testing.T) {
	client, _ := GenerateIdentity()
	server, _ := GenerateIdentity()
	cs, ss, err := handshake(t, client, server, server.PublicKey())
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}

	ct, _ := cs.Encrypt([]byte("authentic"))
	ct[len(ct)-1] ^= 0x01 // flip a tag bit
	if _, err := ss.Decrypt(ct); err == nil {
		t.Fatal("tampered frame accepted")
	}
}

func TestTransportOutOfOrder(t *testing.T) {
	client, _ := GenerateIdentity()
	server, _ := GenerateIdentity()
	cs, ss, err := handshake(t, client, server, server.PublicKey())
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}

	var frames [][]byte
	for i := 0; i < 100; i++ {
		ct, _ := cs.Encrypt([]byte{byte(i)})
		frames = append(frames, ct)
	}
	// Deliver in reverse; all unique and within the window.
	for i := len(frames) - 1; i >= 0; i-- {
		if _, err := ss.Decrypt(frames[i]); err != nil {
			t.Fatalf("reordered frame %d rejected: %v", i, err)
		}
	}
}

func TestIdentityRoundTrip(t *testing.T) {
	id, _ := GenerateIdentity()
	loaded, err := LoadIdentity(id.PrivateKeyBase64())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !bytes.Equal(loaded.PublicKey(), id.PublicKey()) {
		t.Fatalf("public key mismatch after reload")
	}

	parsed, err := ParsePublicKey(id.PublicKeyBase64())
	if err != nil {
		t.Fatalf("parse pub: %v", err)
	}
	if !bytes.Equal(parsed, id.PublicKey()) {
		t.Fatalf("parsed public key mismatch")
	}
}

func TestParsePublicKeyErrors(t *testing.T) {
	if _, err := ParsePublicKey("not base64!!!"); err == nil {
		t.Fatal("accepted invalid base64")
	}
	if _, err := ParsePublicKey("YWJj"); err == nil {
		t.Fatal("accepted wrong-length key")
	}
}
