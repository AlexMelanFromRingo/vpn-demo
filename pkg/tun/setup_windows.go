// +build windows

package tun

import (
	"fmt"
	"os/exec"
	"strings"
)

// SetupInterface configures the TUN interface with IP address
func SetupInterface(ifname, localIP, remoteIP string, mtu int) error {
	// Remove /24 or /32 suffix if present
	localIPClean := strings.Split(localIP, "/")[0]

	// Set IP address using netsh
	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		"name="+ifname, "source=static", "addr="+localIPClean,
		"mask=255.255.255.0")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set IP address: %w, output: %s", err, output)
	}

	// Set MTU
	cmd = exec.Command("netsh", "interface", "ipv4", "set", "subinterface",
		ifname, "mtu="+fmt.Sprintf("%d", mtu), "store=persistent")
	if output, err := cmd.CombinedOutput(); err != nil {
		// MTU setting might fail on some Windows versions, just log it
		fmt.Printf("Warning: failed to set MTU: %s\n", output)
	}

	return nil
}

// AddRoute adds a route through the TUN interface
func AddRoute(destination, ifname string) error {
	cmd := exec.Command("route", "add", destination, "0.0.0.0", "if", ifname)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add route: %w, output: %s", err, output)
	}
	return nil
}
