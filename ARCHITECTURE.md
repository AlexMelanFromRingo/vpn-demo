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
│      ↕      │         AES-256-GCM           │      ↕      │
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

### 2. Crypto Layer (`pkg/crypto/`)

#### 2.1 Key Exchange (`keypair.go`)

**Алгоритм:** Curve25519 ECDH (Elliptic Curve Diffie-Hellman)

**Процесс:**
1. Клиент генерирует пару ключей: `(PrivKey_C, PubKey_C)`
2. Сервер генерирует пару ключей: `(PrivKey_S, PubKey_S)`
3. Обмениваются публичными ключами
4. Вычисляют общий секрет:
   - Клиент: `SharedSecret = ECDH(PrivKey_C, PubKey_S)`
   - Сервер: `SharedSecret = ECDH(PrivKey_S, PubKey_C)`

**Результат:** Обе стороны имеют идентичный 32-байтовый `SharedSecret`

#### 2.2 Encryption (`cipher.go`)

**Алгоритм:** AES-256-GCM (Galois/Counter Mode)

**Почему GCM?**
- Аутентифицированное шифрование (AEAD - Authenticated Encryption with Associated Data)
- Защита от подделки (authentication tag)
- Высокая производительность (аппаратное ускорение AES-NI)
- Параллелизуемое шифрование

**Session Keys (Ротация ключей):**

```go
epoch = UnixTimestamp / 300  // Новый epoch каждые 5 минут
sessionKey = SHA256(sharedSecret || epoch)
```

**Почему ротация?**
- Perfect Forward Secrecy: компрометация одного ключа не открывает старый трафик
- Ограничение количества данных на один ключ (best practice для GCM)

**Формат зашифрованных данных:**
```
┌───────────┬──────────┬─────────────┬──────────────┐
│ Epoch (8) │ Nonce(12)│ Ciphertext  │ Auth Tag(16) │
└───────────┴──────────┴─────────────┴──────────────┘
```

- **Epoch**: Временная метка для определения ключа
- **Nonce**: Случайное значение (должно быть уникальным для каждого сообщения)
- **Ciphertext**: Зашифрованные данные
- **Auth Tag**: GMAC тег для проверки подлинности

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
| Man-in-the-Middle | ECDH key exchange + no cert pinning ⚠️ |
| Eavesdropping | AES-256-GCM encryption |
| Packet tampering | GCM authentication tag |
| Replay attacks | Nonce uniqueness (⚠️ no sequence numbers yet) |
| Traffic analysis | Random padding, random prefix |
| DPI detection | Obfuscation layer |
| Key compromise | Key rotation every 5 min |

### Текущие ограничения

⚠️ **Это базовая реализация для обучения!**

1. **Нет аутентификации клиентов** - любой может подключиться
2. **Нет защиты от replay** - нет sequence numbers
3. **Vulnerable to DoS** - нет rate limiting
4. **No cert validation** - можно сделать MITM при первом подключении

### Улучшения для production

```go
// TODO: Pre-shared key для аутентификации
type Handshake struct {
    PublicKey [32]byte
    Signature [64]byte  // Ed25519 signature with PSK
    Timestamp int64
}

// TODO: Sequence numbers для replay protection
type DataPacket struct {
    SequenceNumber uint64
    EncryptedData  []byte
}

// TODO: Rate limiting
rateLimiter := rate.NewLimiter(rate.Limit(100), 1000)
```

## Производительность

### Бенчмарки (примерные на современном CPU)

```
Операция                    Время
────────────────────────────────────
ECDH key exchange           ~50 μs
AES-256-GCM encrypt (1KB)   ~2 μs
AES-256-GCM decrypt (1KB)   ~2 μs
TUN read/write              ~10 μs
UDP send/receive            ~20 μs
────────────────────────────────────
Total overhead per packet   ~35 μs
```

### Throughput

```
CPU: Modern x86_64 with AES-NI
Throughput: 500-800 Mbps
Latency overhead: 1-2 ms
```

### Оптимизации

1. **AES-NI**: Аппаратное ускорение AES на x86
2. **Большие буферы**: 4MB для UDP socket
3. **Zero-copy**: Минимум аллокаций
4. **Goroutines**: Параллельная обработка

## Сравнение с другими VPN

| Feature | Our VPN | WireGuard | OpenVPN |
|---------|---------|-----------|---------|
| Key Exchange | ECDH | Noise Protocol | RSA/ECDH |
| Encryption | AES-256-GCM | ChaCha20-Poly1305 | AES-256-GCM/CBC |
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
