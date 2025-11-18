#!/bin/bash
# Start VPN server script

set -e

LISTEN_ADDR="${LISTEN_ADDR:-0.0.0.0:51820}"
TUN_IP="${TUN_IP:-10.0.0.1/24}"
PEER_IP="${PEER_IP:-10.0.0.2}"
MTU="${MTU:-1420}"

echo "=== Starting VPN Server ==="
echo "Listen: $LISTEN_ADDR"
echo "TUN IP: $TUN_IP"
echo "Peer IP: $PEER_IP"
echo "MTU: $MTU"
echo ""

if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (sudo)"
    exit 1
fi

# Build if needed
if [ ! -f "../bin/vpn-server" ]; then
    echo "Building server..."
    make -C .. server
fi

# Run server
../bin/vpn-server \
    -listen "$LISTEN_ADDR" \
    -tun-ip "$TUN_IP" \
    -peer-ip "$PEER_IP" \
    -mtu "$MTU"
