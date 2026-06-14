//go:build linux
// +build linux

package tun

import (
	"fmt"
	"os/exec"
)

// SetupInterface configures the TUN interface with IP address and routes
func SetupInterface(ifname, localIP, remoteIP string, mtu int) error {
	// Set IP address
	cmd := exec.Command("ip", "addr", "add", localIP, "peer", remoteIP, "dev", ifname)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set IP address: %w, output: %s", err, output)
	}

	// Set MTU
	cmd = exec.Command("ip", "link", "set", "dev", ifname, "mtu", fmt.Sprintf("%d", mtu))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set MTU: %w, output: %s", err, output)
	}

	// Bring interface up
	cmd = exec.Command("ip", "link", "set", "dev", ifname, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring interface up: %w, output: %s", err, output)
	}

	return nil
}

// AddRoute adds a route through the TUN interface
func AddRoute(destination, ifname string) error {
	cmd := exec.Command("ip", "route", "add", destination, "dev", ifname)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add route: %w, output: %s", err, output)
	}
	return nil
}
