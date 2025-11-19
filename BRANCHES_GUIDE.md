# Руководство по веткам VPN проекта

## 📊 Обзор веток

Проект имеет **две параллельные ветки** как вы запросили:

### 1️⃣ Split-Tunnel (работающая версия)
**Ветка:** `claude/split-tunnel-016wPjhftLEynYoPw3gbzb68`

### 4️⃣ Userspace Proxy (экспериментальная)
**Ветка:** `claude/userspace-proxy-016wPjhftLEynYoPw3gbzb68`

---

## 1️⃣ Split-Tunnel - Работает прямо сейчас

### Что это?

VPN только для внутренней сети (10.0.0.0/24). Интернет идет напрямую.

```
Windows Client (10.0.0.2)
    ↓ VPN сеть (10.0.0.0/24) → Через VPN tunnel
    ↓ Интернет → Напрямую через Windows
```

### Использование

```bash
# Переключиться на ветку
git checkout claude/split-tunnel-016wPjhftLEynYoPw3gbzb68

# Собрать клиент
make windows-client

# Скопировать на Windows
cp bin/vpn-client.exe /mnt/c/Users/Alex_Melan/Desktop/vpn-split.exe
```

**Запуск сервера (WSL2):**
```bash
sudo ./bin/vpn-server
```

**Запуск клиента (Windows):**
- ПКМ на `vpn-split.exe` → "Запуск от имени администратора"

### Проверка

```powershell
# VPN туннель
ping 10.0.0.1
# ✅ Работает

# Интернет
ping google.com
# ✅ Работает (напрямую)

# Ваш IP
Invoke-WebRequest https://api.ipify.org
# Показывает ваш реальный IP
```

### Логи клиента покажут:

```
✓ Split-tunnel mode: only 10.0.0.0/24 routed through VPN
  VPN network: 10.0.0.0/24 → VPN server
  Internet: Direct connection (not through VPN)
```

### Плюсы

- ✅ Работает везде (включая gVisor)
- ✅ Не требует NAT на сервере
- ✅ Простая настройка
- ✅ Надежно
- ✅ Интернет всегда работает

### Минусы

- ❌ Не скрывает ваш реальный IP
- ❌ Интернет не проксируется

### Для чего использовать

✅ Peer-to-peer связь в VPN сети
✅ Удаленный доступ к серверам в VPN
✅ Gaming LAN через интернет
✅ Безопасная связь между устройствами

---

## 4️⃣ Userspace Proxy - Экспериментальная

### Что это?

Попытка проксировать весь трафик (включая интернет) через VPN без kernel NAT.

```
Windows Client (10.0.0.2)
    ↓ ВЕСЬ трафик
VPN Tunnel → Userspace Proxy
    ↓
Internet (IP сервера)
```

### Использование

```bash
# Переключиться на ветку
git checkout claude/userspace-proxy-016wPjhftLEynYoPw3gbzb68

# Собрать
make all && make windows-client

# Скопировать на Windows
cp bin/vpn-client.exe /mnt/c/Users/Alex_Melan/Desktop/vpn-proxy.exe
```

**Запуск сервера (WSL2):**
```bash
sudo ./bin/vpn-server
```

Вы увидите:
```
Server started successfully!
Userspace proxy enabled - TCP/UDP will be proxied to internet
```

**Запуск клиента (Windows):**
- ПКМ на `vpn-proxy.exe` → "Запуск от имени администратора"

### Логи покажут:

**На сервере:**
```
Client → Proxy: TCP from 10.0.0.2 to 93.184.216.34
New TCP connection: 10.0.0.2 -> 93.184.216.34:80
Proxy → Client: TCP from 93.184.216.34 to 10.0.0.2
```

**На клиенте:**
```
✓ Default gateway set to 10.0.0.1 - all traffic will go through VPN!
✓ Windows adapter configured - all traffic will be routed through VPN!
```

### Что работает

✅ Базовая структура userspace proxy
✅ IP packet parsing
✅ TCP/UDP connection tracking
✅ Checksums (IP, TCP, UDP)
✅ UDP proxying (частично)
✅ Интеграция с сервером

### Что НЕ работает

❌ Полная TCP state machine
❌ TCP sequence numbers и ACK
❌ TCP retransmissions
❌ TCP window management
❌ Fragmentation
❌ ICMP proxying

### Статус: Экспериментально

**Ожидаемые результаты:**

| Тест | Результат |
|------|-----------|
| ping 10.0.0.1 | ✅ Работает |
| ping google.com | ⚠️ Может не работать |
| DNS запросы (UDP) | ⚠️ Может работать частично |
| HTTP/HTTPS (TCP) | ❌ Скорее всего не работает |

### Ограничения

Это **proof of concept**, не production код. Для полноценной работы нужно:

1. **Полная TCP state machine** (недели разработки)
2. **Sequence numbers и ACK handling**
3. **Retransmissions**
4. **Window scaling**
5. **Сотни edge cases**

**Или** используйте готовые решения:
- WireGuard
- Tailscale
- OpenVPN
- Cloud VPS с iptables NAT

### Документация

Читайте: `USERSPACE_PROXY_README.md`

---

## 🔄 Сравнение веток

| Функция | Split-Tunnel | Userspace Proxy |
|---------|--------------|-----------------|
| **Статус** | ✅ Работает | ⚠️ Экспериментально |
| **VPN сеть** | ✅ Да | ✅ Да |
| **Интернет через VPN** | ❌ Нет | ⚠️ Частично |
| **Скрывает IP** | ❌ Нет | ⚠️ Теоретически да |
| **Требует NAT** | ❌ Нет | ❌ Нет |
| **Работает в gVisor** | ✅ Да | ✅ Да |
| **Сложность** | ✅ Простая | ❌ Очень высокая |
| **Надежность** | ✅ Высокая | ⚠️ Низкая |
| **Production ready** | ✅ Да | ❌ Нет |
| **UDP proxy** | N/A | ⚠️ Частично |
| **TCP proxy** | N/A | ❌ Не работает полностью |

---

## 🎯 Рекомендации

### Для работы прямо сейчас:

```bash
git checkout claude/split-tunnel-016wPjhftLEynYoPw3gbzb68
```

**Результат:**
- ✅ VPN туннель работает
- ✅ Интернет работает
- ✅ Peer-to-peer связь
- ❌ IP не скрыт

---

### Для экспериментов с proxy:

```bash
git checkout claude/userspace-proxy-016wPjhftLEynYoPw3gbzb68
```

**Результат:**
- ✅ Userspace proxy работает частично
- ⚠️ UDP может работать
- ❌ TCP скорее всего не работает
- ⚠️ Нужна доработка

---

### Для полноценного full-tunnel VPN:

**Вариант A: Cloud VPS ($5/мес)**

```bash
# На Digital Ocean/AWS/Hetzner с настоящим Linux
sudo sysctl -w net.ipv4.ip_forward=1
sudo iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -o eth0 -j MASQUERADE
sudo iptables -A FORWARD -i tun0 -o eth0 -j ACCEPT
sudo iptables -A FORWARD -i eth0 -o tun0 -m state --state RELATED,ESTABLISHED -j ACCEPT

# Запустите VPN сервер из любой ветки
./vpn-server
```

**Результат:**
- ✅ Весь трафик через VPN
- ✅ IP скрыт
- ✅ Работает как прокси
- ✅ Production ready

**Вариант B: Tailscale (бесплатно)**

```bash
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up
```

**Результат:**
- ✅ Userspace NAT уже реализован
- ✅ Работает везде
- ✅ Автоматическая настройка
- ✅ Production ready

---

## 📋 Быстрый старт

### Сценарий 1: Мне нужен работающий VPN сейчас

```bash
git checkout claude/split-tunnel-016wPjhftLEynYoPw3gbzb68
make windows-client
cp bin/vpn-client.exe /mnt/c/Users/Alex_Melan/Desktop/
# Запустите сервер и клиент
```

### Сценарий 2: Хочу протестировать userspace proxy

```bash
git checkout claude/userspace-proxy-016wPjhftLEynYoPw3gbzb68
make all && make windows-client
cp bin/vpn-client.exe /mnt/c/Users/Alex_Melan/Desktop/
# Запустите и смотрите логи
```

### Сценарий 3: Нужен полноценный прокси

- Поднимите VPN на cloud VPS
- Или используйте Tailscale
- Или продолжайте разработку userspace proxy (недели работы)

---

## 📚 Документация по веткам

### Split-Tunnel:
- `SPLIT_TUNNEL_README.md` - Полное руководство
- Простой, надежный, работает везде

### Userspace Proxy:
- `USERSPACE_PROXY_README.md` - Детальная документация
- `pkg/proxy/proxy.go` - Код proxy
- Экспериментально, требует доработки

### Общая:
- `GVISOR_LIMITATIONS.md` - Почему NAT не работает в gVisor
- `VPN_AS_PROXY_SETUP.md` - Как сделать full proxy на cloud VPS

---

## ❓ Какую ветку использовать?

**Если вам нужно:**

| Цель | Ветка | Время |
|------|-------|-------|
| Работающий VPN прямо сейчас | `split-tunnel` | ✅ 5 минут |
| Peer-to-peer связь | `split-tunnel` | ✅ 5 минут |
| Экспериментировать с proxy | `userspace-proxy` | ⚠️ Часы тестирования |
| Production VPN с полным прокси | Cloud VPS + iptables | ✅ 1 час |
| Простое решение | Tailscale | ✅ 30 минут |
| Образовательный проект | `userspace-proxy` | ⚠️ Недели разработки |

---

## 🔗 Полезные ссылки

### Ветки GitHub:
- https://github.com/AlexMelanFromRingo/vpn-demo/tree/claude/split-tunnel-016wPjhftLEynYoPw3gbzb68
- https://github.com/AlexMelanFromRingo/vpn-demo/tree/claude/userspace-proxy-016wPjhftLEynYoPw3gbzb68

### Альтернативы:
- Tailscale: https://tailscale.com
- WireGuard: https://www.wireguard.com
- DigitalOcean: https://www.digitalocean.com (VPS от $5/мес)

---

**Вы сейчас на ветке:** `claude/userspace-proxy-016wPjhftLEynYoPw3gbzb68`

**Рабочая ветка:** `claude/split-tunnel-016wPjhftLEynYoPw3gbzb68`

Переключайтесь между ветками командой:
```bash
git checkout <branch-name>
```
