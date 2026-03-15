<p align="center">
  <a href="README.md">Русский</a> | <a href="README.en.md">English</a>
</p>

<p align="center">
  <img src="promo/logo-placeholder.png" alt="Outpost VPN" width="200" />
</p>

<h1 align="center"><code>&gt;_</code> OUTPOST VPN</h1>

<p align="center">
  <strong>Открытая платформа WireGuard VPN и Zero Trust Network Access</strong><br/>
  Безопасность корпоративного уровня. Без платных ограничений. Apache 2.0 навсегда.
</p>

<p align="center">
  <a href="https://github.com/romashqua/outpost/actions"><img src="https://img.shields.io/github/actions/workflow/status/romashqua/outpost/ci.yml?branch=main&style=flat-square&label=CI" alt="CI" /></a>
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=black" />
  <img src="https://img.shields.io/badge/WireGuard-88171A?style=flat-square&logo=wireguard&logoColor=white" />
  <img src="https://img.shields.io/badge/PostgreSQL-17-4169E1?style=flat-square&logo=postgresql&logoColor=white" />
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-green?style=flat-square" alt="Лицензия" /></a>
  <a href="https://github.com/romashqua/outpost/stargazers"><img src="https://img.shields.io/github/stars/romashqua/outpost?style=flat-square" alt="Звёзды" /></a>
  <a href="https://github.com/romashqua/outpost/releases"><img src="https://img.shields.io/github/v/release/romashqua/outpost?style=flat-square&label=Release" alt="Релиз" /></a>
</p>

<p align="center">
  <a href="#быстрый-старт">Быстрый старт</a> &middot;
  <a href="#возможности">Возможности</a> &middot;
  <a href="#архитектура">Архитектура</a> &middot;
  <a href="#скриншоты">Скриншоты</a> &middot;
  <a href="#сравнение">Сравнение</a> &middot;
  <a href="docs/">Документация</a> &middot;
  <a href="CONTRIBUTING.md">Участие в проекте</a>
</p>

---

## Почему Outpost?

Каждое другое open-source VPN-решение либо прячет критичные функции за платным enterprise-тарифом, либо попросту их не имеет. Outpost даёт вам **всё** -- Zero Trust проверку устройств, панель соответствия стандартам, мультитенантный режим для MSP, аналитику трафика, встроенный OIDC/SAML/SCIM, PKI с ротацией ключей -- полностью в открытом доступе под лицензией Apache 2.0 с первого дня.

**Нет enterprise-тарифа. Нет ограничения функций. Никогда.**

### Для кого создан

- **Команды**, которым нужен корпоративный VPN без привязки к вендору
- **MSP/MSSP**, предоставляющие VPN-as-a-service множеству клиентов
- **DevOps/Platform-инженеры**, строящие Zero Trust инфраструктуру
- **Регулируемые отрасли**, которым нужны аудиторские журналы и автоматические проверки соответствия
- **Все**, кому надоело платить за функции, которые должны быть стандартом

---

## Быстрый старт

Получите полностью рабочий Outpost менее чем за 60 секунд:

```bash
git clone https://github.com/romashqua/outpost.git
cd outpost
docker compose -f deploy/docker/docker-compose.yml up -d
```

Откройте **http://localhost:8080** и войдите с логином `admin` / паролем `admin`.

Вот и всё. Миграции базы данных выполняются автоматически, все сервисы запускаются в правильном порядке, React UI встроен в бинарный файл.

> **Продуктивный деплой?** Смотрите [Руководство по развёртыванию](docs/deployment.md) для Helm-чартов, настройки TLS и рекомендаций по усилению безопасности.

---

## Возможности

### VPN и сеть

| Возможность | Описание |
|---|---|
| **WireGuard-туннели** | Поддержка ядра и userspace с автоматическим управлением ключами |
| **Site-to-Site** | Полная mesh- и hub-spoke-топологии с BGP-lite обменом маршрутами |
| **NAT Traversal** | Встроенные STUN/TURN relay-серверы для ограниченных сетей |
| **Smart Routes** | Селективная маршрутизация доменов/CIDR через прокси-серверы (SOCKS5, HTTP, Shadowsocks, VLESS) |
| **Синхронизация в реальном времени** | gRPC двунаправленный стриминг между core и шлюзами |

### Zero Trust Network Access (ZTNA)

| Возможность | Описание |
|---|---|
| **Проверка состояния устройств** | Шифрование диска, антивирус, файрвол, версия ОС, блокировка экрана |
| **Непрерывная верификация** | Периодическая переоценка уровня доверия устройства |
| **Настраиваемые политики** | Настройка весов и порогов для вашей модели безопасности |
| **MFA на уровне протокола** | WireGuard-пир удаляется со шлюза при истечении сессии |
| **DNS-правила доступа** | DNS-фильтрация по устройствам и split-horizon DNS |

### Управление идентификацией и доступом

| Возможность | Описание |
|---|---|
| **Встроенный OIDC-провайдер** | «Войти через Outpost» (Authorization Code + PKCE, RS256) |
| **MFA/2FA** | TOTP, WebAuthn/FIDO2, email OTP, резервные коды |
| **Внешний SSO** | Google, Azure AD, Okta, Keycloak через OIDC |
| **LDAP/Active Directory** | Полная синхронизация каталога с маппингом групп |
| **SAML 2.0** | Режим Service Provider для корпоративных IdP |
| **SCIM 2.0** | Автоматический провижининг пользователей/групп из Okta, Azure AD |
| **RBAC** | Ролевое управление доступом с гранулярными сетевыми ACL |

### Аналитика и соответствие стандартам

| Возможность | Описание |
|---|---|
| **Аналитика трафика** | Графики пропускной способности в реальном времени, топ пользователей, тепловые карты соединений |
| **Панель соответствия** | Автоматическая оценка готовности к SOC 2, ISO 27001, GDPR |
| **Аудиторский журнал** | Каждое действие администратора записывается с полным контекстом и экспортом |
| **Интеграция с SIEM** | Экспорт через вебхуки и syslog с проверкой HMAC-подписи |

### Платформа

| Возможность | Описание |
|---|---|
| **Мультитенантность** | MSP/реселлер-режим с полной изоляцией организаций |
| **Встроенная PKI** | Автоматическая ротация ключей WireGuard без простоев |
| **Визуальная карта сети** | Интерактивная SVG-топология всей VPN-сети |
| **Email-уведомления** | Настраиваемый SMTP для MFA, регистрации устройств, оповещений (i18n) |
| **Метрики Prometheus** | Полная наблюдаемость с готовыми дашбордами |
| **Docker и Kubernetes** | docker-compose для разработки, Helm-чарты для продакшена |

---

## Архитектура

```
                         Интернет
                            |
                   ┌────────┴────────┐
                   │  outpost-proxy  │  DMZ-безопасная регистрация
                   │     :8081       │  и auth-прокси
                   └────────┬────────┘
                            |
                   ┌────────┴────────┐
                   │  outpost-core   │  API-сервер, OIDC-провайдер,
                   │  :8080  HTTP    │  панель администратора, gRPC-хаб
                   │  :50051 gRPC   │  React UI (go:embed)
                   └───┬────────┬───┘
                       │        │
            gRPC-стриминг   gRPC-стриминг
                       │        │
              ┌────────┴──┐  ┌──┴────────┐
              │  шлюз     │  │  шлюз     │     N шлюзов
              │ :51820/udp│  │ :51821/udp│     по площадкам
              │ WireGuard │  │ WireGuard │
              └───────────┘  └───────────┘
                    |              |
              ┌─────┴─────┐  ┌────┴──────┐
              │  Клиенты  │  │  Клиенты  │
              └───────────┘  └───────────┘
```

| Компонент | Роль | Порты по умолчанию |
|---|---|---|
| `outpost-core` | API, OIDC-провайдер, панель администратора, gRPC control plane | 8080 (HTTP), 50051 (gRPC) |
| `outpost-gateway` | WireGuard data plane -- клиентский VPN и S2S-туннели | 51820/udp |
| `outpost-proxy` | Публичный прокси для регистрации/авторизации, безопасный для DMZ | 8081 |
| `outpost-client` | Кроссплатформенный VPN-клиент с MFA и отчётами о состоянии устройства | -- |
| `outpostctl` | CLI-инструмент для управления и автоматизации | -- |

> Подробный обзор архитектуры смотрите в [docs/architecture.md](docs/architecture.md).

---

## Технологический стек

| Уровень | Технология |
|---|---|
| **Бэкенд** | Go 1.24+, Chi (HTTP-роутер), gRPC (межсервисное взаимодействие), pgx/v5 + sqlc |
| **Фронтенд** | React 19, TypeScript, Vite, Tailwind CSS 4, Zustand, TanStack Query, Recharts |
| **База данных** | PostgreSQL 17 с golang-migrate |
| **Кэш** | Redis 7 (сессии, pub/sub, rate limiting) |
| **VPN** | WireGuard (ядро через netlink + userspace через wireguard-go) |
| **Аутентификация** | Встроенный OIDC, SAML 2.0, LDAP/AD, SCIM 2.0, WebAuthn, TOTP |
| **Protobuf** | Buf-управляемые proto-определения с gRPC-кодогенерацией |
| **Наблюдаемость** | Prometheus, структурированное логирование (slog), аудиторский журнал, SIEM-вебхуки |
| **Деплой** | Docker, docker-compose, Helm, GitHub Actions CI |

---

## Скриншоты

<!-- Добавьте скриншоты интерфейса Outpost сюда -->

| Дашборд | Карта сети | Аналитика |
|---|---|---|
| ![Дашборд](promo/screenshots/dashboard.png) | ![Карта сети](promo/screenshots/network-map.png) | ![Аналитика](promo/screenshots/analytics.png) |

| Устройства | Соответствие | Smart Routes |
|---|---|---|
| ![Устройства](promo/screenshots/devices.png) | ![Соответствие](promo/screenshots/compliance.png) | ![Smart Routes](promo/screenshots/smart-routes.png) |

> Скриншоты скоро появятся. Интерфейс использует тёмную кибер-тему с акцентным цветом `#00ff88` и шрифтом JetBrains Mono.

---

## API

Outpost предоставляет полноценный REST API по адресу `/api/v1/` с JWT-аутентификацией. Полная спецификация OpenAPI доступна по адресу `/api/docs/openapi.yaml`.

```
Аутентификация: POST /auth/login, /auth/mfa/verify, /auth/refresh, /auth/logout
Пользователи:   GET/POST /users, GET/PUT/DELETE /users/{id}
Группы:          GET/POST /groups, участники, ACL
Сети:            GET/POST /networks, GET/PUT/DELETE /networks/{id}
Устройства:      GET/POST /devices, /devices/enroll, одобрение, отзыв, загрузка конфига
Шлюзы:           GET/POST /gateways, GET/PUT/DELETE /gateways/{id}
S2S-туннели:     GET/POST/DELETE /s2s-tunnels, участники, маршруты, конфиг
Smart Routes:    GET/POST/PUT/DELETE /smart-routes, записи, прокси-серверы
ZTNA:            GET/PUT /ztna/trust-config, политики, DNS-правила
Аналитика:       GET /analytics/summary, bandwidth, top-users, heatmap
Соответствие:    GET /compliance/report, soc2, iso27001, gdpr
Тенанты:         GET/POST /tenants, GET/PUT/DELETE /tenants/{id}, статистика
Аудит:           GET /audit, /audit/stats, /audit/export
SCIM 2.0:        /scim/v2/Users, /scim/v2/Groups (bearer token auth)
```

> Полный [справочник по API](docs/API.md) с описанием запросов/ответов и примерами.

---

## Сравнение

| Возможность | Outpost | defguard | NetBird | Tailscale | Firezone |
|---|:---:|:---:|:---:|:---:|:---:|
| **Полностью открытый код** | Да | Частично (enterprise paywall) | Да | Нет | Частично |
| **Zero Trust (ZTNA)** | Да | Нет | Частично | Да | Нет |
| **Site-to-Site mesh** | Да | Нет | Да | Да | Нет |
| **Встроенный OIDC-провайдер** | Да | Да | Нет | Нет | Нет |
| **SAML 2.0 + SCIM 2.0** | Да | Нет | Нет | Да (платно) | Да (платно) |
| **Мультитенантность (MSP-режим)** | Да | Нет | Нет | Нет | Нет |
| **Аналитика трафика** | Да | Нет | Нет | Нет | Нет |
| **Панель соответствия** | Да | Нет | Нет | Нет | Нет |
| **Умная маршрутизация (прокси)** | Да | Нет | Нет | Нет | Нет |
| **Встроенная PKI / ротация ключей** | Да | Нет | Нет | Да | Нет |
| **Визуальная карта сети** | Да | Нет | Нет | Нет | Нет |
| **Self-hosted** | Да | Да | Да | Нет | Да |
| **MFA на уровне протокола** | Да | Нет | Нет | Нет | Нет |
| **Лицензия** | Apache 2.0 | Apache 2.0 | BSD-3 | Проприетарная | Apache 2.0 |

---

## Структура проекта

```
outpost/
├── cmd/
│   ├── outpost-core/         # API + UI сервер
│   ├── outpost-gateway/      # WireGuard-шлюз (агент)
│   ├── outpost-proxy/        # DMZ прокси для регистрации
│   ├── outpost-client/       # Кроссплатформенный VPN-клиент
│   └── outpostctl/           # CLI-инструмент управления
├── internal/
│   ├── core/handler/         # REST API обработчики (Chi)
│   ├── auth/                 # OIDC, SAML, LDAP, SCIM, MFA, RBAC
│   ├── gateway/              # Управление WG-интерфейсами, файрвол
│   ├── wireguard/            # Абстракция WG: ядро + userspace
│   ├── s2s/                  # Движок site-to-site mesh
│   ├── ztna/                 # Движок Zero Trust проверок
│   ├── analytics/            # Аналитика потоков трафика
│   ├── tenant/               # Мультитенантная изоляция
│   ├── compliance/           # Проверки SOC2 / ISO27001 / GDPR
│   ├── pki/                  # Жизненный цикл сертификатов и ключей
│   ├── client/               # Клиентская VPN-библиотека
│   ├── observability/        # Prometheus, аудит, SIEM
│   ├── mail/                 # Email с i18n-шаблонами
│   └── db/                   # pgx-пул, sqlc-запросы
├── pkg/
│   ├── pb/                   # Сгенерированный protobuf Go-код
│   └── version/              # Инъекция версии при сборке
├── web-ui/                   # React 19 фронтенд (встроен через go:embed)
├── proto/                    # Protobuf-определения (Buf)
├── migrations/               # Миграции PostgreSQL (golang-migrate)
├── tests/e2e/                # E2E-тесты API
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
- PostgreSQL 17 (или используйте Docker Compose)

### Сборка из исходников

```bash
# Бэкенд
go build ./...

# Фронтенд
cd web-ui && pnpm install && pnpm build

# Запуск тестов
go test ./... -race -count=1

# E2E-тесты (требуется PostgreSQL)
TEST_DATABASE_URL="postgres://outpost:outpost@localhost:5432/outpost_test?sslmode=disable" \
  go test -v ./tests/e2e/

# Полный стек через Docker
docker compose -f deploy/docker/docker-compose.yml up -d --build
```

> Смотрите [CONTRIBUTING.md](CONTRIBUTING.md) для полного руководства по настройке окружения разработки.

---

## Документация

| Документ | Описание |
|---|---|
| [Быстрый старт](docs/getting-started.md) | Первоначальная настройка и пошаговое руководство |
| [Архитектура](docs/architecture.md) | Проектирование системы и взаимодействие компонентов |
| [Справочник по API](docs/API.md) | Полная документация REST API |
| [Развёртывание](docs/deployment.md) | Руководство по продуктивному развёртыванию (Docker, Helm, bare metal) |
| [Конфигурация](docs/configuration.md) | Переменные окружения и справочник по настройкам |
| [Возможности](docs/features.md) | Подробная документация по функциям |
| [Mesh-сети](docs/mesh-networking.md) | Руководство по site-to-site и mesh-топологии |

---

## Участие в проекте

Мы приветствуем любые вклады -- баг-репорты, запросы функций, улучшения документации и код.

Пожалуйста, прочитайте наше [Руководство по участию](CONTRIBUTING.md) перед отправкой pull request.

---

## Безопасность

Если вы обнаружили уязвимость, пожалуйста, сообщите о ней ответственно. Подробности в нашей [Политике безопасности](SECURITY.md).

---

## Лицензия

[Apache License 2.0](LICENSE)

Все функции являются и всегда будут полностью открытыми. Без enterprise-тарифа. Без ограничения функций. Без подвоха.

Монетизация осуществляется через [Outpost Cloud](https://outpost.dev) (управляемый SaaS), контракты на поддержку и профессиональные услуги -- а не путём ограничения открытого кода.

---

<p align="center">
  <sub>Разработано <a href="https://github.com/romashqua">Romashqua Labs</a></sub>
</p>
