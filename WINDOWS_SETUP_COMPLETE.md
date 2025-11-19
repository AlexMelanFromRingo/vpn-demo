# Полная настройка Windows клиента - Пошаговая инструкция

## Проблемы которые решаем

1. ❌ Нет иконки VPN подключения в Windows
2. ❌ Ping с сервера на клиент не работает

## Решение за 5 шагов

### Шаг 1: Обновите клиент

Скопируйте **новый** `vpn-client.exe` на Windows:

**В WSL2:**
```bash
cd /home/user/vpn-demo
cp bin/vpn-client.exe /mnt/c/Users/<ваш_username>/Desktop/
```

Или через Проводник: `\\wsl$\Ubuntu\home\user\vpn-demo\bin\vpn-client.exe`

---

### Шаг 2: Остановите старый клиент (если запущен)

**В Windows (PowerShell или CMD):**
```cmd
taskkill /IM vpn-client.exe /F
```

---

### Шаг 3: Запустите новый клиент от Администратора

1. Найдите `vpn-client.exe` на Рабочем столе
2. **ПКМ → "Запуск от имени администратора"**
3. Дождитесь сообщения `Client started successfully!`

**Вы должны увидеть:**
```
=== Lightweight VPN Client ===
...
Wintun adapter created: vpn0
TUN interface created: vpn0 (MTU: 1420)
Configuring Windows adapter 'vpn0' with IP 10.0.0.2    ← НОВОЕ!
IP address configured successfully                      ← НОВОЕ!
MTU set to 1420                                        ← НОВОЕ!
TUN interface ready: vpn0
...
Handshake successful!
Client started successfully!
```

**Важно:** Теперь вы увидите сообщения о конфигурации адаптера!

---

### Шаг 4: Проверьте адаптер

**Откройте НОВОЕ окно PowerShell от Администратора:**

```powershell
# Проверить статус адаптера
Get-NetAdapter | Where-Object {$_.InterfaceDescription -like "*Wintun*"}
```

**Ожидаемый вывод:**
```
Name    InterfaceDescription      Status    MacAddress
----    --------------------      ------    ----------
vpn0    Wintun Userspace Tunnel   Up        00-00-00-00-00-00
```

**Важно:** `Status` должен быть `Up`, а не `Disconnected`!

**Проверить IP адрес:**
```powershell
Get-NetIPAddress -InterfaceAlias "vpn0"
```

**Ожидаемый вывод:**
```
IPAddress         : 10.0.0.2
InterfaceAlias    : vpn0
AddressFamily     : IPv4
```

**Или через ipconfig:**
```cmd
ipconfig | findstr /C:"vpn0" /C:"10.0.0"
```

---

### Шаг 5: Настройте Windows Firewall

**Вариант A: Автоматически (скрипт)**

1. Скопируйте скрипт на Windows:
   ```bash
   # В WSL2
   cp scripts/setup-windows-firewall.bat /mnt/c/Users/<username>/Desktop/
   ```

2. **ПКМ → "Запуск от имени администратора"** на `setup-windows-firewall.bat`

3. Дождитесь сообщения `Configuration Complete!`

**Вариант B: Вручную (PowerShell от Администратора)**

```powershell
# 1. Разрешить ICMP (ping)
New-NetFirewallRule -DisplayName "VPN - Allow ICMPv4-In" `
    -Protocol ICMPv4 -IcmpType 8 -Enabled True -Direction Inbound -Action Allow

# 2. Разрешить весь трафик на VPN интерфейсе (опционально, но рекомендуется)
New-NetFirewallRule -DisplayName "VPN - Allow All Inbound" `
    -Enabled True -Direction Inbound -Action Allow -InterfaceAlias "vpn0"

New-NetFirewallRule -DisplayName "VPN - Allow All Outbound" `
    -Enabled True -Direction Outbound -Action Allow -InterfaceAlias "vpn0"

# 3. Проверить правила
Get-NetFirewallRule | Where-Object {$_.DisplayName -like "VPN*"}
```

---

## Проверка работы

### Тест 1: Ping с Windows → Server

**На Windows:**
```cmd
ping 10.0.0.1
```

**Ожидаемый результат:**
```
Ответ от 10.0.0.1: число байт=32 время<1мс TTL=64
Ответ от 10.0.0.1: число байт=32 время=1мс TTL=64
Ответ от 10.0.0.1: число байт=32 время=2мс TTL=64
Ответ от 10.0.0.1: число байт=32 время=2мс TTL=64

Статистика Ping для 10.0.0.1:
    Пакетов: отправлено = 4, получено = 4, потеряно = 0
    (0% потерь)
```

✅ **Должно работать!**

---

### Тест 2: Ping с Server → Windows

**На Linux (WSL2):**
```bash
ping 10.0.0.2
```

**Ожидаемый результат:**
```
PING 10.0.0.2 (10.0.0.2) 56(84) bytes of data.
64 bytes from 10.0.0.2: icmp_seq=1 ttl=128 time=2.05 ms
64 bytes from 10.0.0.2: icmp_seq=2 ttl=128 time=1.23 ms
64 bytes from 10.0.0.2: icmp_seq=3 ttl=128 time=1.45 ms
64 bytes from 10.0.0.2: icmp_seq=4 ttl=128 time=1.67 ms

--- 10.0.0.2 ping statistics ---
4 packets transmitted, 4 received, 0% packet loss, time 3004ms
rtt min/avg/max/mdev = 1.234/1.600/2.054/0.312 ms
```

✅ **Должно работать после настройки Firewall!**

---

## Проверка иконки VPN

### Где искать иконку:

1. **Панель задач (трей):**
   - Нажмите `^` (стрелка вверх) возле часов
   - Нажмите на иконку сети
   - Должен появиться адаптер `vpn0`

2. **Центр управления сетями:**
   - Win + R → `ncpa.cpl` → Enter
   - Вы должны увидеть адаптер **"vpn0"** или **"Ethernet"** с описанием **"Wintun Userspace Tunnel"**
   - Статус должен быть **"Подключено"** (зеленая галочка)

3. **Настройки Windows:**
   - Параметры → Сеть и Интернет → Состояние
   - Прокрутите вниз → "Изменение параметров адаптера"
   - Должен быть адаптер `vpn0`

**Что вы должны увидеть:**
```
┌─────────────────────────┐
│  Ethernet               │  ← Может называться "Ethernet" вместо "vpn0"
│  Wintun Userspace...    │
│  ✓ Подключено           │  ← Статус "Подключено"
│  10.0.0.2               │  ← IP адрес
└─────────────────────────┘
```

---

## Troubleshooting

### Адаптер показывает "Disconnected"

**Причина:** IP адрес не установлен

**Решение:**
```powershell
# Установить IP вручную
netsh interface ip set address name="vpn0" source=static addr=10.0.0.2 mask=255.255.255.0 gateway=10.0.0.1

# Проверить
Get-NetIPAddress -InterfaceAlias "vpn0"
```

### Firewall правило не работает

**Проверьте профиль Firewall:**
```powershell
# Посмотреть активный профиль
Get-NetFirewallProfile | Select-Object Name, Enabled

# Убедитесь правило для всех профилей
Get-NetFirewallRule -DisplayName "VPN - Allow ICMPv4-In" | Select-Object -ExpandProperty Profile
```

**Если нужно, пересоздайте правило для всех профилей:**
```powershell
Remove-NetFirewallRule -DisplayName "VPN - Allow ICMPv4-In" -ErrorAction SilentlyContinue

New-NetFirewallRule -DisplayName "VPN - Allow ICMPv4-In" `
    -Protocol ICMPv4 -IcmpType 8 -Enabled True -Direction Inbound -Action Allow `
    -Profile Domain,Private,Public
```

### Адаптер вообще не создается

**Проверьте wintun.dll:**
```cmd
dir | findstr wintun.dll
```

Если нет - клиент должен скачать автоматически. Если не скачивает:
```powershell
# Скачать вручную
Invoke-WebRequest -Uri "https://www.wintun.net/builds/wintun-0.14.1.zip" -OutFile "wintun.zip"
Expand-Archive wintun.zip
Copy-Item "wintun\wintun\bin\amd64\wintun.dll" -Destination "."
```

### Ping работает только в одну сторону

**Проверьте логи клиента** - должны быть сообщения о конфигурации:
```
Configuring Windows adapter 'vpn0' with IP 10.0.0.2
IP address configured successfully
```

Если этих сообщений нет - используете старый vpn-client.exe!

### Имя адаптера не "vpn0"

На Windows адаптер может называться по-другому. Узнайте реальное имя:
```powershell
Get-NetAdapter | Where-Object {$_.InterfaceDescription -like "*Wintun*"} | Select-Object Name
```

Используйте это имя в командах вместо "vpn0".

---

## Итоговый чеклист

После всех шагов у вас должно быть:

- ✅ Клиент запущен и показывает `Client started successfully!`
- ✅ Адаптер виден: `Get-NetAdapter` показывает Status = Up
- ✅ IP настроен: `ipconfig` показывает 10.0.0.2
- ✅ Firewall настроен: правила VPN созданы
- ✅ Ping Windows → Server работает (0% loss)
- ✅ Ping Server → Windows работает (0% loss)
- ✅ Иконка адаптера видна в `ncpa.cpl`

**Если все пункты ✅ - VPN работает полностью!** 🎉

---

## Быстрая команда для проверки всего

**PowerShell от Администратора:**
```powershell
Write-Host "`n=== VPN Status Check ===`n" -ForegroundColor Cyan

Write-Host "1. Adapter Status:" -ForegroundColor Yellow
Get-NetAdapter | Where-Object {$_.InterfaceDescription -like "*Wintun*"} | Format-Table Name, Status, MacAddress

Write-Host "`n2. IP Configuration:" -ForegroundColor Yellow
Get-NetIPAddress -InterfaceAlias "vpn0" -AddressFamily IPv4 | Format-Table IPAddress, PrefixLength

Write-Host "`n3. Firewall Rules:" -ForegroundColor Yellow
Get-NetFirewallRule | Where-Object {$_.DisplayName -like "VPN*"} | Format-Table DisplayName, Enabled, Direction, Action

Write-Host "`n4. Ping Test:" -ForegroundColor Yellow
ping -n 4 10.0.0.1

Write-Host "`n=== Check Complete ===`n" -ForegroundColor Cyan
```

Сохраните как `check-vpn.ps1` и запустите:
```powershell
.\check-vpn.ps1
```
