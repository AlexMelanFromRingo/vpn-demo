# Quick Start Guide

## Быстрый старт за 5 минут

### 1. Сборка

```bash
# В WSL2/Linux
cd /home/user/vpn-demo
make deps
make all-platforms
```

Получите:
- `bin/vpn-server` - сервер для Linux/WSL2
- `bin/vpn-client` - клиент для Linux/WSL2
- `bin/vpn-client.exe` - клиент для Windows

### 2. Запуск сервера (WSL2/Linux)

```bash
# ВАЖНО: Нужны root права для создания TUN интерфейса
sudo ./bin/vpn-server

# Или с параметрами:
sudo ./bin/vpn-server -listen 0.0.0.0:51820 -tun-ip 10.0.0.1/24 -peer-ip 10.0.0.2
```

Вы увидите:
```
=== Lightweight VPN Server ===
Listen: 0.0.0.0:51820
TUN IP: 10.0.0.1/24
Server Public Key: <base64-key>
TUN interface created: vpn0
TUN interface ready: vpn0
UDP server listening on 0.0.0.0:51820
Server started successfully!
```

**Узнать IP WSL2:**
```bash
ifconfig eth0 | grep "inet "
# Или
ip addr show eth0 | grep "inet "
```

### 3. Запуск клиента

#### На Linux/WSL2:

```bash
sudo ./bin/vpn-client -server 172.26.171.205:51820
```

#### На Windows:

1. **Установите TAP-Windows драйвер** (если еще не установлен):
   - Скачайте: https://build.openvpn.net/downloads/releases/
   - Или установите OpenVPN (включает TAP драйвер)
   - Или используйте Wintun: https://www.wintun.net/

2. **Скопируйте** `bin/vpn-client.exe` на Windows машину

3. **Запустите от имени Администратора:**
   ```cmd
   vpn-client.exe -server 172.26.171.205:51820
   ```

   Или используйте `scripts/start-client.bat`

### 4. Проверка подключения

#### На сервере:
```bash
# Проверить интерфейс
ip addr show vpn0

# Пинг клиента
ping 10.0.0.2
```

#### На клиенте:

**Linux:**
```bash
ip addr show vpn0
ping 10.0.0.1
```

**Windows:**
```cmd
ipconfig
ping 10.0.0.1
```

### 5. Что вы увидите

**Успешное подключение клиента:**
```
=== Lightweight VPN Client ===
Server: 172.26.171.205:51820
TUN IP: 10.0.0.2/24
Client Public Key: <base64-key>
TUN interface created: vpn0
Setting up TUN interface: vpn0
TUN interface ready: vpn0
Initiating handshake with server...
Server Public Key: <base64-key>
Shared secret established (epoch: 123456)
Handshake successful!
Client started successfully!
```

**На сервере при подключении клиента:**
```
Handshake from 192.168.1.100:54321
Client registered: 192.168.1.100:54321 (epoch: 123456)
```

## Проблемы и решения

### Permission denied
```bash
# Запускайте с sudo
sudo ./bin/vpn-server
sudo ./bin/vpn-client
```

### Windows: TUN device creation failed
- Установите TAP-Windows драйвер
- Запустите от имени Администратора

### Connection timeout
- Проверьте firewall (порт 51820 UDP должен быть открыт)
- Проверьте правильность IP адреса сервера

**Linux (UFW):**
```bash
sudo ufw allow 51820/udp
```

**Windows Firewall:**
- Добавьте правило для входящих UDP соединений на порт 51820

### Firewall в WSL2
```bash
# В Windows PowerShell (от администратора):
New-NetFirewallRule -DisplayName "VPN Server" -Direction Inbound -Protocol UDP -LocalPort 51820 -Action Allow
```

## Архитектура шифрования

```
Client                              Server
  |                                    |
  |  1. Generate Curve25519 keypair   |
  |     Generate Curve25519 keypair   |
  |                                    |
  |  2. Send PublicKey_C ─────────────>|
  |<──────────────── Send PublicKey_S  |
  |                                    |
  |  3. Compute SharedSecret           |
  |     SharedSecret = ECDH(Priv_C, Pub_S)
  |                                    |
  |  4. Derive SessionKey              |
  |     SessionKey = SHA256(SharedSecret || Epoch)
  |     Epoch = UnixTime / 300  (5 min)|
  |                                    |
  |  5. Encrypt with AES-256-GCM       |
  |  [epoch|nonce|ciphertext|tag] ───>|
  |<─── [epoch|nonce|ciphertext|tag]  |
  |                                    |
  |  6. Auto key rotation every 5 min  |
```

## Что дальше?

- Настройте маршрутизацию для доступа в интернет через VPN
- Добавьте systemd сервис для автозапуска
- Экспериментируйте с параметрами MTU для оптимизации

См. полную документацию в [README.md](README.md)
