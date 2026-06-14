package ipparse

import (
	"strings"
	"testing"
)

// buildIPv4 builds a minimal IPv4 header with the given IHL (in 32-bit words),
// protocol, src/dst, followed by the supplied L4 bytes.
func buildIPv4(ihlWords int, proto byte, src, dst [4]byte, l4 []byte) []byte {
	hdrLen := ihlWords * 4
	pkt := make([]byte, hdrLen+len(l4))
	pkt[0] = 0x40 | byte(ihlWords) // version 4 + IHL
	pkt[9] = proto
	copy(pkt[12:16], src[:])
	copy(pkt[16:20], dst[:])
	copy(pkt[hdrLen:], l4)
	return pkt
}

func TestDescribeICMPNoOptions(t *testing.T) {
	pkt := buildIPv4(5, protoICMP, [4]byte{10, 0, 0, 2}, [4]byte{10, 0, 0, 1}, []byte{8, 0})
	got := Describe(pkt)
	want := "ICMP type=8 code=0 from 10.0.0.2 to 10.0.0.1"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// The key regression: with IP options present (IHL=6 => 24-byte header) the ICMP
// type/code must be read from the correct offset, not a hard-coded byte 20.
func TestDescribeICMPWithOptions(t *testing.T) {
	// IHL=6: 24-byte header (4 bytes of options). ICMP type=0 code=0 (echo reply).
	pkt := buildIPv4(6, protoICMP, [4]byte{1, 1, 1, 1}, [4]byte{2, 2, 2, 2}, []byte{0, 0})
	got := Describe(pkt)
	want := "ICMP type=0 code=0 from 1.1.1.1 to 2.2.2.2"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDescribeTCPUDP(t *testing.T) {
	tcp := buildIPv4(5, protoTCP, [4]byte{192, 168, 1, 1}, [4]byte{8, 8, 8, 8}, nil)
	if got := Describe(tcp); got != "TCP from 192.168.1.1 to 8.8.8.8" {
		t.Fatalf("tcp: %q", got)
	}
	udp := buildIPv4(5, protoUDP, [4]byte{192, 168, 1, 1}, [4]byte{8, 8, 4, 4}, nil)
	if got := Describe(udp); got != "UDP from 192.168.1.1 to 8.8.4.4" {
		t.Fatalf("udp: %q", got)
	}
}

func TestDescribeUnknownProto(t *testing.T) {
	pkt := buildIPv4(5, 89, [4]byte{1, 2, 3, 4}, [4]byte{5, 6, 7, 8}, nil) // 89 = OSPF
	if got := Describe(pkt); !strings.Contains(got, "Protocol-89") {
		t.Fatalf("got %q", got)
	}
}

func TestDescribeMalformed(t *testing.T) {
	cases := map[string][]byte{
		"too short":  make([]byte, 10),
		"empty":      nil,
		"not ipv4":   {0x60, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		"bad IHL =4": func() []byte { p := make([]byte, 20); p[0] = 0x44; return p }(), // IHL=4 -> 16 bytes < 20
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			// Must return a string and never panic.
			if got := Describe(in); got == "" {
				t.Fatalf("empty description")
			}
		})
	}
}
