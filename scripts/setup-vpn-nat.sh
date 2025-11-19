#!/bin/bash

# VPN NAT Setup Script
# This script configures the Linux server to forward VPN client traffic to the internet
# Run this on the VPN server (Linux/WSL2) with sudo

set -e

echo "=== VPN NAT and Forwarding Setup ==="
echo ""

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
TUN_INTERFACE="tun0"
TUN_NETWORK="10.0.0.0/24"

# Detect the primary internet interface
echo "Detecting primary network interface..."
INTERNET_INTERFACE=$(ip route | grep default | awk '{print $5}' | head -n1)

if [ -z "$INTERNET_INTERFACE" ]; then
    echo "Error: Could not detect internet interface"
    echo "Please specify it manually in the script (e.g., eth0, wlan0, ens33)"
    exit 1
fi

echo -e "${GREEN}✓${NC} Detected internet interface: $INTERNET_INTERFACE"
echo ""

# 1. Enable IP forwarding
echo "1. Enabling IP forwarding..."
sysctl -w net.ipv4.ip_forward=1 > /dev/null
sysctl -w net.ipv6.conf.all.forwarding=1 > /dev/null
echo -e "${GREEN}✓${NC} IP forwarding enabled"

# Make it persistent across reboots
if ! grep -q "^net.ipv4.ip_forward=1" /etc/sysctl.conf 2>/dev/null; then
    echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf
    echo -e "${GREEN}✓${NC} IP forwarding will persist after reboot"
fi

echo ""

# 2. Configure NAT (masquerading)
echo "2. Configuring NAT (iptables)..."

# Check if rule already exists
if iptables -t nat -C POSTROUTING -s $TUN_NETWORK -o $INTERNET_INTERFACE -j MASQUERADE 2>/dev/null; then
    echo -e "${YELLOW}!${NC} NAT rule already exists"
else
    iptables -t nat -A POSTROUTING -s $TUN_NETWORK -o $INTERNET_INTERFACE -j MASQUERADE
    echo -e "${GREEN}✓${NC} NAT rule added: $TUN_NETWORK -> $INTERNET_INTERFACE"
fi

echo ""

# 3. Configure forwarding rules
echo "3. Configuring forwarding rules..."

# Allow forwarding from TUN to internet
if iptables -C FORWARD -i $TUN_INTERFACE -o $INTERNET_INTERFACE -j ACCEPT 2>/dev/null; then
    echo -e "${YELLOW}!${NC} Forward rule (TUN->Internet) already exists"
else
    iptables -A FORWARD -i $TUN_INTERFACE -o $INTERNET_INTERFACE -j ACCEPT
    echo -e "${GREEN}✓${NC} Forward rule added: $TUN_INTERFACE -> $INTERNET_INTERFACE"
fi

# Allow return traffic
if iptables -C FORWARD -i $INTERNET_INTERFACE -o $TUN_INTERFACE -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null; then
    echo -e "${YELLOW}!${NC} Return traffic rule already exists"
else
    iptables -A FORWARD -i $INTERNET_INTERFACE -o $TUN_INTERFACE -m state --state RELATED,ESTABLISHED -j ACCEPT
    echo -e "${GREEN}✓${NC} Return traffic rule added: $INTERNET_INTERFACE -> $TUN_INTERFACE"
fi

echo ""

# 4. Display current configuration
echo "4. Current iptables configuration:"
echo ""
echo "--- NAT rules ---"
iptables -t nat -L POSTROUTING -n -v | grep -E "MASQUERADE|Chain"
echo ""
echo "--- Forward rules ---"
iptables -L FORWARD -n -v | grep -E "$TUN_INTERFACE|Chain" | head -5
echo ""

# 5. Show status
echo "=== Configuration Summary ==="
echo ""
echo -e "${GREEN}✓${NC} IP forwarding:        $(sysctl -n net.ipv4.ip_forward)"
echo -e "${GREEN}✓${NC} TUN interface:        $TUN_INTERFACE"
echo -e "${GREEN}✓${NC} TUN network:          $TUN_NETWORK"
echo -e "${GREEN}✓${NC} Internet interface:   $INTERNET_INTERFACE"
echo -e "${GREEN}✓${NC} NAT:                  Enabled"
echo ""

# 6. Save iptables rules (optional, for persistence)
echo "=== Persistence ==="
echo ""
echo "To make iptables rules persistent across reboots:"
echo ""
echo "On Ubuntu/Debian:"
echo "  sudo apt-get install iptables-persistent"
echo "  sudo netfilter-persistent save"
echo ""
echo "On RHEL/CentOS:"
echo "  sudo service iptables save"
echo ""
echo "Or add this script to /etc/rc.local or systemd"
echo ""

echo -e "${GREEN}✓${NC} VPN NAT setup complete!"
echo ""
echo "You can now connect VPN clients and they will be able to access the internet."
echo "All client traffic will appear to come from the server's IP address."
echo ""
