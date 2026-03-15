# Быстрый старт с Outpost VPN

Это руководство проведёт вас через настройку Outpost VPN с нуля: требования, установка, первый вход и подключение первого устройства.

## Требования

| Программа | Версия | Назначение |
|----------|---------|---------|
| Docker | 24+ | Контейнерная среда выполнения |
| Docker Compose | v2+ | Оркестрация |
| Git | 2.40+ | Клонирование репозитория |

Для сборки из исходников (опционально):

| Программа | Версия | Назначение |
|----------|---------|---------|
| Go | 1.24+ | Компиляция бэкенда |
| Node.js | 22+ | Сборка фронтенда |
| pnpm | 9+ | Менеджер пакетов фронтенда |
| PostgreSQL | 17 | База данных |
| Redis | 7 | Сессии, pub/sub, rate limiting |
| Buf | последняя | Генерация кода из Protobuf |
| sqlc | последняя | Типобезопасная SQL-кодогенерация |
| golangci-lint | последняя | Go-линтер |

## Быстрый старт с Docker Compose

### 1. Клонирование репозитория

```bash
git clone https://github.com/romashqua/outpost.git
cd outpost
```

### 2. Запуск полного стека

```bash
docker compose -f deploy/docker/docker-compose.yml up -d
```

Это запускает пять сервисов:

| Сервис | Порт | Описание |
|---------|------|-------------|
| PostgreSQL 17 | 5432 | База данных |
| Redis 7 | 6379 | Кэш и pub/sub |
| outpost-core | 8080 (HTTP), 50051 (gRPC) | API-сервер, панель администратора, OIDC-провайдер |
| outpost-gateway | 51820/udp | WireGuard-шлюз |
| outpost-proxy | 8081 | Публичный прокси для регистрации/авторизации (безопасен для DMZ) |

Миграции базы данных выполняются автоматически при запуске core (встроены через `go:embed`).

### 3. Проверка работы сервисов

```bash
# Проверка, что все контейнеры работают
docker compose -f deploy/docker/docker-compose.yml ps

# Проверка эндпоинта здоровья
curl http://localhost:8080/healthz
# → {"status":"ok"}

# Проверка готовности (проверяет соединение с БД)
curl http://localhost:8080/readyz
# → {"status":"ok"}
```

### 4. Открытие панели администратора

Перейдите в браузере по адресу **http://localhost:8080**.

## Первый вход

Войдите с учётными данными администратора по умолчанию:

- **Логин:** `admin`
- **Пароль:** `admin`

**Немедленно смените пароль.** Пароль по умолчанию создаётся миграцией `000004_seed_admin_user.up.sql` и не является безопасным для любого развёртывания, кроме локальной разработки.

Чтобы сменить пароль:
1. Войдите с `admin` / `admin`
2. Перейдите в **Настройки** > **Безопасность**
3. Установите надёжный пароль

## Создание первой сети

Сеть по умолчанию (`10.10.0.0/16`, порт 51820) создаётся автоматически миграцией-сидом. Для создания дополнительной сети:

1. Перейдите в **Сети** в боковой панели
2. Нажмите **Создать сеть**
3. Заполните данные:
   - **Название:** `office` (или любое описательное имя)
   - **Адрес (CIDR):** `10.20.0.0/24`
   - **DNS-серверы:** `1.1.1.1`, `8.8.8.8`
   - **Порт:** `51820`
   - **Keepalive:** `25` (секунд)
4. Нажмите **Создать**

Или через API:

```bash
# Получение JWT-токена
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | jq -r '.token')

# Создание сети
curl -X POST http://localhost:8080/api/v1/networks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "office",
    "address": "10.20.0.0/24",
    "dns": ["1.1.1.1", "8.8.8.8"],
    "port": 51820,
    "keepalive": 25
  }'
```

## Регистрация шлюза

Шлюзы -- это WireGuard-эндпоинты, обрабатывающие VPN-трафик. Docker Compose включает один шлюз автоматически, но для дополнительных площадок:

1. Перейдите в **Шлюзы** в боковой панели
2. Нажмите **Создать шлюз**
3. Заполните:
   - **Название:** `gw-office` (описательное имя)
   - **Сеть:** Выберите сеть, созданную ранее
   - **Эндпоинт:** `vpn.example.com:51820` (публичный IP/хостнейм + порт)
4. Нажмите **Создать** -- будет сгенерирован токен шлюза
5. Скопируйте токен и установите его как `OUTPOST_GATEWAY_TOKEN` на машине шлюза

Развёртывание шлюза:

```bash
docker run -d \
  --name outpost-gateway \
  --cap-add=NET_ADMIN \
  --cap-add=SYS_MODULE \
  --device=/dev/net/tun \
  --sysctl net.ipv4.ip_forward=1 \
  -e OUTPOST_GATEWAY_CORE_ADDR=core.example.com:50051 \
  -e OUTPOST_GATEWAY_TOKEN=<вставьте-токен-здесь> \
  -p 51820:51820/udp \
  outpost/gateway:latest
```

## Регистрация устройства

1. Перейдите в **Устройства** в боковой панели
2. Нажмите **Добавить устройство**
3. Введите название устройства (например, `laptop-alice`)
4. Система генерирует пару ключей WireGuard и назначает IP
5. Нажмите **Одобрить** для активации устройства
6. Нажмите **Скачать конфиг** для получения файла `.conf`

Скачанный файл конфигурации выглядит так:

```ini
[Interface]
PrivateKey = <сгенерированный-приватный-ключ>
Address = 10.10.0.2/16
DNS = 1.1.1.1, 8.8.8.8

[Peer]
PublicKey = <публичный-ключ-шлюза>
Endpoint = vpn.example.com:51820
AllowedIPs = 10.10.0.0/16
PersistentKeepalive = 25
```

> **Split tunneling:** `AllowedIPs` содержит CIDR вашей VPN-сети (например `10.10.0.0/16`), а не `0.0.0.0/0`. Это означает, что только VPN-трафик идёт через туннель, а интернет работает напрямую. Если нужно направить весь трафик через VPN, настройте ACL группы с `allowed_ips: ["0.0.0.0/0"]`.

Импортируйте этот файл в любой WireGuard-клиент:
- **Linux:** `wg-quick up ./outpost.conf`
- **macOS/Windows:** Импорт через WireGuard GUI
- **iOS/Android:** Сканирование QR-кода или импорт файла
- **Outpost Client:** `outpost-client up` (после `login` и `enroll`)

## Как пользователи подключаются к сетям

В Outpost пользователи привязываются к сетям **через устройства**:

```
Пользователь (1) → (N) Устройства → (1) Сеть
```

Один пользователь может иметь несколько устройств в разных сетях.

**Пошаговый процесс:**
1. Создайте **сеть** (Networks → Создать сеть, укажите CIDR и DNS)
2. Создайте **шлюз** для сети (Gateways → Создать, привяжите к сети)
3. Создайте **устройство** для пользователя (Devices → Добавить) — оно автоматически получит IP из активной сети
4. **Одобрите** устройство (кнопка Approve)
5. **Скачайте конфиг** (кнопка Download) — файл `.conf` готов к импорту в WireGuard

### Подключение через CLI-клиент

```bash
# Авторизация
outpost-client login --server https://vpn.example.com --username ivan

# Регистрация устройства (автоматически получает конфиг)
outpost-client enroll --name "my-laptop"

# Поднять туннель
outpost-client up

# Опустить туннель
outpost-client down
```

### Контроль доступа

- **Группы + ACL:** Создайте группу → добавьте пользователей → настройте ACL с разрешёнными подсетями
- **ZTNA-политики:** Автоматический контроль на основе posture checks (шифрование диска, антивирус, версия ОС)

## Настройка S2S-туннелей (Site-to-Site)

S2S-туннели соединяют ваши офисы/ЦОД через WireGuard. Всё делается за 4 шага.

### Шаг 1: Создайте шлюзы в каждом офисе

У каждого офиса должен быть свой шлюз, привязанный к своей сети:

```bash
# Офис Москва
POST /api/v1/networks
{"name": "moscow", "address": "10.1.0.0/24", "dns": ["1.1.1.1"], "port": 51820}

POST /api/v1/gateways
{"name": "gw-moscow", "network_id": "<moscow-network-id>", "endpoint": "moscow.vpn.company.com:51820"}

# Офис СПб
POST /api/v1/networks
{"name": "spb", "address": "10.2.0.0/24", "dns": ["1.1.1.1"], "port": 51820}

POST /api/v1/gateways
{"name": "gw-spb", "network_id": "<spb-network-id>", "endpoint": "spb.vpn.company.com:51820"}
```

Или через UI: **Сети → Создать сеть**, затем **Шлюзы → Создать шлюз**.

### Шаг 2: Создайте S2S-туннель

```bash
POST /api/v1/s2s-tunnels
{"name": "office-mesh", "topology": "mesh", "description": "Mesh офисов"}
```

Или через UI: **S2S → Новый туннель**, выберите топологию:
- **Mesh** — все шлюзы напрямую соединены друг с другом
- **Hub & Spoke** — все шлюзы подключены через один центральный (hub)

### Шаг 3: Добавьте шлюзы как участников

```bash
POST /api/v1/s2s-tunnels/<tunnel-id>/members
{"gateway_id": "<gw-moscow-id>", "local_subnets": ["10.1.0.0/24"]}

POST /api/v1/s2s-tunnels/<tunnel-id>/members
{"gateway_id": "<gw-spb-id>", "local_subnets": ["10.2.0.0/24"]}
```

Или через UI: в деталях туннеля, секция **Участники** → выберите шлюз → **Создать**.

### Шаг 4: Добавьте маршруты

```bash
POST /api/v1/s2s-tunnels/<tunnel-id>/routes
{"destination": "10.2.0.0/24", "via_gateway": "<gw-spb-id>", "metric": 100}

POST /api/v1/s2s-tunnels/<tunnel-id>/routes
{"destination": "10.1.0.0/24", "via_gateway": "<gw-moscow-id>", "metric": 100}
```

Или через UI: секция **Маршруты** → укажите CIDR, выберите шлюз → **Создать**.

### Готово!

Скачайте WireGuard-конфиг для каждого шлюза:

```bash
GET /api/v1/s2s-tunnels/<tunnel-id>/config/<gateway-id>
```

Или через UI: нажмите иконку 📥 у участника в деталях туннеля.

Конфиг содержит `[Interface]` с приватным ключом и `[Peer]` секции для каждого другого шлюза с правильными `AllowedIPs` и `Endpoint`.

## Сборка из исходников

### Бэкенд

```bash
# Сборка всех бинарных файлов
make build

# Или сборка отдельных компонентов
make build-core
make build-gateway
make build-proxy
make build-client
make build-ctl

# Бинарные файлы в директории bin/
ls bin/
# outpost-core  outpost-gateway  outpost-proxy  outpost-client  outpostctl
```

### Фронтенд

```bash
cd web-ui
pnpm install
pnpm build    # продакшен-сборка (результат встраивается в бинарный файл core)
pnpm dev      # dev-сервер с HMR
```

### Настройка базы данных (для разработки)

```bash
# Запуск только PostgreSQL
docker compose -f deploy/docker/docker-compose.yml up -d postgres

# Ручной запуск миграций (если не используется авто-миграция core)
export DATABASE_URL="postgres://outpost:outpost-dev-password@localhost:5432/outpost?sslmode=disable"
make migrate-up
```

### Локальный запуск

```bash
# Запуск зависимостей
docker compose -f deploy/docker/docker-compose.yml up -d postgres redis

# Запуск core (миграции выполняются автоматически)
export OUTPOST_DB_HOST=localhost
export OUTPOST_DB_PASSWORD=outpost-dev-password
export OUTPOST_REDIS_ADDR=localhost:6379
export OUTPOST_REDIS_PASSWORD=outpost-dev-password
export OUTPOST_JWT_SECRET=$(openssl rand -hex 32)
./bin/outpost-core

# В другом терминале запустите dev-сервер фронтенда
cd web-ui && pnpm dev
```

## Полезные Make-таргеты

| Команда | Описание |
|---------|-------------|
| `make build` | Сборка всех бинарных файлов |
| `make test` | Запуск всех Go-тестов с детектором гонок |
| `make lint` | Запуск golangci-lint |
| `make fmt` | Форматирование Go-кода |
| `make proto` | Генерация protobuf-кода (требуется Buf) |
| `make sqlc` | Генерация типобезопасного SQL-кода |
| `make docker-up` | Запуск полного стека через Docker Compose |
| `make docker-down` | Остановка стека Docker Compose |
| `make docker-logs` | Просмотр логов всех сервисов |
| `make migrate-up` | Применение всех ожидающих миграций |
| `make migrate-down` | Откат одной миграции |
| `make build-client-all` | Кросс-компиляция клиента для Linux, macOS, Windows |

## Дальнейшие шаги

- [Обзор архитектуры](architecture.md) -- как компоненты работают вместе
- [Справочник по конфигурации](configuration.md) -- все переменные окружения
- [Руководство по развёртыванию](deployment.md) -- варианты продуктивного развёртывания
- [Справочник по API](API.md) -- полная документация REST API
- [Руководство по возможностям](features.md) -- ZTNA, S2S-туннели, Smart Routes и другое
