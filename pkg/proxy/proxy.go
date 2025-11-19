package proxy

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// UserspaceProxy implements userspace NAT and proxying for TCP/UDP
// This allows VPN to work without kernel NAT support (e.g., in gVisor)
type UserspaceProxy struct {
	tcpConns  map[string]*TCPConn
	udpConns  map[string]*UDPConn
	mu        sync.RWMutex
	toClient  chan<- []byte // Channel to send packets back to client
}

type TCPConn struct {
	clientKey  string
	clientIP   net.IP
	clientPort uint16
	remoteAddr string
	conn       net.Conn
	lastSeen   time.Time
}

type UDPConn struct {
	clientKey  string
	clientIP   net.IP
	clientPort uint16
	remoteAddr string
	conn       *net.UDPConn
	lastSeen   time.Time
}

func NewUserspaceProxy(toClient chan<- []byte) *UserspaceProxy {
	p := &UserspaceProxy{
		tcpConns: make(map[string]*TCPConn),
		udpConns: make(map[string]*UDPConn),
		toClient: toClient,
	}

	// Cleanup stale connections
	go p.cleanup()

	return p
}

func (p *UserspaceProxy) cleanup() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		p.mu.Lock()
		now := time.Now()

		// Clean TCP connections
		for key, conn := range p.tcpConns {
			if now.Sub(conn.lastSeen) > 2*time.Minute {
				conn.conn.Close()
				delete(p.tcpConns, key)
			}
		}

		// Clean UDP connections
		for key, conn := range p.udpConns {
			if now.Sub(conn.lastSeen) > 2*time.Minute {
				conn.conn.Close()
				delete(p.udpConns, key)
			}
		}

		p.mu.Unlock()
	}
}

// HandlePacket processes an IP packet and proxies it to the internet
func (p *UserspaceProxy) HandlePacket(packet []byte) error {
	if len(packet) < 20 {
		return fmt.Errorf("packet too short")
	}

	// Parse IP header
	version := packet[0] >> 4
	if version != 4 {
		return fmt.Errorf("not IPv4")
	}

	protocol := packet[9]
	srcIP := net.IP(packet[12:16])
	dstIP := net.IP(packet[16:20])

	// Get header length
	ihl := int(packet[0]&0x0F) * 4
	if len(packet) < ihl {
		return fmt.Errorf("packet smaller than IP header")
	}

	switch protocol {
	case 6: // TCP
		return p.handleTCP(packet, srcIP, dstIP, ihl)
	case 17: // UDP
		return p.handleUDP(packet, srcIP, dstIP, ihl)
	case 1: // ICMP
		log.Printf("ICMP proxying not yet implemented: %s -> %s", srcIP, dstIP)
		return nil
	default:
		log.Printf("Unsupported protocol %d", protocol)
		return nil
	}
}

func (p *UserspaceProxy) handleTCP(packet []byte, srcIP, dstIP net.IP, ihl int) error {
	if len(packet) < ihl+20 {
		return fmt.Errorf("packet too short for TCP")
	}

	tcpHeader := packet[ihl:]
	srcPort := binary.BigEndian.Uint16(tcpHeader[0:2])
	dstPort := binary.BigEndian.Uint16(tcpHeader[2:4])

	flags := tcpHeader[13]
	syn := flags&0x02 != 0
	fin := flags&0x01 != 0
	rst := flags&0x04 != 0

	connKey := fmt.Sprintf("tcp:%s:%d->%s:%d", srcIP, srcPort, dstIP, dstPort)
	remoteAddr := fmt.Sprintf("%s:%d", dstIP, dstPort)

	p.mu.Lock()
	conn, exists := p.tcpConns[connKey]
	p.mu.Unlock()

	// Handle connection termination
	if (fin || rst) && exists {
		p.mu.Lock()
		conn.conn.Close()
		delete(p.tcpConns, connKey)
		p.mu.Unlock()
		log.Printf("TCP connection closed: %s", connKey)
		return nil
	}

	// New connection (SYN)
	if syn && !exists {
		log.Printf("New TCP connection: %s -> %s", srcIP, remoteAddr)

		// Dial remote server
		tcpConn, err := net.DialTimeout("tcp", remoteAddr, 5*time.Second)
		if err != nil {
			log.Printf("Failed to connect to %s: %v", remoteAddr, err)
			// TODO: Send TCP RST back to client
			return err
		}

		conn = &TCPConn{
			clientKey:  connKey,
			clientIP:   srcIP,
			clientPort: srcPort,
			remoteAddr: remoteAddr,
			conn:       tcpConn,
			lastSeen:   time.Now(),
		}

		p.mu.Lock()
		p.tcpConns[connKey] = conn
		p.mu.Unlock()

		// Start reading from remote connection
		go p.readFromTCPRemote(conn, dstIP, dstPort, srcIP, srcPort)

		// TODO: Send SYN-ACK back to client
		return nil
	}

	// Existing connection - forward data
	if exists {
		conn.lastSeen = time.Now()

		// Get TCP payload
		tcpHeaderLen := int(tcpHeader[12]>>4) * 4
		if len(tcpHeader) > tcpHeaderLen {
			payload := tcpHeader[tcpHeaderLen:]
			if len(payload) > 0 {
				_, err := conn.conn.Write(payload)
				if err != nil {
					log.Printf("TCP write error: %v", err)
					p.mu.Lock()
					conn.conn.Close()
					delete(p.tcpConns, connKey)
					p.mu.Unlock()
					return err
				}
			}
		}
	}

	return nil
}

func (p *UserspaceProxy) readFromTCPRemote(conn *TCPConn, srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16) {
	defer conn.conn.Close()

	buf := make([]byte, 16384)
	for {
		n, err := conn.conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("TCP read error: %v", err)
			}
			break
		}

		if n > 0 {
			// Build IP packet with TCP payload
			packet := buildTCPPacket(srcIP, srcPort, dstIP, dstPort, buf[:n], false, false, true)

			// Send to client
			select {
			case p.toClient <- packet:
			default:
				log.Printf("Client channel full, dropping packet")
			}
		}
	}

	// Clean up
	p.mu.Lock()
	delete(p.tcpConns, conn.clientKey)
	p.mu.Unlock()
}

func (p *UserspaceProxy) handleUDP(packet []byte, srcIP, dstIP net.IP, ihl int) error {
	if len(packet) < ihl+8 {
		return fmt.Errorf("packet too short for UDP")
	}

	udpHeader := packet[ihl:]
	srcPort := binary.BigEndian.Uint16(udpHeader[0:2])
	dstPort := binary.BigEndian.Uint16(udpHeader[2:4])

	connKey := fmt.Sprintf("udp:%s:%d->%s:%d", srcIP, srcPort, dstIP, dstPort)
	remoteAddr := fmt.Sprintf("%s:%d", dstIP, dstPort)

	p.mu.Lock()
	conn, exists := p.udpConns[connKey]
	p.mu.Unlock()

	// Create new UDP connection if needed
	if !exists {
		log.Printf("New UDP connection: %s -> %s", srcIP, remoteAddr)

		udpAddr, err := net.ResolveUDPAddr("udp", remoteAddr)
		if err != nil {
			return err
		}

		udpConn, err := net.DialUDP("udp", nil, udpAddr)
		if err != nil {
			log.Printf("Failed to dial UDP %s: %v", remoteAddr, err)
			return err
		}

		conn = &UDPConn{
			clientKey:  connKey,
			clientIP:   srcIP,
			clientPort: srcPort,
			remoteAddr: remoteAddr,
			conn:       udpConn,
			lastSeen:   time.Now(),
		}

		p.mu.Lock()
		p.udpConns[connKey] = conn
		p.mu.Unlock()

		// Start reading responses
		go p.readFromUDPRemote(conn, dstIP, dstPort, srcIP, srcPort)
	}

	// Forward UDP payload
	conn.lastSeen = time.Now()

	if len(udpHeader) > 8 {
		payload := udpHeader[8:]
		_, err := conn.conn.Write(payload)
		if err != nil {
			log.Printf("UDP write error: %v", err)
			return err
		}
	}

	return nil
}

func (p *UserspaceProxy) readFromUDPRemote(conn *UDPConn, srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16) {
	defer conn.conn.Close()

	buf := make([]byte, 65535)
	for {
		conn.conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		n, err := conn.conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout - clean up
				break
			}
			log.Printf("UDP read error: %v", err)
			break
		}

		if n > 0 {
			// Build IP packet with UDP payload
			packet := buildUDPPacket(srcIP, srcPort, dstIP, dstPort, buf[:n])

			// Send to client
			select {
			case p.toClient <- packet:
			default:
				log.Printf("Client channel full, dropping UDP packet")
			}
		}
	}

	// Clean up
	p.mu.Lock()
	delete(p.udpConns, conn.clientKey)
	p.mu.Unlock()
}

// buildTCPPacket creates a basic TCP/IP packet (simplified)
func buildTCPPacket(srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16, payload []byte, syn, fin, ack bool) []byte {
	// This is a simplified implementation
	// In production, you'd use a proper TCP/IP stack like gVisor's netstack
	ipHeaderLen := 20
	tcpHeaderLen := 20
	totalLen := ipHeaderLen + tcpHeaderLen + len(payload)

	packet := make([]byte, totalLen)

	// IP Header
	packet[0] = 0x45 // Version 4, header length 5
	binary.BigEndian.PutUint16(packet[2:4], uint16(totalLen))
	packet[9] = 6 // TCP
	copy(packet[12:16], srcIP.To4())
	copy(packet[16:20], dstIP.To4())

	// TCP Header
	tcpHeader := packet[ipHeaderLen:]
	binary.BigEndian.PutUint16(tcpHeader[0:2], srcPort)
	binary.BigEndian.PutUint16(tcpHeader[2:4], dstPort)
	tcpHeader[12] = 0x50 // Header length 5 (20 bytes)

	// Flags
	flags := byte(0)
	if syn {
		flags |= 0x02
	}
	if fin {
		flags |= 0x01
	}
	if ack {
		flags |= 0x10
	}
	tcpHeader[13] = flags

	// Window size
	binary.BigEndian.PutUint16(tcpHeader[14:16], 65535)

	// Copy payload
	if len(payload) > 0 {
		copy(packet[ipHeaderLen+tcpHeaderLen:], payload)
	}

	// Calculate and set TCP checksum
	tcpHeader[16] = 0 // Clear checksum field
	tcpHeader[17] = 0
	checksum := calculateTCPChecksum(srcIP.To4(), dstIP.To4(), tcpHeader, payload)
	binary.BigEndian.PutUint16(tcpHeader[16:18], checksum)

	// Calculate and set IP checksum
	packet[10] = 0 // Clear IP checksum field
	packet[11] = 0
	ipChecksum := calculateIPChecksum(packet[:ipHeaderLen])
	binary.BigEndian.PutUint16(packet[10:12], ipChecksum)

	return packet
}

// buildUDPPacket creates a UDP/IP packet
func buildUDPPacket(srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16, payload []byte) []byte {
	ipHeaderLen := 20
	udpHeaderLen := 8
	totalLen := ipHeaderLen + udpHeaderLen + len(payload)

	packet := make([]byte, totalLen)

	// IP Header
	packet[0] = 0x45 // Version 4, header length 5
	binary.BigEndian.PutUint16(packet[2:4], uint16(totalLen))
	packet[9] = 17 // UDP
	copy(packet[12:16], srcIP.To4())
	copy(packet[16:20], dstIP.To4())

	// UDP Header
	udpHeader := packet[ipHeaderLen:]
	binary.BigEndian.PutUint16(udpHeader[0:2], srcPort)
	binary.BigEndian.PutUint16(udpHeader[2:4], dstPort)
	binary.BigEndian.PutUint16(udpHeader[4:6], uint16(udpHeaderLen+len(payload)))

	// Copy payload
	if len(payload) > 0 {
		copy(packet[ipHeaderLen+udpHeaderLen:], payload)
	}

	// Calculate and set UDP checksum
	udpHeader[6] = 0 // Clear checksum field
	udpHeader[7] = 0
	checksum := calculateUDPChecksum(srcIP.To4(), dstIP.To4(), udpHeader, payload)
	binary.BigEndian.PutUint16(udpHeader[6:8], checksum)

	// Calculate and set IP checksum
	packet[10] = 0 // Clear IP checksum field
	packet[11] = 0
	ipChecksum := calculateIPChecksum(packet[:ipHeaderLen])
	binary.BigEndian.PutUint16(packet[10:12], ipChecksum)

	return packet
}
// calculateIPChecksum calculates IP header checksum
func calculateIPChecksum(header []byte) uint16 {
	sum := uint32(0)
	
	// Sum all 16-bit words
	for i := 0; i < len(header)-1; i += 2 {
		sum += uint32(header[i])<<8 | uint32(header[i+1])
	}
	
	// Add carry
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	
	// Return one's complement
	return ^uint16(sum)
}

// calculateTCPChecksum calculates TCP checksum including pseudo-header
func calculateTCPChecksum(srcIP, dstIP net.IP, tcpHeader, payload []byte) uint16 {
	// Pseudo-header
	pseudoHeader := make([]byte, 12)
	copy(pseudoHeader[0:4], srcIP.To4())
	copy(pseudoHeader[4:8], dstIP.To4())
	pseudoHeader[8] = 0 // Reserved
	pseudoHeader[9] = 6 // Protocol (TCP)
	binary.BigEndian.PutUint16(pseudoHeader[10:12], uint16(len(tcpHeader)+len(payload)))
	
	// Combine pseudo-header + TCP header + payload
	data := append(pseudoHeader, tcpHeader...)
	data = append(data, payload...)
	
	// Calculate checksum
	sum := uint32(0)
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(data[i])<<8 | uint32(data[i+1])
	}
	
	// Handle odd length
	if len(data)%2 == 1 {
		sum += uint32(data[len(data)-1]) << 8
	}
	
	// Add carry
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	
	return ^uint16(sum)
}

// calculateUDPChecksum calculates UDP checksum including pseudo-header
func calculateUDPChecksum(srcIP, dstIP net.IP, udpHeader, payload []byte) uint16 {
	// Pseudo-header
	pseudoHeader := make([]byte, 12)
	copy(pseudoHeader[0:4], srcIP.To4())
	copy(pseudoHeader[4:8], dstIP.To4())
	pseudoHeader[8] = 0 // Reserved
	pseudoHeader[9] = 17 // Protocol (UDP)
	binary.BigEndian.PutUint16(pseudoHeader[10:12], uint16(len(udpHeader)+len(payload)))
	
	// Combine pseudo-header + UDP header + payload
	data := append(pseudoHeader, udpHeader...)
	data = append(data, payload...)
	
	// Calculate checksum
	sum := uint32(0)
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(data[i])<<8 | uint32(data[i+1])
	}
	
	// Handle odd length
	if len(data)%2 == 1 {
		sum += uint32(data[len(data)-1]) << 8
	}
	
	// Add carry
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	
	checksum := ^uint16(sum)
	
	// UDP checksum of 0 means no checksum
	if checksum == 0 {
		checksum = 0xFFFF
	}
	
	return checksum
}
