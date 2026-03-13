<p align="center">
  <h1 align="center">Outpost VPN</h1>
  <p align="center">
    Open-source WireGuard VPN & Zero-Trust Network Access Platform<br/>
    <strong>100% Apache 2.0 — никаких enterprise-пейволлов</strong>
  </p>
  <p align="center">
    <a href="#quick-start">Quick Start</a> ·
    <a href="#возможности">Возможности</a> ·
    <a href="#api-documentation">API Docs</a> ·
    <a href="#поддержать-проект">Поддержать</a>
  </p>
</p>

---

## Почему Outpost?

В отличие от defguard или NetBird, Outpost предоставляет **все enterprise-фичи бесплатно и без ограничений**: ZTNA проверки устройств, compliance-дашборды, мультитенантность для MSP, аналитика трафика, встроенный OIDC-провайдер и PKI — всё open-source с первого дня.

**Для кого:**
- Компании, которым нужен корпоративный VPN без vendor lock-in
- MSP/интеграторы, предоставляющие VPN как сервис
- DevOps-команды, строящие Zero-Trust сети
- Те, кто устал платить за enterprise-фичи в других решениях

## Возможности

### VPN и сетевое взаимодействие
- **WireGuard VPN** — поддержка kernel и userspace реализаций
- **Site-to-Site тунели** — full mesh и hub-and-spoke топологии с BGP-lite обменом маршрутами
- **Gateway HA** — несколько гейтвеев на сеть с автоматическим failover
- **Split tunneling** — маршрутизация по группам
- **Real-time синхронизация** через gRPC bidirectional streaming

### Zero-Trust (ZTNA)
- **Проверки состояния устройств** — шифрование диска, антивирус, файрвол, версия ОС, блокировка экрана
- **Непрерывная верификация** — повторная проверка posture по расписанию
- **Политики posture по сетям** — разные требования для разных сетей
- **MFA на уровне WireGuard** — пир удаляется из гейтвея при истечении MFA-сессии

### Идентификация и доступ
- **Встроенный OIDC-провайдер** — "Войти через Outpost" (Authorization Code + PKCE, RS256)
- **MFA/2FA** — TOTP, WebAuthn/FIDO2, email-токены, backup-коды
- **Внешнее SSO** — Google, Azure AD, Okta, Keycloak
- **LDAP/Active Directory** синхронизация
- **SAML 2.0** Service Provider
- **SCIM 2.0** автоматический provisioning пользователей (Okta, Azure AD)
- **RBAC** с гранулярными ACL по сетям

### Аналитика и Compliance
- **Аналитика трафика** — графики bandwidth, top-пользователи, heatmap подключений
- **Compliance-дашборд** — автоматическая оценка готовности SOC2, ISO 27001, GDPR
- **Аудит-лог** — каждое действие администратора записывается с полным контекстом
- **SIEM-интеграция** — webhook + syslog экспорт с HMAC-подписью
- **CSV/JSON экспорт** аудит-лога

### Платформа
- **Мультитенантность** — MSP/reseller режим с изоляцией по организациям
- **Встроенный PKI** — автоматическая ротация WireGuard-ключей без простоя
- **Prometheus-метрики** — полная observability из коробки
- **Webhook-уведомления** — интеграция с внешними системами (Slack, PagerDuty, etc.)
- **Email-уведомления** — SMTP-маилер для MFA, enrollment, password reset
- **Управление сессиями** — просмотр и отзыв активных сессий
- **Docker** и **Kubernetes** (Helm) ready
- **Интерактивная карта сети** — визуализация топологии

## Quick Start

```bash
git clone https://github.com/romashqua/outpost.git
cd outpost
docker compose -f deploy/docker/docker-compose.yml up -d
```

После запуска:
- **API**: http://localhost:8080
- **WireGuard**: порт 51820/udp
- **Healthcheck**: http://localhost:8080/healthz
- **Метрики**: http://localhost:8080/metrics

### Первые шаги

```bash
# Установить CLI
go install ./cmd/outpostctl

# Проверить статус
outpostctl status

# Авторизоваться
outpostctl login -u admin -p <password>

# Создать VPN-сеть
outpostctl networks create -n "Office VPN" -a 10.10.0.0/24

# Посмотреть compliance-отчёт
outpostctl compliance report
```

## VPN-клиент (outpost-client)

Стандартный WireGuard клиент **не поддерживает 2FA/MFA**. Outpost включает собственный клиент, который реализует полный flow аутентификации:

```
Пользователь → Login (username/password) → MFA Challenge (TOTP/WebAuthn/backup)
    → Получение WireGuard конфигурации → Подключение к VPN
    → Периодическая проверка posture → Обновление сессии
```

### Установка

```bash
go install ./cmd/outpost-client
```

### Использование

```bash
# Подключиться к VPN (интерактивный логин + MFA)
outpost-client connect

# Подключиться к конкретной сети
outpost-client connect <network-id>

# Отключиться
outpost-client disconnect

# Только авторизоваться (без подключения)
outpost-client login

# Показать статус устройства
outpost-client status

# Показать posture-проверку устройства
outpost-client posture
```

### Что делает клиент

| Функция | Описание |
|---|---|
| **Аутентификация с MFA** | Login → TOTP/WebAuthn/backup code → получение JWT |
| **Управление туннелем** | Генерация ключей, enrollment, wg-quick up/down |
| **Device Posture** | Проверка шифрования диска, файрвола, блокировки экрана, версии ОС |
| **Обновление сессии** | Автоматический refresh токена, re-auth при истечении MFA |
| **Кроссплатформенность** | macOS (FileVault), Linux (LUKS, iptables/nft/ufw), Windows (BitLocker) |

### Переменные окружения клиента

| Переменная | Описание | По умолчанию |
|---|---|---|
| `OUTPOST_SERVER` | URL сервера Outpost | `http://localhost:8080` |
| `OUTPOST_USER` | Имя пользователя (для неинтерактивного режима) | — |
| `OUTPOST_PASS` | Пароль (для неинтерактивного режима) | — |

### Как работает MFA на уровне WireGuard

```
1. Клиент проходит login + MFA → получает JWT-токен
2. Клиент отправляет enrollment с публичным ключом → сервер добавляет пир на gateway
3. При истечении MFA-сессии → сервер отправляет PeerUpdate{REMOVE} → gateway удаляет пир
4. Трафик мгновенно прекращается → клиент запрашивает повторную MFA-верификацию
5. После верификации → PeerUpdate{ADD} → VPN восстанавливается
```

## Архитектура

```
┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
│  Web UI     │  │outpost-client│  │  outpostctl │  │  External   │
│  (React)    │  │  VPN+MFA    │  │  (CLI)      │  │  OIDC/SAML  │
└──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘
       │                │                │                │
       ▼                ▼                ▼                ▼
┌──────────────────────────────────────────────────────────────────┐
│                        outpost-core                              │
│  HTTP API (Chi) · gRPC · OIDC Provider · SCIM 2.0               │
│  Auth+MFA · RBAC · Analytics · Compliance · Webhooks · Sessions  │
└──────────┬───────────────────────────────┬───────────────────────┘
           │ gRPC streaming                │ gRPC
           ▼                               ▼
┌───────────────────┐          ┌───────────────────┐
│  outpost-gateway  │          │  outpost-proxy    │
│  WireGuard agent  │◄────────►│  Enrollment proxy │
│  S2S tunnels      │          │  (DMZ)            │
└───────────────────┘          └───────────────────┘
```

| Компонент | Описание | Порты |
|---|---|---|
| `outpost-core` | API, бизнес-логика, OIDC, admin panel | 8080, 9090 (gRPC) |
| `outpost-gateway` | WireGuard agent (клиенты + S2S) | 51820/udp |
| `outpost-proxy` | Enrollment/auth прокси для DMZ | 8081 |
| `outpost-client` | VPN-клиент с поддержкой MFA/2FA | — |
| `outpostctl` | CLI-утилита управления | — |

## Разработка

### Требования

- Go 1.25+
- PostgreSQL 17, Redis 7
- Node.js 20+ и pnpm
- [Buf](https://buf.build/) (protobuf)

### Сборка

```bash
make build          # Собрать все бинарники
make test           # Запустить тесты
make lint           # Запустить линтер
make proto          # Сгенерировать protobuf-код
make docker-up      # Поднять полный Docker-стек
cd web-ui && pnpm dev  # Frontend dev-сервер
```

### Переменные окружения

| Переменная | Описание | По умолчанию |
|---|---|---|
| `OUTPOST_HTTP_ADDR` | HTTP-адрес | `:8080` |
| `OUTPOST_GRPC_ADDR` | gRPC-адрес | `:9090` |
| `OUTPOST_DB_HOST` | Хост PostgreSQL | `localhost` |
| `OUTPOST_DB_NAME` | Имя базы данных | `outpost` |
| `OUTPOST_JWT_SECRET` | Секрет для подписи JWT | — |
| `OUTPOST_OIDC_ISSUER` | URL OIDC-провайдера | `http://localhost:8080` |
| `OUTPOST_LDAP_ENABLED` | Включить LDAP-синхронизацию | `false` |
| `OUTPOST_SAML_ENABLED` | Включить SAML 2.0 SP | `false` |
| `OUTPOST_REDIS_ADDR` | Адрес Redis | `localhost:6379` |

Полный список — в [internal/config/config.go](internal/config/config.go).

### Структура проекта

```
outpost/
├── cmd/                       # Точки входа сервисов
│   ├── outpost-core/          # API + gRPC сервер
│   ├── outpost-gateway/       # WireGuard gateway agent
│   ├── outpost-proxy/         # Enrollment proxy (DMZ)
│   ├── outpost-client/        # VPN-клиент с MFA
│   └── outpostctl/            # CLI-утилита
├── internal/
│   ├── core/                  # HTTP-хендлеры, gRPC
│   │   └── handler/           # REST API (users, networks, devices, gateways)
│   ├── gateway/               # Управление WG-интерфейсами
│   ├── auth/                  # Аутентификация и авторизация
│   │   ├── oidc/              # Встроенный OIDC-провайдер
│   │   ├── saml/              # SAML 2.0 Service Provider
│   │   ├── ldap/              # LDAP/AD синхронизация
│   │   ├── scim/              # SCIM 2.0 provisioning
│   │   ├── mfa/               # TOTP, WebAuthn, email, backup-коды
│   │   └── rbac/              # Role-Based Access Control
│   ├── s2s/                   # Site-to-Site движок
│   ├── ztna/                  # Zero-Trust проверки устройств
│   ├── analytics/             # Аналитика трафика
│   ├── compliance/            # Compliance-проверки (SOC2, ISO27001, GDPR)
│   ├── tenant/                # Мультитенантность
│   ├── pki/                   # Ротация ключей и PKI
│   ├── webhook/               # Исходящие вебхуки
│   ├── session/               # Управление сессиями
│   ├── mail/                  # Email-уведомления (SMTP)
│   ├── observability/         # Метрики, аудит-лог, SIEM
│   ├── client/                # VPN-клиент (API, tunnel, posture)
│   ├── wireguard/             # WireGuard-абстракция
│   └── db/                    # PostgreSQL (pgx)
├── web-ui/                    # React + TypeScript + Tailwind
├── proto/                     # Protobuf-определения (Buf)
├── migrations/                # SQL-миграции
├── docs/                      # OpenAPI спецификация
└── deploy/                    # Docker и Helm charts
```

## API Documentation

Полная OpenAPI 3.1 спецификация: [`docs/openapi.yaml`](docs/openapi.yaml)

### Основные группы эндпоинтов

| Группа | Путь | Описание |
|---|---|---|
| Health | `/healthz`, `/readyz`, `/metrics` | Healthcheck и метрики |
| Auth | `/api/v1/auth/*` | Аутентификация (login/logout) |
| OIDC | `/oidc/*` | OpenID Connect провайдер |
| MFA | `/api/v1/mfa/*` | Управление MFA (TOTP, WebAuthn, backup) |
| SAML | `/saml/*` | SAML 2.0 Service Provider |
| SCIM | `/api/v1/scim/v2/*` | SCIM 2.0 provisioning |
| Users | `/api/v1/users/*` | CRUD пользователей |
| Networks | `/api/v1/networks/*` | CRUD VPN-сетей |
| Devices | `/api/v1/devices/*` | Управление устройствами |
| Gateways | `/api/v1/gateways/*` | Управление гейтвеями |
| Sessions | `/api/v1/sessions/*` | Управление сессиями |
| Audit | `/api/v1/audit/*` | Аудит-лог |
| Webhooks | `/api/v1/webhooks/*` | Подписки на вебхуки |
| Analytics | `/api/v1/analytics/*` | Аналитика трафика |
| Compliance | `/api/v1/compliance/*` | Compliance-отчёты |

### Пример запроса

```bash
# Авторизация
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret"}' | jq -r .token)

# Получить compliance-отчёт
curl -s http://localhost:8080/api/v1/compliance/report \
  -H "Authorization: Bearer $TOKEN" | jq .

# Создать VPN-сеть
curl -s -X POST http://localhost:8080/api/v1/networks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Office","address":"10.10.0.0/24"}'
```

## Стек технологий

| Слой | Технология |
|---|---|
| Backend | Go 1.25+, Chi, gRPC, pgx |
| Frontend | React 19, TypeScript, Vite, Tailwind CSS 4 |
| База данных | PostgreSQL 17 |
| Кэш | Redis 7 |
| VPN | WireGuard (kernel + userspace) |
| Аутентификация | OIDC, SAML 2.0, LDAP, SCIM 2.0, WebAuthn, TOTP |
| Observability | Prometheus, slog, audit log, SIEM |
| Деплой | Docker, Helm, single binary |

## Docker

```bash
# Поднять всё
docker compose -f deploy/docker/docker-compose.yml up -d

# Только БД для разработки
docker compose -f deploy/docker/docker-compose.yml up -d postgres redis
```

Контейнеры:
- `postgres:17` — база данных
- `redis:7` — кэш и pub/sub
- `outpost-core` — API-сервер
- `outpost-gateway` — WireGuard agent (NET_ADMIN + /dev/net/tun)
- `outpost-proxy` — enrollment proxy (DMZ)

Две сети: `internal` (все сервисы) и `dmz` (только proxy).

## Миграции

```bash
# Применить миграции
make migrate-up

# Откатить последнюю миграцию
make migrate-down

# Статус миграций
make migrate-status
```

Файлы миграций:
- `000001_create_core_schema` — основная схема (users, networks, devices, gateways, S2S, OIDC, audit)
- `000002_add_ztna_analytics_tenants` — ZTNA posture, аналитика, мультитенантность, PKI
- `000003_add_webhooks` — подписки на вебхуки

## Поддержать проект

Outpost — полностью open-source проект. Если он полезен вам или вашей компании, вы можете поддержать разработку:

### Финансовая поддержка

- **GitHub Sponsors** — [github.com/sponsors/romashqua](https://github.com/sponsors/romashqua)
- **Buy Me a Coffee** — [buymeacoffee.com/romashqua](https://buymeacoffee.com/romashqua)
- **Crypto**:
  - BTC: `bc1q...` _(добавьте свой адрес)_
  - ETH: `0x...` _(добавьте свой адрес)_
  - USDT (TRC20): `T...` _(добавьте свой адрес)_

### Другие способы помочь

- Поставьте звезду на GitHub
- Расскажите коллегам о проекте
- Создайте issue или pull request
- Напишите статью или обзор
- Помогите с переводами

### Коммерческая поддержка

Для компаний, которым нужна профессиональная поддержка:

- **Managed Cloud** — мы разворачиваем и управляем Outpost за вас
- **Support Contract** — приоритетная поддержка, SLA, помощь с интеграцией
- **Custom Development** — доработка под ваши нужды

Свяжитесь: [outpost@romashqua.com](mailto:outpost@romashqua.com)

## Содействие

Мы рады любому вкладу! Смотрите [CONTRIBUTING.md](CONTRIBUTING.md) для деталей.

```bash
# Форкните репозиторий, создайте ветку
git checkout -b feature/my-feature

# Сделайте изменения, запустите тесты
make test && make lint

# Создайте PR
```

## Лицензия

Apache License 2.0 — см. [LICENSE](LICENSE).

Все фичи полностью open-source. Без enterprise-модулей, без пейволлов, без ограничений.

---

<p align="center">
  Сделано с любовью в <a href="https://github.com/romashqua">Romashqua Labs</a>
</p>
