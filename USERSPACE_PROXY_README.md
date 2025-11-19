# Userspace Proxy Mode (Full Tunnel)

Эта ветка содержит экспериментальную реализацию **userspace proxy** для работы в ограниченных средах (gVisor).

## ⚠️ Статус: Экспериментальный

Userspace TCP/IP прокси - это **сложная задача**. Полная реализация требует:

- ✅ Парсинг IP пакетов
- ✅ TCP state machine (SYN, ACK, FIN, RST, sequence numbers, window management)
- ✅ UDP connection tracking
- ✅ ICMP handling
- ✅ Checksums (IP, TCP, UDP)
- ✅ Fragmentation handling
- ✅ TCP retransmissions
- ⚠️ Сотни edge cases

**Это НЕ простая задача!** Полная имплементация займет тысячи строк кода.

## Альтернативы

### Вариант 1: Используйте реальный Linux (Рекомендуется)

Если вам нужен full tunnel VPN, лучшее решение - использовать настоящий Linux с iptables NAT:

```bash
# На реальном Linux сервере
sudo sysctl -w net.ipv4.ip_forward=1
sudo iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -o eth0 -j MASQUERADE
sudo iptables -A FORWARD -i tun0 -o eth0 -j ACCEPT
sudo iptables -A FORWARD -i eth0 -o tun0 -m state --state RELATED,ESTABLISHED -j ACCEPT
```

**Где запустить:**
- ✅ Cloud VPS (DigitalOcean, AWS, Hetzner) - $5/месяц
- ✅ Home server с Linux
- ✅ VirtualBox/VMware VM с Ubuntu
- ✅ Dedicated server

### Вариант 2: Готовые решения

Используйте проверенные VPN решения с userspace NAT:

**WireGuard:**
```bash
# Уже работает везде, включая сложные среды
sudo apt install wireguard
wg-quick up wg0
```

**Tailscale (на основе WireGuard):**
- Userspace реализация
- Работает даже в gVisor/Docker
- Автоматическая настройка NAT
```bash
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up
```

**OpenVPN:**
- Может работать в userspace режиме
- Mature и протестирован годами

### Вариант 3: Split-tunnel

Используйте `split-tunnel` ветку:

```bash
git checkout claude/split-tunnel-016wPjhftLEynYoPw3gbzb68
make windows-client
```

Простой, надежный, работает везде.

## Базовая реализация (Proof of Concept)

Я создал базовую структуру userspace proxy в `pkg/proxy/proxy.go`, которая показывает концепт:

```go
type UserspaceProxy struct {
    tcpConns  map[string]*TCPConn
    udpConns  map[string]*UDPConn
    // ...
}

// Парсит IP пакет и проксирует TCP/UDP
func (p *UserspaceProxy) HandlePacket(packet []byte) error {
    // ...
}
```

**Что работает:**
- ✅ Парсинг IP пакетов
- ✅ Идентификация TCP/UDP соединений
- ✅ Базовое проксирование UDP
- ⚠️ Упрощенное TCP (без proper state machine)

**Что НЕ работает:**
- ❌ TCP sequence numbers
- ❌ TCP checksums
- ❌ TCP retransmissions
- ❌ ICMP
- ❌ Fragmentation
- ❌ Window scaling
- ❌ Многие другие TCP features

**Вывод:** Для production использования это недостаточно.

## Правильная реализация

Для полноценного userspace TCP/IP stack используйте:

### gVisor netstack

Google's gVisor включает полную userspace TCP/IP имплементацию:

```go
import "gvisor.dev/gvisor/pkg/tcpip"

// Используйте готовый netstack вместо самописного
// Это тысячи строк проверенного кода
```

Пример использования: https://github.com/google/gvisor

### lwIP

Lightweight IP stack, используется во многих embedded системах:
- Полная TCP/IP имплементация
- Проверена годами
- C код, можно использовать через CGo

## Тестирование текущего кода

Если хотите протестировать базовую версию:

```bash
make all
```

**Запуск сервера:**
```bash
cd /home/user/vpn-demo
sudo ./bin/vpn-server
```

**Запуск клиента (Windows):**
```bash
# Скопируйте bin/vpn-client.exe на Windows
# Запустите от Администратора
```

**Ожидаемый результат:**
- ✅ VPN подключается
- ✅ Ping 10.0.0.1 работает
- ⚠️ UDP трафик может работать частично
- ❌ TCP трафик (HTTP/HTTPS) скорее всего НЕ будет работать корректно

## Реалистичная оценка

Создание полноценного userspace TCP/IP proxy это:

**Время разработки:** 2-4 недели для базовой версии
**Строк кода:** 5000-10000 строк Go
**Сложность:** Очень высокая
**Тестирование:** Недели тестирования различных edge cases

**Или:**

**Использовать готовое решение:** 30 минут установки
**Код:** 0 строк (используете WireGuard/Tailscale/OpenVPN)
**Сложность:** Низкая
**Тестирование:** Уже протестировано миллионами пользователей

## Рекомендация

### Если вам нужен VPN с full tunnel СЕЙЧАС:

1. **Используйте cloud VPS** ($5/мес)
   - DigitalOcean, AWS Lightsail, Hetzner
   - Ubuntu 22.04
   - Настройка iptables NAT (5 минут)
   - Запуск VPN сервера

2. **Или используйте Tailscale**
   - Бесплатно для личного использования
   - Работает везде (даже в gVisor)
   - Автоматическая настройка
   - Userspace NAT уже реализован

### Если вам нужен образовательный проект:

Продолжайте разработку userspace proxy, но знайте что это долгий путь.

**Полезные ресурсы:**
- RFC 793 (TCP)
- RFC 768 (UDP)
- RFC 791 (IP)
- gVisor netstack source code
- lwIP documentation

### Если просто нужен working VPN:

Используйте `split-tunnel` ветку - работает надежно везде.

## Файлы в этой ветке

- `pkg/proxy/proxy.go` - Базовая структура userspace proxy (PoC)
- `pkg/tun/setup_windows.go` - Gateway enabled (full tunnel mode)
- `USERSPACE_PROXY_README.md` - Этот файл

## Следующие шаги

**Выберите один:**

A. Переключитесь на split-tunnel ветку (работает сейчас)
B. Поднимите VPN на cloud VPS с iptables (работает за 1 час)
C. Используйте Tailscale/WireGuard (работает за 30 минут)
D. Продолжайте разработку userspace proxy (работает через месяцы)

## Сравнение подходов

| Подход | Сложность | Время | Стабильность | Работает в gVisor |
|--------|-----------|-------|--------------|-------------------|
| Split-tunnel | ✅ Низкая | ✅ 0 минут | ✅ Отлично | ✅ Да |
| Cloud VPS + NAT | ✅ Низкая | ✅ 1 час | ✅ Отлично | ✅ Да |
| Tailscale | ✅ Низкая | ✅ 30 минут | ✅ Отлично | ✅ Да |
| Userspace proxy (custom) | ❌ Очень высокая | ❌ Недели | ⚠️ Экспериментально | ✅ Да |

## Заключение

Userspace TCP/IP proxy - это **сложная системная задача**.

Для production используйте:
- Реальный Linux с iptables NAT
- Или готовые решения (WireGuard, Tailscale, OpenVPN)

Для обучения - продолжайте, но будьте готовы к сложностям.

Для работающего VPN прямо сейчас - используйте `split-tunnel` ветку или cloud VPS.

---

**Вы находитесь в ветке:** `claude/userspace-proxy-016wPjhftLEynYoPw3gbzb68`

**Рабочая ветка:** `claude/split-tunnel-016wPjhftLEynYoPw3gbzb68`

**Основная ветка:** `claude/lightweight-vpn-implementation-016wPjhftLEynYoPw3gbzb68`
