@echo off
REM VPN Firewall Setup Script
REM Run as Administrator!

echo ============================================
echo   VPN Firewall Configuration
echo ============================================
echo.

REM Check if running as Administrator
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo ERROR: This script must be run as Administrator!
    echo.
    echo Right-click on this file and select "Run as administrator"
    pause
    exit /b 1
)

echo [1/3] Checking VPN adapter status...
echo.
powershell -Command "Get-NetAdapter | Where-Object {$_.InterfaceDescription -like '*Wintun*'} | Format-Table Name, InterfaceDescription, Status, MacAddress"
echo.

echo [2/3] Allowing ICMP (ping) through Windows Firewall...
echo.

REM Remove old rule if exists
netsh advfirewall firewall delete rule name="VPN - Allow ICMPv4-In" >nul 2>&1

REM Add new rule
netsh advfirewall firewall add rule name="VPN - Allow ICMPv4-In" protocol=icmpv4:8,any dir=in action=allow

if %errorLevel% equ 0 (
    echo [OK] ICMP rule added successfully
) else (
    echo [ERROR] Failed to add ICMP rule
)
echo.

echo [3/3] Allowing all traffic on VPN interface...
echo.

REM Allow all inbound on VPN interface
powershell -Command "New-NetFirewallRule -DisplayName 'VPN - Allow All Inbound' -Enabled True -Direction Inbound -Action Allow -InterfaceAlias 'vpn0' -ErrorAction SilentlyContinue"

REM Allow all outbound on VPN interface
powershell -Command "New-NetFirewallRule -DisplayName 'VPN - Allow All Outbound' -Enabled True -Direction Outbound -Action Allow -InterfaceAlias 'vpn0' -ErrorAction SilentlyContinue"

echo.
echo ============================================
echo   Configuration Complete!
echo ============================================
echo.
echo Firewall rules created:
netsh advfirewall firewall show rule name="VPN - Allow ICMPv4-In"
echo.
echo ============================================
echo.
echo You can now test ping from server:
echo   Linux: ping 10.0.0.2
echo.
echo Press any key to exit...
pause >nul
