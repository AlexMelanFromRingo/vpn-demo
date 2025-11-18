@echo off
REM Start VPN client on Windows
REM Run as Administrator!

set SERVER_ADDR=172.26.171.205:51820
set TUN_IP=10.0.0.2/24
set PEER_IP=10.0.0.1
set MTU=1420

echo === Starting VPN Client for Windows ===
echo Server: %SERVER_ADDR%
echo TUN IP: %TUN_IP%
echo Peer IP: %PEER_IP%
echo MTU: %MTU%
echo.
echo Make sure you run this as Administrator!
echo.

..\bin\vpn-client.exe -server %SERVER_ADDR% -tun-ip %TUN_IP% -peer-ip %PEER_IP% -mtu %MTU%

pause
