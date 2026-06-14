//go:build windows

package tun

import (
	"crypto/sha256"
	"fmt"
	"log"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
)

const (
	wintunPoolName = "LightVPN"
)

type wintunDevice struct {
	adapter *wintun.Adapter
	session wintun.Session
}

func (w *wintunDevice) Read(buf []byte) (int, error) {
	packet, err := w.session.ReceivePacket()
	if err != nil {
		return 0, err
	}
	defer w.session.ReleaseReceivePacket(packet)

	n := copy(buf, packet)
	return n, nil
}

func (w *wintunDevice) Write(buf []byte) (int, error) {
	packet, err := w.session.AllocateSendPacket(len(buf))
	if err != nil {
		return 0, err
	}

	copy(packet, buf)
	w.session.SendPacket(packet)
	return len(buf), nil
}

func (w *wintunDevice) Close() error {
	// End session first
	w.session.End()

	// Just close - allows reuse on next run
	w.adapter.Close()
	// DON'T delete - prevents duplicates!

	return nil
}

// generateDeterministicGUID creates deterministic GUID for adapter
// This ensures the same GUID is used across restarts, preventing duplicates
func generateDeterministicGUID(name string) *windows.GUID {
	// Generate deterministic hash from adapter name
	hash := sha256.Sum256([]byte("LightVPN-Wintun-" + name))

	guid := &windows.GUID{}
	// Convert hash to GUID format
	guid.Data1 = uint32(hash[0]) | uint32(hash[1])<<8 | uint32(hash[2])<<16 | uint32(hash[3])<<24
	guid.Data2 = uint16(hash[4]) | uint16(hash[5])<<8
	guid.Data3 = uint16(hash[6]) | uint16(hash[7])<<8
	copy(guid.Data4[:], hash[8:16])

	return guid
}

// createDevice creates a TUN device for Windows using Wintun
func createDevice(cfg Config) (device, string, error) {
	deviceName := cfg.DeviceName
	if deviceName == "" {
		deviceName = "vpn0"
	}

	var adapter *wintun.Adapter
	var err error

	// CRITICAL: Try to open existing adapter first to prevent duplicates!
	log.Printf("Checking for existing Wintun adapter '%s'...", deviceName)
	adapter, err = wintun.OpenAdapter(deviceName)

	if err != nil {
		// Adapter doesn't exist, create new one with deterministic GUID
		log.Printf("Creating new Wintun adapter '%s'...", deviceName)
		guid := generateDeterministicGUID(deviceName)
		adapter, err = wintun.CreateAdapter(deviceName, wintunPoolName, guid)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create Wintun adapter: %w", err)
		}
		log.Printf("✓ Created new Wintun adapter: %s", deviceName)
	} else {
		log.Printf("✓ Reusing existing Wintun adapter: %s (prevents duplicates!)", deviceName)
	}

	// Start session
	log.Printf("Starting Wintun session...")
	session, err := adapter.StartSession(0x800000) // 8MB ring buffer
	if err != nil {
		adapter.Close()
		return nil, "", fmt.Errorf("failed to start Wintun session: %w", err)
	}

	log.Printf("✓ Wintun adapter ready: %s", deviceName)

	return &wintunDevice{
		adapter: adapter,
		session: session,
	}, deviceName, nil
}
