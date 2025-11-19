# gVisor Limitations for VPN NAT

## Problem

Your environment is running **gVisor**, not a real Linux kernel. This is confirmed by:
```bash
$ dmesg | head -1
[0.000000] Starting gVisor...
```

gVisor is Google's userspace kernel sandbox. It does **NOT** support:
- ❌ iptables NAT (MASQUERADE/SNAT)
- ❌ netfilter kernel modules
- ❌ Full networking stack for routing/forwarding

This means **you cannot run a VPN server with NAT forwarding** in this environment using traditional methods.

## Why NAT Doesn't Work

```bash
$ iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -j MASQUERADE
Warning: Extension MASQUERADE revision 0 not supported, missing kernel module?
iptables: Invalid argument
```

gVisor doesn't have the netfilter kernel modules needed for NAT.

## Solutions

### Option 1: Use Real Linux (Recommended)

Run your VPN server on:
- Real Linux VM (VirtualBox, VMware)
- Cloud server (AWS EC2, DigitalOcean, etc.)
- Native Linux machine
- WSL2 with real kernel (if you can switch)

### Option 2: Split-Tunnel Mode (No NAT)

Keep current setup but **don't route all traffic** through VPN:
- Only VPN network (10.0.0.0/24) works
- Internet goes through normal connection
- This is what we had before

**To re-enable split-tunnel:**

In `pkg/tun/setup_windows.go`, remove the gateway parameter again:
```go
cmd := exec.Command("netsh", "interface", "ip", "set", "address",
    fmt.Sprintf("name=%s", ifname), "source=static",
    fmt.Sprintf("addr=%s", localIPClean),
    fmt.Sprintf("mask=%s", mask))
// NO GATEWAY - split tunnel mode
```

Rebuild and you'll have working VPN for peer-to-peer, just not full internet proxy.

### Option 3: Userspace NAT (Complex)

Implement NAT in the VPN application itself using Go:
- Parse IP packets
- Rewrite source/destination IPs and ports
- Track connections
- Forward to/from internet using raw sockets

This is complex and has performance issues.

### Option 4: Use socat/relay (Limited)

For specific protocols, use relay tools:
```bash
# Relay HTTP
socat TCP-LISTEN:80,fork TCP:INTERNET_HOST:80

# Relay DNS
socat UDP-LISTEN:53,fork UDP:8.8.8.8:53
```

But this won't work for "all traffic" - only specific ports.

## Recommendation

Since you want **all traffic to go through VPN** (работать как прокси), you need one of:

1. **Best**: Run VPN server on real Linux machine/VM
2. **OK**: Use cloud VPS ($5/month) with real Linux kernel
3. **Temporary**: Use split-tunnel mode (no NAT, only VPN network works)

## Current Status

✅ VPN tunnel works (ping 10.0.0.1 succeeds)
❌ Internet forwarding doesn't work (gVisor limitation)

Your error:
```
Ответ от 172.26.160.1: Превышен срок жизни (TTL) при передаче пакета.
```

This happens because:
1. Client sends packet to VPN server (10.0.0.1)
2. Server receives it on TUN interface
3. Server tries to forward to internet
4. gVisor can't do NAT, so packet dies
5. TTL exceeded error returned

## Next Steps

**Choose one:**

A. **Get real Linux environment** for VPN server
B. **Accept split-tunnel** mode (no full proxy, but VPN peer-to-peer works)
C. **Use cloud server** ($5/month DigitalOcean/AWS/Hetzner with real kernel)

Which would you prefer?
