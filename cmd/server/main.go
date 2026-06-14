package main

import (
	"encoding/base64"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AlexMelanFromRingo/vpn-demo/pkg/ipparse"
	"github.com/AlexMelanFromRingo/vpn-demo/pkg/ratelimit"
	"github.com/AlexMelanFromRingo/vpn-demo/pkg/session"
	"github.com/AlexMelanFromRingo/vpn-demo/pkg/transport"
	"github.com/AlexMelanFromRingo/vpn-demo/pkg/tun"
)

type Client struct {
	addr     *net.UDPAddr
	session  *session.Session
	lastSeen time.Time
}

type Server struct {
	identity  *session.Identity
	allowlist map[string]bool // base64 public keys authorised to connect
	allowAny  bool            // true when no allowlist is configured
	tunDev    *tun.Interface
	udpTrans  *transport.UDPTransport
	clients   map[string]*Client
	clientsMu sync.RWMutex

	// Handshake DoS protection: a global cap plus an independent per-source-IP
	// limiter so one source cannot starve the others.
	handshakeGlobal *ratelimit.TokenBucket
	handshakePerIP  *ratelimit.IPLimiter
}

func main() {
	listenAddr := flag.String("listen", "0.0.0.0:51820", "UDP listen address")
	tunIP := flag.String("tun-ip", "10.0.0.1/24", "TUN interface IP")
	peerIP := flag.String("peer-ip", "10.0.0.2", "Peer IP for TUN")
	mtu := flag.Int("mtu", 1420, "MTU size")
	keyFile := flag.String("key-file", "server.key", "path to the server's static identity key (created if missing)")
	peers := flag.String("peers", "", "comma-separated base64 client public keys allowed to connect")
	peersFile := flag.String("peers-file", "", "file with one base64 client public key per line")
	flag.Parse()

	log.Println("=== Lightweight VPN Server ===")
	log.Printf("Listen: %s", *listenAddr)
	log.Printf("TUN IP: %s", *tunIP)

	// Load (or create) the server's long-term static identity. Its public key is
	// the trust anchor clients must pin via -server-key.
	identity, created, err := session.LoadOrCreateIdentityFile(*keyFile)
	if err != nil {
		log.Fatalf("Failed to load identity: %v", err)
	}
	if created {
		log.Printf("Generated new server identity, saved to %s", *keyFile)
	}
	log.Printf("Server static public key: %s", identity.PublicKeyBase64())
	log.Printf("  -> clients connect with: -server-key %s", identity.PublicKeyBase64())

	allowlist, err := buildAllowlist(*peers, *peersFile)
	if err != nil {
		log.Fatalf("Failed to parse peers: %v", err)
	}
	allowAny := len(allowlist) == 0
	if allowAny {
		log.Println("WARNING: no -peers/-peers-file allowlist set — any cryptographically valid client is accepted")
	} else {
		log.Printf("Client authorisation: ENABLED (%d allowed peer key(s))", len(allowlist))
	}

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

	server := &Server{
		identity:  identity,
		allowlist: allowlist,
		allowAny:  allowAny,
		tunDev:    tunDev,
		udpTrans:  udpTrans,
		clients:   make(map[string]*Client),
		// Global backstop: ~100 handshakes/sec, burst 200.
		handshakeGlobal: ratelimit.NewTokenBucket(100, 200),
		// Per-source-IP: ~5 handshakes/sec, burst 10, tracking up to 4096 IPs.
		handshakePerIP: ratelimit.NewIPLimiter(5, 10, 4096),
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start goroutines
	go server.handleUDP()
	go server.handleTUN()
	go server.cleanupClients()

	log.Println("Server started successfully!")
	<-sigChan
	log.Println("Shutting down...")
}

func (s *Server) handleUDP() {
	for {
		packet, addr, err := s.udpTrans.Receive()
		if err != nil {
			// The socket was closed (graceful shutdown) — stop the loop instead
			// of spinning on a permanent error.
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("UDP receive error: %v", err)
			time.Sleep(10 * time.Millisecond)
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
	// DoS protection: global cap, then a per-source-IP bucket.
	if !s.handshakeGlobal.Allow() {
		log.Printf("Handshake from %s dropped (global rate limit)", addr)
		return
	}
	if !s.handshakePerIP.Allow(addr.IP.String()) {
		log.Printf("Handshake from %s dropped (per-IP rate limit)", addr)
		return
	}

	log.Printf("Handshake from %s", addr)

	// Run the Noise_IK responder. ReadMsg1 authenticates the handshake against
	// our static key and recovers the client's static public key.
	resp, err := session.NewResponder(s.identity)
	if err != nil {
		log.Printf("Failed to start responder: %v", err)
		return
	}
	clientStatic, err := resp.ReadMsg1(packet.Payload)
	if err != nil {
		log.Printf("Rejected handshake from %s: %v", addr, err)
		return
	}

	// Authorisation: is this client's identity allowed?
	clientKeyB64 := base64.StdEncoding.EncodeToString(clientStatic)
	if !s.allowAny && !s.allowlist[clientKeyB64] {
		log.Printf("Rejected handshake from %s: unauthorised client key %s", addr, clientKeyB64)
		return
	}

	msg2, sess, err := resp.WriteMsg2()
	if err != nil {
		log.Printf("Failed to complete handshake with %s: %v", addr, err)
		return
	}

	// Store client
	clientKey := addr.String()
	s.clientsMu.Lock()
	s.clients[clientKey] = &Client{
		addr:     addr,
		session:  sess,
		lastSeen: time.Now(),
	}
	s.clientsMu.Unlock()

	log.Printf("Client registered: %s (key %s)", addr, clientKeyB64)

	// Send the Noise response message.
	response := transport.NewHandshakePacket(msg2)
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

	// Decrypt packet (also enforces the anti-replay window).
	plaintext, err := client.session.Decrypt(packet.Payload)
	if err != nil {
		log.Printf("Decryption failed from %s: %v", addr, err)
		return
	}

	// Update last seen only after a packet authenticates, so spoofed/garbage
	// traffic cannot keep a dead client alive.
	s.clientsMu.Lock()
	client.lastSeen = time.Now()
	s.clientsMu.Unlock()

	// Write to TUN
	if err := s.tunDev.WritePacket(plaintext); err != nil {
		log.Printf("TUN write error: %v", err)
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
			// Device closed on shutdown — stop cleanly.
			if errors.Is(err, os.ErrClosed) || errors.Is(err, io.EOF) {
				return
			}
			// Don't spam logs for normal read timeouts/empty reads
			errStr := err.Error()
			if !strings.Contains(errStr, "EOF") &&
				!strings.Contains(errStr, "No more data is available") &&
				!strings.Contains(errStr, "timeout") {
				log.Printf("TUN read error: %v", err)
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}

		// Debug: Log packet info (especially ICMP)
		packetInfo := ipparse.Describe(packet)
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
			encrypted, err := client.session.Encrypt(packet)
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

// buildAllowlist parses authorised client public keys from a comma-separated
// flag and/or a file (one base64 key per line, '#' comments allowed). Keys are
// normalised to canonical base64 so lookups are exact.
func buildAllowlist(peers, peersFile string) (map[string]bool, error) {
	set := make(map[string]bool)

	add := func(raw string) error {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			return nil
		}
		key, err := session.ParsePublicKey(raw)
		if err != nil {
			return err
		}
		set[base64.StdEncoding.EncodeToString(key)] = true
		return nil
	}

	for _, p := range strings.Split(peers, ",") {
		if err := add(p); err != nil {
			return nil, err
		}
	}

	if peersFile != "" {
		data, err := os.ReadFile(peersFile)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if err := add(line); err != nil {
				return nil, err
			}
		}
	}

	return set, nil
}
