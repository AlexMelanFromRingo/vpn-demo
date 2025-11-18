// +build windows

package tun

import (
	"fmt"
	"log"
	"os"

	"golang.zx2c4.com/wintun"
)

const (
	// Wintun pool name
	wintunPoolName = "LightVPN"
)

// wintunDevice wraps Wintun adapter
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
	w.session.End()
	w.adapter.Close()
	return nil
}

func (w *wintunDevice) File() *os.File {
	return nil // Windows doesn't use file descriptor for Wintun
}

// createDevice creates a TUN device for Windows using Wintun
func createDevice(cfg Config) (device, string, error) {
	deviceName := cfg.DeviceName
	if deviceName == "" {
		deviceName = "vpn0"
	}

	// Create or open Wintun adapter
	adapter, err := wintun.CreateAdapter(deviceName, wintunPoolName, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create Wintun adapter (ensure wintun.dll is present): %w", err)
	}

	// Start session
	session, err := adapter.StartSession(0x800000) // 8MB ring buffer
	if err != nil {
		adapter.Close()
		return nil, "", fmt.Errorf("failed to start Wintun session: %w", err)
	}

	log.Printf("Wintun adapter created: %s", deviceName)
	log.Printf("Note: Wintun driver is automatically installed")

	return &wintunDevice{
		adapter: adapter,
		session: session,
	}, deviceName, nil
}
