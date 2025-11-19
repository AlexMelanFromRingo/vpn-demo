package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AlexMelanFromRingo/vpn-demo/pkg/crypto"
	"github.com/AlexMelanFromRingo/vpn-demo/pkg/proxy"
	"github.com/AlexMelanFromRingo/vpn-demo/pkg/transport"
	"github.com/AlexMelanFromRingo/vpn-demo/pkg/tun"
)

// Helper function to parse IP packet for debugging
func parseIPPacket(packet []byte) string {
	if len(packet) < 20 {
		return "Invalid IP packet (too short)"
	}

	version := packet[0] >> 4
	if version != 4 {
		return "Not IPv4"
	}

	protocol := packet[9]
	srcIP := net.IP(packet[12:16])
	dstIP := net.IP(packet[16:20])

	var protoName string
	switch protocol {
	case 1:
		protoName = "ICMP"
		if len(packet) >= 24 {
			icmpType := packet[20]
			icmpCode := packet[21]
			return fmt.Sprintf("ICMP type=%d code=%d from %s to %s", icmpType, icmpCode, srcIP, dstIP)
		}
		return fmt.Sprintf("ICMP from %s to %s", srcIP, dstIP)
	case 6:
		protoName = "TCP"
	case 17:
		protoName = "UDP"
	default:
		protoName = fmt.Sprintf("Protocol-%d", protocol)
	}

	return fmt.Sprintf("%s from %s to %s", protoName, srcIP, dstIP)
}

type Client struct {
	addr   *net.UDPAddr
	cipher *crypto.SessionCipher
	lastSeen time.Time
}

type Server struct {
	keyPair       *crypto.KeyPair
	tunDev        *tun.Interface
	udpTrans      *transport.UDPTransport
	clients       map[string]*Client
	clientsMu     sync.RWMutex
	proxy         *proxy.UserspaceProxy
	proxyPackets  chan ProxyPacket
}

type ProxyPacket struct {
	clientAddr *net.UDPAddr
	data       []byte
}

func main() {
	listenAddr := flag.String("listen", "0.0.0.0:51820", "UDP listen address")
	tunIP := flag.String("tun-ip", "10.0.0.1/24", "TUN interface IP")
	peerIP := flag.String("peer-ip", "10.0.0.2", "Peer IP for TUN")
	mtu := flag.Int("mtu", 1420, "MTU size")
	flag.Parse()

	log.Println("=== Lightweight VPN Server ===")
	log.Printf("Listen: %s", *listenAddr)
	log.Printf("TUN IP: %s", *tunIP)

	// Generate server key pair
	keyPair, err := crypto.GenerateKeyPair()
	if err != nil {
		log.Fatalf("Failed to generate key pair: %v", err)
	}
	log.Printf("Server Public Key: %s", keyPair.PublicKeyToString())

	// Ensure Wintun is available (Windows only, auto-downloads if needed)
	if err := tun.EnsureWintun(); err != nil {
		log.Fatalf("Failed to ensure Wintun: %v", err)
	}

	// Create TUN interface
	tunDev, err := tun.New(tun.Config{
		DeviceName: "vpn0",
		MTU:        *mtu,
	})
	if err != nil {
		log.Fatalf("Failed to create TUN: %v", err)
	}
	defer tunDev.Close()

	// Setup TUN interface
	log.Printf("Setting up TUN interface: %s", tunDev.Name())
	if err := tun.SetupInterface(tunDev.Name(), *tunIP, *peerIP, *mtu); err != nil {
		log.Fatalf("Failed to setup TUN: %v", err)
	}
	log.Printf("TUN interface ready: %s", tunDev.Name())

	// Create UDP transport
	udpTrans, err := transport.NewUDPServer(*listenAddr)
	if err != nil {
		log.Fatalf("Failed to create UDP transport: %v", err)
	}
	defer udpTrans.Close()
	log.Printf("UDP server listening on %s", *listenAddr)

	// Create proxy packets channel
	proxyPackets := make(chan ProxyPacket, 100)

	// Create userspace proxy
	toClientChan := make(chan []byte, 100)
	userspaceProxy := proxy.NewUserspaceProxy(toClientChan)

	server := &Server{
		keyPair:      keyPair,
		tunDev:       tunDev,
		udpTrans:     udpTrans,
		clients:      make(map[string]*Client),
		proxy:        userspaceProxy,
		proxyPackets: proxyPackets,
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start goroutines
	go server.handleUDP()
	go server.handleTUN()
	go server.cleanupClients()
	go server.handleProxyPackets(toClientChan) // New: handle packets from proxy

	log.Println("Server started successfully!")
	log.Println("Userspace proxy enabled - TCP/UDP will be proxied to internet")
	<-sigChan
	log.Println("Shutting down...")
}

func (s *Server) handleUDP() {
	for {
		packet, addr, err := s.udpTrans.Receive()
		if err != nil {
			log.Printf("UDP receive error: %v", err)
			continue
		}

		switch packet.Type {
		case transport.PacketTypeHandshake:
			s.handleHandshake(packet, addr)
		case transport.PacketTypeData:
			s.handleData(packet, addr)
		case transport.PacketTypePing:
			s.handlePing(addr)
		}
	}
}

func (s *Server) handleHandshake(packet *transport.Packet, addr *net.UDPAddr) {
	log.Printf("Handshake from %s", addr)

	// Parse client public key
	clientPubKey, err := crypto.PublicKeyFromString(string(packet.Payload))
	if err != nil {
		log.Printf("Invalid public key from %s: %v", addr, err)
		return
	}

	// Compute shared secret
	sharedSecret, err := s.keyPair.ComputeSharedSecret(clientPubKey)
	if err != nil {
		log.Printf("Failed to compute shared secret: %v", err)
		return
	}

	// Create cipher
	cipher, err := crypto.NewSessionCipher(sharedSecret)
	if err != nil {
		log.Printf("Failed to create cipher: %v", err)
		return
	}

	// Store client
	clientKey := addr.String()
	s.clientsMu.Lock()
	s.clients[clientKey] = &Client{
		addr:     addr,
		cipher:   cipher,
		lastSeen: time.Now(),
	}
	s.clientsMu.Unlock()

	log.Printf("Client registered: %s (epoch: %d)", addr, cipher.GetCurrentEpoch())

	// Send our public key back
	response := transport.NewHandshakePacket([]byte(s.keyPair.PublicKeyToString()))
	if err := s.udpTrans.Send(response, addr); err != nil {
		log.Printf("Failed to send handshake response: %v", err)
	}
}

func (s *Server) handleData(packet *transport.Packet, addr *net.UDPAddr) {
	clientKey := addr.String()

	s.clientsMu.RLock()
	client, exists := s.clients[clientKey]
	s.clientsMu.RUnlock()

	if !exists {
		log.Printf("Data from unknown client: %s", addr)
		return
	}

	// Update last seen
	s.clientsMu.Lock()
	client.lastSeen = time.Now()
	s.clientsMu.Unlock()

	// Decrypt packet
	plaintext, err := client.cipher.Decrypt(packet.Payload)
	if err != nil {
		log.Printf("Decryption failed from %s: %v", addr, err)
		return
	}

	// Debug logging
	packetInfo := parseIPPacket(plaintext)
	if strings.Contains(packetInfo, "TCP") || strings.Contains(packetInfo, "UDP") {
		log.Printf("Client → Proxy: %s", packetInfo)
	}

	// Send to userspace proxy for internet forwarding
	if err := s.proxy.HandlePacket(plaintext); err != nil {
		// If proxy fails, try writing directly to TUN (fallback for ICMP, etc.)
		if err := s.tunDev.WritePacket(plaintext); err != nil {
			log.Printf("TUN write error: %v", err)
		}
	}
}

func (s *Server) handlePing(addr *net.UDPAddr) {
	clientKey := addr.String()
	s.clientsMu.Lock()
	if client, exists := s.clients[clientKey]; exists {
		client.lastSeen = time.Now()
	}
	s.clientsMu.Unlock()
}

func (s *Server) handleTUN() {
	for {
		packet, err := s.tunDev.ReadPacket()
		if err != nil {
			// Don't spam logs for normal read timeouts/empty reads
			errStr := err.Error()
			if !strings.Contains(errStr, "EOF") &&
			   !strings.Contains(errStr, "No more data is available") &&
			   !strings.Contains(errStr, "timeout") {
				log.Printf("TUN read error: %v", err)
			}
			continue
		}

		// Debug: Log packet info (especially ICMP)
		packetInfo := parseIPPacket(packet)
		if strings.Contains(packetInfo, "ICMP") {
			log.Printf("TUN → Client: %s (len=%d)", packetInfo, len(packet))
		}

		// Send to all active clients
		s.clientsMu.RLock()
		clients := make([]*Client, 0, len(s.clients))
		for _, client := range s.clients {
			clients = append(clients, client)
		}
		s.clientsMu.RUnlock()

		if len(clients) == 0 && strings.Contains(packetInfo, "ICMP") {
			log.Printf("WARNING: ICMP packet received but no active clients!")
		}

		for _, client := range clients {
			// Encrypt packet
			encrypted, err := client.cipher.Encrypt(packet)
			if err != nil {
				log.Printf("Encryption failed: %v", err)
				continue
			}

			// Send to client
			dataPacket := transport.NewDataPacket(encrypted)
			if err := s.udpTrans.Send(dataPacket, client.addr); err != nil {
				log.Printf("Failed to send to %s: %v", client.addr, err)
			} else if strings.Contains(packetInfo, "ICMP") {
				log.Printf("✓ Sent ICMP packet to client %s", client.addr)
			}
		}
	}
}

func (s *Server) cleanupClients() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.clientsMu.Lock()
		for key, client := range s.clients {
			if time.Since(client.lastSeen) > 2*time.Minute {
				log.Printf("Removing inactive client: %s", key)
				delete(s.clients, key)
			}
		}
		s.clientsMu.Unlock()
	}
}
// handleProxyPackets receives packets from userspace proxy and sends them to clients
func (s *Server) handleProxyPackets(toClientChan <-chan []byte) {
	for packet := range toClientChan {
		// Debug logging
		packetInfo := parseIPPacket(packet)
		if strings.Contains(packetInfo, "TCP") || strings.Contains(packetInfo, "UDP") {
			log.Printf("Proxy → Client: %s", packetInfo)
		}

		// Determine which client should receive this packet
		// by looking at the destination IP in the packet
		if len(packet) < 20 {
			continue
		}

		dstIP := net.IP(packet[16:20])
		
		// Find client with this IP
		s.clientsMu.RLock()
		var targetClient *Client
		for _, client := range s.clients {
			// For now, send to all clients (typically only one)
			// In future, track IP->Client mapping
			targetClient = client
			break
		}
		s.clientsMu.RUnlock()

		if targetClient == nil {
			log.Printf("No client found for proxied packet to %s", dstIP)
			continue
		}

		// Encrypt packet
		encrypted, err := targetClient.cipher.Encrypt(packet)
		if err != nil {
			log.Printf("Failed to encrypt proxy packet: %v", err)
			continue
		}

		// Send to client
		dataPacket := transport.NewDataPacket(encrypted)
		if err := s.udpTrans.Send(dataPacket, targetClient.addr); err != nil {
			log.Printf("Failed to send proxy packet to client: %v", err)
		}
	}
}
