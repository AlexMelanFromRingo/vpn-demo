// +build windows

package tun

import (
	"fmt"
	"os/exec"
	"strings"
)

// SetupInterface configures the TUN interface with IP address
// On Windows with Wintun, we use netsh to configure the interface
func SetupInterface(ifname, localIP, remoteIP string, mtu int) error {
	// Remove /24 or /32 suffix if present
	localIPClean := strings.Split(localIP, "/")[0]
	remoteIPClean := strings.Split(remoteIP, "/")[0]

	// Extract subnet mask from CIDR if present
	mask := "255.255.255.0"
	if strings.Contains(localIP, "/24") {
		mask = "255.255.255.0"
	} else if strings.Contains(localIP, "/16") {
		mask = "255.255.0.0"
	} else if strings.Contains(localIP, "/8") {
		mask = "255.0.0.0"
	}

	fmt.Printf("Configuring Windows adapter '%s' with IP %s\n", ifname, localIPClean)

	// Set IP address using netsh
	// On Windows, Wintun adapter name might have spaces or special characters
	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		fmt.Sprintf("name=%s", ifname), "source=static",
		fmt.Sprintf("addr=%s", localIPClean),
		fmt.Sprintf("mask=%s", mask),
		fmt.Sprintf("gateway=%s", remoteIPClean))

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try without gateway
		cmd = exec.Command("netsh", "interface", "ip", "set", "address",
			fmt.Sprintf("name=%s", ifname), "source=static",
			fmt.Sprintf("addr=%s", localIPClean),
			fmt.Sprintf("mask=%s", mask))
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to set IP address: %w, output: %s", err, output)
		}
	}

	fmt.Printf("IP address configured successfully\n")

	// Set MTU - this might fail on some Windows versions, which is OK
	cmd = exec.Command("netsh", "interface", "ipv4", "set", "subinterface",
		ifname, fmt.Sprintf("mtu=%d", mtu), "store=persistent")
	if output, err := cmd.CombinedOutput(); err != nil {
		// MTU setting might fail, just log warning
		fmt.Printf("Warning: failed to set MTU (this is usually OK): %s\n", output)
	} else {
		fmt.Printf("MTU set to %d\n", mtu)
	}

	return nil
}

// AddRoute adds a route through the TUN interface
func AddRoute(destination, ifname string) error {
	// Get interface index
	// On Windows, we might need to use interface index instead of name
	cmd := exec.Command("route", "add", destination, "0.0.0.0", "if", ifname, "metric", "1")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add route: %w, output: %s", err, output)
	}
	return nil
}
