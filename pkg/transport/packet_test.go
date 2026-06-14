package transport

import (
	"bytes"
	"testing"
)

func TestPacketRoundTrip(t *testing.T) {
	sizes := []int{0, 1, 4, 64, 1420, 1500, 60000}
	for _, size := range sizes {
		payload := bytes.Repeat([]byte{0x5A}, size)
		p := &Packet{Type: PacketTypeData, Payload: payload}

		data, err := p.Marshal()
		if err != nil {
			t.Fatalf("marshal size=%d: %v", size, err)
		}

		var out Packet
		if err := out.Unmarshal(data); err != nil {
			t.Fatalf("unmarshal size=%d: %v", size, err)
		}
		if out.Type != PacketTypeData {
			t.Fatalf("type mismatch size=%d: got 0x%02x", size, out.Type)
		}
		if !bytes.Equal(out.Payload, payload) {
			t.Fatalf("payload mismatch size=%d", size)
		}
	}
}

func TestPacketTypesPreserved(t *testing.T) {
	for _, typ := range []byte{PacketTypeHandshake, PacketTypeData, PacketTypePing} {
		p := &Packet{Type: typ, Payload: []byte("x")}
		data, err := p.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		var out Packet
		if err := out.Unmarshal(data); err != nil {
			t.Fatal(err)
		}
		if out.Type != typ {
			t.Fatalf("type 0x%02x not preserved (got 0x%02x)", typ, out.Type)
		}
	}
}

// TestObfuscationVaries confirms that two marshals of the same packet differ on
// the wire (random prefix + padding) while still decoding to the same payload.
func TestObfuscationVaries(t *testing.T) {
	p := &Packet{Type: PacketTypeData, Payload: []byte("identical payload")}
	a, _ := p.Marshal()
	b, _ := p.Marshal()
	if bytes.Equal(a, b) {
		t.Fatalf("obfuscation produced identical bytes twice")
	}
	var oa, ob Packet
	_ = oa.Unmarshal(a)
	_ = ob.Unmarshal(b)
	if !bytes.Equal(oa.Payload, ob.Payload) {
		t.Fatalf("payloads differ after obfuscation")
	}
}

func TestUnmarshalTooSmall(t *testing.T) {
	for n := 0; n < 11; n++ {
		var p Packet
		if err := p.Unmarshal(make([]byte, n)); err == nil {
			t.Fatalf("expected error for input of len %d", n)
		}
	}
}

func TestUnmarshalTruncatedPayload(t *testing.T) {
	// Build a header claiming a 100-byte payload but provide fewer bytes.
	data := make([]byte, 11+10)
	data[8] = PacketTypeData
	data[9] = 0x00
	data[10] = 0x64 // payload length = 100
	var p Packet
	if err := p.Unmarshal(data); err == nil {
		t.Fatalf("expected error for truncated payload")
	}
}

func TestMarshalRejectsOversizedPayload(t *testing.T) {
	p := &Packet{Type: PacketTypeData, Payload: make([]byte, MaxPacketSize)}
	if _, err := p.Marshal(); err == nil {
		t.Fatalf("expected error for oversized payload")
	}
}

func TestConstructors(t *testing.T) {
	if NewHandshakePacket([]byte("k")).Type != PacketTypeHandshake {
		t.Fatal("handshake type wrong")
	}
	if NewDataPacket([]byte("d")).Type != PacketTypeData {
		t.Fatal("data type wrong")
	}
	if NewPingPacket().Type != PacketTypePing {
		t.Fatal("ping type wrong")
	}
}

// FuzzUnmarshal ensures Unmarshal never panics on arbitrary input.
func FuzzUnmarshal(f *testing.F) {
	f.Add([]byte{})
	f.Add(make([]byte, 11))
	good, _ := (&Packet{Type: PacketTypeData, Payload: []byte("seed")}).Marshal()
	f.Add(good)

	f.Fuzz(func(t *testing.T, data []byte) {
		var p Packet
		_ = p.Unmarshal(data) // must not panic regardless of error
	})
}
