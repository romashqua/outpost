<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=black" />
  <img src="https://img.shields.io/badge/WireGuard-88171A?style=flat-square&logo=wireguard&logoColor=white" />
  <img src="https://img.shields.io/badge/PostgreSQL-17-4169E1?style=flat-square&logo=postgresql&logoColor=white" />
  <img src="https://img.shields.io/badge/License-Apache%202.0-green?style=flat-square" />
</p>

<h1 align="center"><code>&gt;_</code> OUTPOST VPN</h1>

<p align="center">
  <strong>Open-Source WireGuard VPN & Zero Trust Network Access</strong><br/>
  Корпоративная безопасность без корпоративной цены.
</p>

<p align="center">
  <a href="README.md">English</a> &middot;
  <a href="#быстрый-старт">Быстрый старт</a> &middot;
  <a href="#возможности">Возможности</a> &middot;
  <a href="#архитектура">Архитектура</a>
</p>

---

## Почему Outpost?

Все остальные VPN-решения либо прячут критичные фичи за enterprise-пейволлом, либо вообще их не имеют. Outpost даёт **всё** — ZTNA проверки устройств, compliance-дашборды, мультитенантность для MSP, аналитику трафика, встроенный OIDC, PKI — полностью open-source с первого дня.

**Для кого:**
- Компании, которым нужен корпоративный VPN без vendor lock-in
- MSP/MSSP, предоставляющие VPN как сервис
- DevOps-команды, строящие Zero Trust сети
- Те, кто устал платить за enterprise-фичи

## Возможности

### VPN и сети
- **WireGuard-туннели** — kernel & userspace, автоматическое управление ключами
- **Site-to-Site** — full mesh & hub-spoke с BGP-lite обменом маршрутами
- **NAT Traversal** — STUN/TURN relay для ограниченных сетей
- **Smart Routes** — селективная маршрутизация через прокси (SOCKS5, HTTP, Shadowsocks)
- **Real-time синхронизация** — gRPC bidirectional streaming между core и gateway

### Zero Trust (ZTNA)
- **Проверки устройств** — шифрование диска, антивирус, файрвол, версия ОС, блокировка экрана
- **Непрерывная верификация** — периодическая пересчёт trust score
- **Настраиваемые веса и пороги** — настройте что важно для вашей модели безопасности
- **MFA на уровне протокола** — пир удаляется из gateway при истечении MFA-сессии

### Идентификация и доступ
- **Встроенный OIDC-провайдер** — "Войти через Outpost" (Authorization Code + PKCE)
- **MFA/2FA** — TOTP, WebAuthn/FIDO2, email-токены, backup-коды
- **Внешнее SSO** — Google, Azure AD, Okta, Keycloak
- **LDAP/Active Directory** синхронизация
- **SAML 2.0** Service Provider
- **SCIM 2.0** автопровижинг (Okta, Azure AD)
- **RBAC** с гранулярными ACL по сетям

### Аналитика и Compliance
- **Аналитика трафика** — графики bandwidth, top-пользователи, heatmap подключений
- **Compliance-дашборд** — автоматическая оценка SOC2, ISO 27001, GDPR
- **Аудит-лог** — каждое действие записано с полным контекстом
- **SIEM-интеграция** — webhook + syslog с HMAC-подписью

### Платформа
- **Мультитенантность** — MSP-режим с изоляцией организаций
- **Встроенный PKI** — автоматическая ротация ключей без простоя
- **Интерактивная карта сети** — SVG-визуализация топологии
- **Email-уведомления** — SMTP для MFA, enrollment, алертов
- **Prometheus-метрики** — полная observability из коробки
- **Docker & Kubernetes** — compose + Helm ready

## Быстрый старт

```bash
git clone https://github.com/romashqua/outpost.git
cd outpost
docker compose -f deploy/docker/docker-compose.yml up -d
```

Откройте **http://localhost:8080** — логин: `admin` / `admin`

Миграции запускаются автоматически, все сервисы стартуют в правильном порядке.

## Архитектура

```
                    ┌─────────────────────┐
                    │   outpost-core      │
                    │   :8080 HTTP/API    │
                    │   :50051 gRPC       │
                    │   React UI (embed)  │
                    └────────┬────────────┘
                             │ gRPC streaming
              ┌──────────────┼──────────────┐
              │              │              │
    ┌─────────▼──┐  ┌───────▼────┐  ┌──────▼──────┐
    │  gateway   │  │  gateway   │  │   proxy     │
    │  :51820/udp│  │  :51821/udp│  │   :8081     │
    │  WireGuard │  │  WireGuard │  │   DMZ-safe  │
    └────────────┘  └────────────┘  └─────────────┘
```

| Компонент | Роль | Порты |
|---|---|---|
| `outpost-core` | API, OIDC-провайдер, админ-панель, gRPC control plane | 8080, 50051 |
| `outpost-gateway` | WireGuard data plane (клиентский VPN + S2S) | 51820/udp |
| `outpost-proxy` | Enrollment/auth прокси для DMZ | 8081 |
| `outpost-client` | VPN-клиент с MFA и posture-отчётами | — |
| `outpostctl` | CLI-утилита управления | — |

## Стек

| Слой | Технология |
|---|---|
| Backend | Go 1.25, Chi, gRPC, pgx + sqlc |
| Frontend | React 19, TypeScript, Vite, Tailwind CSS 4, Zustand, TanStack Query |
| БД | PostgreSQL 17 |
| Кэш | Redis 7 |
| VPN | WireGuard (kernel + userspace) |
| Auth | OIDC, SAML 2.0, LDAP, SCIM 2.0, WebAuthn, TOTP |
| Observability | Prometheus, slog, audit log, SIEM |
| Deploy | Docker, docker-compose, Helm |

## Сравнение

| | Outpost | defguard | NetBird | Tailscale |
|---|:---:|:---:|:---:|:---:|
| Полностью open-source | :white_check_mark: | :x: Enterprise paywall | :white_check_mark: | :x: |
| Zero Trust (ZTNA) | :white_check_mark: | :x: | Частично | :white_check_mark: |
| Site-to-Site mesh | :white_check_mark: | :x: | :white_check_mark: | :white_check_mark: |
| Встроенный OIDC | :white_check_mark: | :white_check_mark: | :x: | :x: |
| Мультитенантность | :white_check_mark: | :x: | :x: | :x: |
| Аналитика трафика | :white_check_mark: | :x: | :x: | :x: |
| Compliance-дашборд | :white_check_mark: | :x: | :x: | :x: |
| Smart routing | :white_check_mark: | :x: | :x: | :x: |
| Self-hosted | :white_check_mark: | :white_check_mark: | :white_check_mark: | :x: |

## Разработка

```bash
# Сборка
go build ./...

# Тесты
go test ./... -race -count=1

# E2E тесты (нужна PostgreSQL)
TEST_DATABASE_URL="postgres://..." go test -v ./tests/e2e/

# Фронтенд
cd web-ui && pnpm install && pnpm dev

# Полный стек
docker compose -f deploy/docker/docker-compose.yml up -d --build
```

## Структура проекта

```
outpost/
├── cmd/
│   ├── outpost-core/         # API + UI сервер
│   ├── outpost-gateway/      # WireGuard gateway agent
│   ├── outpost-proxy/        # DMZ enrollment proxy
│   ├── outpost-client/       # VPN-клиент с MFA
│   └── outpostctl/           # CLI-утилита
├── internal/
│   ├── core/handler/         # REST API хендлеры
│   ├── auth/                 # OIDC, SAML, LDAP, SCIM, MFA, RBAC
│   ├── gateway/              # Управление WG-интерфейсами
│   ├── wireguard/            # Kernel + userspace абстракция
│   ├── s2s/                  # Site-to-site mesh движок
│   ├── ztna/                 # Zero Trust проверки устройств
│   ├── analytics/            # Аналитика трафика
│   ├── tenant/               # Мультитенантность
│   ├── compliance/           # SOC2/ISO27001/GDPR проверки
│   ├── pki/                  # Жизненный цикл сертификатов
│   └── db/                   # pgx pool, sqlc запросы
├── web-ui/                   # React фронтенд (embedded via go:embed)
├── migrations/               # SQL миграции
├── proto/                    # Protobuf (Buf)
├── tests/e2e/                # E2E тесты
└── deploy/
    ├── docker/               # Dockerfiles + docker-compose.yml
    └── helm/                 # Helm charts
```

## Лицензия

[Apache License 2.0](LICENSE) — используйте как хотите, в том числе коммерчески.

Все фичи полностью open-source. Без enterprise-модулей. Без пейволлов.

---

<p align="center">
  Сделано в <a href="https://github.com/romashqua">Romashqua Labs</a>
</p>
