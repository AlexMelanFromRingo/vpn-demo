# Сводка последних изменений VPN

## Что было исправлено

### ✅ 1. Дубликаты адаптеров Windows (vpn0, vpn0 2, vpn0 20...)
- **Проблема:** При каждом запуске создавался новый адаптер
- **Решение:**
  - Используется детерминированный GUID на основе SHA256 хеша имени адаптера
  - При запуске сначала проверяется существующий адаптер (OpenAdapter)
  - Только если адаптер не найден, создается новый с фиксированным GUID
  - Теперь адаптер переиспользуется вместо создания дубликатов
- **Файлы:** `pkg/tun/tun_windows.go`
- **Скрипт очистки:** `scripts/cleanup-wintun-duplicates.ps1`

### ✅ 2. Пропадание интернета при подключении VPN
- **Проблема:** Когда VPN подключался, интернет переставал работать
- **Решение:**
  - Убран параметр `gateway` из конфигурации IP адаптера
  - VPN больше не становится маршрутом по умолчанию
  - Только трафик для сети 10.0.0.0/24 идет через VPN
  - Весь остальной трафик (интернет) идет через обычное подключение
- **Файлы:** `pkg/tun/setup_windows.go`

### 🔍 3. Добавлено детальное логирование для отладки ping
- **Проблема:** Ping от сервера к клиенту не работает (100% packet loss)
- **Решение:** Добавлено подробное логирование ICMP пакетов:
  - Сервер логирует чтение ICMP из TUN и отправку клиенту
  - Клиент логирует получение ICMP от сервера и запись в TUN
  - Клиент логирует чтение ICMP из TUN и отправку на сервер
  - Теперь видно точно, на каком этапе теряются пакеты
- **Файлы:** `cmd/server/main.go`, `cmd/client/main.go`
- **Документация:** `PING_DEBUG_GUIDE.md`, `QUICK_PING_FIX.md`
- **Скрипт:** `scripts/debug-server-ping.sh`

### ℹ️ 4. Иконка VPN в системном трее
- **Проблема:** Windows не показывает иконку VPN в системном трее
- **Статус:** Невозможно исправить (ограничение Wintun)
- **Объяснение:**
  - Windows 11 показывает иконку VPN только для встроенного Windows VPN (RAS/NDIS)
  - Wintun - это Layer 3 TUN драйвер, а не Windows RAS VPN
  - Все современные VPN на Wintun имеют такое же поведение:
    * WireGuard - нет иконки VPN
    * Tailscale - нет иконки VPN
    * Наш VPN - нет иконки VPN
  - OpenVPN меняет WiFi на Ethernet потому что TAP-Windows это Ethernet адаптер
  - Это не означает, что VPN не работает - это просто визуальное ограничение

## Новые файлы

- `PING_DEBUG_GUIDE.md` - Подробное руководство по отладке ping
- `QUICK_PING_FIX.md` - Быстрое руководство по исправлению ping
- `scripts/cleanup-wintun-duplicates.ps1` - Скрипт очистки дубликатов адаптеров
- `scripts/debug-server-ping.sh` - Скрипт отладки на Linux

## Измененные файлы

- `pkg/tun/tun_windows.go` - Детерминированный GUID и OpenAdapter
- `pkg/tun/setup_windows.go` - Убран параметр gateway
- `cmd/server/main.go` - Логирование ICMP пакетов
- `cmd/client/main.go` - Логирование ICMP пакетов

## Что нужно сделать сейчас

### 1. Очистить старые дубликаты адаптеров (если есть)

**PowerShell от Администратора:**
```powershell
# Скопировать скрипт из WSL2 на Desktop
# В WSL2:
cp /home/user/vpn-demo/scripts/cleanup-wintun-duplicates.ps1 /mnt/c/Users/<username>/Desktop/

# На Windows:
# ПКМ → "Запуск от имени администратора"
```

### 2. Обновить клиент на Windows

```bash
# В WSL2:
cd /home/user/vpn-demo
cp bin/vpn-client.exe /mnt/c/Users/<username>/Desktop/vpn-client-new.exe
```

**На Windows:**
- Остановить старый клиент: `taskkill /IM vpn-client.exe /F`
- ПКМ на `vpn-client-new.exe` → "Запуск от имени администратора"

### 3. Перезапустить сервер

```bash
# В WSL2:
cd /home/user/vpn-demo
sudo ./bin/vpn-server
```

### 4. Проверить что дубликаты больше не создаются

Перезапустите клиент 2-3 раза и проверьте:

```powershell
Get-NetAdapter | Where-Object {$_.InterfaceDescription -like "*Light*"}
```

Должен быть **только один** адаптер `vpn0`.

### 5. Проверить что интернет работает

```powershell
# При запущенном VPN клиенте:
ping google.com
# Должен работать!

ping 10.0.0.1
# Тоже должен работать!
```

### 6. Отладить ping от сервера к клиенту

Следуйте инструкциям в `QUICK_PING_FIX.md`:

1. Запустить сервер и клиент с новыми бинарниками
2. Выполнить `ping 10.0.0.2` на сервере
3. Скопировать логи сервера и клиента
4. Определить где теряются пакеты по логам
5. Применить исправление (скорее всего Windows Firewall)

## Вероятная причина проблемы с ping

Скорее всего, Windows Firewall блокирует входящий ICMP на VPN адаптере.

**Быстрый тест (PowerShell от Администратора):**
```powershell
# Временно отключить Firewall
Set-NetFirewallProfile -Profile Domain,Public,Private -Enabled False

# Проверить ping с сервера (в WSL2: ping 10.0.0.2)

# Включить обратно
Set-NetFirewallProfile -Profile Domain,Public,Private -Enabled True
```

Если с отключенным Firewall ping работает, добавьте правило:
```powershell
New-NetFirewallRule -DisplayName "VPN - Allow All Inbound" `
    -InterfaceAlias "vpn0" `
    -Direction Inbound `
    -Action Allow `
    -Enabled True
```

## Коммиты

1. **9cbf2e1** - Fix critical Windows VPN issues: duplicate adapters and internet connectivity
2. **b165e98** - Add comprehensive ICMP/ping debugging with detailed packet logging

## Следующие шаги

1. Протестируйте новые бинарники
2. Проверьте что дубликаты больше не создаются
3. Проверьте что интернет работает при подключенном VPN
4. Отладьте ping с помощью новых логов
5. Отправьте мне логи если проблема остается
