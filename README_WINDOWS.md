# Windows Quick Start - Быстрый старт для Windows

## ✨ Главное преимущество

**Больше НЕ НУЖНО устанавливать TAP-Windows или OpenVPN!**

VPN клиент:
- ✅ Автоматически скачивает wintun.dll
- ✅ Автоматически устанавливает драйвер
- ✅ Всё работает "из коробки"!

## 🚀 Запуск за 3 шага

### 1. Скопируйте файл на Windows

Из WSL2 скопируйте клиент на Windows диск:
```bash
cp bin/vpn-client.exe /mnt/c/Users/<ваш_username>/Desktop/
```

Или через Проводник: откройте `\\wsl$\Ubuntu\home\user\vpn-demo\bin\`

### 2. Запустите от Администратора

1. Найдите `vpn-client.exe` на Рабочем столе
2. **Правая кнопка мыши** → **"Запуск от имени администратора"**

### 3. Дождитесь автоматической установки

При **первом запуске** вы увидите:

```
=== Lightweight VPN Client ===
Server: 172.26.171.205:51820
TUN IP: 10.0.0.2/24
Client Public Key: ABC123...

wintun.dll not found, downloading...           ← Скачивание DLL
Downloading Wintun 0.14.1...
wintun.dll installed to: C:\Users\...\wintun.dll

Wintun adapter created: vpn0                   ← Создание адаптера
Note: Wintun driver is automatically installed ← Драйвер установлен!

TUN interface created: vpn0 (MTU: 1420)
Initiating handshake with server...
Server Public Key: XYZ789...
Shared secret established (epoch: 123456)
Handshake successful!                          ← Готово!
Client started successfully!
```

**Готово!** VPN подключен! 🎉

## 🔍 Проверка

### Проверить адаптер создан

```cmd
ipconfig
```

Должна быть секция:
```
Ethernet adapter vpn0:
   IPv4 Address. . . . . . . . . : 10.0.0.2
   Subnet Mask . . . . . . . . . : 255.255.255.0
```

### Пинг сервера

```cmd
ping 10.0.0.1
```

Если пакеты идут - **VPN работает!** ✅

## 📝 Параметры запуска

Полная команда с параметрами:
```cmd
vpn-client.exe -server 172.26.171.205:51820 -tun-ip 10.0.0.2/24 -peer-ip 10.0.0.1
```

Где:
- `-server` - IP и порт сервера (замените на IP вашего WSL2)
- `-tun-ip` - ваш IP в VPN сети
- `-peer-ip` - IP сервера в VPN

## 🛡️ Firewall

Если не работает, разрешите UDP порт 51820:

```powershell
# PowerShell от Администратора
New-NetFirewallRule -DisplayName "VPN Client" -Direction Outbound -Protocol UDP -RemotePort 51820 -Action Allow
```

## 📦 Что делает первый запуск?

1. **Проверяет wintun.dll** в папке с exe
2. Если нет - **скачивает** с https://www.wintun.net/
3. **Создает Wintun адаптер** (виртуальный сетевой интерфейс)
4. **Устанавливает драйвер** автоматически
5. **Подключается к серверу** и устанавливает VPN туннель

Всё это **без вашего участия!**

## ⚠️ Возможные проблемы

### "Access denied" или "Permission denied"

**Причина:** Не запущено от Администратора

**Решение:** ПКМ → "Запуск от имени администратора"

### "Failed to download wintun.dll"

**Причина:** Firewall блокирует HTTPS

**Решение:** Скачайте вручную:
1. Откройте в браузере: https://www.wintun.net/builds/wintun-0.14.1.zip
2. Распакуйте архив
3. Скопируйте `wintun\bin\amd64\wintun.dll` в папку с `vpn-client.exe`

### Антивирус удаляет wintun.dll

**Решение:**
```powershell
# Добавить в исключения Windows Defender
Add-MpPreference -ExclusionPath "C:\Users\...\vpn-client.exe"
Add-MpPreference -ExclusionPath "C:\Users\...\wintun.dll"
```

### Connection timeout

**Причины:**
- Неправильный IP сервера
- Firewall блокирует UDP 51820
- Сервер не запущен

**Решение:**
1. Узнайте правильный IP WSL2:
   ```bash
   # В WSL2:
   ip addr show eth0 | grep "inet "
   ```
2. Используйте этот IP в `-server IP:51820`

## 🎯 Автоматический запуск

Создайте `start-vpn.bat`:

```bat
@echo off
cd /d "%~dp0"

echo === VPN Client ===
echo Starting VPN connection...
echo.

vpn-client.exe -server 172.26.171.205:51820

pause
```

Сохраните рядом с `vpn-client.exe` и запускайте этот файл от Администратора.

## 📚 Подробная документация

- [WINTUN_GUIDE.md](WINTUN_GUIDE.md) - Полное руководство по Wintun
- [README.md](README.md) - Основная документация
- [QUICKSTART.md](QUICKSTART.md) - Быстрый старт для всех платформ

## 🆚 Wintun vs TAP-Windows

| | Wintun (наш VPN) | TAP-Windows (OpenVPN) |
|---------|------------------|----------------------|
| Установка | Автоматически | Вручную |
| Скорость | ~2 Gbps | ~800 Mbps |
| Задержка | ~0.3 ms | ~1 ms |
| Используется | WireGuard, Tailscale | OpenVPN |
| Подпись Microsoft | ✅ Да | ✅ Да |

## ❓ FAQ

**Q: Нужно ли устанавливать OpenVPN?**
A: **НЕТ!** Wintun устанавливается автоматически.

**Q: Нужно ли устанавливать TAP-Windows?**
A: **НЕТ!** Wintun заменяет TAP-Windows.

**Q: Безопасен ли Wintun?**
A: **ДА!** Используется в WireGuard и Tailscale, подписан Microsoft.

**Q: Можно ли встроить wintun.dll в exe?**
A: Да, через `go:embed`, но это увеличит размер на ~200KB.

**Q: Какая архитектура поддерживается?**
A: x86, x64 (amd64), ARM64

**Q: Работает ли на Windows 7?**
A: Да! Windows 7, 8, 10, 11, Server 2008 R2+

---

**Итог:** Просто запустите от Администратора - всё остальное автоматически! 🚀
