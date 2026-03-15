# Справочник по конфигурации

Outpost VPN настраивается полностью через переменные окружения с префиксом `OUTPOST_`. Отсутствующие переменные используют разумные значения по умолчанию. Конфигурация загружается при старте в `internal/config/config.go`.

## outpost-core

### Сервер

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_HTTP_ADDR` | `:8080` | Адрес HTTP-сервера (API + панель администратора) |
| `OUTPOST_GRPC_ADDR` | `:9090` | Адрес gRPC-сервера (связь со шлюзами) |
| `OUTPOST_TLS_CERT` | _(пусто)_ | Путь к файлу TLS-сертификата (включает HTTPS) |
| `OUTPOST_TLS_KEY` | _(пусто)_ | Путь к файлу приватного ключа TLS |

Примечание: в Docker Compose core использует `OUTPOST_SERVER_HTTP_ADDR` и `OUTPOST_SERVER_GRPC_ADDR` (с префиксом `SERVER_`) для тех же целей. Загрузчик конфигурации читает `OUTPOST_HTTP_ADDR`.

### База данных (PostgreSQL)

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_DB_HOST` | `localhost` | Хост PostgreSQL |
| `OUTPOST_DB_PORT` | `5432` | Порт PostgreSQL |
| `OUTPOST_DB_NAME` | `outpost` | Имя базы данных |
| `OUTPOST_DB_USER` | `outpost` | Пользователь базы данных |
| `OUTPOST_DB_PASSWORD` | _(пусто)_ | Пароль базы данных |
| `OUTPOST_DB_SSLMODE` | `disable` | SSL-режим PostgreSQL (`disable`, `require`, `verify-ca`, `verify-full`) |
| `OUTPOST_DB_MAX_CONNS` | `20` | Максимальное число соединений с БД |
| `OUTPOST_DB_MIN_CONNS` | `2` | Минимальное число простаивающих соединений |

### Redis

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_REDIS_ADDR` | `localhost:6379` | Адрес Redis (хост:порт) |
| `OUTPOST_REDIS_PASSWORD` | _(пусто)_ | Пароль Redis |
| `OUTPOST_REDIS_DB` | `0` | Номер базы данных Redis |

### Аутентификация

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_JWT_SECRET` | _(авто-генерация)_ | Секрет для подписи JWT. **Критично: установите в продакшене.** Если не задан, при каждом запуске генерируется случайная 32-байтная hex-строка, и токены не переживут перезапуск. |
| `OUTPOST_TOKEN_TTL` | `15m` | Время жизни JWT-токена (формат Go duration: `15m`, `1h`, `24h`) |
| `OUTPOST_SESSION_TTL` | `24h` | Время жизни сессии |

### OIDC-провайдер

Outpost включает встроенный OpenID Connect провайдер идентификации.

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_OIDC_ISSUER` | `http://localhost:8080` | URL OIDC-эмитента. Должен совпадать с публичным URL вашего экземпляра Outpost. |
| `OUTPOST_OIDC_SIGNING_KEY` | _(пусто)_ | Путь к PEM-файлу приватного ключа RSA для подписи OIDC-токенов. Если пусто, ключ генерируется при запуске. |

### SAML 2.0

Outpost может выступать в роли SAML 2.0 Service Provider, делегируя аутентификацию внешнему Identity Provider (Okta, Azure AD, OneLogin и т.д.).

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_SAML_ENABLED` | `false` | Включить SAML 2.0 SP |
| `OUTPOST_SAML_ENTITY_ID` | _(пусто)_ | Entity ID SP (уникальный идентификатор для данного SP) |
| `OUTPOST_SAML_ACS_URL` | _(пусто)_ | URL Assertion Consumer Service (например, `https://vpn.example.com/saml/acs`) |
| `OUTPOST_SAML_IDP_METADATA_URL` | _(пусто)_ | URL метаданных IDP (загружается при запуске для конфигурации SP) |
| `OUTPOST_SAML_CERT_FILE` | _(пусто)_ | Путь к X.509 сертификату SP |
| `OUTPOST_SAML_KEY_FILE` | _(пусто)_ | Путь к приватному ключу SP |

### LDAP / Active Directory

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_LDAP_ENABLED` | `false` | Включить синхронизацию LDAP/AD |
| `OUTPOST_LDAP_URL` | _(пусто)_ | URL LDAP-сервера (например, `ldap://ad.example.com:389` или `ldaps://ad.example.com:636`) |
| `OUTPOST_LDAP_BIND_DN` | _(пусто)_ | Bind DN для LDAP-запросов (например, `cn=outpost,ou=service,dc=example,dc=com`) |
| `OUTPOST_LDAP_BIND_PASSWORD` | _(пусто)_ | Пароль для Bind |
| `OUTPOST_LDAP_BASE_DN` | _(пусто)_ | Base DN для поиска пользователей/групп (например, `dc=example,dc=com`) |
| `OUTPOST_LDAP_USER_FILTER` | `(objectClass=person)` | LDAP-фильтр для поиска пользователей |
| `OUTPOST_LDAP_GROUP_FILTER` | `(objectClass=group)` | LDAP-фильтр для поиска групп |
| `OUTPOST_LDAP_TLS` | `false` | Включить STARTTLS |
| `OUTPOST_LDAP_SKIP_VERIFY` | `false` | Пропустить проверку TLS-сертификата (не рекомендуется для продакшена) |
| `OUTPOST_LDAP_SYNC_INTERVAL` | `15m` | Периодичность синхронизации пользователей/групп из LDAP |

### WireGuard

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_WG_INTERFACE` | `wg0` | Имя WireGuard-интерфейса |
| `OUTPOST_WG_LISTEN_PORT` | `51820` | UDP-порт WireGuard |
| `OUTPOST_WG_MTU` | `1420` | MTU WireGuard-интерфейса |

### SMTP (email-уведомления)

Email опционален. Если `OUTPOST_SMTP_HOST` пуст, email-функции отключены.

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_SMTP_HOST` | _(пусто)_ | Хост SMTP-сервера |
| `OUTPOST_SMTP_PORT` | `587` | Порт SMTP-сервера |
| `OUTPOST_SMTP_USERNAME` | _(пусто)_ | Логин для SMTP-аутентификации |
| `OUTPOST_SMTP_PASSWORD` | _(пусто)_ | Пароль для SMTP-аутентификации |
| `OUTPOST_SMTP_FROM` | _(пусто)_ | Email-адрес отправителя (например, `noreply@example.com`) |
| `OUTPOST_SMTP_FROM_NAME` | `Outpost VPN` | Отображаемое имя отправителя |
| `OUTPOST_SMTP_TLS` | `false` | Включить STARTTLS |

#### Пример: Gmail SMTP

```bash
OUTPOST_SMTP_HOST=smtp.gmail.com
OUTPOST_SMTP_PORT=587
OUTPOST_SMTP_USERNAME=yourapp@gmail.com
OUTPOST_SMTP_PASSWORD=app-specific-password
OUTPOST_SMTP_FROM=yourapp@gmail.com
OUTPOST_SMTP_TLS=true
```

#### Пример: AWS SES

```bash
OUTPOST_SMTP_HOST=email-smtp.us-east-1.amazonaws.com
OUTPOST_SMTP_PORT=587
OUTPOST_SMTP_USERNAME=AKIA...
OUTPOST_SMTP_PASSWORD=...
OUTPOST_SMTP_FROM=noreply@example.com
OUTPOST_SMTP_TLS=true
```

### NAT Traversal

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_NAT_ENABLED` | `false` | Включить NAT traversal (STUN/TURN) |
| `OUTPOST_STUN_PORT` | `3478` | Порт STUN-сервера |
| `OUTPOST_TURN_PORT` | `3479` | Порт TURN-сервера |
| `OUTPOST_TURN_REALM` | `outpost` | Realm TURN-сервера |
| `OUTPOST_EXTERNAL_IP` | _(пусто)_ | Внешний IP для NAT traversal (определяется автоматически, если пусто) |

### Логирование

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_LOG_LEVEL` | `info` | Уровень логирования: `debug`, `info`, `warn`, `error` |
| `OUTPOST_LOG_FORMAT` | `json` | Формат логов: `json` (продакшен), `text` (разработка) |

---

## outpost-gateway

Бинарный файл шлюза использует ту же структуру конфигурации, но в основном использует следующие переменные:

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_GATEWAY_TOKEN` | _(пусто)_ | Токен аутентификации для подключения к core. Генерируется при создании шлюза в панели администратора. |
| `OUTPOST_GATEWAY_CORE_ADDR` | `localhost:9090` | gRPC-адрес outpost-core (например, `core.example.com:50051`) |
| `OUTPOST_LOG_LEVEL` | `info` | Уровень логирования |
| `OUTPOST_LOG_FORMAT` | `json` | Формат логов |

Шлюз также читает `OUTPOST_WG_INTERFACE`, `OUTPOST_WG_LISTEN_PORT` и `OUTPOST_WG_MTU` для конфигурации WireGuard.

### Системные требования

Шлюз требует:
- Capability `NET_ADMIN` (управление WireGuard-интерфейсами)
- Capability `SYS_MODULE` (загрузка модуля ядра WireGuard)
- Доступ к устройству `/dev/net/tun`
- sysctl `net.ipv4.ip_forward=1` (IP-форвардинг)
- sysctl `net.ipv4.conf.all.src_valid_mark=1` (валидация источника)

---

## outpost-proxy

Прокси -- легковесный компонент без состояния для развёртывания в DMZ.

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_PROXY_LISTEN_ADDR` | `:8081` | Адрес HTTP-сервера |
| `OUTPOST_PROXY_CORE_ADDR` | `localhost:9090` | gRPC-адрес outpost-core |
| `OUTPOST_LOG_LEVEL` | `info` | Уровень логирования |
| `OUTPOST_LOG_FORMAT` | `json` | Формат логов |

Прокси не имеет соединений с базой данных или Redis. Он перенаправляет запросы регистрации и аутентификации к core через gRPC.

---

## Формат длительности

Переменные, принимающие длительность, используют формат Go `time.Duration`:

| Формат | Пример | Значение |
|--------|---------|---------|
| `s` | `30s` | 30 секунд |
| `m` | `15m` | 15 минут |
| `h` | `24h` | 24 часа |
| Комбинированный | `1h30m` | 1 час 30 минут |

---

## Приоритет конфигурации

1. Переменные окружения (наивысший приоритет)
2. Встроенные значения по умолчанию (наименьший приоритет)

Конфигурационного файла нет. Вся конфигурация -- через переменные окружения. Это следует методологии twelve-factor app и хорошо работает с Docker, Kubernetes и systemd.

---

## Рекомендации по безопасности

1. **Всегда устанавливайте `OUTPOST_JWT_SECRET`** в продакшене. Если не задан, генерируется случайный секрет, и все JWT-токены становятся недействительными при перезапуске.

2. **Используйте `OUTPOST_DB_SSLMODE=require`** (или `verify-full`) при подключении к удалённой базе данных.

3. **Никогда не выставляйте порты PostgreSQL или Redis** в интернет. Только outpost-core должен иметь к ним доступ.

4. **Периодически ротируйте токены шлюзов.** Сгенерируйте новый токен в панели администратора и обновите переменную окружения шлюза.

5. **Включите SMTP** для email-уведомлений о сбросе пароля и регистрации устройств.

6. **Установите `OUTPOST_LOG_LEVEL=info`** в продакшене. Используйте `debug` только для отладки -- он логирует чувствительные детали запросов.
