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

	// Extract remote IP (gateway) without CIDR suffix
	remoteIPClean := strings.Split(remoteIP, "/")[0]

	fmt.Printf("Configuring Windows adapter '%s' with IP %s, gateway %s\n", ifname, localIPClean, remoteIPClean)

	// Set IP address WITH gateway to route all traffic through VPN
	// This makes VPN the default route, sending all internet traffic through the server
	// The server must have NAT/forwarding configured to proxy traffic to the internet
	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		fmt.Sprintf("name=%s", ifname), "source=static",
		fmt.Sprintf("addr=%s", localIPClean),
		fmt.Sprintf("mask=%s", mask),
		fmt.Sprintf("gateway=%s", remoteIPClean))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set IP address: %w, output: %s", err, output)
	}

	fmt.Printf("✓ IP address configured successfully\n")
	fmt.Printf("✓ Default gateway set to %s - all traffic will go through VPN!\n", remoteIPClean)

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

	fmt.Printf("✓ Windows adapter configured - all traffic will be routed through VPN!\n")
	fmt.Printf("  Make sure the VPN server has NAT/forwarding enabled for internet access.\n")

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
