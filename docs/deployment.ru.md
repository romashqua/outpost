# Руководство по развёртыванию

Это руководство описывает развёртывание Outpost VPN в продуктивных средах с использованием Docker Compose, Kubernetes (Helm) или bare-metal.

## Docker Compose (один сервер)

Подходит для: до ~500 пиров, одна локация.

### 1. Подготовка окружения

```bash
git clone https://github.com/romashqua/outpost.git
cd outpost
```

Создайте `.env`-файл для Docker Compose:

```bash
cat > deploy/docker/.env << 'EOF'
POSTGRES_PASSWORD=<сгенерируйте-надёжный-пароль>
REDIS_PASSWORD=<сгенерируйте-надёжный-пароль>
JWT_SECRET=<сгенерируйте-64-символьную-случайную-строку>
GATEWAY_TOKEN=<сгенерируйте-64-символьную-случайную-строку>
EOF
```

Генерация безопасных значений:

```bash
openssl rand -hex 32   # для POSTGRES_PASSWORD
openssl rand -hex 32   # для REDIS_PASSWORD
openssl rand -hex 32   # для JWT_SECRET
openssl rand -hex 32   # для GATEWAY_TOKEN
```

### 2. Запуск стека

```bash
docker compose -f deploy/docker/docker-compose.yml up -d
```

Сервисы и порты:

| Сервис | Внутренний порт | Открытый порт | Сеть |
|---------|---------------|--------------|---------|
| PostgreSQL | 5432 | 5432 | internal |
| Redis | 6379 | 6379 | internal |
| outpost-core | 8080, 50051 | 8080, 50051 | internal + dmz |
| outpost-gateway | 51820/udp | 51820/udp | internal |
| outpost-proxy | 8080 | 8081 | dmz |

### 3. Проверка

```bash
# Проверка, что все сервисы работают
docker compose -f deploy/docker/docker-compose.yml ps

# Проверка эндпоинта здоровья
curl http://localhost:8080/healthz
```

### Сетевая топология Docker Compose

Стандартный `docker-compose.yml` определяет две сети:

- **internal** -- core, шлюз, PostgreSQL, Redis (приватная, без доступа из интернета)
- **dmz** -- core, прокси (прокси доступен из интернета для регистрации)

```yaml
networks:
  internal:
    driver: bridge
  dmz:
    driver: bridge
```

### Справочник по Docker Compose

Полный compose-файл находится в `deploy/docker/docker-compose.yml`. Ключевая конфигурация:

- **core** использует переменные окружения с префиксом `OUTPOST_` (см. [Справочник по конфигурации](configuration.md))
- **gateway** требует capabilities `NET_ADMIN` и `SYS_MODULE`, а также устройство `/dev/net/tun`
- **gateway** включает sysctl `net.ipv4.ip_forward=1` и `net.ipv4.conf.all.src_valid_mark=1`
- **proxy** без состояния -- нет соединений с БД или Redis

---

## Kubernetes (Helm)

Helm-чарт находится в `deploy/helm/outpost/`.

### 1. Установка

```bash
# Из локального чарта
helm install outpost deploy/helm/outpost \
  --namespace outpost \
  --create-namespace \
  --set secret.jwtSecret=$(openssl rand -hex 32) \
  --set secret.databasePassword=$(openssl rand -hex 32)
```

Или из Helm-репозитория (когда будет опубликован):

```bash
helm repo add outpost https://charts.outpost-vpn.io
helm repo update

helm install outpost outpost/outpost \
  --namespace outpost \
  --create-namespace \
  --set secret.jwtSecret=$(openssl rand -hex 32) \
  --set secret.databasePassword=$(openssl rand -hex 32)
```

### 2. Продакшен-values

Создайте `values-production.yaml`:

```yaml
core:
  replicaCount: 3
  resources:
    requests:
      cpu: 500m
      memory: 256Mi
    limits:
      cpu: 2000m
      memory: 1Gi
  env:
    OUTPOST_LOG_LEVEL: info
    OUTPOST_LOG_FORMAT: json

gateway:
  enabled: true
  # Деплой как DaemonSet на помеченных нодах для мульти-шлюзов
  nodeSelector:
    outpost.io/gateway: "true"
  env:
    OUTPOST_GATEWAY_TOKEN: "<токен>"
  # Шлюз требует повышенных привилегий
  # securityContext задан в шаблоне чарта

proxy:
  replicaCount: 2
  service:
    type: LoadBalancer

# Используйте внешнюю управляемую БД в продакшене
postgresql:
  enabled: false

externalDatabase:
  host: outpost-db.rds.amazonaws.com
  port: 5432
  database: outpost
  username: outpost

# Используйте внешний управляемый Redis в продакшене
redis:
  enabled: false

externalRedis:
  host: outpost-redis.elasticache.amazonaws.com
  port: 6379

secret:
  jwtSecret: ""          # задайте через --set или внешний менеджер секретов
  databasePassword: ""   # задайте через --set или внешний менеджер секретов
  redisPassword: ""

ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: vpn.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: outpost-tls
      hosts:
        - vpn.example.com
```

Установка с продакшен-values:

```bash
helm install outpost deploy/helm/outpost \
  --namespace outpost \
  --create-namespace \
  -f values-production.yaml \
  --set secret.jwtSecret=$(openssl rand -hex 32) \
  --set secret.databasePassword=$DB_PASSWORD \
  --set secret.redisPassword=$REDIS_PASSWORD
```

### Справочник значений Helm-чарта

| Ключ | По умолчанию | Описание |
|-----|---------|-------------|
| `core.replicaCount` | `1` | Количество реплик core |
| `core.image.repository` | `ghcr.io/romashqua/outpost-core` | Образ core |
| `core.service.httpPort` | `8080` | Порт HTTP API |
| `core.service.grpcPort` | `9090` | Порт gRPC |
| `gateway.enabled` | `true` | Деплоить шлюз |
| `gateway.service.type` | `LoadBalancer` | Тип сервиса шлюза |
| `gateway.service.port` | `51820` | UDP-порт WireGuard |
| `proxy.enabled` | `true` | Деплоить прокси |
| `proxy.replicaCount` | `1` | Количество реплик прокси |
| `postgresql.enabled` | `true` | Деплоить встроенный PostgreSQL |
| `postgresql.storage.size` | `5Gi` | Размер PVC PostgreSQL |
| `redis.enabled` | `true` | Деплоить встроенный Redis |
| `redis.storage.size` | `1Gi` | Размер PVC Redis |
| `config.logLevel` | `info` | Уровень логирования |
| `config.coreUrl` | `http://outpost-core:8080` | URL core для межсервисного взаимодействия |
| `config.coreGrpcUrl` | `outpost-core:9090` | gRPC URL core |
| `ingress.enabled` | `false` | Включить Ingress |
| `secret.jwtSecret` | `""` | Ключ подписи JWT |
| `secret.databasePassword` | `outpost` | Пароль БД |
| `secret.redisPassword` | `""` | Пароль Redis |

---

## Мульти-нодовая / высокодоступная конфигурация

outpost-core не хранит состояние (всё в PostgreSQL + Redis), поэтому масштабируется горизонтально за балансировщиком нагрузки.

### Архитектура

```
                  ┌──────────────┐
                  │   HAProxy /  │
                  │  Nginx LB    │
                  │ :443  :50051 │
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
     │       + Redis (sentinel/cluster)   │
     └───────────────────────────────────┘
```

### Требования

1. **PostgreSQL HA**: используйте Patroni, CloudNativePG или управляемую БД (RDS, Cloud SQL)
2. **Redis HA**: используйте Redis Sentinel или Redis Cluster
3. **Балансировщик нагрузки**: маршрутизируйте HTTP (:8080) и gRPC (:50051) на все экземпляры core
4. **Общий JWT_SECRET**: все экземпляры core должны использовать один `OUTPOST_JWT_SECRET`

---

## TLS / Reverse Proxy

### Nginx

```nginx
upstream outpost_http {
    least_conn;
    server core-1:8080;
    server core-2:8080;
}

upstream outpost_grpc {
    least_conn;
    server core-1:50051;
    server core-2:50051;
}

# HTTP API + панель администратора
server {
    listen 443 ssl http2;
    server_name vpn.example.com;

    ssl_certificate     /etc/ssl/certs/vpn.example.com.pem;
    ssl_certificate_key /etc/ssl/private/vpn.example.com.key;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;

    # Заголовки безопасности
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options nosniff;
    add_header X-Frame-Options DENY;

    location / {
        proxy_pass http://outpost_http;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

# gRPC (для подключений шлюзов)
server {
    listen 50051 http2;

    location / {
        grpc_pass grpc://outpost_grpc;
    }
}

# Редирект HTTP на HTTPS
server {
    listen 80;
    server_name vpn.example.com;
    return 301 https://$host$request_uri;
}
```

### Caddy

```Caddyfile
vpn.example.com {
    reverse_proxy core-1:8080 core-2:8080 {
        lb_policy least_conn
        health_uri /healthz
        health_interval 10s
    }
}
```

Caddy автоматически получает и обновляет TLS-сертификаты через Let's Encrypt.

---

## Мульти-сайтовое развёртывание шлюзов

Разверните шлюзы на каждой площадке, все подключающиеся к кластеру core.

```bash
# На каждой машине шлюза
docker run -d \
  --name outpost-gateway \
  --cap-add=NET_ADMIN \
  --cap-add=SYS_MODULE \
  --device=/dev/net/tun \
  --sysctl net.ipv4.ip_forward=1 \
  --sysctl net.ipv4.conf.all.src_valid_mark=1 \
  -e OUTPOST_GATEWAY_CORE_ADDR=core-lb.example.com:50051 \
  -e OUTPOST_GATEWAY_TOKEN=$GATEWAY_TOKEN \
  -e OUTPOST_LOG_LEVEL=info \
  -e OUTPOST_LOG_FORMAT=json \
  -p 51820:51820/udp \
  ghcr.io/romashqua/outpost-gateway:latest
```

Зарегистрируйте каждый шлюз через панель администратора:
1. Перейдите в **Шлюзы** > **Создать**
2. Укажите название, сеть и публичный эндпоинт
3. Скопируйте сгенерированный токен в `OUTPOST_GATEWAY_TOKEN` шлюза
4. Перезапустите контейнер шлюза

---

## Настройка PostgreSQL

Для продакшен-PostgreSQL с Outpost:

```sql
-- Рекомендуемые настройки для сервера с 4 ГБ RAM и ~500 пиров
ALTER SYSTEM SET shared_buffers = '1GB';
ALTER SYSTEM SET effective_cache_size = '3GB';
ALTER SYSTEM SET work_mem = '16MB';
ALTER SYSTEM SET maintenance_work_mem = '256MB';
ALTER SYSTEM SET max_connections = 100;
ALTER SYSTEM SET checkpoint_completion_target = 0.9;
ALTER SYSTEM SET wal_buffers = '64MB';
ALTER SYSTEM SET random_page_cost = 1.1;  -- для SSD
```

Таблица `peer_stats` партиционирована по `recorded_at` для эффективных time-series запросов. Создавайте ежемесячные партиции:

```sql
CREATE TABLE peer_stats_2026_03 PARTITION OF peer_stats
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
```

---

## Конфигурация Redis

Рекомендуемые настройки Redis:

```
maxmemory 256mb
maxmemory-policy allkeys-lru
save ""
appendonly no
```

Redis используется для:
- Хранения сессий
- Pub/sub трансляции событий
- Хранения состояния rate limiting

Сессии пересоздаются при перезапуске Redis, поэтому персистентность опциональна. Отключите RDB/AOF-персистентность для лучшей производительности, если допускаете потерю сессий при перезапуске.

---

## Резервное копирование и восстановление

### Резервная копия PostgreSQL

```bash
# Полная резервная копия
pg_dump -h localhost -U outpost -d outpost -Fc > outpost_$(date +%Y%m%d).dump

# Восстановление
pg_restore -h localhost -U outpost -d outpost -c outpost_20260315.dump
```

### Какие данные копировать

| Данные | Расположение | Критичность |
|------|----------|-------------|
| База данных PostgreSQL | Всё состояние приложения | Критично |
| JWT-секрет | Переменная окружения `OUTPOST_JWT_SECRET` | Критично (токены недействительны без него) |
| Токены шлюзов | `OUTPOST_GATEWAY_TOKEN` на каждом шлюзе | Высокая (шлюзы не смогут переподключиться) |
| TLS-сертификаты | Конфигурация reverse proxy | Высокая |
| Redis | Сессии, rate limits | Низкая (пересоздаются при перезапуске) |

---

## Мониторинг

outpost-core предоставляет метрики Prometheus по адресу `/metrics` (без аутентификации).

### Конфигурация Prometheus

```yaml
scrape_configs:
  - job_name: 'outpost'
    static_configs:
      - targets: ['core-1:8080', 'core-2:8080']
    metrics_path: /metrics
    scrape_interval: 15s
```

### Ключевые метрики

| Метрика | Тип | Описание |
|--------|------|-------------|
| `outpost_active_peers` | Gauge | Текущие подключённые WireGuard-пиры |
| `outpost_bandwidth_bytes_total` | Counter | Общий трафик (метки rx/tx) |
| `outpost_gateway_last_seen_seconds` | Gauge | Время с последнего heartbeat шлюза |
| `outpost_auth_attempts_total` | Counter | Попытки аутентификации (метки success/failure) |
| `outpost_s2s_tunnel_status` | Gauge | Состояние S2S-туннеля (1=работает, 0=не работает) |

### Эндпоинты проверки здоровья

| Эндпоинт | Назначение | Использование |
|----------|---------|---------|
| `GET /healthz` | Проверка жизнеспособности | Kubernetes liveness probe, проверка здоровья LB |
| `GET /readyz` | Проверка готовности (проверяет БД) | Kubernetes readiness probe |

---

## Устранение неполадок

| Симптом | Что проверить |
|---------|-------|
| Не подключается к VPN | `wg show` на шлюзе, правила файрвола, статус одобрения устройства |
| Шлюз показывается офлайн | Логи core, токен шлюза, связность gRPC, `OUTPOST_GATEWAY_CORE_ADDR` |
| Медленные хендшейки | Настройки MTU, DNS-резолвинг, доступность эндпоинта |
| S2S-туннель не работает | Оба шлюза онлайн, обмен маршрутами, конфликты подсетей |
| MFA не работает | Синхронизация времени (NTP), валидность TOTP-секрета |
| 429 Too Many Requests | Rate limit аутентификации (10/мин на IP), проверьте, нет ли прокси без `X-Real-IP` |
| Core не запускается | Проверьте `OUTPOST_JWT_SECRET`, связность с БД, ошибки миграций |
| Пустая страница фронтенда | Проверьте, встроен ли `web-ui/dist`, директиву `go:embed` |
