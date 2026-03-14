# Outpost VPN — Руководство по развёртыванию

## Быстрый старт (Разработка)

```bash
cd deploy/docker
docker compose up -d
```

Это запускает: PostgreSQL 17, Redis 7, outpost-core, outpost-gateway, outpost-proxy.

- Панель администратора: http://localhost:8080
- Учётные данные по умолчанию: `admin` / `outpost` (смените немедленно)
- API: http://localhost:8080/api/v1/
- gRPC: localhost:50051
- WireGuard: localhost:51820/udp
- Прокси для регистрации: http://localhost:8081

## Обзор архитектуры

```
┌────────────────────────────────────────────────────────────┐
│                     DMZ / Интернет                          │
│   ┌──────────────┐         ┌──────────────┐                │
│   │ outpost-proxy│         │  Клиенты     │                │
│   │    :8081     │         │  (WireGuard) │                │
│   └──────┬───────┘         └──────┬───────┘                │
└──────────┼────────────────────────┼────────────────────────┘
           │ gRPC                   │ UDP :51820
┌──────────┼────────────────────────┼────────────────────────┐
│          │    Внутренняя сеть     │                         │
│   ┌──────▼───────┐         ┌──────▼───────┐                │
│   │ outpost-core │◄───────►│outpost-gateway│               │
│   │  :8080 :50051│  gRPC   │   :51820/udp │               │
│   └──────┬───────┘  stream └──────────────┘                │
│          │                                                  │
│   ┌──────▼──────┐   ┌──────────┐                           │
│   │ PostgreSQL  │   │  Redis   │                           │
│   │    :5432    │   │  :6379   │                           │
│   └─────────────┘   └──────────┘                           │
└────────────────────────────────────────────────────────────┘
```

## Промышленное развёртывание

### Один узел (Docker Compose)

Подходит для: до ~500 пиров, одна локация.

1. Клонируйте репозиторий и настройте `.env`:

```bash
cp deploy/docker/.env.example deploy/docker/.env
# Отредактируйте .env:
#   POSTGRES_PASSWORD=<надёжный-случайный>
#   REDIS_PASSWORD=<надёжный-случайный>
#   JWT_SECRET=<случайные-64-символа>
#   GATEWAY_TOKEN=<случайные-64-символа>
```

2. Запустите стек:

```bash
cd deploy/docker
docker compose up -d
```

3. Настройте SMTP для email-уведомлений (необязательно):

```bash
export OUTPOST_SMTP_HOST=smtp.example.com
export OUTPOST_SMTP_PORT=587
export OUTPOST_SMTP_USERNAME=outpost@example.com
export OUTPOST_SMTP_PASSWORD=...
export OUTPOST_SMTP_FROM=noreply@example.com
export OUTPOST_SMTP_TLS=true
```

### Многоузловой кластер

Подходит для: высокая доступность, несколько локаций, >500 пиров.

#### Кластер Core (2-3 узла)

outpost-core не хранит состояние (всё в PostgreSQL + Redis), поэтому масштабируется горизонтально за балансировщиком нагрузки.

```
                  ┌──────────────┐
                  │   HAProxy /  │
                  │  Nginx LB    │
                  │  :8080 :50051│
                  └──────┬───────┘
                         │
            ┌────────────┼────────────┐
            │            │            │
     ┌──────▼──┐  ┌──────▼──┐  ┌─────▼───┐
     │ core-1  │  │ core-2  │  │ core-3  │
     └────┬────┘  └────┬────┘  └────┬────┘
          │            │            │
     ┌────▼────────────▼────────────▼────┐
     │       PostgreSQL (primary)         │
     │       + Redis (cluster/sentinel)   │
     └───────────────────────────────────┘
```

**Шаги:**

1. Настройте PostgreSQL HA (Patroni, CloudNativeDB или управляемая БД):

```bash
# Использование управляемого PostgreSQL (рекомендуется):
OUTPOST_DB_HOST=outpost-db.example.com
OUTPOST_DB_PORT=5432
OUTPOST_DB_NAME=outpost
OUTPOST_DB_USER=outpost
OUTPOST_DB_PASSWORD=<надёжный>
OUTPOST_DB_SSLMODE=require
```

2. Настройте Redis Sentinel или Redis Cluster:

```bash
OUTPOST_REDIS_ADDR=redis-sentinel.example.com:26379
OUTPOST_REDIS_PASSWORD=<надёжный>
```

3. Разверните несколько экземпляров core:

```bash
# На каждом узле core:
docker run -d \
  --name outpost-core \
  -e OUTPOST_HTTP_ADDR=:8080 \
  -e OUTPOST_GRPC_ADDR=:50051 \
  -e OUTPOST_DB_HOST=outpost-db.example.com \
  -e OUTPOST_DB_PASSWORD=$DB_PASSWORD \
  -e OUTPOST_REDIS_ADDR=redis.example.com:6379 \
  -e OUTPOST_JWT_SECRET=$JWT_SECRET \
  -p 8080:8080 \
  -p 50051:50051 \
  outpost/core:latest
```

4. Настройте балансировщик нагрузки:

```nginx
# nginx.conf пример
upstream outpost_http {
    least_conn;
    server core-1:8080;
    server core-2:8080;
    server core-3:8080;
}

upstream outpost_grpc {
    least_conn;
    server core-1:50051;
    server core-2:50051;
    server core-3:50051;
}

server {
    listen 443 ssl http2;
    server_name vpn.example.com;

    ssl_certificate /etc/ssl/certs/vpn.example.com.pem;
    ssl_certificate_key /etc/ssl/private/vpn.example.com.key;

    location / {
        proxy_pass http://outpost_http;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

server {
    listen 50051 http2;

    location / {
        grpc_pass grpc://outpost_grpc;
    }
}
```

#### Развёртывание шлюзов (Несколько локаций)

Каждый шлюз работает на отдельной машине с доступом к WireGuard.

```bash
# На каждом узле шлюза (требуется NET_ADMIN):
docker run -d \
  --name outpost-gateway \
  --cap-add=NET_ADMIN \
  --cap-add=SYS_MODULE \
  --device=/dev/net/tun \
  --sysctl net.ipv4.ip_forward=1 \
  -e OUTPOST_GATEWAY_CORE_ADDR=core-lb.example.com:50051 \
  -e OUTPOST_GATEWAY_TOKEN=$GATEWAY_TOKEN \
  -p 51820:51820/udp \
  outpost/gateway:latest
```

Зарегистрируйте каждый шлюз через панель администратора:
1. Перейдите на страницу **Шлюзы**
2. Нажмите **Создать** — укажите имя, сеть, endpoint (публичный IP:51820)
3. Скопируйте сгенерированный токен в переменную окружения `OUTPOST_GATEWAY_TOKEN` шлюза
4. Перезапустите контейнер шлюза

#### Топология S2S для нескольких локаций

Для соединения нескольких офисных сетей:

```
  Москва                Санкт-Петербург        Новосибирск
  ┌──────────┐          ┌──────────┐              ┌──────────┐
  │ gw-msk   │◄────────►│ gw-spb   │◄────────────►│ gw-nsk   │
  │10.1.0.0  │  S2S     │10.2.0.0  │    S2S       │10.3.0.0  │
  │  /24     │  туннель  │  /24     │   туннель    │  /24     │
  └──────────┘          └──────────┘              └──────────┘
       ▲                      ▲                        ▲
       │ WG                   │ WG                     │ WG
  ┌────┴────┐            ┌────┴────┐              ┌────┴────┐
  │ Клиенты │            │ Клиенты │              │ Клиенты │
  └─────────┘            └─────────┘              └─────────┘
```

Настройка через панель администратора:
1. Создайте **Сеть** для каждой локации (например, `10.1.0.0/24`, `10.2.0.0/24`, `10.3.0.0/24`)
2. Разверните **Шлюз** в каждой локации
3. Перейдите в **S2S-туннели** → Создать туннель → Выберите топологию **mesh** или **hub-spoke**
4. Добавьте участников-шлюзы с их локальными подсетями
5. Маршруты автоматически обмениваются между шлюзами через gRPC

### Kubernetes (Helm)

```bash
# Добавьте Helm-репозиторий Outpost
helm repo add outpost https://charts.outpost-vpn.io
helm repo update

# Установка с параметрами по умолчанию
helm install outpost outpost/outpost \
  --namespace outpost \
  --create-namespace \
  --set core.replicas=3 \
  --set postgresql.auth.password=$DB_PASSWORD \
  --set redis.auth.password=$REDIS_PASSWORD \
  --set core.env.OUTPOST_JWT_SECRET=$JWT_SECRET

# Или используйте файл значений
helm install outpost outpost/outpost \
  --namespace outpost \
  --create-namespace \
  -f values-production.yaml
```

Пример `values-production.yaml`:

```yaml
core:
  replicas: 3
  resources:
    requests:
      cpu: 500m
      memory: 256Mi
    limits:
      cpu: 2000m
      memory: 1Gi
  env:
    OUTPOST_JWT_SECRET: "<secret>"
    OUTPOST_LOG_LEVEL: info
    OUTPOST_LOG_FORMAT: json
  ingress:
    enabled: true
    className: nginx
    hosts:
      - host: vpn.example.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: vpn-tls
        hosts:
          - vpn.example.com

gateway:
  # Развёртывание как DaemonSet на помеченных узлах
  nodeSelector:
    outpost.io/gateway: "true"
  hostNetwork: true
  tolerations:
    - key: "outpost.io/gateway"
      operator: "Exists"
  env:
    OUTPOST_GATEWAY_TOKEN: "<token>"
  securityContext:
    capabilities:
      add: [NET_ADMIN, SYS_MODULE]

proxy:
  replicas: 2
  service:
    type: LoadBalancer
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-type: nlb

postgresql:
  # В продакшене используйте внешнюю управляемую БД
  enabled: false
  external:
    host: outpost-db.rds.amazonaws.com
    port: 5432
    database: outpost
    existingSecret: outpost-db-credentials

redis:
  # В продакшене используйте внешний управляемый Redis
  enabled: false
  external:
    host: outpost-redis.elasticache.amazonaws.com
    port: 6379
    existingSecret: outpost-redis-credentials

monitoring:
  serviceMonitor:
    enabled: true
    interval: 15s
  prometheusRule:
    enabled: true
```

## Справочник переменных окружения

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `OUTPOST_HTTP_ADDR` | `:8080` | Адрес прослушивания HTTP |
| `OUTPOST_GRPC_ADDR` | `:9090` | Адрес прослушивания gRPC |
| `OUTPOST_DB_HOST` | `localhost` | Хост PostgreSQL |
| `OUTPOST_DB_PORT` | `5432` | Порт PostgreSQL |
| `OUTPOST_DB_NAME` | `outpost` | База данных PostgreSQL |
| `OUTPOST_DB_USER` | `outpost` | Пользователь PostgreSQL |
| `OUTPOST_DB_PASSWORD` | | Пароль PostgreSQL |
| `OUTPOST_DB_SSLMODE` | `disable` | Режим SSL PostgreSQL |
| `OUTPOST_DB_MAX_CONNS` | `20` | Максимум соединений с БД |
| `OUTPOST_REDIS_ADDR` | `localhost:6379` | Адрес Redis |
| `OUTPOST_REDIS_PASSWORD` | | Пароль Redis |
| `OUTPOST_JWT_SECRET` | | Секрет подписи JWT (обязательный) |
| `OUTPOST_TOKEN_TTL` | `15m` | Время жизни JWT-токена |
| `OUTPOST_SESSION_TTL` | `24h` | Время жизни сессии |
| `OUTPOST_WG_INTERFACE` | `wg0` | Имя WireGuard-интерфейса |
| `OUTPOST_WG_LISTEN_PORT` | `51820` | Порт прослушивания WireGuard |
| `OUTPOST_GATEWAY_TOKEN` | | Токен аутентификации шлюза |
| `OUTPOST_GATEWAY_CORE_ADDR` | `localhost:9090` | Адрес gRPC core |
| `OUTPOST_SMTP_HOST` | | SMTP-сервер (необязательный) |
| `OUTPOST_SMTP_PORT` | `587` | Порт SMTP |
| `OUTPOST_SMTP_USERNAME` | | Пользователь SMTP |
| `OUTPOST_SMTP_PASSWORD` | | Пароль SMTP |
| `OUTPOST_SMTP_FROM` | | Адрес отправителя email |
| `OUTPOST_SMTP_TLS` | `false` | Включить STARTTLS |
| `OUTPOST_LOG_LEVEL` | `info` | Уровень логирования (debug/info/warn/error) |
| `OUTPOST_LOG_FORMAT` | `json` | Формат логов (json/text) |
| `OUTPOST_OIDC_ISSUER` | `http://localhost:8080` | URL издателя OIDC |
| `OUTPOST_SAML_ENABLED` | `false` | Включить SAML 2.0 SP |
| `OUTPOST_LDAP_ENABLED` | `false` | Включить синхронизацию LDAP |

## Резервное копирование и восстановление

### PostgreSQL

```bash
# Резервное копирование
pg_dump -h localhost -U outpost -d outpost > outpost_backup_$(date +%Y%m%d).sql

# Восстановление
psql -h localhost -U outpost -d outpost < outpost_backup_20260313.sql
```

### Ключевые данные для резервного копирования:
- База данных PostgreSQL (всё состояние)
- Redis (необязательно — сессии будут пересозданы)
- SSL/TLS сертификаты
- Переменные окружения / секреты

## Мониторинг

outpost-core предоставляет метрики Prometheus по адресу `/metrics`:

```yaml
# prometheus.yml конфигурация сбора метрик
scrape_configs:
  - job_name: 'outpost'
    static_configs:
      - targets: ['core-1:8080', 'core-2:8080', 'core-3:8080']
    metrics_path: /metrics
```

Ключевые метрики:
- `outpost_active_peers` — текущие подключённые WireGuard-пиры
- `outpost_bandwidth_bytes_total` — общий трафик (rx/tx)
- `outpost_gateway_last_seen_seconds` — состояние шлюза
- `outpost_auth_attempts_total` — попытки аутентификации
- `outpost_s2s_tunnel_status` — состояние S2S-туннелей

## Устранение неполадок

| Симптом | Что проверить |
|---------|-------|
| Не удаётся подключиться к VPN | `wg show` на шлюзе, проверьте файрвол, проверьте одобрение устройства |
| Шлюз офлайн | Логи core, токен шлюза, подключение gRPC |
| Медленные рукопожатия | Настройки MTU, разрешение DNS, доступность endpoint |
| S2S-туннель не работает | Оба шлюза онлайн, обмен маршрутами, конфликты подсетей |
| MFA не работает | Синхронизация времени (NTP), валидность секрета TOTP |
