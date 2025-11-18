#!/bin/bash
# Start VPN client script

set -e

SERVER_ADDR="${SERVER_ADDR:-172.26.171.205:51820}"
TUN_IP="${TUN_IP:-10.0.0.2/24}"
PEER_IP="${PEER_IP:-10.0.0.1}"
MTU="${MTU:-1420}"

echo "=== Starting VPN Client ==="
echo "Server: $SERVER_ADDR"
echo "TUN IP: $TUN_IP"
echo "Peer IP: $PEER_IP"
echo "MTU: $MTU"
echo ""

if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (sudo)"
    exit 1
fi

# Build if needed
if [ ! -f "../bin/vpn-client" ]; then
    echo "Building client..."
    make -C .. client
fi

# Run client
../bin/vpn-client \
    -server "$SERVER_ADDR" \
    -tun-ip "$TUN_IP" \
    -peer-ip "$PEER_IP" \
    -mtu "$MTU"
