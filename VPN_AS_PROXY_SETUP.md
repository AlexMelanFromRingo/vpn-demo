# Настройка VPN как прокси (весь трафик через сервер)

Это руководство покажет, как настроить VPN так, чтобы **весь интернет-трафик** клиента шёл через сервер (как прокси).

## Как это работает

```
Windows Client (10.0.0.2)
    ↓ весь трафик
VPN Tunnel (зашифрованный)
    ↓
Linux Server (10.0.0.1)
    ↓ NAT
Internet (через IP сервера)
```

Весь ваш трафик будет:
- ✅ Зашифрован между клиентом и сервером (AES-256-GCM)
- ✅ Иметь IP адрес сервера (скрывает ваш реальный IP)
- ✅ Проходить через сервер как через прокси
- ✅ Обфусцирован от DPI (Deep Packet Inspection)

## Шаг 1: Настройте NAT на сервере (Linux/WSL2)

Без этого шага интернет не будет работать на клиенте!

### Автоматическая настройка (рекомендуется)

```bash
cd /home/user/vpn-demo
sudo chmod +x scripts/setup-vpn-nat.sh
sudo ./scripts/setup-vpn-nat.sh
```

Скрипт автоматически:
- Включит IP forwarding
- Настроит NAT (masquerading)
- Добавит правила iptables для пересылки трафика
- Покажет текущую конфигурацию

### Ручная настройка (если скрипт не работает)

```bash
# 1. Включить IP forwarding
sudo sysctl -w net.ipv4.ip_forward=1

# Сделать постоянным (после перезагрузки)
echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf

# 2. Определить ваш интерфейс для интернета
ip route | grep default
# Вывод: default via 172.26.160.1 dev eth0
# Интерфейс: eth0 (у вас может быть другой!)

# 3. Настроить NAT (замените eth0 на ваш интерфейс!)
sudo iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -o eth0 -j MASQUERADE

# 4. Разрешить пересылку трафика
sudo iptables -A FORWARD -i tun0 -o eth0 -j ACCEPT
sudo iptables -A FORWARD -i eth0 -o tun0 -m state --state RELATED,ESTABLISHED -j ACCEPT
```

### Проверка настроек NAT

```bash
# Проверить IP forwarding
sysctl net.ipv4.ip_forward
# Должно быть: net.ipv4.ip_forward = 1

# Проверить правила NAT
sudo iptables -t nat -L POSTROUTING -n -v
# Должна быть строка с MASQUERADE для 10.0.0.0/24

# Проверить правила FORWARD
sudo iptables -L FORWARD -n -v
# Должны быть правила для tun0
```

## Шаг 2: Пересоберите клиент

Я уже изменил код - теперь клиент будет устанавливать VPN как default gateway.

```bash
cd /home/user/vpn-demo
make windows-client
```

## Шаг 3: Запустите сервер

```bash
cd /home/user/vpn-demo
sudo ./bin/vpn-server
```

**Важно:** Сервер должен быть запущен **после** настройки NAT!

## Шаг 4: Обновите клиент на Windows

```bash
# В WSL2:
cp bin/vpn-client.exe /mnt/c/Users/<ваш_username>/Desktop/vpn-client-fullroute.exe
```

**На Windows:**
1. Остановите старый клиент: `taskkill /IM vpn-client.exe /F`
2. ПКМ на `vpn-client-fullroute.exe` → "Запуск от имени администратора"

### Что вы увидите в новых логах клиента:

```
Configuring Windows adapter 'vpn0' with IP 10.0.0.2, gateway 10.0.0.1
✓ IP address configured successfully
✓ Default gateway set to 10.0.0.1 - all traffic will go through VPN!
✓ Windows adapter configured - all traffic will be routed through VPN!
  Make sure the VPN server has NAT/forwarding enabled for internet access.
```

## Шаг 5: Проверьте что всё работает

### Проверка 1: Ping до сервера

```powershell
ping 10.0.0.1
```

Должен работать!

### Проверка 2: Ping до интернета

```powershell
ping 8.8.8.8
ping google.com
```

Должен работать! Если не работает - проблема в NAT на сервере.

### Проверка 3: Проверить внешний IP

```powershell
# PowerShell:
Invoke-WebRequest -Uri "https://api.ipify.org" -UseBasicParsing | Select-Object -ExpandProperty Content
```

Или в браузере: https://ifconfig.me

**Должен показать IP адрес вашего VPN сервера, а не ваш реальный IP!**

### Проверка 4: Проверить маршруты

```powershell
route print
```

Должны увидеть:
```
Network Destination        Netmask          Gateway       Interface  Metric
          0.0.0.0          0.0.0.0       10.0.0.1       10.0.0.2     <низкий metric>
```

Это означает, что VPN стал маршрутом по умолчанию.

## Отладка проблем

### Проблема 1: Ping до 10.0.0.1 работает, но интернет не работает

**Причина:** NAT не настроен или IP forwarding выключен на сервере

**Решение:**
```bash
# На сервере проверить:
sysctl net.ipv4.ip_forward
# Если = 0, то:
sudo sysctl -w net.ipv4.ip_forward=1

# Проверить правила NAT:
sudo iptables -t nat -L POSTROUTING -n -v
# Должна быть строка MASQUERADE для 10.0.0.0/24
```

### Проблема 2: Вообще ничего не работает

**Причина:** Старый клиент без gateway

**Решение:** Убедитесь что используете НОВЫЙ клиент (`vpn-client-fullroute.exe`)

Проверьте логи - должно быть:
```
✓ Default gateway set to 10.0.0.1 - all traffic will go through VPN!
```

### Проблема 3: DNS не работает (ping по IP работает, по имени - нет)

**Решение (PowerShell от Администратора):**
```powershell
# Установить Google DNS для VPN адаптера
netsh interface ipv4 set dnsservers "vpn0" static 8.8.8.8 primary
netsh interface ipv4 add dnsservers "vpn0" 8.8.4.4 index=2
```

### Проблема 4: Медленный интернет через VPN

**Причины:**
- Низкая пропускная способность сервера
- Большой ping между клиентом и сервером (WSL2 может иметь большие задержки)
- MTU слишком большой

**Решение:**
```powershell
# Попробуйте уменьшить MTU на клиенте
netsh interface ipv4 set subinterface "vpn0" mtu=1280 store=persistent
```

## Логирование трафика

### На сервере - посмотреть весь трафик через VPN:

```bash
# Все пакеты через TUN
sudo tcpdump -i tun0 -n

# Только HTTP(S)
sudo tcpdump -i tun0 -n 'tcp port 80 or tcp port 443'

# Только DNS
sudo tcpdump -i tun0 -n 'udp port 53'
```

### На клиенте - проверить что трафик идёт через VPN:

Логи клиента теперь показывают все отправленные пакеты.
Для ICMP вы увидите:
```
TUN → Server: ICMP type=8 from 10.0.0.2 to 8.8.8.8
✓ Sent ICMP packet to server
```

## Безопасность

Теперь ваш VPN работает как полноценный прокси:

✅ **Весь трафик зашифрован** между клиентом и сервером (AES-256-GCM)
✅ **Внешний IP** - IP адрес сервера (скрывает ваш реальный IP)
✅ **DPI обфускация** - случайные префиксы и padding затрудняют анализ
✅ **Ротация ключей** - сессионные ключи меняются каждые 5 минут
✅ **Эллиптическая криптография** - Curve25519 ECDH для обмена ключами

⚠️ **НО:**
- DNS запросы могут утекать (используйте DNS через VPN адаптер)
- WebRTC может раскрыть локальный IP (отключите в браузере)
- VPN сервер видит весь ваш трафик в расшифрованном виде
- Скорость интернета будет ограничена пропускной способностью сервера

## Отключение режима прокси

Если хотите вернуться к режиму "только VPN сеть":

1. Пересоберите клиент с изменениями в `setup_windows.go` (уберите gateway)
2. Или вручную на Windows:
```powershell
# Удалить default gateway
netsh interface ipv4 delete route 0.0.0.0/0 "vpn0"
```

## Мониторинг на сервере

Посмотреть статистику NAT:
```bash
# Активные соединения через NAT
sudo conntrack -L | grep 10.0.0.2

# Статистика iptables
sudo iptables -t nat -L POSTROUTING -n -v
sudo iptables -L FORWARD -n -v
```

## Производительность

Для лучшей производительности:

1. **На сервере:**
```bash
# Увеличить размеры буферов
sudo sysctl -w net.core.rmem_max=26214400
sudo sysctl -w net.core.wmem_max=26214400
```

2. **На клиенте:**
```powershell
# Оптимизировать TCP
netsh interface tcp set global autotuninglevel=normal
```

---

**Всё готово!** Теперь весь ваш интернет-трафик идёт через VPN сервер как через прокси.
