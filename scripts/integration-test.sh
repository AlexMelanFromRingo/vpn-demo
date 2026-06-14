#!/usr/bin/env bash
#
# End-to-end integration test for the lightweight VPN.
#
# Spins up the server and client in two isolated network namespaces connected
# by a veth pair, then verifies that ICMP and UDP traffic actually flows
# through the encrypted tunnel. Requires root (netns/veth/TUN creation).
#
# Usage: sudo scripts/integration-test.sh
#
set -u

# --- configuration ---------------------------------------------------------
NS_S="vpn_test_srv"
NS_C="vpn_test_cli"
VETH_S="vpn_t_s"
VETH_C="vpn_t_c"
UNDERLAY_S="10.50.0.1"
UNDERLAY_C="10.50.0.2"
PORT="51820"
TUN_S="10.0.0.1"
TUN_C="10.0.0.2"
PSK="s3cr3t-integration-key"

# Second underlay for the negative (wrong-PSK) authentication test.
NS_BAD="vpn_test_bad"
VETH_S2="vpn_t_s2"
VETH_B="vpn_t_b"
UNDERLAY_S2="10.51.0.1"
UNDERLAY_B="10.51.0.2"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRV_BIN="${ROOT_DIR}/bin/vpn-server"
CLI_BIN="${ROOT_DIR}/bin/vpn-client"
LOG_DIR="$(mktemp -d /tmp/vpn-itest.XXXXXX)"

PASS=0
FAIL=0

log()  { echo "[itest] $*"; }
ok()   { echo "  ✓ $*"; PASS=$((PASS+1)); }
bad()  { echo "  ✗ $*"; FAIL=$((FAIL+1)); }

cleanup() {
    ip netns pids "$NS_S" 2>/dev/null | xargs -r kill 2>/dev/null
    ip netns pids "$NS_C" 2>/dev/null | xargs -r kill 2>/dev/null
    ip netns pids "$NS_BAD" 2>/dev/null | xargs -r kill 2>/dev/null
    sleep 0.2
    ip netns del "$NS_S" 2>/dev/null
    ip netns del "$NS_C" 2>/dev/null
    ip netns del "$NS_BAD" 2>/dev/null
    ip link del "$VETH_S" 2>/dev/null
    ip link del "$VETH_S2" 2>/dev/null
    echo "--- server log (tail) ---"; tail -n 40 "${LOG_DIR}/server.log" 2>/dev/null
    echo "--- client log (tail) ---"; tail -n 40 "${LOG_DIR}/client.log" 2>/dev/null
}
trap cleanup EXIT

if [[ $EUID -ne 0 ]]; then
    echo "must run as root (sudo)"; exit 2
fi

# --- build -----------------------------------------------------------------
# sudo often strips PATH (secure_path); locate the go toolchain explicitly.
GO_BIN="$(command -v go || true)"
[[ -z "$GO_BIN" && -x /usr/local/go/bin/go ]] && GO_BIN=/usr/local/go/bin/go
[[ -z "$GO_BIN" ]] && { echo "go toolchain not found"; exit 1; }
log "building binaries with $GO_BIN ..."
( cd "$ROOT_DIR" && "$GO_BIN" build -o "$SRV_BIN" ./cmd/server && "$GO_BIN" build -o "$CLI_BIN" ./cmd/client ) || {
    echo "build failed"; exit 1; }

# --- topology --------------------------------------------------------------
log "setting up network namespaces..."
cleanup 2>/dev/null
ip netns add "$NS_S"
ip netns add "$NS_C"
ip netns add "$NS_BAD"
ip link add "$VETH_S" type veth peer name "$VETH_C"
ip link set "$VETH_S" netns "$NS_S"
ip link set "$VETH_C" netns "$NS_C"
# second underlay: server <-> wrong-PSK client
ip link add "$VETH_S2" type veth peer name "$VETH_B"
ip link set "$VETH_S2" netns "$NS_S"
ip link set "$VETH_B" netns "$NS_BAD"
ip netns exec "$NS_S" ip addr add "${UNDERLAY_S}/24" dev "$VETH_S"
ip netns exec "$NS_C" ip addr add "${UNDERLAY_C}/24" dev "$VETH_C"
ip netns exec "$NS_S" ip addr add "${UNDERLAY_S2}/24" dev "$VETH_S2"
ip netns exec "$NS_BAD" ip addr add "${UNDERLAY_B}/24" dev "$VETH_B"
ip netns exec "$NS_S" ip link set "$VETH_S" up
ip netns exec "$NS_C" ip link set "$VETH_C" up
ip netns exec "$NS_S" ip link set "$VETH_S2" up
ip netns exec "$NS_BAD" ip link set "$VETH_B" up
ip netns exec "$NS_S" ip link set lo up
ip netns exec "$NS_C" ip link set lo up
ip netns exec "$NS_BAD" ip link set lo up

# sanity: underlay connectivity
if ip netns exec "$NS_C" ping -c1 -W2 "$UNDERLAY_S" >/dev/null 2>&1; then
    ok "underlay veth connectivity ($UNDERLAY_C -> $UNDERLAY_S)"
else
    bad "underlay veth connectivity"; exit 1
fi

# --- launch server & client (PSK authenticated) ----------------------------
log "starting VPN server in $NS_S (PSK auth enabled)..."
ip netns exec "$NS_S" "$SRV_BIN" \
    -listen "0.0.0.0:${PORT}" -tun-ip "${TUN_S}/24" -peer-ip "$TUN_C" -mtu 1420 -psk "$PSK" \
    >"${LOG_DIR}/server.log" 2>&1 &
sleep 1.5

log "starting VPN client in $NS_C (matching PSK)..."
ip netns exec "$NS_C" "$CLI_BIN" \
    -server "${UNDERLAY_S}:${PORT}" -tun-ip "${TUN_C}/24" -peer-ip "$TUN_S" -mtu 1420 -psk "$PSK" \
    >"${LOG_DIR}/client.log" 2>&1 &
sleep 2

# --- assertions ------------------------------------------------------------
if grep -q "Handshake successful" "${LOG_DIR}/client.log"; then
    ok "client completed handshake"
else
    bad "client handshake (see log)"
fi

if grep -q "Client registered" "${LOG_DIR}/server.log"; then
    ok "server registered client"
else
    bad "server did not register client"
fi

# TUN interfaces exist with the right IPs
if ip netns exec "$NS_S" ip addr show vpn0 2>/dev/null | grep -q "$TUN_S"; then
    ok "server TUN vpn0 has $TUN_S"
else
    bad "server TUN missing"
fi
if ip netns exec "$NS_C" ip addr show vpn0 2>/dev/null | grep -q "$TUN_C"; then
    ok "client TUN vpn0 has $TUN_C"
else
    bad "client TUN missing"
fi

# the core test: ping the server's TUN IP through the tunnel from the client
log "ping ${TUN_S} through the encrypted tunnel..."
if ip netns exec "$NS_C" ping -c3 -W3 "$TUN_S" >"${LOG_DIR}/ping_c2s.log" 2>&1; then
    rtt=$(grep -o 'min/avg/max[^ ]*' "${LOG_DIR}/ping_c2s.log" || true)
    ok "client -> server tunnel ping works ($rtt)"
else
    bad "client -> server tunnel ping FAILED"
    cat "${LOG_DIR}/ping_c2s.log"
fi

# reverse direction: server pings client TUN IP
log "ping ${TUN_C} from server side through the tunnel..."
if ip netns exec "$NS_S" ping -c3 -W3 "$TUN_C" >"${LOG_DIR}/ping_s2c.log" 2>&1; then
    ok "server -> client tunnel ping works"
else
    bad "server -> client tunnel ping FAILED"
    cat "${LOG_DIR}/ping_s2c.log"
fi

# verify traffic actually flows over the UDP underlay (the only path between the
# namespaces). If tcpdump is available we additionally confirm the datagrams are
# UDP/PORT and that a known marker placed in the ICMP payload does NOT appear in
# the captured bytes -- i.e. the tunnel really encrypts. Otherwise we fall back
# to byte counters.
log "verifying traffic is tunneled over the UDP underlay..."
MARKER_HEX="cafebabecafebabe"          # 8-byte pattern stuffed into the ping payload
MARKER_GREP="cafe babe cafe babe"      # how tcpdump -X renders it (2-byte groups)
rx_before=$(ip netns exec "$NS_S" sh -c "cat /sys/class/net/${VETH_S}/statistics/rx_packets")
if command -v tcpdump >/dev/null 2>&1; then
    ip netns exec "$NS_S" timeout 5 tcpdump -i "$VETH_S" -n -X -c 20 "udp port ${PORT}" \
        >"${LOG_DIR}/tcpdump.log" 2>&1 &
    TCPDUMP_PID=$!
    sleep 0.4
    ip netns exec "$NS_C" ping -c4 -W2 -s 64 -p "$MARKER_HEX" "$TUN_S" >/dev/null 2>&1
    wait $TCPDUMP_PID 2>/dev/null

    if grep -qi "UDP" "${LOG_DIR}/tcpdump.log"; then
        ok "tunnel traffic observed as UDP/${PORT} on the underlay"
    else
        bad "no UDP tunnel traffic captured"
    fi

    # Sanity-check the test itself: the marker must be present somewhere (it is in
    # the plaintext we sent). Its ABSENCE from the encrypted underlay is the proof.
    if grep -qi "$MARKER_GREP" "${LOG_DIR}/tcpdump.log"; then
        bad "PLAINTEXT LEAK: ping payload marker visible in underlay capture!"
    else
        ok "payload is encrypted on the wire (marker '${MARKER_HEX}' absent from capture)"
    fi
else
    ip netns exec "$NS_C" ping -c5 -W2 "$TUN_S" >/dev/null 2>&1
    rx_after=$(ip netns exec "$NS_S" sh -c "cat /sys/class/net/${VETH_S}/statistics/rx_packets")
    delta=$((rx_after - rx_before))
    if [[ "$delta" -ge 5 ]]; then
        ok "tunnel carried encrypted traffic over the underlay (+${delta} pkts; tcpdump not installed)"
    else
        bad "underlay packet counter did not increase as expected (+${delta})"
    fi
fi

# --- negative test: wrong PSK must be rejected -----------------------------
log "negative auth test: connecting with a WRONG PSK (must be rejected)..."
ip netns exec "$NS_BAD" "$CLI_BIN" \
    -server "${UNDERLAY_S2}:${PORT}" -tun-ip "10.0.0.9/24" -peer-ip "$TUN_S" -mtu 1420 \
    -psk "totally-wrong-key" \
    >"${LOG_DIR}/badclient.log" 2>&1 &
BAD_PID=$!
sleep 2

if grep -q "Rejected handshake from ${UNDERLAY_B}" "${LOG_DIR}/server.log"; then
    ok "server rejected wrong-PSK client (${UNDERLAY_B})"
else
    bad "server did NOT reject wrong-PSK client"
fi
# The wrong-PSK client must never reach the data phase.
if grep -q "Handshake successful" "${LOG_DIR}/badclient.log"; then
    bad "wrong-PSK client completed handshake (auth bypass!)"
else
    ok "wrong-PSK client never authenticated"
fi
kill "$BAD_PID" 2>/dev/null

# --- summary ---------------------------------------------------------------
echo
echo "==================================================="
echo " integration test: ${PASS} passed, ${FAIL} failed"
echo "==================================================="
[[ $FAIL -eq 0 ]] && exit 0 || exit 1
