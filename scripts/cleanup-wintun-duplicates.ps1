# Cleanup Wintun Duplicate Adapters
# Run as Administrator!

Write-Host "`n=== Wintun Adapter Cleanup ===" -ForegroundColor Cyan
Write-Host "This script removes duplicate vpn0 adapters (vpn0 2, vpn0 20, etc.)`n" -ForegroundColor Yellow

# Check if running as Administrator
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "ERROR: This script must be run as Administrator!" -ForegroundColor Red
    Write-Host "Right-click and select 'Run as Administrator'" -ForegroundColor Yellow
    pause
    exit 1
}

# Find all Wintun adapters
$adapters = Get-NetAdapter | Where-Object {$_.InterfaceDescription -like "*Wintun*"}

if ($adapters.Count -eq 0) {
    Write-Host "No Wintun adapters found" -ForegroundColor Yellow
    pause
    exit 0
}

Write-Host "Found $($adapters.Count) Wintun adapter(s):" -ForegroundColor Yellow
$adapters | Format-Table Name, Status, InterfaceDescription, InterfaceIndex

# Identify duplicates
$duplicates = $adapters | Where-Object {$_.Name -match "vpn0 \d+"}

if ($duplicates.Count -eq 0) {
    Write-Host "`nNo duplicate adapters found! ✓" -ForegroundColor Green
    Write-Host "Your system is clean." -ForegroundColor Green
    pause
    exit 0
}

Write-Host "`nFound $($duplicates.Count) duplicate(s) to remove:" -ForegroundColor Red
$duplicates | Format-Table Name, Status

# Confirm removal
Write-Host "`nWARNING: This will remove the duplicate adapters listed above." -ForegroundColor Yellow
$confirm = Read-Host "Continue? (y/N)"

if ($confirm -ne 'y' -and $confirm -ne 'Y') {
    Write-Host "Cancelled by user" -ForegroundColor Yellow
    exit 0
}

# Remove duplicates
$removed = 0
foreach ($dup in $duplicates) {
    try {
        Write-Host "Removing: $($dup.Name)..." -ForegroundColor Red
        Remove-NetAdapter -Name $dup.Name -Confirm:$false -ErrorAction Stop
        $removed++
        Write-Host "  ✓ Removed" -ForegroundColor Green
    } catch {
        Write-Host "  ✗ Failed: $_" -ForegroundColor Red
    }
}

Write-Host "`n=== Cleanup Complete ===" -ForegroundColor Cyan
Write-Host "Removed $removed duplicate adapter(s)" -ForegroundColor Green
Write-Host "`nRemaining Wintun adapters:" -ForegroundColor Yellow
Get-NetAdapter | Where-Object {$_.InterfaceDescription -like "*Wintun*"} | Format-Table Name, Status

Write-Host "`nNOTE: After cleanup, your VPN client will reuse the same 'vpn0' adapter" -ForegroundColor Cyan
Write-Host "No more duplicates will be created! ✓`n" -ForegroundColor Green

pause
