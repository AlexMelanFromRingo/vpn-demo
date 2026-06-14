// Package ipparse provides a tiny, dependency-free helper for producing a
// human-readable one-line description of an IPv4 packet. It is used only for
// debug logging on the data path, so it is deliberately allocation-light and
// never panics on malformed input.
package ipparse

import (
	"fmt"
	"net"
)

// Protocol numbers we care about for logging.
const (
	protoICMP = 1
	protoTCP  = 6
	protoUDP  = 17
)

// Describe returns a short, human-readable summary of an IPv4 packet such as
// "ICMP type=8 code=0 from 10.0.0.2 to 10.0.0.1". It tolerates short or
// non-IPv4 input by returning a descriptive string instead of panicking.
func Describe(packet []byte) string {
	if len(packet) < 20 {
		return "Invalid IP packet (too short)"
	}

	version := packet[0] >> 4
	if version != 4 {
		return "Not IPv4"
	}

	// Internet Header Length is measured in 32-bit words; honour it so that we
	// locate the L4 header correctly even when IP options are present.
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || ihl > len(packet) {
		return "Invalid IP packet (bad IHL)"
	}

	protocol := packet[9]
	srcIP := net.IP(packet[12:16])
	dstIP := net.IP(packet[16:20])

	switch protocol {
	case protoICMP:
		// ICMP type/code live in the first two bytes after the IP header.
		if len(packet) >= ihl+2 {
			icmpType := packet[ihl]
			icmpCode := packet[ihl+1]
			return fmt.Sprintf("ICMP type=%d code=%d from %s to %s", icmpType, icmpCode, srcIP, dstIP)
		}
		return fmt.Sprintf("ICMP from %s to %s", srcIP, dstIP)
	case protoTCP:
		return fmt.Sprintf("TCP from %s to %s", srcIP, dstIP)
	case protoUDP:
		return fmt.Sprintf("UDP from %s to %s", srcIP, dstIP)
	default:
		return fmt.Sprintf("Protocol-%d from %s to %s", protocol, srcIP, dstIP)
	}
}
