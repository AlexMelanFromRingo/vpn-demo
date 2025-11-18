package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AlexMelanFromRingo/vpn-demo/pkg/crypto"
	"github.com/AlexMelanFromRingo/vpn-demo/pkg/transport"
	"github.com/AlexMelanFromRingo/vpn-demo/pkg/tun"
)

type Client struct {
	keyPair    *crypto.KeyPair
	cipher     *crypto.SessionCipher
	tunDev     *tun.Interface
	udpTrans   *transport.UDPTransport
	serverAddr string
}

func main() {
	serverAddr := flag.String("server", "172.26.171.205:51820", "Server address")
	tunIP := flag.String("tun-ip", "10.0.0.2/24", "TUN interface IP")
	peerIP := flag.String("peer-ip", "10.0.0.1", "Peer IP for TUN (server)")
	mtu := flag.Int("mtu", 1420, "MTU size")
	flag.Parse()

	log.Println("=== Lightweight VPN Client ===")
	log.Printf("Server: %s", *serverAddr)
	log.Printf("TUN IP: %s", *tunIP)

	// Generate client key pair
	keyPair, err := crypto.GenerateKeyPair()
	if err != nil {
		log.Fatalf("Failed to generate key pair: %v", err)
	}
	log.Printf("Client Public Key: %s", keyPair.PublicKeyToString())

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
		keyPair:    keyPair,
		tunDev:     tunDev,
		udpTrans:   udpTrans,
		serverAddr: *serverAddr,
	}

	// Perform handshake
	log.Println("Initiating handshake with server...")
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

func (c *Client) handshake() error {
	// Send our public key
	handshakePacket := transport.NewHandshakePacket([]byte(c.keyPair.PublicKeyToString()))
	if err := c.udpTrans.Send(handshakePacket, nil); err != nil {
		return err
	}

	// Wait for server response with timeout
	c.udpTrans.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer c.udpTrans.SetReadDeadline(time.Time{})

	packet, _, err := c.udpTrans.Receive()
	if err != nil {
		return err
	}

	if packet.Type != transport.PacketTypeHandshake {
		return err
	}

	// Parse server public key
	serverPubKey, err := crypto.PublicKeyFromString(string(packet.Payload))
	if err != nil {
		return err
	}

	log.Printf("Server Public Key: %s", string(packet.Payload))

	// Compute shared secret
	sharedSecret, err := c.keyPair.ComputeSharedSecret(serverPubKey)
	if err != nil {
		return err
	}

	// Create cipher
	cipher, err := crypto.NewSessionCipher(sharedSecret)
	if err != nil {
		return err
	}

	c.cipher = cipher
	log.Printf("Shared secret established (epoch: %d)", cipher.GetCurrentEpoch())

	return nil
}

func (c *Client) handleUDP() {
	for {
		packet, _, err := c.udpTrans.Receive()
		if err != nil {
			log.Printf("UDP receive error: %v", err)
			continue
		}

		if packet.Type == transport.PacketTypeData {
			// Decrypt packet
			plaintext, err := c.cipher.Decrypt(packet.Payload)
			if err != nil {
				log.Printf("Decryption failed: %v", err)
				continue
			}

			// Write to TUN
			if err := c.tunDev.WritePacket(plaintext); err != nil {
				log.Printf("TUN write error: %v", err)
			}
		}
	}
}

func (c *Client) handleTUN() {
	for {
		packet, err := c.tunDev.ReadPacket()
		if err != nil {
			log.Printf("TUN read error: %v", err)
			continue
		}

		if c.cipher == nil {
			continue
		}

		// Encrypt packet
		encrypted, err := c.cipher.Encrypt(packet)
		if err != nil {
			log.Printf("Encryption failed: %v", err)
			continue
		}

		// Send to server
		dataPacket := transport.NewDataPacket(encrypted)
		if err := c.udpTrans.Send(dataPacket, nil); err != nil {
			log.Printf("Failed to send: %v", err)
		}
	}
}

func (c *Client) keepAlive() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if c.cipher == nil {
			continue
		}

		pingPacket := transport.NewPingPacket()
		if err := c.udpTrans.Send(pingPacket, nil); err != nil {
			log.Printf("Failed to send ping: %v", err)
		}
	}
}
