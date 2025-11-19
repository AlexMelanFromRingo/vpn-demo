//go:build windows

package tun

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SetupInterface configures the TUN interface with IP address
// On Windows with Wintun, we use netsh to configure the interface
func SetupInterface(ifname, localIP, remoteIP string, mtu int) error {
	// Remove /24 or /32 suffix if present
	localIPClean := strings.Split(localIP, "/")[0]

	// Extract subnet mask from CIDR if present
	mask := "255.255.255.0"
	if strings.Contains(localIP, "/24") {
		mask = "255.255.255.0"
	} else if strings.Contains(localIP, "/16") {
		mask = "255.255.0.0"
	} else if strings.Contains(localIP, "/8") {
		mask = "255.0.0.0"
	}

	fmt.Printf("Configuring Windows adapter '%s' with IP %s (split-tunnel mode)\n", ifname, localIPClean)

	// Set IP address WITHOUT gateway - split-tunnel mode
	// Only VPN network (10.0.0.0/24) will route through VPN
	// Internet traffic will use the default Windows connection
	// This mode works in all environments, no NAT required on server
	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		fmt.Sprintf("name=%s", ifname), "source=static",
		fmt.Sprintf("addr=%s", localIPClean),
		fmt.Sprintf("mask=%s", mask))
	// NO GATEWAY parameter - split-tunnel mode

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set IP address: %w, output: %s", err, output)
	}

	fmt.Printf("✓ IP address configured successfully\n")
	fmt.Printf("✓ Split-tunnel mode: only 10.0.0.0/24 routed through VPN\n")

	// Set MTU - this might fail on some Windows versions, which is OK
	cmd = exec.Command("netsh", "interface", "ipv4", "set", "subinterface",
		ifname, fmt.Sprintf("mtu=%d", mtu), "store=persistent")
	if output, err := cmd.CombinedOutput(); err != nil {
		// MTU setting might fail, just log warning
		fmt.Printf("Warning: failed to set MTU (this is usually OK): %s\n", output)
	} else {
		fmt.Printf("✓ MTU set to %d\n", mtu)
	}

	// Enable adapter explicitly
	cmd = exec.Command("netsh", "interface", "set", "interface", ifname, "admin=enabled")
	cmd.Run() // Ignore errors - may already be enabled

	// Give Windows time to update adapter status
	time.Sleep(300 * time.Millisecond)

	fmt.Printf("✓ Windows adapter configured in split-tunnel mode\n")
	fmt.Printf("  VPN network: 10.0.0.0/24 → VPN server\n")
	fmt.Printf("  Internet: Direct connection (not through VPN)\n")

	return nil
}

// AddRoute adds a route through the TUN interface
func AddRoute(destination, ifname string) error {
	// Get interface index
	// On Windows, we might need to use interface index instead of name
	cmd := exec.Command("route", "add", destination, "0.0.0.0", "if", ifname, "metric", "1")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Check if route already exists
		if !strings.Contains(string(output), "already exists") {
			return fmt.Errorf("failed to add route: %w, output: %s", err, output)
		}
	}
	return nil
}
