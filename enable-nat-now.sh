#!/bin/bash

echo "=== Enabling VPN NAT ==="

# 1. Enable IP forwarding
echo "1. Enabling IP forwarding..."
sysctl -w net.ipv4.ip_forward=1
echo "✓ IP forwarding enabled"

# 2. Detect internet interface
INTERNET_IF=$(ip route | grep default | awk '{print $5}' | head -n1)
echo "2. Detected internet interface: $INTERNET_IF"

# 3. Configure NAT
echo "3. Setting up NAT..."
iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -o $INTERNET_IF -j MASQUERADE
echo "✓ NAT rule added"

# 4. Configure forwarding
echo "4. Setting up forwarding..."
iptables -A FORWARD -i tun0 -o $INTERNET_IF -j ACCEPT
iptables -A FORWARD -i $INTERNET_IF -o tun0 -m state --state RELATED,ESTABLISHED -j ACCEPT
echo "✓ Forwarding rules added"

echo ""
echo "✓ NAT configured successfully!"
echo ""
echo "Verify with:"
echo "  iptables -t nat -L POSTROUTING -n -v"
echo ""
