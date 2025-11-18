// +build windows

package tun

import (
	"fmt"
	"log"

	"github.com/songgao/water"
)

// createDevice creates a TUN device for Windows
func createDevice(cfg Config) (*water.Interface, error) {
	config := water.Config{
		DeviceType: water.TUN,
	}

	// On Windows, device name is not set via Config.Name
	// It's determined by the system/driver

	device, err := water.New(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUN device: %w", err)
	}

	log.Printf("TUN interface created: %s", device.Name())
	return device, nil
}
