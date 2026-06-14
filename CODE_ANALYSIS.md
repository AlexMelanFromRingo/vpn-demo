# Code Analysis & Fixes

Полный аудит кода, логики и архитектуры VPN с исправлениями. Дата: 2026-06-14.

Проверено: `go build` (linux+windows), `go vet`, `gofmt`, `go test -race ./...`,
end-to-end тест в network namespaces (`scripts/integration-test.sh`).

## Найденные и исправленные баги

| # | Серьёзность | Файл | Проблема | Исправление |
|---|-------------|------|----------|-------------|
| 1 | **High** | `cmd/client/main.go` | В `handshake()` при получении пакета не-handshake типа выполнялось `return err`, где `err == nil` → функция «успешно» завершалась с `cipher == nil`, клиент тихо не работал. | Возврат настоящей ошибки с типом пакета. |
| 2 | **High** | `pkg/crypto/cipher.go` | `Decrypt` пересоздавал AES-cipher + GCM + SHA-256 **на каждый пакет** — убивало throughput. | Кэш AEAD по эпохам. Decrypt ускорился до ~2.1 ГБ/с (бенчмарк). |
| 3 | **Med** | `pkg/crypto/cipher.go` | Гонки данных: `keyEpoch`/`lastRotation` читались без блокировки в `Encrypt`/`rotateKey`. | Переписано на потокобезопасный кэш под `sync.Mutex`; эпоха берётся прямо из времени. Подтверждено `-race`. |
| 4 | **Med** | `cmd/{server,client}/main.go` | Busy-loop при завершении: закрытие UDP-сокета давало бесконечный поток ошибок «use of closed network connection» (100% CPU до выхода процесса). | `errors.Is(err, net.ErrClosed)` → выход из горутины; backoff на прочих ошибках. То же для чтения TUN (`os.ErrClosed`/`io.EOF`). |
| 5 | **Med** | `pkg/crypto/cipher.go` | `Decrypt` принимал любую эпоху → неограниченный рост кэша при атаке (мелкий DoS). | Кэшируются только эпохи в окне ±1 от текущей; остальные обрабатываются, но не кэшируются. |
| 6 | **Low** | `cmd/{server,client}/main.go` | Разбор ICMP читал тип/код по фиксированному смещению 20 — неверно при наличии IP-опций (IHL>5). Плюс дублирование функции в двух файлах. | Вынесено в `pkg/ipparse` с учётом IHL; покрыто тестами. |
| 7 | **Low** | `pkg/transport/packet.go` | Игнорировались ошибки `rand.Read` (3 вызова). | Один проверяемый вызов CSPRNG на пакет. |
| 8 | **Low** | `pkg/tun/*` | Мёртвый метод `File()` в интерфейсе `device` (нигде не использовался). | Удалён из интерфейса и реализаций. |
| 9 | **Low** | весь репозиторий | 6 файлов не проходили `gofmt`; устаревшие `// +build` теги. | `gofmt -w`, теги обновлены до `//go:build`. |
| 10 | **Doc** | `ARCHITECTURE.md` | Ссылки на несуществующие файлы (`interface_linux.go`, `interface_windows.go`). | Список файлов приведён в соответствие. |

## Что НЕ менялось (формат провода сохранён)

Формат пакета и зашифрованных данных (`[epoch|nonce|ciphertext|tag]`) оставлен
совместимым — клиент и сервер новой версии полностью совместимы между собой, а
рефакторинг крипто-слоя не меняет байты на проводе.

## Добавленные тесты

- `pkg/crypto`: ECDH-согласование, уникальность ключей, base64 round-trip,
  interop encrypt/decrypt, детект подделки (ciphertext и nonce), неверный ключ,
  детерминизм ключа по эпохе, ограниченность кэша, конкурентный доступ
  (`-race`), бенчмарки.
- `pkg/transport`: round-trip разных размеров, сохранение типа, вариативность
  обфускации, обрезанный/слишком маленький/слишком большой пакет, **фаззинг**
  `Unmarshal` (≈1M запусков без паник).
- `pkg/ipparse`: ICMP без/с IP-опциями, TCP/UDP/unknown, битые входные данные.
- `scripts/integration-test.sh`: поднимает server+client в netns, проверяет
  handshake, двусторонний ping через туннель, факт туннелирования по UDP и
  **отсутствие открытого payload на проводе** (маркер `cafebabe…` не виден в
  перехвате tcpdump).

## Security hardening (раунд 2)

Реализовано поверх исправлений выше (формат провода обновлён согласованно на
обоих концах):

- **PSK-аутентификация handshake** (`pkg/crypto/handshake.go`):
  `pubkey‖timestamp‖HMAC-SHA256(psk, …)`, constant-time проверка. Чужой клиент
  без PSK не подключится; привязка pubkey к MAC закрывает MITM. Флаг `-psk` /
  env `VPN_PSK` у сервера и клиента.
- **Анти-replay** (`pkg/crypto/replay.go`, RFC 6479): в каждый data-пакет добавлен
  монотонный sequence number, он же — детерминированный GCM-nonce. На приёме —
  sliding-window (1024): каждый seq принимается один раз, дубликаты/устаревшие
  отбрасываются (`ErrReplay`). Замечание: ротация ключей по эпохам (TOTP) сама по
  себе от replay **не** защищает — она лишь меняет ключ.
- **Rate limiting** (`pkg/ratelimit/`): token-bucket на handshake (≈25/с, burst
  50); PSK-проверка (дешёвый HMAC) выполняется **до** дорогого ECDH.

Покрыто тестами: `handshake_test.go`, `replay_test.go`, `ratelimit_test.go`,
`TestReplayRejected`/`TestOutOfOrderWithinWindowAccepted`, а e2e-тест проверяет
успешную аутентификацию по PSK и **отклонение клиента с неверным PSK**.

## Security hardening (раунд 3): Noise + per-IP rate limiting

Заменил симметричный PSK на асимметричную идентификацию по ключам (как WireGuard)
и сделал rate limiting пер-IP. Формат провода снова обновлён согласованно.

- **Noise Protocol** (`pkg/session`, библиотека `github.com/flynn/noise`):
  паттерн `Noise_IK_25519_ChaChaPoly_BLAKE2s`. Свою криптографию для handshake не
  писал — использована зрелая реализация Noise.
  - У каждой стороны статическая Curve25519-идентичность (`-key-file`, генерится
    `cmd/keygen`). Клиент **пинит** публичный ключ сервера (`-server-key`) →
    защита от MITM. Сервер сверяет ключ клиента с **allowlist** (`-peers`) →
    авторизация по «сертификату»-ключу вместо общего PSK.
  - **PFS**: эфемерные ключи в каждом handshake.
  - Транспорт: два ChaCha20-Poly1305 CipherState (по направлению); явный 64-бит
    counter в каждом кадре служит nonce + входом для прежнего sliding-window
    анти-replay (`SetNonce` + окно). Это в точности подход WireGuard поверх UDP.
- **Per-source-IP rate limiting** (`pkg/ratelimit.IPLimiter`): отдельный
  token-bucket на каждый IP (≈5/с, burst 10, до 4096 IP с вытеснением
  least-recently-seen) + глобальный backstop (≈100/с, burst 200).
- Удалён пакет `pkg/crypto` (PSK-handshake, эпохи, AES-GCM SessionCipher) —
  заменён на `pkg/session`.

Тесты: `pkg/session` (полный handshake, MITM-reject, replay, tamper, out-of-order,
round-trip идентичностей), `ratelimit` (изоляция источников, вытеснение, гонки).
E2E (13/13): рабочий туннель, шифрование на проводе, **+2 негативных сценария**
(неавторизованный клиент; MITM с неверным `-server-key`).

## Периодический rehandshake (раунд 4)

Добавлена ротация ключей для долгих сессий (как в WireGuard):

- `pkg/session/channel.go` — `Channel` хранит **current + previous** сессии.
  Encrypt/Decrypt идут через current; при неуспехе расшифровки current и в
  пределах **grace-окна** (~30 c) пробуется previous — пакеты «в полёте» не
  теряются. Genuine replay на current не ретраится на previous.
- Клиент: флаг `-rekey` (по умолчанию 120 c); горутина `rekeyLoop` периодически
  шлёт новый Noise msg1, ответ (msg2) асинхронно завершается в `handleUDP` →
  `Channel.Rotate`.
- Сервер: handshake от уже подключённого адреса вызывает `Channel.Rotate` вместо
  замены клиента.
- Тесты `channel_test.go`: grace-расшифровка старой сессии, истечение grace,
  replay по-прежнему отклоняется, серия ротаций. E2E (15/15): `-rekey 3s`,
  6 ротаций за ~9 c при **0% потерь** пакетов.

## Известные ограничения (по дизайну демо — не баги)

- Allowlist публичных ключей вручную; CA/сертификатов нет.
- Rekey только по времени (не по числу пакетов/байт).
- Сервер не настраивает NAT/forwarding автоматически (см. `scripts/setup-vpn-nat.sh`).
