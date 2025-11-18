# VPN Cheat Sheet - Шпаргалка

## 🚀 Быстрый старт

### Сборка
```bash
make deps && make all-platforms
```

### Запуск сервера (WSL2)
```bash
# Узнать свой IP
ip addr show eth0 | grep "inet "

# Запустить сервер
sudo ./bin/vpn-server
```

### Запуск клиента (Linux)
```bash
sudo ./bin/vpn-client -server 172.26.171.205:51820
```

### Запуск клиента (Windows)
```cmd
REM От Администратора!
vpn-client.exe -server 172.26.171.205:51820
```

### Проверка
```bash
# Пинг через VPN
ping 10.0.0.1   # с клиента на сервер
ping 10.0.0.2   # с сервера на клиент
```

---

## 📦 Команды Make

| Команда | Описание |
|---------|----------|
| `make deps` | Установить зависимости |
| `make build` | Собрать сервер + клиент (Linux) |
| `make server` | Только сервер (Linux) |
| `make client` | Только клиент (Linux) |
| `make windows-client` | Клиент для Windows |
| `make all-platforms` | Всё (Linux + Windows) |
| `make clean` | Очистить bin/ |

---

## 🔧 Параметры командной строки

### Сервер
```bash
sudo ./bin/vpn-server \
  -listen 0.0.0.0:51820 \      # UDP адрес:порт
  -tun-ip 10.0.0.1/24 \         # IP сервера в VPN
  -peer-ip 10.0.0.2 \           # IP клиента
  -mtu 1420                     # MTU (по умолчанию 1420)
```

### Клиент
```bash
sudo ./bin/vpn-client \
  -server 172.26.171.205:51820 \ # Адрес сервера
  -tun-ip 10.0.0.2/24 \          # IP клиента в VPN
  -peer-ip 10.0.0.1 \            # IP сервера в VPN
  -mtu 1420                      # MTU
```

---

## 🐛 Типичные проблемы

### Permission denied
```bash
# Используйте sudo
sudo ./bin/vpn-server
sudo ./bin/vpn-client
```

### ip: command not found
```bash
sudo apt install iproute2
```

### TUN creation failed (Windows)
```cmd
REM Установите TAP-Windows драйвер
REM https://openvpn.net/community-downloads/
```

### Connection timeout
```bash
# 1. Проверить firewall
sudo ufw allow 51820/udp

# 2. Проверить сервер слушает
sudo netstat -ulnp | grep 51820

# 3. Проверить IP правильный
ip addr show eth0
```

### Handshake failed
```bash
# Убедитесь что сервер запущен
# Проверьте логи обеих сторон
```

---

## 🔍 Отладка

### Посмотреть сетевые интерфейсы
```bash
ip addr show vpn0          # VPN интерфейс
ip addr show eth0          # Физический интерфейс
```

### Посмотреть маршруты
```bash
ip route
ip route show dev vpn0
```

### Перехват трафика
```bash
# Зашифрованный (на физическом интерфейсе)
sudo tcpdump -i eth0 -n port 51820

# Расшифрованный (на VPN интерфейсе)
sudo tcpdump -i vpn0 -n
```

### Проверить UDP порт
```bash
# Слушает ли сервер?
sudo netstat -ulnp | grep 51820
sudo ss -ulnp | grep 51820

# Открыт ли порт?
nc -zuv 172.26.171.205 51820
```

---

## 🔥 Firewall

### Linux (UFW)
```bash
sudo ufw allow 51820/udp
sudo ufw status
```

### Windows (PowerShell от Администратора)
```powershell
New-NetFirewallRule -DisplayName "VPN Server" `
  -Direction Inbound -Protocol UDP -LocalPort 51820 -Action Allow
```

### WSL2 Port Forward (Windows PowerShell)
```powershell
# Узнать IP WSL2
wsl hostname -I

# Port forwarding
netsh interface portproxy add v4tov4 `
  listenport=51820 listenaddress=0.0.0.0 `
  connectport=51820 connectaddress=<WSL2_IP>
```

---

## 📊 Тестирование

### Ping
```bash
# С клиента
ping -c 10 10.0.0.1

# С сервера
ping -c 10 10.0.0.2
```

### Traceroute
```bash
traceroute -n 10.0.0.1
```

### Bandwidth (iperf3)
```bash
# Сервер
iperf3 -s -B 10.0.0.1

# Клиент
iperf3 -c 10.0.0.1 -t 30
```

### Latency
```bash
ping -c 100 10.0.0.1 | tail -1
```

---

## 🎯 Полезные команды

### Проверить процесс работает
```bash
ps aux | grep vpn-server
ps aux | grep vpn-client
```

### Убить процесс
```bash
sudo pkill vpn-server
sudo pkill vpn-client
```

### Посмотреть логи systemd
```bash
sudo journalctl -u vpn-server -f
```

### Проверить модуль TUN
```bash
lsmod | grep tun
ls -l /dev/net/tun
```

### Включить IP forwarding (для NAT)
```bash
sudo sysctl -w net.ipv4.ip_forward=1
echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf
```

### NAT для доступа в интернет
```bash
sudo iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -o eth0 -j MASQUERADE
sudo iptables -A FORWARD -i vpn0 -j ACCEPT
```

---

## 📚 Документация

| Файл | Описание |
|------|----------|
| `README.md` | Полная документация |
| `QUICKSTART.md` | Быстрый старт (5 минут) |
| `BUILD.md` | Сборка и тестирование |
| `ARCHITECTURE.md` | Архитектура и криптография |
| `CHEATSHEET.md` | Эта шпаргалка |

---

## 🔐 Криптография

```
Key Exchange:  Curve25519 ECDH
Encryption:    AES-256-GCM
Key Rotation:  Every 5 minutes
Transport:     UDP
Obfuscation:   Random prefix + padding
```

---

## 🌐 Сетевая схема

```
Client (10.0.0.2)  ←→  Server (10.0.0.1)
       ↕                      ↕
   TUN vpn0              TUN vpn0
       ↕                      ↕
   Encrypted UDP (port 51820)
```

---

## 💡 Tips

- VPN интерфейс называется `vpn0` на обеих сторонах
- Порт по умолчанию: `51820` (как у WireGuard)
- MTU по умолчанию: `1420` байт
- Ключи генерируются при каждом запуске (ephemeral)
- Ротация ключей: автоматически каждые 5 минут
- Keep-alive: каждые 30 секунд
- Неактивные клиенты удаляются через 2 минуты

---

## ⚠️ Важно для продакшена

Текущая версия - базовая реализация для обучения!

**TODO для production:**
- [ ] Аутентификация клиентов (PSK или certs)
- [ ] Replay attack protection
- [ ] DoS protection (rate limiting)
- [ ] Более сложная обфускация
- [ ] Логирование в файл
- [ ] Metrics (Prometheus)
- [ ] Health checks
- [ ] Graceful shutdown
- [ ] Config файл (вместо флагов)
