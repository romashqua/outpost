# Журнал изменений

Все значимые изменения Outpost VPN документируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
проект следует [Семантическому версионированию](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Добавлено
- **Gateway HA** — multi-gateway failover: enrollment возвращает список gateway'ов, клиент автоматически переключается на backup при падении primary (мониторинг handshake, 45-секундный timeout)
- **Gateway health monitoring** — core отслеживает heartbeat'ы, помечает gateway'ы как unhealthy после 3 пропущенных (~4.5 мин), пушит resync здоровым gateway'ам
- **Smart Routes enforcement** — CIDR block/direct правила применяются через iptables цепочку OUTPOST-SMART на gateway'ах, live-push при CRUD
- **ZTNA enforcement** — trust scores влияют на firewall: auto_block_below_low и auto_restrict_below_medium
- **Conntrack flush** — немедленное применение ACL правил без переподключения клиента (flush conntrack для DROP-правил)
- **Имя владельца устройства** — DevicesPage показывает username вместо UUID
- **Единый стиль чекбоксов** — reusable CheckboxItem компонент, одинаковый стиль во всех страницах
- **Health status badge** — GatewaysPage показывает статус здоровья каждого шлюза
- **PUT /devices/{id}** — эндпоинт обновления устройства (имя), IDOR-protected
- **PUT /s2s-tunnels/{id}** — эндпоинт обновления S2S-туннеля (имя, описание)
- **S2S route recalculation** — автоматический пересчёт S2S маршрутов и firewall при изменении сетей шлюза
- **Token refresh** — проактивный refresh JWT за 5 мин до истечения + retry при 401
- **Auth guard loading state** — устранён flash of protected content при загрузке
- **MFA token persistence** — mfaToken сохраняется в sessionStorage, не теряется при обновлении страницы
- **Edit модалки** — добавлены на страницы Devices, Networks, Groups, S2S, Smart Routes
- **DB interface** — все хендлеры принимают интерфейс DB вместо конкретного *pgxpool.Pool (тестируемость)
- **Redis session store** — сессии хранятся в Redis для горизонтального масштабирования (fallback на in-memory если Redis недоступен)
- **Handler unit tests** — pgxmock-based тесты для всех HTTP хендлеров
- **OpenAPI annotations** — swaggo аннотации для всех HTTP-эндпоинтов (174 endpoints: sessions, tenants, compliance, analytics, MFA, NAT, SCIM, OIDC, SAML, webhooks, notifications, audit)
- **i18n** — добавлены недостающие ключи перевода (common.saving, gateways.searchNetworks, gateways.status, settings.webhooks)

- **Фильтрация событий** — audit middleware логирует только `/api/` запросы (бот-трафик `/sdk`, `/GponForm/diag_Form` больше не засоряет аудит)
- **Фильтр уведомлений** — NotificationHandler фильтрует по `notificationActionPatterns` (login, CRUD устройств/пользователей/сетей/шлюзов, семантические события)
- **Человекочитаемые события** — Dashboard и NotificationDropdown показывают «Device approved», «Users created» вместо сырых «POST /api/v1/devices/.../approve»
- **CheckboxItem suffix** — компонент поддерживает дополнительный контент справа (адрес сети, метаданные)
- **peer_stats auto-partitioning** — PL/pgSQL функции `create_peer_stats_partitions()` и `drop_old_peer_stats_partitions()` для автоматического управления месячными партициями; core вызывает при старте и ежедневно
- **E2E workflow tests** — новый файл `workflow_test.go`: ZTNA policies, webhooks CRUD, tenants CRUD, notifications/audit, MFA status, sessions, auth flow (login → change password → re-login), RBAC (non-admin access denied)
- **Load test script** — `scripts/loadtest.sh` для нагрузочного тестирования API через `hey` (10 эндпоинтов, configurable concurrency/requests)
- **Документация Multi-Core Scaling** — описание горизонтального масштабирования core: Redis Pub/Sub для cross-core событий, `pg_advisory_lock` для singleton-задач, без etcd
- **Документация TLS gRPC** — описание настройки mTLS между core и gateway'ами, переменные окружения
- **Документация Gateway Clusters** — обновлены диаграммы архитектуры в README (RU/EN) и docs с multi-core + multi-gateway топологией

### Исправлено
- ACL firewall bypass: устройства без ACL записей получают default-deny правило
- Удаление сети обновляет firewall на связанных gateway'ах
- ZTNA trust scores = 0 при первом подключении (добавлен immediate posture report)
- Firewall правила не применялись на лету (conntrack ESTABLISHED,RELATED блокировал обновления)
- **handlePeerStats scoping** — статистика пиров ограничена сетями отчитывающегося шлюза (fix IDOR)
- **Heartbeat UUID cast** — `WHERE id = $1::uuid` вместо `WHERE id::text = $1` (использование индекса)
- **S2S hub_spoke validation** — `hub_gateway_id` обязателен для hub_spoke топологии
- **DNS server validation** — проверка формата IP для DNS серверов при создании/обновлении сети
- **S2S dead code** — удалены нефункциональные domain-запросы из S2SPage
- **DashboardPage stale dates** — useMemo с корректными зависимостями
- **bcrypt cost** — увеличен с 10 до 12 (OWASP рекомендация)
- **test_setup.sh credentials** — пароли вынесены из скрипта в env vars (OUTPOST_ADMIN_PASS, OUTPOST_NEW_PASS)
- **Gateway failover** — реализован wgctrl-based live peer endpoint update (macOS IPC + Linux kernel), getLastHandshake через wgctrl
- **Audit scan error** — `ip_address` (INET) и `user_agent` (TEXT) nullable-колонки вызывали «failed to scan audit log row» при NULL значениях (fix: `COALESCE(host(ip_address), '')`)
- **Dashboard audit widget** — неправильные параметры запроса (`limit` → `per_page`) и несовпадающие поля интерфейса (`created_at` → `timestamp`, отсутствующий `username`)
- **GatewaysPage/ZTNAPage checkbox** — заменены inline/native чекбоксы на единый CheckboxItem компонент
- **Tenants page 500** — listNetworks() не мог сканировать NULL DNS array (TEXT[]), listGateways() не возвращал health_status/is_online; frontend ожидал массивы но получал обёрнутые объекты, несовпадение полей (`cidr` vs `address`, `is_online` vs `is_active`)

### Миграции
- `000026_add_gateway_health` — добавлены `health_status` и `consecutive_failures` в таблицу gateways
- `000027_peer_stats_partitions` — PL/pgSQL функции для автоматического создания/удаления месячных партиций peer_stats

## [0.1.0] - 2026-03-15

### Добавлено

#### Базовая платформа
- Go-бэкенд с HTTP-роутером Chi и gRPC межсервисным взаимодействием
- Фронтенд на React 19 + TypeScript + Vite, встроенный через `go:embed`
- База данных PostgreSQL 17 с типобезопасными запросами sqlc и миграциями golang-migrate
- Redis 7 для сессий, pub/sub и rate limiting
- Развёртывание через Docker Compose с автоматическим запуском миграций
- Helm-чарты для развёртывания в Kubernetes
- Эндпоинт метрик Prometheus для наблюдаемости
- Структурированное логирование через `slog` с аудиторским журналом

#### VPN и сеть
- Управление WireGuard-туннелями (ядро через netlink + userspace через wireguard-go)
- Site-to-site туннели с mesh- и hub-spoke-топологиями
- Автоматическая синхронизация маршрутов для S2S-сетей через gRPC
- NAT traversal со STUN/TURN relay-серверами
- Smart Routes: селективная маршрутизация доменов/CIDR через прокси-серверы (SOCKS5, HTTP, Shadowsocks, VLESS)
- gRPC двунаправленный стриминг между core и шлюзами (StreamHub, PeerNotifier)
- Генерация конфигурации WireGuard и отправка по email
- Мониторинг здоровья шлюзов

#### Управление идентификацией и доступом
- Встроенный OIDC-провайдер (Authorization Code + PKCE, RS256)
- JWT-аутентификация с чёрным списком токенов при выходе
- Многофакторная аутентификация: TOTP, WebAuthn/FIDO2, email OTP, резервные коды
- MFA на уровне протокола: WireGuard-пир удаляется со шлюза при истечении сессии
- Внешний SSO через OIDC (Google, Azure AD, Okta, Keycloak)
- Синхронизация с LDAP/Active Directory
- SAML 2.0 Service Provider
- Автоматический провижининг пользователей/групп через SCIM 2.0
- RBAC с гранулярными сетевыми ACL и политиками на основе групп
- Политика паролей (длина, сложность)
- Блокировка аккаунта после неудачных попыток входа

#### Zero Trust Network Access (ZTNA)
- Проверки состояния устройств: шифрование диска, антивирус, файрвол, версия ОС, блокировка экрана
- Непрерывная оценка уровня доверия устройства
- Настраиваемые веса доверия и пороги
- Движок ZTNA-политик с DNS-правилами доступа

#### Аналитика и соответствие стандартам
- Аналитика трафика: сводка по пропускной способности, топ пользователей, тепловая карта соединений
- Панель соответствия: автоматическая оценка готовности к SOC 2, ISO 27001, GDPR
- Аудиторский журнал с полным контекстом, статистикой и экспортом в CSV/JSON
- Интеграция с SIEM через вебхуки с проверкой HMAC-подписи

#### Мультитенантность
- MSP/реселлер-режим с полной изоляцией организаций
- Управление пользователями, устройствами, сетями и шлюзами по тенантам
- Статистика и отслеживание потребления ресурсов по тенантам

#### Безопасность
- Встроенная PKI с автоматической ротацией ключей WireGuard
- Валидация публичных ключей WireGuard (44-символьный base64)
- Выделение IP-адресов с атомарным CTE для предотвращения гонок
- Защита от IDOR на всех эндпоинтах устройств и пользователей
- Принудительная аутентификация gRPC
- Rate limiting с предотвращением утечки памяти
- Редактирование паролей прокси-серверов в API-ответах
- Обновление токенов перечитывает статус администратора из базы данных
- Исправление TOCTOU для токенов сброса пароля (атомарный UPDATE...RETURNING)

#### Фронтенд
- Тёмная кибер-тема UI (#0a0a0f фон, #00ff88 акцент, JetBrains Mono)
- Страницы: Вход, Дашборд, Пользователи, Сети, Устройства, Шлюзы, S2S-туннели, Smart Routes, Аналитика, Соответствие, Настройки
- Интерактивная SVG-карта сети с визуализацией топологии в реальном времени
- Настройки: Общие, Аутентификация/SSO, Безопасность/MFA, WireGuard, SMTP, Интеграции
- Поддержка i18n для английского и русского (react-i18next)
- Система уведомлений (ошибки 10 сек, успех 5 сек)

#### DevOps
- CI-пайплайн в GitHub Actions
- Docker multi-stage сборка (Node 22 + Go 1.24 + Alpine)
- E2E-тесты API (23+ тестовых функций)
- OpenAPI/Swagger документация через swaggo

[Unreleased]: https://github.com/romashqua/outpost/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/romashqua/outpost/releases/tag/v0.1.0
