package transport

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// UDPTransport handles UDP communication with encryption and obfuscation
type UDPTransport struct {
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
	mu         sync.RWMutex
}

// NewUDPServer creates a UDP server transport
func NewUDPServer(listenAddr string) (*UDPTransport, error) {
	addr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}

	// Set buffer sizes for better performance
	conn.SetReadBuffer(4 * 1024 * 1024)  // 4MB
	conn.SetWriteBuffer(4 * 1024 * 1024) // 4MB

	return &UDPTransport{
		conn: conn,
	}, nil
}

// NewUDPClient creates a UDP client transport
func NewUDPClient(serverAddr string) (*UDPTransport, error) {
	remoteAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve server address: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial server: %w", err)
	}

	// Set buffer sizes
	conn.SetReadBuffer(4 * 1024 * 1024)
	conn.SetWriteBuffer(4 * 1024 * 1024)

	return &UDPTransport{
		conn:       conn,
		remoteAddr: remoteAddr,
	}, nil
}

// Send sends a packet to remote address
func (t *UDPTransport) Send(packet *Packet, addr *net.UDPAddr) error {
	data, err := packet.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal packet: %w", err)
	}

	var n int
	if addr != nil {
		n, err = t.conn.WriteToUDP(data, addr)
	} else {
		t.mu.RLock()
		remoteAddr := t.remoteAddr
		t.mu.RUnlock()

		if remoteAddr == nil {
			return fmt.Errorf("no remote address set")
		}
		n, err = t.conn.WriteToUDP(data, remoteAddr)
	}

	if err != nil {
		return fmt.Errorf("failed to send packet: %w", err)
	}

	if n != len(data) {
		return fmt.Errorf("incomplete send: %d/%d bytes", n, len(data))
	}

	return nil
}

// Receive receives a packet from any address
func (t *UDPTransport) Receive() (*Packet, *net.UDPAddr, error) {
	buf := make([]byte, MaxPacketSize)

	n, addr, err := t.conn.ReadFromUDP(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to receive: %w", err)
	}

	packet := &Packet{}
	if err := packet.Unmarshal(buf[:n]); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal packet: %w", err)
	}

	return packet, addr, nil
}

// SetRemoteAddr sets the remote address for client
func (t *UDPTransport) SetRemoteAddr(addr *net.UDPAddr) {
	t.mu.Lock()
	t.remoteAddr = addr
	t.mu.Unlock()
}

// GetRemoteAddr gets the current remote address
func (t *UDPTransport) GetRemoteAddr() *net.UDPAddr {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.remoteAddr
}

// Close closes the UDP connection
func (t *UDPTransport) Close() error {
	return t.conn.Close()
}

// SetDeadline sets read/write deadline
func (t *UDPTransport) SetDeadline(deadline time.Time) error {
	return t.conn.SetDeadline(deadline)
}

// SetReadDeadline sets read deadline
func (t *UDPTransport) SetReadDeadline(deadline time.Time) error {
	return t.conn.SetReadDeadline(deadline)
}

// LocalAddr returns the local address
func (t *UDPTransport) LocalAddr() net.Addr {
	return t.conn.LocalAddr()
}
