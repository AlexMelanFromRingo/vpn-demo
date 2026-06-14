# Architecture Documentation

## Обзор архитектуры

Lightweight VPN - это современный VPN с упором на безопасность, производительность и простоту.

```
┌──────────────────────────────────────────────────────────────┐
│                        VPN Architecture                       │
└──────────────────────────────────────────────────────────────┘

┌─────────────┐                               ┌─────────────┐
│   Client    │                               │   Server    │
│             │                               │             │
│  App Layer  │                               │  App Layer  │
│      ↕      │                               │      ↕      │
│  TUN (L3)   │                               │  TUN (L3)   │
│      ↕      │                               │      ↕      │
│  VPN Tunnel │←────── Encrypted UDP ────────→│  VPN Tunnel │
│      ↕      │    Noise + ChaCha20-Poly1305  │      ↕      │
│ UDP Socket  │                               │ UDP Socket  │
│      ↕      │                               │      ↕      │
│  Ethernet   │                               │  Ethernet   │
└─────────────┘                               └─────────────┘
   10.0.0.2                                      10.0.0.1
```

## Компоненты системы

### 1. TUN Interface (`pkg/tun/`)

**Назначение:** Создание виртуального сетевого интерфейса уровня 3 (IP)

**Как работает:**
- Создает виртуальное устройство `/dev/tunX` (Linux) или TAP adapter (Windows)
- ОС видит его как реальное сетевое подключение
- Пакеты, отправленные в TUN, попадают в наше приложение
- Пакеты, записанные в TUN, попадают в сетевой стек ОС

**Файлы:**
- `interface.go` - общий интерфейс (Read/Write/Close, ReadPacket/WritePacket)
- `device.go` - внутренний интерфейс `device` + файловая реализация для Linux
- `tun_linux.go` - создание TUN через `/dev/net/tun` (ioctl TUNSETIFF)
- `tun_windows.go` - создание адаптера через Wintun
- `wintun_windows.go` / `wintun_linux.go` - авто-загрузка `wintun.dll` (Windows) / no-op (Linux)
- `setup_linux.go` - настройка IP/routes для Linux (`ip addr`/`ip link`)
- `setup_windows.go` - настройка IP/routes для Windows (`netsh`)

**Пример потока:**
```
App: ping 10.0.0.1
  ↓
OS routing: destination 10.0.0.1 → vpn0 interface
  ↓
TUN device: kernel → userspace (наше приложение)
  ↓
VPN читает пакет через tunDev.ReadPacket()
```

### 2. Session Layer (`pkg/session/`)

#### 2.1 Handshake — Noise Protocol (`session.go`)

**Паттерн:** `Noise_IK_25519_ChaChaPoly_BLAKE2s` (примитивы как у WireGuard),
реализация — библиотека `github.com/flynn/noise` (собственной криптографии нет).

**Идентичности:** у каждой стороны долговременная статическая Curve25519-пара.
Приватный ключ — в файле (`-key-file`), генерируется `cmd/keygen`.

**Процесс (Noise_IK, 2 сообщения):**
1. Клиент (initiator) **заранее знает и пинит** статический публичный ключ
   сервера (`-server-key`).
2. msg1 (`-> e, es, s, ss`): клиент шифрует свою статическую идентичность для
   сервера. Сервер расшифровывает только своим приватным ключом → MITM невозможен.
3. Сервер узнаёт статический ключ клиента и сверяет его с **allowlist** (`-peers`).
4. msg2 (`<- e, ee, se`): обе стороны выводят два симметричных ключа (Split) с
   **Perfect Forward Secrecy** (эфемерные ключи уникальны на сессию).

#### 2.2 Transport Encryption (`session.go`)

**Алгоритм:** ChaCha20-Poly1305 (AEAD), отдельный ключ на каждое направление
(Noise CipherState).

**Формат кадра:**
```
┌──────────────┬─────────────┬──────────────┐
│ Counter (8)  │ Ciphertext  │ Auth Tag(16) │
└──────────────┴─────────────┴──────────────┘
```

- **Counter**: монотонный 64-битный счётчик; задаётся как nonce шифра
  (`SetNonce`), гарантируя уникальность nonce в сессии, и служит входом для
  sliding-window анти-replay фильтра (RFC 6479, окно 1024) на приёме.
- Counter обновляет окно только **после** успешной AEAD-аутентификации — поэтому
  подделанный счётчик не может сдвинуть окно.

Это в точности подход WireGuard: Noise для аутентификации/ключей, а поверх UDP —
собственное обрамление с явным счётчиком и анти-replay (т.к. UDP переупорядочивает
и теряет пакеты, прямые transport-сообщения Noise неприменимы).

### 3. Transport Layer (`pkg/transport/`)

#### 3.1 Packet Format (`packet.go`)

**Структура пакета:**
```
┌────────────────┬──────┬────────┬─────────┬─────────────┐
│ Random Prefix  │ Type │ Length │ Payload │   Padding   │
│     (8 bytes)  │ (1)  │  (2)   │  (var)  │  (0-64)     │
└────────────────┴──────┴────────┴─────────┴─────────────┘
```

**Типы пакетов:**
- `0x01` - Handshake (обмен ключами)
- `0x02` - Data (зашифрованные данные)
- `0x03` - Ping (keep-alive)

**DPI Obfuscation:**

1. **Random Prefix (8 байт):**
   - Каждый пакет начинается со случайных данных
   - DPI системы не могут найти постоянный header pattern

2. **Random Padding (0-64 байта):**
   - Скрывает реальный размер пакета
   - Усложняет traffic fingerprinting

3. **Encrypted Payload:**
   - AES-GCM делает содержимое неотличимым от случайных данных
   - Нет plaintext patterns для DPI

**Почему это работает против DPI?**
- Нет фиксированного magic number
- Нет постоянного header pattern
- Размер пакетов варьируется
- Содержимое выглядит как random bytes
- Невозможно отличить от DTLS, QUIC или просто шума

#### 3.2 UDP Transport (`udp.go`)

**Почему UDP?**
- Нет overhead от TCP handshake
- Нет head-of-line blocking
- VPN уже работает поверх IP - нет смысла в TCP
- Меньшая latency
- Приложения сами решают нужна ли надежность (TCP over VPN работает нормально)

**Оптимизации:**
```go
conn.SetReadBuffer(4 * 1024 * 1024)  // 4MB read buffer
conn.SetWriteBuffer(4 * 1024 * 1024) // 4MB write buffer
```

### 4. Server (`cmd/server/`)

**Архитектура:**

```
┌──────────────────────────────────────────────┐
│              VPN Server                      │
│                                              │
│  ┌────────────┐          ┌────────────┐     │
│  │   UDP      │          │    TUN     │     │
│  │  Listener  │          │  Interface │     │
│  └─────┬──────┘          └──────┬─────┘     │
│        │                        │           │
│        ↓                        ↓           │
│  ┌─────────────────────────────────────┐   │
│  │      Client Manager                 │   │
│  │  map[addr]*Client{cipher, lastSeen} │   │
│  └─────────────────────────────────────┘   │
│                                              │
│  Goroutines:                                 │
│  1. handleUDP()    - receive from clients   │
│  2. handleTUN()    - forward to clients     │
│  3. cleanupClients() - remove inactive      │
└──────────────────────────────────────────────┘
```

**Поток данных:**

```
Internet → Client_A
             ↓
Client_A: encrypt packet → UDP
             ↓
Server: UDP → decrypt → TUN
             ↓
OS routing → TUN → Server app
             ↓
Server: encrypt for all clients → UDP
             ↓
Client_A, Client_B, ... receive packet
```

**Multi-client support:**
- Каждый клиент имеет свой cipher (разные shared secrets)
- Сервер хранит: `map[clientAddr]*Client`
- Broadcast: пакет из TUN отправляется всем клиентам

### 5. Client (`cmd/client/`)

**Архитектура:**

```
┌──────────────────────────────────────────────┐
│              VPN Client                      │
│                                              │
│  ┌────────────┐          ┌────────────┐     │
│  │    UDP     │          │    TUN     │     │
│  │   Socket   │          │  Interface │     │
│  └─────┬──────┘          └──────┬─────┘     │
│        │                        │           │
│        ↓                        ↓           │
│  ┌─────────────────────────────────────┐   │
│  │    Session Cipher                   │   │
│  │    (sharedSecret, epoch)            │   │
│  └─────────────────────────────────────┘   │
│                                              │
│  Goroutines:                                 │
│  1. handleUDP()   - receive from server     │
│  2. handleTUN()   - send to server          │
│  3. keepAlive()   - ping every 30s          │
└──────────────────────────────────────────────┘
```

**Handshake sequence:**

```
Client                           Server
  |                                |
  |-- Handshake(PubKey_C) -------->|
  |                                | Compute SharedSecret
  |                                | Store client
  |<------ Handshake(PubKey_S) ---|
  | Compute SharedSecret           |
  |                                |
  |-- Encrypted Data ------------->|
  |<---------- Encrypted Data -----|
```

## Детальный поток пакета

### Outbound (Client → Server → Internet)

```
1. Application
   User: curl http://example.com
   ↓
2. OS Network Stack
   Routing: example.com → vpn0 interface
   ↓
3. TUN Device (Kernel → Userspace)
   Raw IP packet read by VPN client
   ↓
4. VPN Client Encryption
   - Get current epoch
   - Generate random nonce
   - Encrypt: AES-GCM(packet, sessionKey, nonce)
   - Build: [epoch|nonce|ciphertext|tag]
   ↓
5. Transport Layer Obfuscation
   - Add random prefix (8 bytes)
   - Wrap: PacketTypeData
   - Add random padding (0-64 bytes)
   - Result: [rand_prefix|type|len|encrypted|padding]
   ↓
6. UDP Send
   Send to server:port
   ↓
7. Server UDP Receive
   ↓
8. Server Decrypt
   - Extract epoch from encrypted data
   - Derive sessionKey from SharedSecret + epoch
   - Decrypt: AES-GCM
   - Verify auth tag
   ↓
9. Server TUN Write
   Write decrypted IP packet to TUN
   ↓
10. Server OS
    Route packet to internet
```

### Inbound (Internet → Server → Client)

```
1. Server OS receives packet from internet
   ↓
2. Routing decides: destination → vpn0
   ↓
3. TUN device (Kernel → Userspace)
   ↓
4. Server encrypts for each client
   - For each connected client:
   - Encrypt with client's cipher
   - Send via UDP
   ↓
5. Client receives UDP packet
   ↓
6. Client decrypts
   ↓
7. Client writes to TUN
   ↓
8. OS delivers to application
```

## Безопасность

### Угрозы и защита

| Угроза | Защита |
|--------|--------|
| Man-in-the-Middle | Noise_IK + пиннинг статического ключа сервера (`-server-key`) |
| Unauthorized clients | Allowlist статических ключей клиентов (`-peers`) |
| Eavesdropping | ChaCha20-Poly1305 (AEAD) |
| Packet tampering | Poly1305 authentication tag |
| Replay attacks | Counter + sliding window (RFC 6479) |
| Traffic analysis | Random padding, random prefix |
| DPI detection | Obfuscation layer |
| Key compromise | Perfect Forward Secrecy (эфемерные ключи на сессию) |
| DoS (handshake flood) | Per-source-IP rate limiting + глобальный backstop |

### Текущие ограничения

⚠️ **Это учебная реализация.** Реализовано: Noise-аутентификация по ключам,
анти-replay, per-IP rate limiting. Остаётся для production:

1. **Нет периодического rehandshake** — одна сессия на подключение
2. **Allowlist ключей вручную** — без CA/сертификатов
3. **Глобальный TUN/маршрутизация** — NAT настраивается отдельным скриптом

### Уже реализовано (бывшие TODO)

- ✅ Аутентификация — Noise_IK с пиннингом ключа сервера и allowlist клиентов
  (`pkg/session`). PSK больше не нужен.
- ✅ Replay protection — явный counter + sliding window (`pkg/session/replay.go`).
- ✅ Rate limiting — per-source-IP + глобальный backstop (`pkg/ratelimit`).

### Что ещё можно добавить
- Периодический rehandshake (rekey) для долгоживущих сессий, как в WireGuard.
- CA/сертификаты вместо ручного allowlist публичных ключей.

## Производительность

### Бенчмарки (примерные на современном CPU)

```
Операция                       Время
────────────────────────────────────
Noise handshake (на сессию)    ~100 μs
ChaCha20-Poly1305 (1KB)        ~1-2 μs
TUN read/write                 ~10 μs
UDP send/receive               ~20 μs
────────────────────────────────────
Overhead на пакет данных       ~35 μs
```

### Оптимизации

1. **ChaCha20-Poly1305**: быстрый AEAD без зависимости от AES-NI
2. **Большие буферы**: 4MB для UDP socket
3. **Кэш ключей**: один handshake на подключение, потоковый AEAD на data-path
4. **Goroutines**: Параллельная обработка

## Сравнение с другими VPN

| Feature | Our VPN | WireGuard | OpenVPN |
|---------|---------|-----------|---------|
| Key Exchange | Noise_IK | Noise Protocol | RSA/ECDH |
| Encryption | ChaCha20-Poly1305 | ChaCha20-Poly1305 | AES-256-GCM/CBC |
| Transport | UDP | UDP | UDP/TCP |
| Lines of Code | ~1000 | ~4000 | ~100,000 |
| Performance | Good | Excellent | Moderate |
| Obfuscation | Basic | No | OpenVPN XOR |
| Maturity | Experimental | Production | Production |

## Debugging

### Логи

```bash
# Server
[INFO] TUN interface created: vpn0
[INFO] UDP server listening on 0.0.0.0:51820
[INFO] Handshake from 192.168.1.100:54321
[INFO] Client registered: ... (epoch: 123456)

# Client
[INFO] Client Public Key: ABC...
[INFO] Server Public Key: XYZ...
[INFO] Shared secret established (epoch: 123456)
```

### Инструменты

```bash
# Посмотреть трафик (зашифрованный)
sudo tcpdump -i any -n port 51820

# Посмотреть VPN интерфейс
ip addr show vpn0

# Трассировка пакетов
traceroute -i vpn0 8.8.8.8
```

## Расширения

### Идеи для улучшения

1. **Obfuscation v2:**
   - Имитация HTTPS (TLS-like headers)
   - Имитация DNS (DNS-over-UDP camouflage)
   - Polymorphic packets

2. **Better handshake:**
   - Noise Protocol Framework
   - Post-quantum cryptography (Kyber)

3. **Routing:**
   - Split-tunneling
   - Policy-based routing
   - Multiple exit nodes

4. **Management:**
   - Web UI
   - REST API
   - Metrics (Prometheus)

5. **Performance:**
   - io_uring на Linux
   - eBPF для packet filtering
   - SIMD для crypto
