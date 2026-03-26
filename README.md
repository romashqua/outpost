<p align="center">
  <a href="README.md"><b>Русский</b></a> | <a href="README.en.md">English</a>
</p>

<p align="center">
  <img src="assets/banner.svg" alt="Outpost VPN" width="700" />
</p>

<p align="center">
  <strong>Open-Source WireGuard VPN & Zero Trust Network Access</strong><br/>
  Корпоративная безопасность без корпоративной цены. Apache 2.0 навсегда.
</p>

<p align="center">
  <a href="https://github.com/romashqua/outpost/actions"><img src="https://img.shields.io/github/actions/workflow/status/romashqua/outpost/ci.yml?branch=main&style=flat-square&label=CI" alt="CI" /></a>
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=black" />
  <img src="https://img.shields.io/badge/WireGuard-88171A?style=flat-square&logo=wireguard&logoColor=white" />
  <img src="https://img.shields.io/badge/PostgreSQL-17-4169E1?style=flat-square&logo=postgresql&logoColor=white" />
  <a href="LICENSE"><img src="https://img.shields.io/badge/%D0%9B%D0%B8%D1%86%D0%B5%D0%BD%D0%B7%D0%B8%D1%8F-Apache%202.0-green?style=flat-square" alt="Лицензия" /></a>
  <a href="https://github.com/romashqua/outpost/stargazers"><img src="https://img.shields.io/github/stars/romashqua/outpost?style=flat-square" alt="Звёзды" /></a>
  <a href="https://github.com/romashqua/outpost/releases"><img src="https://img.shields.io/github/v/release/romashqua/outpost?style=flat-square&label=Релиз&include_prereleases" alt="Релиз" /></a>
</p>

<p align="center">
  <a href="#быстрый-старт">Быстрый старт</a> &middot;
  <a href="#возможности">Возможности</a> &middot;
  <a href="#архитектура">Архитектура</a> &middot;
  <a href="#скриншоты">Скриншоты</a> &middot;
  <a href="#сравнение">Сравнение</a> &middot;
  <a href="docs/">Документация</a> &middot;
  <a href="CONTRIBUTING.md">Участие</a>
</p>

---

## Почему Outpost?

Все остальные open-source VPN-решения либо прячут критичные фичи за enterprise-пейволлом, либо вообще их не имеют. Outpost даёт **всё** — Zero Trust проверки устройств, compliance-дашборды, мультитенантность для MSP, аналитику трафика, встроенный OIDC/SAML/SCIM, PKI с автоматической ротацией ключей — полностью open-source под Apache 2.0 с первого дня.

**Без enterprise-модулей. Без пейволлов. Никогда.**

### Для кого

- **Команды**, которым нужен корпоративный VPN без vendor lock-in
- **MSP/MSSP**, предоставляющие VPN как сервис множеству клиентов
- **DevOps/Platform-инженеры**, строящие Zero Trust инфраструктуру
- **Регулируемые отрасли**, которым нужны аудит-логи и автоматические проверки compliance
- **Все**, кто устал платить за фичи, которые должны быть стандартом

---

## Быстрый старт

Полностью работающий Outpost за 60 секунд:

```bash
git clone https://github.com/romashqua/outpost.git
cd outpost
docker compose -f deploy/docker/docker-compose.yml up -d
```

Откройте **http://localhost:8080** и войдите с `admin` / `admin`.

Всё. Миграции запускаются автоматически, все сервисы стартуют в правильном порядке, React UI встроен в бинарник.

> **Продакшен?** Смотрите [Руководство по развёртыванию](docs/deployment.md) — Helm-чарты, TLS, рекомендации по hardening.

---

## Возможности

### VPN и сети

| Фича | Описание |
|---|---|
| **WireGuard-туннели** | Kernel и userspace с автоматическим управлением ключами |
| **Site-to-Site** | Full mesh и hub-spoke топологии с автоматической синхронизацией маршрутов |
| **NAT Traversal** | Встроенные STUN/TURN relay-серверы для ограниченных сетей |
| **Smart Routes** | Селективная маршрутизация доменов/CIDR через прокси (SOCKS5, HTTP, Shadowsocks, VLESS) |
| **Real-time синхронизация** | gRPC bidirectional streaming между core и gateway |

### Zero Trust Network Access (ZTNA)

| Фича | Описание |
|---|---|
| **Проверки устройств** | Шифрование диска, антивирус, файрвол, версия ОС, блокировка экрана |
| **Непрерывная верификация** | Периодический пересчёт trust score устройства |
| **Настраиваемые политики** | Кастомизация весов и порогов под вашу модель безопасности |
| **MFA на уровне протокола** | WireGuard-пир удаляется из gateway при истечении MFA-сессии |
| **DNS-правила** | DNS-фильтрация по устройствам и split-horizon DNS |

### Идентификация и доступ

| Фича | Описание |
|---|---|
| **Встроенный OIDC-провайдер** | "Войти через Outpost" (Authorization Code + PKCE, RS256) |
| **MFA/2FA** | TOTP, WebAuthn/FIDO2, email OTP, backup-коды |
| **Внешнее SSO** | Google, Azure AD, Okta, Keycloak через OIDC |
| **LDAP/Active Directory** | Полная синхронизация каталога с маппингом групп |
| **SAML 2.0** | Service Provider режим для корпоративных IdP |
| **SCIM 2.0** | Автоматический провижинг пользователей/групп из Okta, Azure AD |
| **RBAC** | Ролевой контроль доступа с гранулярными ACL по сетям |

### Аналитика и Compliance

| Фича | Описание |
|---|---|
| **Аналитика трафика** | Графики bandwidth в реальном времени, top-пользователи, heatmap подключений |
| **Compliance-дашборд** | Автоматическая оценка готовности SOC 2, ISO 27001, GDPR |
| **Аудит-лог** | Каждое действие администратора записано с полным контекстом и экспортом |
| **SIEM-интеграция** | Экспорт через webhooks и syslog с HMAC-подписью |

### Платформа

| Фича | Описание |
|---|---|
| **Мультитенантность** | MSP/reseller-режим с полной изоляцией организаций |
| **Встроенный PKI** | Автоматическая ротация ключей WireGuard без простоя |
| **Интерактивная карта сети** | SVG-визуализация всей VPN-топологии |
| **Gateway HA** | Multi-gateway failover с автоматическим мониторингом здоровья и переключением |
| **Горизонтальное масштабирование** | Multi-core за LB без etcd — PG + Redis Pub/Sub, zero overhead при 1 core |
| **Email-уведомления** | Настраиваемый SMTP для MFA, enrollment, алертов (i18n) |
| **Prometheus-метрики** | Полная observability с готовыми дашбордами |
| **Docker & Kubernetes** | docker-compose для разработки, Helm-чарты для продакшена |

---

## Архитектура

```
                          Internet
                              │
                   ┌──────────┴──────────┐
                   │   outpost-proxy     │  DMZ-safe enrollment
                   │       :8081         │  и auth прокси
                   └──────────┬──────────┘
                              │
                   ┌──────────┴──────────┐
                   │   Load Balancer     │  L4/L7 (nginx, envoy, HAProxy)
                   └────┬───────────┬────┘
                        │           │
              ┌─────────┴──┐  ┌────┴─────────┐
              │  core-1    │  │  core-2      │  N core-ов (stateless)
              │ :8080 HTTP │  │ :8080 HTTP   │  Redis Pub/Sub для
              │ :50051 gRPC│  │ :50051 gRPC  │  cross-core событий
              └──┬─────┬───┘  └──┬─────┬─────┘
                 │     │         │     │
          gRPC streaming    gRPC streaming
                 │     │         │     │
              ┌──┴──┐ ┌┴────┐ ┌─┴───┐
              │GW-1 │ │GW-2 │ │GW-3 │  N gateway-ов на каждую сеть
              │51820│ │51820│ │51820│  WireGuard UDP
              └──┬──┘ └──┬──┘ └──┬──┘
                 │       │       │
             Клиенты  Клиенты  Клиенты
```

| Компонент | Роль | Порты |
|---|---|---|
| `outpost-core` | API, OIDC-провайдер, админ-панель, gRPC control plane | 8080 (HTTP), 50051 (gRPC) |
| `outpost-gateway` | WireGuard data plane — клиентский VPN и S2S-туннели | 51820/udp |
| `outpost-proxy` | Enrollment/auth прокси для DMZ | 8081 |
| `outpost-client` | Кроссплатформенный VPN-клиент с MFA и posture-отчётами | — |
| `outpostctl` | CLI-утилита управления и автоматизации | — |

**Масштабирование:** Core полностью stateless — любое количество экземпляров за LB. PostgreSQL — единственный источник истины. Redis Pub/Sub координирует события между core-инстансами (peer updates, firewall changes). Никакого etcd или внешнего consensus — PG advisory locks для singleton-задач (health monitor, cron). Подробнее — [docs/architecture.md](docs/architecture.md)

---

## Стек технологий

| Слой | Технология |
|---|---|
| **Backend** | Go 1.24+, Chi (HTTP-роутер), gRPC (межсервисная коммуникация), pgx/v5 + sqlc |
| **Frontend** | React 19, TypeScript, Vite, Tailwind CSS 4, Zustand, TanStack Query, Recharts |
| **БД** | PostgreSQL 17 с golang-migrate |
| **Кэш** | Redis 7 (сессии, pub/sub, rate limiting) |
| **VPN** | WireGuard (kernel через netlink + userspace через wireguard-go) |
| **Аутентификация** | Встроенный OIDC, SAML 2.0, LDAP/AD, SCIM 2.0, WebAuthn, TOTP |
| **Protobuf** | Buf-управляемые proto с gRPC кодогенерацией |
| **Observability** | Prometheus, slog, аудит-лог, SIEM-вебхуки |
| **Деплой** | Docker, docker-compose, Helm, GitHub Actions CI |

---

## Скриншоты

> Скриншоты скоро. UI использует тёмную кибепанк-тему с акцентным цветом `#00ff88` и шрифтом JetBrains Mono.
>
> Для превью: `docker compose -f deploy/docker/docker-compose.yml up -d` и откройте http://localhost:8080

---

## API

Outpost предоставляет полнофункциональный REST API на `/api/v1/` с JWT-аутентификацией. OpenAPI-спецификация доступна на `/api/docs/openapi.yaml`.

```
Auth:         POST /auth/login, /auth/mfa/verify, /auth/refresh, /auth/logout
Users:        GET/POST /users, GET/PUT/DELETE /users/{id}
Groups:       GET/POST /groups, members, ACLs
Networks:     GET/POST /networks, GET/PUT/DELETE /networks/{id}
Devices:      GET/POST /devices, /devices/enroll, approve, revoke, download config
Gateways:     GET/POST /gateways, GET/PUT/DELETE /gateways/{id}
S2S Tunnels:  GET/POST/DELETE /s2s-tunnels, members, routes, config
Smart Routes: GET/POST/PUT/DELETE /smart-routes, entries, proxy servers
ZTNA:         GET/PUT /ztna/trust-config, policies, DNS rules
Analytics:    GET /analytics/summary, bandwidth, top-users, heatmap
Compliance:   GET /compliance/report, soc2, iso27001, gdpr
Tenants:      GET/POST /tenants, GET/PUT/DELETE /tenants/{id}, stats
Audit:        GET /audit, /audit/stats, /audit/export
SCIM 2.0:     /scim/v2/Users, /scim/v2/Groups (bearer token auth)
```

> Полный [справочник API](docs/API.md) с описаниями запросов/ответов и примерами.

---

## Сравнение

| Фича | Outpost | defguard | NetBird | Tailscale | Firezone |
|---|:---:|:---:|:---:|:---:|:---:|
| **Полностью Open Source** | Да | Частично (enterprise paywall) | Да | Нет | Частично |
| **Zero Trust (ZTNA)** | Да | Нет | Частично | Да | Нет |
| **Site-to-Site Mesh** | Да | Да (multi-location) | Да | Да | Нет |
| **Встроенный OIDC-провайдер** | Да | Да | Нет | Нет | Нет |
| **SAML 2.0 + SCIM 2.0** | Да | Нет | Нет | Да (платно) | Нет |
| **Мультитенантность (MSP)** | Да | Нет | Нет | Нет | Нет |
| **Аналитика трафика** | Да | Нет | Нет | Нет | Нет |
| **Compliance-дашборд** | Да | Нет | Нет | Нет | Нет |
| **Smart Routing (прокси)** | Да | Нет | Нет | Нет | Нет |
| **Встроенный PKI / ротация ключей** | Да | Нет | Нет | Да | Нет |
| **Интерактивная карта сети** | Да | Нет | Нет | Нет | Нет |
| **Gateway HA (failover)** | Да | Нет | Нет | Да (DERP) | Нет |
| **Горизонтальное масштабирование** | Да (без etcd) | Нет | Нет | Да (платно) | Нет |
| **Self-Hosted** | Да | Да | Да | Ограниченно (Headscale) | Да |
| **MFA на уровне протокола** | Да | Да | Нет | Нет | Нет |
| **NAT Traversal** | В разработке | Нет | Да | Да (DERP) | Нет |
| **Лицензия** | Apache 2.0 | Apache 2.0 | BSD-3 | Проприетарная | Apache 2.0 |

---

## Структура проекта

```
outpost/
├── cmd/
│   ├── outpost-core/         # API + UI сервер
│   ├── outpost-gateway/      # WireGuard gateway agent
│   ├── outpost-proxy/        # DMZ enrollment прокси
│   ├── outpost-client/       # Кроссплатформенный VPN-клиент
│   └── outpostctl/           # CLI-утилита управления
├── internal/
│   ├── core/handler/         # REST API хендлеры (Chi)
│   ├── auth/                 # OIDC, SAML, LDAP, SCIM, MFA, RBAC
│   ├── gateway/              # Управление WG-интерфейсами, файрвол
│   ├── wireguard/            # WG-абстракция: kernel + userspace
│   ├── s2s/                  # Site-to-site mesh движок
│   ├── ztna/                 # Zero Trust верификация
│   ├── analytics/            # Аналитика трафика
│   ├── tenant/               # Мультитенантная изоляция
│   ├── compliance/           # SOC2 / ISO27001 / GDPR проверки
│   ├── pki/                  # Жизненный цикл сертификатов и ключей
│   ├── client/               # Клиентская VPN-библиотека
│   ├── observability/        # Prometheus, аудит, SIEM
│   ├── mail/                 # Email с i18n-шаблонами
│   └── db/                   # pgx pool, sqlc запросы
├── pkg/
│   ├── pb/                   # Сгенерированный protobuf Go-код
│   └── version/              # Инъекция версии сборки
├── web-ui/                   # React 19 фронтенд (embedded через go:embed)
├── proto/                    # Protobuf-определения (Buf)
├── migrations/               # PostgreSQL миграции (golang-migrate)
├── tests/e2e/                # API E2E тесты
├── docs/                     # Документация проекта
└── deploy/
    ├── docker/               # Dockerfiles + docker-compose.yml
    └── helm/                 # Helm-чарты для Kubernetes
```

---

## Разработка

### Требования

- Go 1.24+
- Node.js 22+ и pnpm
- Docker и Docker Compose
- PostgreSQL 17 (или через Docker Compose)

### Сборка из исходников

```bash
# Backend
go build ./...

# Frontend
cd web-ui && pnpm install && pnpm build

# Тесты
go test ./... -race -count=1

# E2E тесты (нужна PostgreSQL)
TEST_DATABASE_URL="postgres://outpost:outpost@localhost:5432/outpost_test?sslmode=disable" \
  go test -v ./tests/e2e/

# Полный стек через Docker
docker compose -f deploy/docker/docker-compose.yml up -d --build
```

> Смотрите [CONTRIBUTING.md](CONTRIBUTING.md) — полное руководство по настройке окружения разработки.

---

## Документация

| Документ | Описание |
|---|---|
| [Быстрый старт](docs/getting-started.md) | Установка и пошаговое руководство |
| [Архитектура](docs/architecture.md) | Дизайн системы и взаимодействие компонентов |
| [Справочник API](docs/API.md) | Полная документация REST API |
| [Развёртывание](docs/deployment.md) | Руководство по продакшен-деплою (Docker, Helm, bare metal) |
| [Конфигурация](docs/configuration.md) | Переменные окружения и настройки |
| [Возможности](docs/features.md) | Подробная документация по фичам |
| [Mesh-сети](docs/mesh-networking.md) | Руководство по Site-to-Site и mesh-топологиям |

---

## Участие

Мы приветствуем любой вклад — баг-репорты, запросы фич, улучшения документации и код.

Пожалуйста, прочитайте [руководство по участию](CONTRIBUTING.md) перед отправкой pull request.

---

## Безопасность

Если вы обнаружили уязвимость, пожалуйста, сообщите ответственно. Подробности в нашей [Политике безопасности](SECURITY.md).

---

## Лицензия

[Apache License 2.0](LICENSE)

Все фичи полностью open-source — сейчас и всегда. Без enterprise-модулей. Без пейволлов. Без подвоха.

Монетизация через Outpost Cloud (управляемый SaaS), контракты поддержки и профессиональные услуги — не через ограничение open-source кода.

---

<p align="center">
  <sub>Сделано в <a href="https://github.com/romashqua">Romashqua Labs</a></sub>
</p>
