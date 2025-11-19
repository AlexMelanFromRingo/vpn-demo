# Split-Tunnel Mode

Эта ветка содержит **split-tunnel** версию VPN.

## Как это работает

```
Windows Client (10.0.0.2)
    ↓
VPN Tunnel (только 10.0.0.0/24)
    ↓
Linux Server (10.0.0.1)

Internet → Напрямую через Windows (НЕ через VPN)
```

## Особенности

✅ **Работает везде** - не требует NAT на сервере
✅ **Работает в gVisor** - не нужны iptables
✅ **Простая настройка** - никаких дополнительных скриптов
✅ **Стабильно** - интернет всегда работает

❌ **Не скрывает IP** - интернет идет напрямую, не через VPN
❌ **Не прокси** - не может проксировать весь трафик

## Использование

### Сборка

```bash
make windows-client
```

### На сервере (Linux/WSL2)

```bash
cd /home/user/vpn-demo
sudo ./bin/vpn-server
```

**Не нужно настраивать NAT!** Этот режим работает без NAT.

### На клиенте (Windows)

1. Скопируйте `bin/vpn-client.exe` на Windows
2. Запустите от имени Администратора
3. Дождитесь успешного handshake

**Логи клиента покажут:**
```
Configuring Windows adapter 'vpn0' with IP 10.0.0.2 (split-tunnel mode)
✓ IP address configured successfully
✓ Split-tunnel mode: only 10.0.0.0/24 routed through VPN
✓ Windows adapter configured in split-tunnel mode
  VPN network: 10.0.0.0/24 → VPN server
  Internet: Direct connection (not through VPN)
```

## Проверка

### Проверка 1: VPN соединение работает

```powershell
ping 10.0.0.1
```

Должен работать! ✅

### Проверка 2: Интернет работает напрямую

```powershell
ping google.com
```

Должен работать! ✅

### Проверка 3: Маршруты

```powershell
route print | findstr "10.0.0"
```

Должна быть строка:
```
10.0.0.0    255.255.255.0    <on link>    10.0.0.2    <метрика>
```

**НЕ должно быть:**
```
0.0.0.0    0.0.0.0    10.0.0.1    ...
```

Это означает что VPN НЕ является default gateway.

### Проверка 4: Внешний IP

```powershell
Invoke-WebRequest -Uri "https://api.ipify.org" -UseBasicParsing
```

Покажет **ваш реальный IP**, не IP сервера.

## Для чего использовать

✅ **Peer-to-peer связь** - подключение к другим устройствам в VPN сети
✅ **Удаленный доступ** - доступ к серверу и другим клиентам VPN
✅ **Безопасная связь** - зашифрованное соединение для внутренней сети
✅ **Gaming LAN** - создание виртуальной локальной сети для игр
✅ **File sharing** - обмен файлами между устройствами в VPN

❌ **НЕ для:**
- Скрытия вашего IP в интернете
- Обхода блокировок (интернет идет напрямую)
- Использования как прокси

## Переключение на Full-Tunnel

Если нужен полный туннель (весь трафик через VPN), переключитесь на ветку `userspace-proxy`:

```bash
git checkout userspace-proxy
make windows-client
```

## Безопасность

В этом режиме:

✅ Трафик VPN сети (10.0.0.0/24) зашифрован AES-256-GCM
✅ Обфускация от DPI для VPN пакетов
✅ Ротация ключей каждые 5 минут

⚠️ Интернет трафик:
- Идет через обычное подключение Windows
- НЕ зашифрован дополнительно VPN
- Виден вашему ISP
- Использует ваш реальный IP

## Преимущества split-tunnel

1. **Скорость** - интернет не замедляется двойной маршрутизацией
2. **Надежность** - работает даже если VPN сервер имеет проблемы с NAT
3. **Совместимость** - работает в gVisor и любых ограниченных средах
4. **Простота** - не требует настройки iptables/firewall на сервере

## Сравнение с Full-Tunnel

| Функция | Split-Tunnel | Full-Tunnel |
|---------|--------------|-------------|
| VPN сеть (10.0.0.0/24) | ✅ Через VPN | ✅ Через VPN |
| Интернет | ❌ Напрямую | ✅ Через VPN |
| Скрывает IP | ❌ Нет | ✅ Да |
| Требует NAT на сервере | ❌ Нет | ✅ Да |
| Работает в gVisor | ✅ Да | ⚠️ Требует userspace proxy |
| Скорость интернета | ✅ Полная | ⚠️ Зависит от сервера |
| Сложность настройки | ✅ Простая | ⚠️ Средняя |

## Отладка

Если VPN не работает:

```powershell
# Проверить адаптер
Get-NetAdapter | Where-Object {$_.InterfaceDescription -like "*Light*"}

# Проверить IP
Get-NetIPAddress -InterfaceAlias "vpn0"

# Проверить маршруты
route print

# Проверить подключение
ping 10.0.0.1
```

Если интернет не работает (хотя должен):

```powershell
# Проверить default gateway (должен быть НЕ 10.0.0.1)
route print | findstr "0.0.0.0"

# Проверить DNS
nslookup google.com
```

## Ветка

Вы находитесь в ветке: `split-tunnel`

Основная ветка с full-tunnel попыткой: `claude/lightweight-vpn-implementation-*`

Ветка с userspace proxy: `userspace-proxy`
