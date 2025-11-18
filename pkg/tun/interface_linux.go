// +build linux

package tun

import (
	"fmt"
	"log"

	"github.com/songgao/water"
)

// createDevice creates a TUN device for Linux
func createDevice(cfg Config) (*water.Interface, error) {
	config := water.Config{
		DeviceType: water.TUN,
	}

	// On Linux, we can set device name
	if cfg.DeviceName != "" {
		config.Name = cfg.DeviceName
	}

	device, err := water.New(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUN device: %w", err)
	}

	log.Printf("TUN interface created: %s", device.Name())
	return device, nil
}
