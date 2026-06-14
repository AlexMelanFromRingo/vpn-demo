package main

import (
	"errors"
	"flag"
	"fmt"
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
	"github.com/AlexMelanFromRingo/vpn-demo/pkg/session"
	"github.com/AlexMelanFromRingo/vpn-demo/pkg/transport"
	"github.com/AlexMelanFromRingo/vpn-demo/pkg/tun"
)

type Client struct {
	identity     *session.Identity
	serverStatic []byte
	session      *session.Session
	sessionMu    sync.RWMutex
	tunDev       *tun.Interface
	udpTrans     *transport.UDPTransport
	serverAddr   string
}

func main() {
	serverAddr := flag.String("server", "172.26.171.205:51820", "Server address")
	tunIP := flag.String("tun-ip", "10.0.0.2/24", "TUN interface IP")
	peerIP := flag.String("peer-ip", "10.0.0.1", "Peer IP for TUN (server)")
	mtu := flag.Int("mtu", 1420, "MTU size")
	keyFile := flag.String("key-file", "client.key", "path to the client's static identity key (created if missing)")
	serverKey := flag.String("server-key", "", "REQUIRED: server's static public key (base64) to pin")
	flag.Parse()

	log.Println("=== Lightweight VPN Client ===")
	log.Printf("Server: %s", *serverAddr)
	log.Printf("TUN IP: %s", *tunIP)

	if *serverKey == "" {
		log.Fatalf("-server-key is required (the server prints its static public key at startup)")
	}
	serverStatic, err := session.ParsePublicKey(*serverKey)
	if err != nil {
		log.Fatalf("Invalid -server-key: %v", err)
	}

	// Load (or create) the client's long-term static identity.
	identity, created, err := session.LoadOrCreateIdentityFile(*keyFile)
	if err != nil {
		log.Fatalf("Failed to load identity: %v", err)
	}
	if created {
		log.Printf("Generated new client identity, saved to %s", *keyFile)
	}
	log.Printf("Client static public key: %s", identity.PublicKeyBase64())
	log.Printf("  -> add to the server's allowlist: -peers %s", identity.PublicKeyBase64())

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
	udpTrans, err := transport.NewUDPClient(*serverAddr)
	if err != nil {
		log.Fatalf("Failed to create UDP transport: %v", err)
	}
	defer udpTrans.Close()

	client := &Client{
		identity:     identity,
		serverStatic: serverStatic,
		tunDev:       tunDev,
		udpTrans:     udpTrans,
		serverAddr:   *serverAddr,
	}

	// Perform handshake
	log.Println("Initiating Noise handshake with server...")
	if err := client.handshake(); err != nil {
		log.Fatalf("Handshake failed: %v", err)
	}
	log.Println("Handshake successful!")

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start goroutines
	go client.handleUDP()
	go client.handleTUN()
	go client.keepAlive()

	log.Println("Client started successfully!")
	<-sigChan
	log.Println("Shutting down...")
}

// getSession returns the established session, or nil if the handshake has not
// completed yet.
func (c *Client) getSession() *session.Session {
	c.sessionMu.RLock()
	defer c.sessionMu.RUnlock()
	return c.session
}

func (c *Client) handshake() error {
	ini, err := session.NewInitiator(c.identity, c.serverStatic)
	if err != nil {
		return err
	}

	// Send the first Noise message.
	msg1, err := ini.WriteMsg1()
	if err != nil {
		return err
	}
	if err := c.udpTrans.Send(transport.NewHandshakePacket(msg1), nil); err != nil {
		return err
	}

	// Wait for the server response with a timeout.
	c.udpTrans.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer c.udpTrans.SetReadDeadline(time.Time{})

	packet, _, err := c.udpTrans.Receive()
	if err != nil {
		return err
	}
	if packet.Type != transport.PacketTypeHandshake {
		return fmt.Errorf("unexpected packet type during handshake: got 0x%02x, want handshake (0x%02x)", packet.Type, transport.PacketTypeHandshake)
	}

	// Completing the handshake authenticates the server: it proves the server
	// holds the private key for the pinned -server-key, defeating MITM.
	sess, err := ini.ReadMsg2(packet.Payload)
	if err != nil {
		return fmt.Errorf("server authentication failed (wrong -server-key or unauthorised): %w", err)
	}

	c.sessionMu.Lock()
	c.session = sess
	c.sessionMu.Unlock()
	log.Printf("Authenticated session established with %s", c.serverAddr)
	return nil
}

func (c *Client) handleUDP() {
	for {
		packet, _, err := c.udpTrans.Receive()
		if err != nil {
			// Socket closed on shutdown — exit cleanly instead of busy-looping.
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("UDP receive error: %v", err)
			time.Sleep(10 * time.Millisecond)
			continue
		}

		if packet.Type == transport.PacketTypeData {
			sess := c.getSession()
			if sess == nil {
				continue
			}

			// Decrypt packet (also enforces the anti-replay window).
			plaintext, err := sess.Decrypt(packet.Payload)
			if err != nil {
				log.Printf("Decryption failed: %v", err)
				continue
			}

			// Debug: Log packet info (especially ICMP)
			packetInfo := ipparse.Describe(plaintext)
			if strings.Contains(packetInfo, "ICMP") {
				log.Printf("Server → TUN: %s (len=%d)", packetInfo, len(plaintext))
			}

			// Write to TUN
			if err := c.tunDev.WritePacket(plaintext); err != nil {
				log.Printf("TUN write error: %v", err)
			} else if strings.Contains(packetInfo, "ICMP") {
				log.Printf("✓ Wrote ICMP packet to TUN interface")
			}
		}
	}
}

func (c *Client) handleTUN() {
	for {
		packet, err := c.tunDev.ReadPacket()
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

		sess := c.getSession()
		if sess == nil {
			continue
		}

		// Debug: Log packet info (especially ICMP)
		packetInfo := ipparse.Describe(packet)
		if strings.Contains(packetInfo, "ICMP") {
			log.Printf("TUN → Server: %s (len=%d)", packetInfo, len(packet))
		}

		// Encrypt packet
		encrypted, err := sess.Encrypt(packet)
		if err != nil {
			log.Printf("Encryption failed: %v", err)
			continue
		}

		// Send to server
		dataPacket := transport.NewDataPacket(encrypted)
		if err := c.udpTrans.Send(dataPacket, nil); err != nil {
			log.Printf("Failed to send: %v", err)
		} else if strings.Contains(packetInfo, "ICMP") {
			log.Printf("✓ Sent ICMP packet to server")
		}
	}
}

func (c *Client) keepAlive() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if c.getSession() == nil {
			continue
		}

		pingPacket := transport.NewPingPacket()
		if err := c.udpTrans.Send(pingPacket, nil); err != nil {
			log.Printf("Failed to send ping: %v", err)
		}
	}
}
