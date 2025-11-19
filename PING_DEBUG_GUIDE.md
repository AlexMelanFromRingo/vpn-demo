# Руководство по отладке ICMP/Ping

Эта версия сервера и клиента включает детальное логирование ICMP пакетов для отладки проблем с ping.

## Что было добавлено

### В сервер (`cmd/server/main.go`):
- Функция `parseIPPacket()` - парсит IP пакеты и определяет тип (ICMP, TCP, UDP)
- Логирование ICMP пакетов из TUN интерфейса
- Логирование отправки ICMP пакетов клиентам
- Предупреждение, если ICMP пакет получен, но нет активных клиентов

### В клиент (`cmd/client/main.go`):
- Функция `parseIPPacket()` - парсит IP пакеты
- Логирование ICMP пакетов от сервера
- Логирование ICMP пакетов к серверу
- Подтверждение записи ICMP в TUN интерфейс

## Как использовать для отладки

### Шаг 1: Перезапустите сервер на Linux/WSL2

```bash
cd /home/user/vpn-demo
sudo ./bin/vpn-server
```

Вы должны увидеть обычные сообщения запуска.

### Шаг 2: Перезапустите клиент на Windows

1. Скопируйте новый `bin/vpn-client.exe` на Windows
2. Запустите от имени Администратора
3. Дождитесь успешного handshake

### Шаг 3: Тест ping от Windows к Серверу

**На Windows (PowerShell):**
```powershell
ping 10.0.0.1 -n 3
```

**Что вы должны увидеть в логах клиента:**
```
TUN → Server: ICMP type=8 code=0 from 10.0.0.2 to 10.0.0.1 (len=84)
✓ Sent ICMP packet to server
Server → TUN: ICMP type=0 code=0 from 10.0.0.1 to 10.0.0.2 (len=84)
✓ Wrote ICMP packet to TUN interface
```

**Что вы должны увидеть в логах сервера:**
```
Received DATA packet from client
(запись в TUN без явного логирования, т.к. это входящий пакет)
TUN → Client: ICMP type=0 code=0 from 10.0.0.1 to 10.0.0.2 (len=84)
✓ Sent ICMP packet to client 172.26.171.205:xxxxx
```

### Шаг 4: Тест ping от Сервера к Windows (ПРОБЛЕМА)

**На Linux/WSL2:**
```bash
ping 10.0.0.2 -c 3
```

**Что вы должны увидеть в логах сервера:**

Если пакеты ПОПАДАЮТ в TUN интерфейс:
```
TUN → Client: ICMP type=8 code=0 from 10.0.0.1 to 10.0.0.2 (len=84)
✓ Sent ICMP packet to client 172.26.171.205:xxxxx
```

Если НЕ видите этих сообщений - проблема в маршрутизации Linux/WSL2!

**Что вы должны увидеть в логах клиента (Windows):**

Если клиент ПОЛУЧАЕТ пакеты:
```
Server → TUN: ICMP type=8 code=0 from 10.0.0.1 to 10.0.0.2 (len=84)
✓ Wrote ICMP packet to TUN interface
```

Если клиент ОТПРАВЛЯЕТ ответ:
```
TUN → Server: ICMP type=0 code=0 from 10.0.0.2 to 10.0.0.1 (len=84)
✓ Sent ICMP packet to server
```

Если НЕ видите этих сообщений - проблема в Windows Firewall или TUN интерфейсе!

## Диагностика по логам

### Сценарий 1: Сервер не видит ICMP пакеты в TUN
**Симптомы:** При `ping 10.0.0.2` на сервере НЕТ логов "TUN → Client: ICMP"

**Причина:** Пакеты не попадают в TUN интерфейс

**Решение:**
```bash
# Проверить TUN интерфейс
ip addr show tun0

# Проверить маршрутизацию
ip route | grep "10.0.0"

# Должна быть запись:
# 10.0.0.0/24 dev tun0 scope link

# Если нет, добавить вручную:
sudo ip route add 10.0.0.0/24 dev tun0

# Проверить с помощью tcpdump
sudo tcpdump -i tun0 -n icmp
# В другом терминале: ping 10.0.0.2
```

### Сценарий 2: Сервер отправляет, клиент не получает
**Симптомы:** Сервер показывает "✓ Sent ICMP packet to client", но клиент не показывает "Server → TUN: ICMP"

**Причина:** Проблема с UDP транспортом или шифрованием

**Решение:**
- Проверить соединение UDP
- Проверить логи на ошибки декодирования

### Сценарий 3: Клиент получает, но не может записать в TUN
**Симптомы:** Клиент показывает "Server → TUN: ICMP", но НЕ показывает "✓ Wrote ICMP packet to TUN interface"

**Причина:** Ошибка записи в TUN интерфейс (должна быть в логах "TUN write error")

**Решение:**
- Проверить, что клиент запущен от имени Администратора
- Проверить статус адаптера в `ncpa.cpl`

### Сценарий 4: Клиент записал в TUN, но Windows Firewall блокирует
**Симптомы:** Клиент показывает "✓ Wrote ICMP packet to TUN interface", но ping на сервере показывает 100% packet loss

**Причина:** Windows Firewall блокирует входящий ICMP на TUN интерфейсе

**Решение (PowerShell от Администратора):**

```powershell
# Вариант 1: Разрешить весь трафик на VPN интерфейс
New-NetFirewallRule -DisplayName "VPN - Allow All Inbound" `
    -InterfaceAlias "vpn0" `
    -Direction Inbound `
    -Action Allow `
    -Enabled True

# Вариант 2: Разрешить только ICMP
New-NetFirewallRule -DisplayName "VPN - ICMPv4 All" `
    -Protocol ICMPv4 `
    -Direction Inbound `
    -Action Allow `
    -Enabled True `
    -Profile Any

# Вариант 3: Временно отключить Firewall (ДЛЯ ТЕСТА!)
Set-NetFirewallProfile -Profile Domain,Public,Private -Enabled False
# Проверить ping с сервера
# ОБЯЗАТЕЛЬНО включить обратно:
Set-NetFirewallProfile -Profile Domain,Public,Private -Enabled True
```

### Сценарий 5: Клиент не отправляет ICMP Reply обратно
**Симптомы:** Клиент записал ICMP Request в TUN, но НЕ показывает "TUN → Server: ICMP type=0"

**Причина:** Windows не генерирует ICMP Reply для TUN интерфейса

**Решение:**
```powershell
# Проверить адаптер
Get-NetAdapter | Where-Object {$_.InterfaceDescription -like "*Light*"}

# Убедиться что Status = Up
# Проверить IP адрес
Get-NetIPAddress -InterfaceAlias "vpn0"

# Проверить маршрутизацию Windows
route print | Select-String "10.0.0"

# Должен быть маршрут для 10.0.0.0/24 через vpn0
```

## Типы ICMP

- **Type 8, Code 0** = Echo Request (ping запрос)
- **Type 0, Code 0** = Echo Reply (ping ответ)
- **Type 3** = Destination Unreachable
- **Type 11** = Time Exceeded

## Следующие шаги

После запуска с детальным логированием:

1. Выполните оба теста ping (Windows→Server и Server→Windows)
2. Скопируйте ВСЕ логи сервера и клиента
3. Определите на каком этапе пакеты теряются по сценариям выше
4. Примените соответствующее решение

## Полезные команды

### Linux/WSL2:
```bash
# Посмотреть TUN интерфейс
ip addr show tun0

# Посмотреть маршруты
ip route | grep 10.0.0

# Мониторинг трафика
sudo tcpdump -i tun0 -n icmp

# Ручная проверка ICMP
sudo hping3 -1 --icmp-type 8 -c 3 10.0.0.2
```

### Windows PowerShell:
```powershell
# Посмотреть адаптеры
Get-NetAdapter | Where-Object {$_.InterfaceDescription -like "*Light*"}

# Посмотреть IP адреса
Get-NetIPAddress -InterfaceAlias "vpn0"

# Посмотреть маршруты
route print

# Посмотреть правила Firewall
Get-NetFirewallRule | Where-Object {$_.DisplayName -like "*VPN*" -or $_.DisplayName -like "*ICMP*"} | Select-Object DisplayName, Enabled, Direction, Action

# Проверить все профили Firewall
Get-NetFirewallProfile | Select-Object Name, Enabled
```
