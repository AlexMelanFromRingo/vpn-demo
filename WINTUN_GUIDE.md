# Wintun Guide - Windows Installation

## Что такое Wintun?

**Wintun** - это современный TUN драйвер для Windows от создателей WireGuard. Он:
- ✅ Быстрее TAP-Windows (в 2-3 раза!)
- ✅ Автоматически устанавливается при первом запуске
- ✅ Не требует установки отдельных программ
- ✅ Используется в WireGuard, Tailscale и других VPN

## Подготовка Windows клиента

### Вариант 1: Автоматическая загрузка (рекомендуется)

VPN клиент **автоматически скачает** wintun.dll при первом запуске!

1. **Скопируйте** `vpn-client.exe` на Windows
2. **Запустите от Администратора** (ПКМ → "Запуск от имени администратора")
3. При первом запуске появится:
   ```
   wintun.dll not found, downloading...
   Downloading Wintun 0.14.1...
   wintun.dll installed to: C:\path\to\wintun.dll
   ```

### Вариант 2: Ручная установка wintun.dll

Если автоматическая загрузка не работает (блокирует firewall):

1. **Скачайте Wintun** с официального сайта:
   - https://www.wintun.net/
   - Или прямая ссылка: https://www.wintun.net/builds/wintun-0.14.1.zip

2. **Распакуйте архив**

3. **Скопируйте правильную версию wintun.dll**:
   - Для 64-bit Windows: `wintun\bin\amd64\wintun.dll`
   - Для 32-bit Windows: `wintun\bin\x86\wintun.dll`
   - Для ARM64: `wintun\bin\arm64\wintun.dll`

4. **Поместите** `wintun.dll` в ту же папку где `vpn-client.exe`:
   ```
   C:\VPN\
   ├── vpn-client.exe
   └── wintun.dll     ← здесь!
   ```

### Вариант 3: Встроенная wintun.dll (для разработчиков)

Можно встроить wintun.dll прямо в exe через `go:embed`:

```go
//go:embed wintun.dll
var wintunDLL []byte
```

Но это увеличит размер exe на ~200KB.

## Первый запуск

1. **Запустите PowerShell или CMD от Администратора**:
   - Нажмите Win + X
   - Выберите "Windows PowerShell (Администратор)"

2. **Перейдите в папку с клиентом**:
   ```cmd
   cd C:\VPN
   ```

3. **Запустите клиент**:
   ```cmd
   .\vpn-client.exe -server 172.26.171.205:51820
   ```

4. **Первый запуск установит Wintun драйвер**:
   ```
   === Lightweight VPN Client ===
   Server: 172.26.171.205:51820
   TUN IP: 10.0.0.2/24
   wintun.dll not found, downloading...      ← Скачивание (если нужно)
   Downloading Wintun 0.14.1...
   wintun.dll installed
   Wintun adapter created: vpn0              ← Создание адаптера
   Note: Wintun driver is automatically installed  ← Драйвер установлен!
   TUN interface created: vpn0 (MTU: 1420)
   Initiating handshake with server...
   ```

## Проверка установки

### Проверить wintun.dll

```powershell
# Проверить файл существует
Test-Path .\wintun.dll
# Должно вернуть: True

# Посмотреть размер
Get-Item .\wintun.dll | Select-Object Name, Length
# Должно быть: ~200 KB
```

### Проверить сетевой адаптер

```powershell
# Посмотреть все сетевые адаптеры
Get-NetAdapter

# Найти Wintun адаптер
Get-NetAdapter | Where-Object {$_.InterfaceDescription -like "*Wintun*"}
```

Вы должны увидеть что-то вроде:
```
Name         InterfaceDescription         Status
----         --------------------         ------
vpn0         Wintun Userspace Tunnel      Up
```

### Проверить IP адрес

```cmd
ipconfig

# Найдите секцию Wintun:
Ethernet adapter vpn0:
   IPv4 Address. . . . . . . . . : 10.0.0.2
   Subnet Mask . . . . . . . . . : 255.255.255.0
```

## Использование

### Запуск с параметрами

```cmd
vpn-client.exe -server 172.26.171.205:51820 -tun-ip 10.0.0.2/24 -peer-ip 10.0.0.1
```

### Использование BAT скрипта

Создайте `start-vpn.bat`:
```bat
@echo off
cd /d "%~dp0"
vpn-client.exe -server 172.26.171.205:51820
pause
```

Запустите от Администратора (ПКМ → "Запуск от имени администратора")

## Устранение проблем

### wintun.dll not found (даже после скачивания)

**Причина:** Антивирус блокирует или удаляет файл

**Решение:**
```powershell
# 1. Добавьте папку в исключения Windows Defender
Add-MpPreference -ExclusionPath "C:\VPN"

# 2. Скачайте wintun.dll вручную (см. Вариант 2)
```

### Failed to create Wintun adapter

**Причина:** Не запущено от Администратора

**Решение:**
- Закройте программу
- ПКМ на vpn-client.exe → "Запуск от имени администратора"

### Access denied

**Причина:** Нет прав администратора

**Решение:**
```powershell
# Проверить права
[Security.Principal.WindowsIdentity]::GetCurrent().Groups -contains 'S-1-5-32-544'
# Должно вернуть: True

# Если False - запустите PowerShell от Администратора
```

### Driver installation failed

**Причина 1:** Windows блокирует неподписанные драйверы

**Решение:**
```powershell
# Временно отключить проверку подписей (требует перезагрузки)
bcdedit /set testsigning on
# Перезагрузите компьютер
# После установки верните обратно:
bcdedit /set testsigning off
```

**Причина 2:** Антивирус блокирует установку

**Решение:**
- Временно отключите антивирус
- Или добавьте vpn-client.exe и wintun.dll в исключения

### Cannot download wintun.dll

**Причина:** Firewall блокирует HTTPS

**Решение:**
```powershell
# Скачайте вручную через браузер
Start-Process "https://www.wintun.net/builds/wintun-0.14.1.zip"

# Или используйте PowerShell:
Invoke-WebRequest -Uri "https://www.wintun.net/builds/wintun-0.14.1.zip" -OutFile "wintun.zip"
Expand-Archive wintun.zip
Copy-Item "wintun\wintun\bin\amd64\wintun.dll" -Destination "."
```

## Удаление

### Удалить Wintun адаптер

```powershell
# Найти адаптер
Get-NetAdapter | Where-Object {$_.InterfaceDescription -like "*Wintun*"}

# Удалить (замените Name на реальное имя)
Remove-NetAdapter -Name "vpn0" -Confirm:$false
```

### Удалить Wintun драйвер

```cmd
# В Device Manager (Диспетчер устройств):
1. Win + X → Device Manager
2. View → Show hidden devices
3. Network adapters → Wintun Userspace Tunnel
4. ПКМ → Uninstall device
5. Поставьте галочку "Delete the driver software"
6. OK
```

Или через командную строку:
```powershell
pnputil /enum-drivers
# Найдите wintun.inf
pnputil /delete-driver oem##.inf /uninstall
```

## Преимущества Wintun

| | Wintun | TAP-Windows |
|---------|--------|-------------|
| **Скорость** | ~2 Gbps | ~800 Mbps |
| **Latency** | ~0.3 ms | ~1 ms |
| **Установка** | Автоматически | Вручную |
| **Драйвер** | Userspace | Kernel |
| **Используется в** | WireGuard, Tailscale | OpenVPN |

## Безопасность

- ✅ Wintun драйвер **подписан Microsoft**
- ✅ Код открыт: https://git.zx2c4.com/wintun/
- ✅ Проверен в WireGuard (миллионы пользователей)
- ✅ Нет kernel-mode кода (безопаснее)

## Системные требования

- **ОС**: Windows 7, 8, 10, 11, Server 2008 R2+
- **Архитектура**: x86, x64, ARM64
- **Права**: Администратор (только при установке)
- **Размер**: wintun.dll ~200 KB

## Дополнительная информация

- **Официальный сайт**: https://www.wintun.net/
- **Документация**: https://git.zx2c4.com/wintun/about/
- **WireGuard**: https://www.wireguard.com/

## Альтернативы

Если Wintun не работает, можно использовать TAP-Windows:
1. Скачайте: https://build.openvpn.net/downloads/releases/
2. Установите tap-windows-*.exe
3. Но наш VPN **оптимизирован для Wintun**, TAP-Windows не поддерживается

---

**Итог**: Wintun устанавливается автоматически! Просто запустите `vpn-client.exe` от Администратора и всё! 🚀
