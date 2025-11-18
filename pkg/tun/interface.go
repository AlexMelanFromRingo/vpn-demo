package tun

import (
	"fmt"
	"io"

	"github.com/songgao/water"
)

// Interface represents a TUN virtual network interface
type Interface struct {
	device *water.Interface
	mtu    int
}

// Config holds TUN interface configuration
type Config struct {
	DeviceName string
	MTU        int
}

// New creates a new TUN interface
func New(cfg Config) (*Interface, error) {
	if cfg.MTU == 0 {
		cfg.MTU = 1500
	}

	// Use platform-specific device creation
	device, err := createDevice(cfg)
	if err != nil {
		return nil, err
	}

	return &Interface{
		device: device,
		mtu:    cfg.MTU,
	}, nil
}

// Name returns the interface name
func (t *Interface) Name() string {
	return t.device.Name()
}

// Read reads a packet from the TUN interface
func (t *Interface) Read(buf []byte) (int, error) {
	n, err := t.device.Read(buf)
	if err != nil {
		return 0, fmt.Errorf("TUN read error: %w", err)
	}
	return n, nil
}

// Write writes a packet to the TUN interface
func (t *Interface) Write(buf []byte) (int, error) {
	n, err := t.device.Write(buf)
	if err != nil {
		return 0, fmt.Errorf("TUN write error: %w", err)
	}
	return n, nil
}

// Close closes the TUN interface
func (t *Interface) Close() error {
	return t.device.Close()
}

// ReadPacket reads a single packet (helper)
func (t *Interface) ReadPacket() ([]byte, error) {
	buf := make([]byte, t.mtu+100) // Extra space for headers
	n, err := t.Read(buf)
	if err != nil {
		if err == io.EOF {
			return nil, err
		}
		return nil, err
	}
	return buf[:n], nil
}

// WritePacket writes a single packet (helper)
func (t *Interface) WritePacket(packet []byte) error {
	_, err := t.Write(packet)
	return err
}
