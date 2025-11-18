# Build and Test Guide

## Проверено и работает! ✅

Все компоненты собраны и протестированы:
- ✅ Компиляция без ошибок
- ✅ Кросс-компиляция для Windows
- ✅ Бинарники запускаются
- ✅ Криптография работает (ключи генерируются)
- ✅ TUN интерфейс создается

## Сборка проекта

### Требования

**Linux/WSL2:**
```bash
# Ubuntu/Debian
sudo apt update
sudo apt install -y golang-1.21 iproute2 build-essential

# Или если Go уже установлен
go version  # Должно быть >= 1.21
```

**Windows (для сборки Windows клиента из Linux):**
- Не требуется! Кросс-компиляция работает из Linux

### Быстрая сборка

```bash
cd /home/user/vpn-demo

# 1. Установить зависимости Go
make deps

# 2. Собрать всё
make all-platforms

# Или только для текущей платформы
make build
```

### Результат сборки

```
bin/
├── vpn-server       - Сервер для Linux (4.1 MB)
├── vpn-server.exe   - Сервер для Windows (4.4 MB)
├── vpn-client       - Клиент для Linux (4.1 MB)
└── vpn-client.exe   - Клиент для Windows (4.4 MB)
```

### Makefile команды

```bash
make deps            # Скачать Go зависимости
make server          # Собрать только сервер (Linux)
make client          # Собрать только клиент (Linux)
make windows-client  # Собрать клиент для Windows
make all-platforms   # Собрать всё (Linux + Windows)
make clean           # Удалить bin/ и очистить кеш
make test            # Запустить тесты
```

## Первый запуск (тест)

### Узнать IP WSL2

```bash
# В WSL2
ip addr show eth0 | grep "inet "
# Пример вывода: inet 172.26.171.205/20

# Или
hostname -I
```

Этот IP будет использоваться клиентом для подключения к серверу.

### Запуск сервера

```bash
# Терминал 1 (WSL2/Linux)
sudo ./bin/vpn-server
```

**Ожидаемый вывод:**
```
=== Lightweight VPN Server ===
Listen: 0.0.0.0:51820
TUN IP: 10.0.0.1/24
Server Public Key: ABC123...xyz==
TUN interface created: vpn0
Setting up TUN interface: vpn0
TUN interface ready: vpn0
UDP server listening on 0.0.0.0:51820
Server started successfully!
```

**Если ошибка `Permission denied`:**
- Нужны root права: используйте `sudo`
- Создание TUN устройства требует CAP_NET_ADMIN

**Если ошибка `ip: command not found`:**
```bash
sudo apt install iproute2
```

### Запуск клиента (в том же WSL2 для теста)

```bash
# Терминал 2 (WSL2/Linux)
# Замените IP на ваш IP из команды выше
sudo ./bin/vpn-client -server 172.26.171.205:51820
```

**Ожидаемый вывод:**
```
=== Lightweight VPN Client ===
Server: 172.26.171.205:51820
TUN IP: 10.0.0.2/24
Client Public Key: XYZ789...abc==
TUN interface created: vpn0
Setting up TUN interface: vpn0
TUN interface ready: vpn0
Initiating handshake with server...
Server Public Key: ABC123...xyz==
Shared secret established (epoch: 123456)
Handshake successful!
Client started successfully!
```

**На сервере появится:**
```
Handshake from 127.0.0.1:54321
Client registered: 127.0.0.1:54321 (epoch: 123456)
```

### Проверка соединения

**Терминал 3 (на сервере):**
```bash
# Проверить интерфейс создан
ip addr show vpn0
# Вы должны увидеть: 10.0.0.1/24

# Пинг клиента
ping 10.0.0.2
```

**Терминал 4 (на клиенте):**
```bash
# Проверить интерфейс
ip addr show vpn0
# Вы должны увидеть: 10.0.0.2/24

# Пинг сервера
ping 10.0.0.1
```

**Успех! 🎉** Если пинги работают - VPN туннель установлен и шифрование работает!

## Запуск клиента на Windows

### Подготовка Windows

1. **Установите TAP-Windows драйвер:**

   **Вариант 1 (рекомендуется):** Установите OpenVPN
   - Скачайте: https://openvpn.net/community-downloads/
   - Установите (драйвер TAP-Windows включен)

   **Вариант 2:** Только TAP драйвер
   - Скачайте: https://build.openvpn.net/downloads/releases/
   - Установите tap-windows-*.exe

   **Вариант 3:** Wintun (современный)
   - Скачайте: https://www.wintun.net/
   - Распакуйте wintun.dll рядом с vpn-client.exe

2. **Скопируйте клиент на Windows:**
   ```bash
   # В WSL2 (ваш Windows диск доступен через /mnt/c)
   cp bin/vpn-client.exe /mnt/c/Users/<ваш_username>/Desktop/
   ```

3. **Запустите PowerShell или CMD от Администратора**

4. **Запустите клиент:**
   ```cmd
   cd C:\Users\<username>\Desktop
   vpn-client.exe -server 172.26.171.205:51820
   ```

5. **Проверьте подключение:**
   ```cmd
   # Проверить интерфейс
   ipconfig

   # Пинг сервера в VPN
   ping 10.0.0.1
   ```

### Использование bat скрипта (Windows)

```cmd
cd scripts
start-client.bat
```

Отредактируйте `scripts/start-client.bat` если нужно изменить IP сервера.

## Firewall настройка

### Linux/WSL2 (UFW)

```bash
sudo ufw allow 51820/udp
```

### Windows Firewall

**PowerShell (от Администратора):**
```powershell
New-NetFirewallRule -DisplayName "VPN Server" -Direction Inbound -Protocol UDP -LocalPort 51820 -Action Allow
```

**Или через GUI:**
1. Windows Defender Firewall → Advanced Settings
2. Inbound Rules → New Rule
3. Port → UDP → 51820 → Allow
4. Имя: "VPN Server"

### WSL2 особенности

WSL2 использует NAT, поэтому для доступа извне Windows нужно:

```powershell
# В Windows PowerShell (от Администратора)
# Узнать IP WSL2
wsl hostname -I

# Добавить port forwarding
netsh interface portproxy add v4tov4 listenport=51820 listenaddress=0.0.0.0 connectport=51820 connectaddress=<WSL2_IP>

# Добавить firewall rule
New-NetFirewallRule -DisplayName "VPN WSL2" -Direction Inbound -Protocol UDP -LocalPort 51820 -Action Allow
```

## Производственный запуск

### Systemd сервис (Linux)

Создайте `/etc/systemd/system/vpn-server.service`:

```ini
[Unit]
Description=Lightweight VPN Server
After=network.target

[Service]
Type=simple
User=root
ExecStart=/home/user/vpn-demo/bin/vpn-server -listen 0.0.0.0:51820 -tun-ip 10.0.0.1/24 -peer-ip 10.0.0.2
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Включите:
```bash
sudo systemctl daemon-reload
sudo systemctl enable vpn-server
sudo systemctl start vpn-server
sudo systemctl status vpn-server
```

### Логи

```bash
# Systemd логи
sudo journalctl -u vpn-server -f

# Или просто запустить в foreground
sudo ./bin/vpn-server
```

## Отладка

### Проблема: Permission denied

**Решение:** Нужны root права
```bash
sudo ./bin/vpn-server
sudo ./bin/vpn-client
```

### Проблема: TUN interface creation failed

**Linux:**
```bash
# Проверить модуль TUN
lsmod | grep tun
# Если нет:
sudo modprobe tun

# Проверить права
ls -l /dev/net/tun
# Должно быть: crw-rw-rw- 1 root root
```

**Windows:**
- Установите TAP-Windows драйвер (см. выше)
- Запустите от Администратора

### Проблема: Connection timeout

**Проверки:**
```bash
# 1. Сервер слушает?
sudo netstat -ulnp | grep 51820

# 2. Firewall разрешает?
sudo ufw status

# 3. Правильный IP?
ip addr show eth0
```

### Проблема: Handshake failed

**Причины:**
- Сервер не запущен
- Firewall блокирует UDP 51820
- Неправильный IP адрес сервера

**Решение:**
```bash
# Посмотреть что сервер получает
# В коде сервера все логируется
sudo ./bin/vpn-server  # Смотрите логи

# Проверить UDP пакеты
sudo tcpdump -i any -n port 51820
```

### Проблема: ip: command not found

```bash
sudo apt install iproute2

# Или запускайте с PATH:
sudo PATH=/usr/sbin:$PATH ./bin/vpn-server
```

### Посмотреть сетевой трафик

```bash
# Зашифрованный трафик между клиентом и сервером
sudo tcpdump -i eth0 -n port 51820 -X

# Расшифрованный трафик в TUN
sudo tcpdump -i vpn0 -n
```

## Тестирование производительности

### Пропускная способность

```bash
# На сервере (через VPN интерфейс)
iperf3 -s -B 10.0.0.1

# На клиенте
iperf3 -c 10.0.0.1
```

### Latency

```bash
# От клиента к серверу
ping -c 100 10.0.0.1 | tail -1
```

### Проверка шифрования

```bash
# Перехват трафика
sudo tcpdump -i eth0 -n port 51820 -X -c 10

# Вы должны увидеть только случайные байты (зашифрованные данные)
# Никаких plaintext данных!
```

## Следующие шаги

1. **Маршрутизация трафика через VPN:**
   ```bash
   # На клиенте добавить маршрут
   sudo ip route add 192.168.0.0/16 dev vpn0
   ```

2. **NAT на сервере (доступ в интернет):**
   ```bash
   sudo iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -o eth0 -j MASQUERADE
   sudo sysctl -w net.ipv4.ip_forward=1
   ```

3. **Мониторинг:**
   - Добавить Prometheus metrics
   - Логирование в файл
   - Графана дашборд

4. **Безопасность:**
   - Pre-shared key для аутентификации
   - Rate limiting
   - IP whitelist

См. полную документацию в [ARCHITECTURE.md](ARCHITECTURE.md)
