# VPN Routing Guide - Настройка маршрутизации

## Проблема

VPN туннель установлен, но трафик не идет через него, потому что:
1. Нет маршрутов через VPN интерфейс
2. Нет NAT на сервере (если нужен выход в интернет)

## Решение

### Вариант 1: Ping между клиентом и сервером (базовый тест)

Это уже должно работать после установки туннеля!

**На Windows клиенте:**
```cmd
ping 10.0.0.1
```

**На Linux сервере:**
```bash
ping 10.0.0.2
```

Если пинги работают - **VPN туннель установлен правильно!** ✅

---

### Вариант 2: Доступ к локальной сети сервера через VPN

Допустим, сервер (WSL2) находится в сети `172.26.0.0/16` и вы хотите получить доступ к этой сети с Windows клиента.

**На Windows клиенте (PowerShell от Администратора):**
```powershell
# Добавить маршрут к сети сервера через VPN
route add 172.26.0.0 mask 255.255.0.0 10.0.0.1 metric 1

# Проверить маршруты
route print
```

Теперь весь трафик к `172.26.0.0/16` пойдет через VPN!

**Проверка:**
```cmd
# Пинг WSL2 сервера из Windows
ping 172.26.171.205
```

---

### Вариант 3: Весь трафик через VPN (как настоящий VPN)

⚠️ **Внимание:** Это перенаправит ВСЬ интернет трафик через VPN!

#### Шаг 1: Настройка NAT на сервере (WSL2/Linux)

**На сервере:**
```bash
# Включить IP forwarding
sudo sysctl -w net.ipv4.ip_forward=1

# Сделать постоянным (опционально)
echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf

# Настроить NAT для VPN клиентов
sudo iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -o eth0 -j MASQUERADE

# Разрешить forwarding
sudo iptables -A FORWARD -i vpn0 -j ACCEPT
sudo iptables -A FORWARD -o vpn0 -j ACCEPT

# Проверить правила
sudo iptables -t nat -L -n -v
```

#### Шаг 2: Настройка маршрутизации на клиенте (Windows)

**На Windows клиенте (PowerShell от Администратора):**
```powershell
# Узнать имя VPN интерфейса
Get-NetAdapter | Where-Object {$_.InterfaceDescription -like "*Wintun*"}
# Запомните "ifIndex"

# Установить VPN как шлюз по умолчанию
# Замените <ifIndex> на реальный индекс интерфейса vpn0
New-NetRoute -DestinationPrefix "0.0.0.0/0" -InterfaceIndex <ifIndex> -NextHop 10.0.0.1 -RouteMetric 1

# Или используйте имя интерфейса
New-NetRoute -DestinationPrefix "0.0.0.0/0" -InterfaceAlias "vpn0" -NextHop 10.0.0.1 -RouteMetric 1
```

**Вернуть обратно (отключить VPN routing):**
```powershell
# Удалить маршрут по умолчанию через VPN
Remove-NetRoute -DestinationPrefix "0.0.0.0/0" -InterfaceAlias "vpn0" -Confirm:$false
```

---

### Вариант 4: Split-tunneling (только определенные сайты через VPN)

Например, направить только трафик к Google DNS через VPN:

**На Windows:**
```powershell
# Маршрут только для 8.8.8.8 через VPN
route add 8.8.8.8 mask 255.255.255.255 10.0.0.1 metric 1

# Проверка
ping 8.8.8.8
tracert 8.8.8.8
```

**Удалить маршрут:**
```cmd
route delete 8.8.8.8
```

---

## Проверка маршрутизации

### Windows

**Посмотреть все маршруты:**
```cmd
route print
```

**Посмотреть активные соединения:**
```cmd
netstat -rn
```

**Трассировка пути:**
```cmd
tracert 8.8.8.8
```

### Linux (сервер)

**Посмотреть маршруты:**
```bash
ip route show
```

**Посмотреть правила NAT:**
```bash
sudo iptables -t nat -L -n -v
```

**Трассировка:**
```bash
traceroute 8.8.8.8
```

---

## Автоматическая настройка (скрипты)

### Windows BAT скрипт

Создайте `setup-vpn-routing.bat`:

```bat
@echo off
REM Run as Administrator!

echo === Setting up VPN routing ===

REM Start VPN client
start /B vpn-client.exe -server 172.26.171.205:51820

REM Wait for connection
timeout /t 5

REM Add route to server network
route add 172.26.0.0 mask 255.255.0.0 10.0.0.1 metric 1

echo VPN routing configured!
echo Press any key to disconnect and cleanup...
pause

REM Cleanup
route delete 172.26.0.0
taskkill /IM vpn-client.exe /F
```

### Linux скрипт для сервера

Создайте `setup-vpn-nat.sh`:

```bash
#!/bin/bash
# Run as root

echo "=== Setting up VPN NAT ==="

# Enable IP forwarding
sysctl -w net.ipv4.ip_forward=1

# Setup NAT
iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -o eth0 -j MASQUERADE
iptables -A FORWARD -i vpn0 -j ACCEPT
iptables -A FORWARD -o vpn0 -j ACCEPT

echo "NAT configured!"
echo "Press Ctrl+C to cleanup and exit"

# Trap cleanup
trap cleanup INT

cleanup() {
    echo "Cleaning up..."
    iptables -t nat -D POSTROUTING -s 10.0.0.0/24 -o eth0 -j MASQUERADE
    iptables -D FORWARD -i vpn0 -j ACCEPT
    iptables -D FORWARD -o vpn0 -j ACCEPT
    exit 0
}

# Keep script running
while true; do
    sleep 1
done
```

**Использование:**
```bash
sudo chmod +x setup-vpn-nat.sh
sudo ./setup-vpn-nat.sh
```

---

## Примеры использования

### Пример 1: Доступ к WSL2 из Windows через VPN

**Проблема:** У WSL2 динамический IP который меняется

**Решение:** Всегда подключаться к `10.0.0.1`

```cmd
REM Вместо ping 172.26.171.205 (который меняется)
ping 10.0.0.1

REM SSH к WSL2
ssh user@10.0.0.1

REM Веб-сервер в WSL2
curl http://10.0.0.1:8080
```

### Пример 2: Доступ к Windows из WSL2 через VPN

```bash
# Вместо поиска IP Windows
ping 10.0.0.2

# RDP к Windows
xfreerdp /v:10.0.0.2 /u:username
```

### Пример 3: Безопасное подключение к удаленному серверу

**Сервер:** VPS в облаке (публичный IP: 1.2.3.4)
**Клиент:** Ваш компьютер дома

**На сервере (VPS):**
```bash
# Запустить VPN сервер
sudo ./vpn-server -listen 0.0.0.0:51820

# Настроить NAT
sudo iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -o eth0 -j MASQUERADE
```

**На клиенте (дома):**
```cmd
REM Подключиться к VPS
vpn-client.exe -server 1.2.3.4:51820

REM Добавить маршрут по умолчанию (весь трафик через VPN)
route add 0.0.0.0 mask 0.0.0.0 10.0.0.1 metric 1
```

Теперь весь ваш интернет трафик идет через VPS! 🚀

---

## DNS через VPN

Если хотите чтобы DNS запросы тоже шли через VPN:

**Windows:**
```powershell
# Установить DNS сервер для VPN интерфейса
Set-DnsClientServerAddress -InterfaceAlias "vpn0" -ServerAddresses ("8.8.8.8", "8.8.4.4")

# Или использовать DNS сервер на VPN сервере
Set-DnsClientServerAddress -InterfaceAlias "vpn0" -ServerAddresses ("10.0.0.1")
```

**На сервере (если хотите DNS сервер):**
```bash
# Установить dnsmasq
sudo apt install dnsmasq

# Настроить слушать на VPN интерфейсе
echo "interface=vpn0" | sudo tee -a /etc/dnsmasq.conf
echo "server=8.8.8.8" | sudo tee -a /etc/dnsmasq.conf

sudo systemctl restart dnsmasq
```

---

## Устранение проблем

### Трафик не идет через VPN

**Проверки:**
```cmd
# 1. VPN подключен?
ping 10.0.0.1

# 2. Маршрут существует?
route print | findstr "10.0.0"

# 3. Интерфейс UP?
ipconfig | findstr "vpn0"
```

### Нет доступа в интернет через VPN

**На сервере проверьте:**
```bash
# 1. IP forwarding включен?
cat /proc/sys/net/ipv4/ip_forward
# Должно быть: 1

# 2. NAT настроен?
sudo iptables -t nat -L -n -v | grep MASQUERADE

# 3. Firewall не блокирует?
sudo iptables -L FORWARD -n -v
```

### Медленная скорость

**Проверьте MTU:**
```cmd
# Windows - узнать MTU
netsh interface ipv4 show subinterfaces

# Установить меньший MTU если есть фрагментация
netsh interface ipv4 set subinterface "vpn0" mtu=1380 store=persistent
```

**На Linux:**
```bash
# Проверить
ip link show vpn0

# Изменить
sudo ip link set vpn0 mtu 1380
```

---

## Итого: Быстрый старт

**Самый простой способ проверить что VPN работает:**

1. **Запустите сервер:**
   ```bash
   sudo ./vpn-server
   ```

2. **Запустите клиент:**
   ```cmd
   vpn-client.exe -server 172.26.171.205:51820
   ```

3. **Проверьте ping:**
   ```cmd
   ping 10.0.0.1
   ```

**Если пинг работает - VPN туннель установлен!** ✅

Для маршрутизации трафика смотрите примеры выше! 🚀
