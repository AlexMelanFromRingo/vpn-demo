package transport

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

// Packet types
const (
	PacketTypeHandshake = 0x01
	PacketTypeData      = 0x02
	PacketTypePing      = 0x03
)

const (
	// MaxPacketSize is the maximum UDP packet size
	MaxPacketSize = 65535
	// MinPaddingSize is minimum random padding
	MinPaddingSize = 0
	// MaxPaddingSize is maximum random padding for obfuscation
	MaxPaddingSize = 64
)

// Packet represents a VPN packet with obfuscation
type Packet struct {
	Type    byte
	Payload []byte
}

// Marshal serializes packet with random padding for DPI obfuscation
// Format: [random_prefix(8)] [type(1)] [payload_len(2)] [payload] [random_padding(0-64)]
func (p *Packet) Marshal() ([]byte, error) {
	payloadLen := len(p.Payload)
	if payloadLen > MaxPacketSize-256 {
		return nil, fmt.Errorf("payload too large: %d", payloadLen)
	}

	// Random prefix + padding size + padding are drawn from a single random read
	// so we only touch the CSPRNG once per packet and can check the error.
	randomBytes := make([]byte, 8+1+MaxPaddingSize)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("failed to read random bytes: %w", err)
	}
	randomPrefix := randomBytes[0:8]

	// Random padding size in [MinPaddingSize, MaxPaddingSize].
	paddingSize := MinPaddingSize
	if MaxPaddingSize > MinPaddingSize {
		paddingSize = MinPaddingSize + int(randomBytes[8])%(MaxPaddingSize-MinPaddingSize+1)
	}
	padding := randomBytes[9 : 9+paddingSize]

	// Total size: random_prefix(8) + type(1) + len(2) + payload + padding
	totalSize := 8 + 1 + 2 + payloadLen + paddingSize
	data := make([]byte, totalSize)

	// Write random prefix
	copy(data[0:8], randomPrefix)

	// Write type
	data[8] = p.Type

	// Write payload length
	binary.BigEndian.PutUint16(data[9:11], uint16(payloadLen))

	// Write payload
	copy(data[11:11+payloadLen], p.Payload)

	// Write random padding
	if paddingSize > 0 {
		copy(data[11+payloadLen:], padding)
	}

	return data, nil
}

// Unmarshal deserializes packet
func (p *Packet) Unmarshal(data []byte) error {
	if len(data) < 11 {
		return fmt.Errorf("packet too small: %d", len(data))
	}

	// Skip random prefix (8 bytes)
	p.Type = data[8]

	// Read payload length
	payloadLen := binary.BigEndian.Uint16(data[9:11])

	if len(data) < 11+int(payloadLen) {
		return fmt.Errorf("invalid payload length: %d, packet size: %d", payloadLen, len(data))
	}

	// Extract payload (ignore padding)
	p.Payload = make([]byte, payloadLen)
	copy(p.Payload, data[11:11+payloadLen])

	return nil
}

// NewHandshakePacket creates a handshake packet with public key
func NewHandshakePacket(publicKey []byte) *Packet {
	return &Packet{
		Type:    PacketTypeHandshake,
		Payload: publicKey,
	}
}

// NewDataPacket creates a data packet
func NewDataPacket(data []byte) *Packet {
	return &Packet{
		Type:    PacketTypeData,
		Payload: data,
	}
}

// NewPingPacket creates a ping packet
func NewPingPacket() *Packet {
	return &Packet{
		Type:    PacketTypePing,
		Payload: []byte("ping"),
	}
}
