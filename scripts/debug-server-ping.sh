#!/bin/bash

# Скрипт для отладки ping на сервере (Linux/WSL2)

echo "=== VPN Server Ping Debug ==="
echo ""

echo "1. Checking TUN interface..."
ip addr show tun0
echo ""

echo "2. Checking routing table for 10.0.0.0/24..."
ip route | grep "10.0.0"
echo ""

echo "3. Checking if TUN interface can ping itself..."
ping -c 3 10.0.0.1
echo ""

echo "4. Enabling ICMP debugging (requires root)..."
echo "   Run: sudo tcpdump -i tun0 -n icmp"
echo "   Then in another terminal: ping 10.0.0.2"
echo ""

echo "5. Checking if packets are being sent to TUN..."
echo "   Run these commands in separate terminals:"
echo "   Terminal 1: sudo tcpdump -i tun0 -n"
echo "   Terminal 2: ping 10.0.0.2"
echo ""

echo "6. Manual test - Send ICMP packet directly to TUN:"
echo "   sudo hping3 -1 --icmp-type 8 -c 3 10.0.0.2"
echo ""

echo "=== Setup commands if needed ==="
echo ""
echo "# Add route if missing:"
echo "sudo ip route add 10.0.0.0/24 dev tun0"
echo ""
echo "# Enable IP forwarding:"
echo "sudo sysctl -w net.ipv4.ip_forward=1"
echo ""
echo "# Check if TUN is UP:"
echo "sudo ip link set tun0 up"
echo ""
