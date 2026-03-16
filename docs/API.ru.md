# Справочник по API Outpost VPN

Базовый URL: `/api/v1`

Все ответы используют `Content-Type: application/json`, если не указано иное.
Все временные метки в формате RFC 3339 (например, `2026-03-13T12:00:00Z`).
Для всех идентификаторов ресурсов используются UUID.

## Аутентификация

Все эндпоинты требуют JWT Bearer-токен в заголовке `Authorization`, если явно не отмечены как **публичные**.

```
Authorization: Bearer <token>
```

Токены выпускаются эндпоинтом входа и истекают через настраиваемое время (по умолчанию 15 минут). Используйте эндпоинт обновления для получения нового токена.

### Формат ошибок

Все ответы с ошибками имеют единый формат:

```json
{
  "error": "описание ошибки",
  "message": "описание ошибки"
}
```

Оба поля (`error` и `message`) содержат одинаковое сообщение.

---

## Содержание

1. [Проверка здоровья](#проверка-здоровья)
2. [Аутентификация](#эндпоинты-аутентификации)
3. [Пользователи](#пользователи)
4. [Группы](#группы)
5. [Сети](#сети)
6. [Устройства](#устройства)
7. [Шлюзы](#шлюзы)
8. [S2S-туннели](#s2s-туннели)
9. [MFA](#mfa)
10. [Настройки](#настройки)
11. [Аудит](#аудит)
12. [Аналитика](#аналитика)
13. [Соответствие стандартам](#соответствие-стандартам)
14. [Smart Routes](#smart-routes)
15. [Вебхуки](#вебхуки)
16. [ZTNA](#ztna)
17. [NAT Traversal](#nat-traversal)
18. [Сессии](#сессии)
19. [Тенанты](#тенанты)
20. [Email](#email)
21. [SCIM 2.0](#scim-20)
22. [Дашборд](#дашборд)

---

## Проверка здоровья

Эндпоинты проверки здоровья **публичные** (аутентификация не требуется).

### GET /healthz

Проверка жизнеспособности. Всегда возвращает 200, если сервер запущен.

**Ответ:** `200 OK`

```json
{
  "status": "ok"
}
```

### GET /readyz

Проверка готовности. Проверяет соединение с базой данных.

**Ответ:** `200 OK`

```json
{
  "status": "ok"
}
```

**Ошибка:** `503 Service Unavailable` если база данных недоступна.

---

## Эндпоинты аутентификации

Эндпоинты аутентификации **публичные** (JWT не требуется).

### POST /api/v1/auth/login

Аутентификация по логину и паролю. Возвращает JWT-токен или MFA-вызов, если MFA включена.

**Тело запроса:**

```json
{
  "username": "string",
  "password": "string"
}
```

**Ответ (без MFA):** `200 OK`

```json
{
  "token": "string (JWT)",
  "expires_at": 1710345600
}
```

**Ответ (требуется MFA):** `200 OK`

```json
{
  "mfa_required": true,
  "mfa_token": "string (краткосрочный JWT)"
}
```

| Статус | Описание |
|--------|----------|
| `200`  | Вход успешен или выдан MFA-вызов |
| `400`  | Отсутствует логин или пароль |
| `401`  | Неверные учётные данные |
| `403`  | Аккаунт деактивирован |

### POST /api/v1/auth/mfa/verify

Проверка MFA-кода (TOTP или резервный код) после входа, вернувшего `mfa_required: true`.

**Тело запроса:**

```json
{
  "mfa_token": "string (из ответа login)",
  "code": "string (6-значный TOTP или 8-символьный резервный код)",
  "method": "string (totp | email | backup)"
}
```

**Ответ:** `200 OK`

```json
{
  "token": "string (полный JWT сессии)",
  "expires_at": 1710345600
}
```

| Статус | Описание |
|--------|----------|
| `200`  | MFA подтверждена, выдан полный токен |
| `400`  | Отсутствует mfa_token или код |
| `401`  | Недействительный или истёкший MFA-токен / неверный код |

### POST /api/v1/auth/refresh

Обновление JWT-токена. Требует валидный (не истёкший) Bearer-токен.

**Заголовки:** `Authorization: Bearer <текущий_токен>`

**Ответ:** `200 OK`

```json
{
  "token": "string (новый JWT)",
  "expires_at": 1710345600
}
```

| Статус | Описание |
|--------|----------|
| `200`  | Токен обновлён |
| `401`  | Отсутствует или недействительный токен |
| `403`  | Аккаунт деактивирован |

### POST /api/v1/auth/logout

Выход из текущей сессии.

**Ответ:** `200 OK`

```json
{
  "status": "ok"
}
```

---

## Пользователи

Все эндпоинты пользователей требуют JWT-аутентификации.

### GET /api/v1/users

Список пользователей с пагинацией.

**Параметры запроса:**

| Параметр  | Тип | По умолчанию | Описание |
|-----------|-----|--------------|----------|
| `page`    | int  | 1       | Номер страницы |
| `per_page`| int  | 50      | Элементов на странице (макс. 1000) |

**Ответ:** `200 OK`

```json
{
  "users": [
    {
      "id": "uuid",
      "username": "string",
      "email": "string",
      "first_name": "string",
      "last_name": "string",
      "phone": "string | null",
      "is_active": true,
      "is_admin": false,
      "created_at": "2026-03-13T12:00:00Z",
      "updated_at": "2026-03-13T12:00:00Z"
    }
  ],
  "page": 1,
  "per_page": 50
}
```

### POST /api/v1/users

Создание нового пользователя. При настроенном SMTP отправляется приветственное письмо.

**Тело запроса:**

```json
{
  "username": "string (required)",
  "email": "string (required)",
  "password": "string (required)",
  "first_name": "string",
  "last_name": "string",
  "is_admin": false
}
```

**Ответ:** `201 Created`

```json
{
  "id": "uuid",
  "username": "string",
  "email": "string",
  "first_name": "string",
  "last_name": "string",
  "phone": null,
  "is_active": true,
  "is_admin": false,
  "created_at": "2026-03-13T12:00:00Z",
  "updated_at": "2026-03-13T12:00:00Z"
}
```

| Статус | Описание |
|--------|----------|
| `201`  | Пользователь создан |
| `400`  | Отсутствуют обязательные поля |
| `409`  | Логин или email уже существует |

### GET /api/v1/users/{id}

Получение пользователя по ID.

**Ответ:** `200 OK` -- объект пользователя (аналогичная структура).

| Статус | Описание |
|--------|----------|
| `200`  | Пользователь найден |
| `400`  | Некорректный UUID |
| `404`  | Пользователь не найден |

### PUT /api/v1/users/{id}

Обновление пользователя. Все поля опциональны (частичное обновление).

**Тело запроса:**

```json
{
  "email": "string | null",
  "first_name": "string | null",
  "last_name": "string | null",
  "phone": "string | null",
  "is_active": "bool | null",
  "is_admin": "bool | null"
}
```

**Ответ:** `200 OK` -- обновлённый объект пользователя.

| Статус | Описание |
|--------|----------|
| `200`  | Пользователь обновлён |
| `400`  | Некорректный UUID или тело запроса |
| `404`  | Пользователь не найден |

### DELETE /api/v1/users/{id}

Удаление пользователя. Последний активный администратор не может быть удалён.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Пользователь удалён |
| `400`  | Некорректный UUID |
| `404`  | Пользователь не найден |
| `409`  | Невозможно удалить последнего администратора |

### PATCH /api/v1/users/{id}/activate

Активация пользователя (устанавливает `is_active = true`).

**Ответ:** `200 OK` -- обновлённый объект пользователя.

| Статус | Описание |
|--------|----------|
| `200`  | Пользователь активирован |
| `400`  | Некорректный UUID |
| `404`  | Пользователь не найден |

---

## Группы

Все эндпоинты групп требуют JWT-аутентификации.

### GET /api/v1/groups

Список всех групп с количеством участников.

**Ответ:** `200 OK`

```json
[
  {
    "id": "uuid",
    "name": "string",
    "description": "string",
    "is_system": false,
    "member_count": 5,
    "created_at": "2026-03-13T12:00:00Z"
  }
]
```

### POST /api/v1/groups

Создание новой группы.

**Тело запроса:**

```json
{
  "name": "string (required)",
  "description": "string"
}
```

**Ответ:** `201 Created`

```json
{
  "id": "uuid",
  "name": "string",
  "description": "string",
  "is_system": false,
  "member_count": 0,
  "created_at": "2026-03-13T12:00:00Z"
}
```

| Статус | Описание |
|--------|----------|
| `201`  | Группа создана |
| `400`  | Имя обязательно |
| `409`  | Группа с таким именем уже существует |

### GET /api/v1/groups/{id}

Получение деталей группы, включая участников и ACL.

**Ответ:** `200 OK`

```json
{
  "id": "uuid",
  "name": "string",
  "description": "string",
  "is_system": false,
  "created_at": "2026-03-13T12:00:00Z",
  "members": [
    {
      "user_id": "uuid",
      "username": "string",
      "email": "string"
    }
  ],
  "acls": [
    {
      "id": "uuid",
      "network_id": "uuid",
      "network_name": "string",
      "allowed_ips": ["10.0.0.0/24"]
    }
  ]
}
```

| Статус | Описание |
|--------|----------|
| `200`  | Группа найдена |
| `400`  | Некорректный UUID |
| `404`  | Группа не найдена |

### PUT /api/v1/groups/{id}

Обновление группы. Все поля опциональны.

**Тело запроса:**

```json
{
  "name": "string | null",
  "description": "string | null"
}
```

**Ответ:** `200 OK` -- объект группы.

| Статус | Описание |
|--------|----------|
| `200`  | Группа обновлена |
| `404`  | Группа не найдена |
| `409`  | Группа с таким именем уже существует |

### DELETE /api/v1/groups/{id}

Удаление группы. Системные группы не могут быть удалены.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Группа удалена |
| `403`  | Невозможно удалить системную группу |
| `404`  | Группа не найдена |

### POST /api/v1/groups/{id}/members

Добавление пользователя в группу.

**Тело запроса:**

```json
{
  "user_id": "uuid (required)"
}
```

**Ответ:** `201 Created` (пустое тело)

| Статус | Описание |
|--------|----------|
| `201`  | Участник добавлен (или уже состоит в группе) |
| `400`  | Некорректный user_id или пользователь/группа не найдены |

### DELETE /api/v1/groups/{id}/members/{userId}

Удаление пользователя из группы.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Участник удалён |
| `404`  | Участник не найден |

### GET /api/v1/groups/{id}/acls

Список ACL группы.

**Ответ:** `200 OK`

```json
[
  {
    "id": "uuid",
    "network_id": "uuid",
    "network_name": "string",
    "allowed_ips": ["10.0.0.0/24"]
  }
]
```

### POST /api/v1/groups/{id}/acls

Создание ACL для группы.

**Тело запроса:**

```json
{
  "network_id": "uuid (required)",
  "allowed_ips": ["10.0.0.0/24"]
}
```

Если `allowed_ips` не указан, по умолчанию `["0.0.0.0/0"]`.

**Ответ:** `201 Created`

```json
{
  "id": "uuid",
  "network_id": "uuid",
  "network_name": "",
  "allowed_ips": ["10.0.0.0/24"]
}
```

| Статус | Описание |
|--------|----------|
| `201`  | ACL создан |
| `400`  | Некорректный network_id, формат CIDR или сеть/группа не найдены |
| `409`  | ACL для этой сети и группы уже существует |

### DELETE /api/v1/groups/{id}/acls/{aclId}

Удаление ACL.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | ACL удалён |
| `404`  | ACL не найден |

---

## Сети

Все эндпоинты сетей требуют JWT-аутентификации.

### GET /api/v1/networks

Список всех сетей.

**Ответ:** `200 OK`

```json
[
  {
    "id": "uuid",
    "name": "string",
    "address": "10.0.0.0/24",
    "dns": ["1.1.1.1", "8.8.8.8"],
    "port": 51820,
    "keepalive": 25,
    "is_active": true,
    "created_at": "2026-03-13T12:00:00Z",
    "updated_at": "2026-03-13T12:00:00Z"
  }
]
```

### POST /api/v1/networks

Создание новой WireGuard-сети.

**Тело запроса:**

```json
{
  "name": "string (required)",
  "address": "string (required, CIDR, e.g. 10.0.0.0/24)",
  "dns": ["1.1.1.1"],
  "port": 51820,
  "keepalive": 25
}
```

По умолчанию: `port` = 51820, `keepalive` = 25, `dns` = [].

**Ответ:** `201 Created` -- объект сети.

| Статус | Описание |
|--------|----------|
| `201`  | Сеть создана |
| `400`  | Отсутствуют обязательные поля, некорректный формат CIDR или установлены биты хоста |
| `409`  | Сеть с таким именем или адресом уже существует |

### GET /api/v1/networks/{id}

Получение сети по ID.

**Ответ:** `200 OK` -- объект сети.

| Статус | Описание |
|--------|----------|
| `200`  | Сеть найдена |
| `404`  | Сеть не найдена |

### PUT /api/v1/networks/{id}

Обновление сети. Все поля опциональны.

**Тело запроса:**

```json
{
  "name": "string | null",
  "address": "string | null",
  "dns": ["string"] | null,
  "port": "int | null",
  "keepalive": "int | null",
  "is_active": "bool | null"
}
```

**Ответ:** `200 OK` -- обновлённый объект сети.

| Статус | Описание |
|--------|----------|
| `200`  | Сеть обновлена |
| `404`  | Сеть не найдена |

### DELETE /api/v1/networks/{id}

Удаление сети.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Сеть удалена |
| `404`  | Сеть не найдена |

---

## Устройства

Все эндпоинты устройств требуют JWT-аутентификации.

### GET /api/v1/devices

Список всех устройств (все пользователи).

**Ответ:** `200 OK`

```json
[
  {
    "id": "uuid",
    "user_id": "uuid",
    "name": "string",
    "wireguard_pubkey": "string (base64)",
    "assigned_ip": "10.0.0.2",
    "is_approved": true,
    "last_handshake": "2026-03-13T12:00:00Z | null",
    "created_at": "2026-03-13T12:00:00Z",
    "updated_at": "2026-03-13T12:00:00Z"
  }
]
```

### GET /api/v1/devices/my

Список устройств текущего пользователя.

**Ответ:** `200 OK` -- массив объектов устройств.

### POST /api/v1/devices

Создание нового устройства (действие администратора).

**Тело запроса:**

```json
{
  "name": "string (required)",
  "user_id": "uuid (required)",
  "wireguard_pubkey": "string (optional, auto-generated if empty or 'auto-generated')"
}
```

IP-адрес выделяется автоматически из первой активной сети.

**Ответ:** `201 Created` -- объект устройства.

| Статус | Описание |
|--------|----------|
| `201`  | Устройство создано |
| `400`  | Отсутствуют обязательные поля или некорректный user_id |
| `409`  | Устройство уже существует или некорректные данные |

### POST /api/v1/devices/enroll

Самостоятельная регистрация устройства. Создаёт устройство и возвращает параметры WireGuard-подключения.

**Тело запроса:**

```json
{
  "name": "string (required)",
  "wireguard_pubkey": "string (required)"
}
```

**Ответ:** `201 Created`

```json
{
  "device_id": "uuid",
  "address": "10.0.0.3/24",
  "dns": ["1.1.1.1", "8.8.8.8"],
  "endpoint": "vpn.example.com:51820",
  "server_public_key": "string (base64)",
  "allowed_ips": ["0.0.0.0/0"],
  "persistent_keepalive": 25
}
```

| Статус | Описание |
|--------|----------|
| `201`  | Устройство зарегистрировано |
| `400`  | Отсутствует имя или wireguard_pubkey |
| `409`  | Устройство уже существует |
| `422`  | Нет доступного активного шлюза |

### GET /api/v1/devices/{id}

Получение устройства по ID.

**Ответ:** `200 OK` -- объект устройства.

| Статус | Описание |
|--------|----------|
| `200`  | Устройство найдено |
| `404`  | Устройство не найдено |

### GET /api/v1/devices/{id}/config

Скачивание конфигурации WireGuard для устройства. Генерирует новую пару ключей, обновляет публичный ключ устройства и возвращает полный конфиг.

**Ответ:** `200 OK`

```json
{
  "config": "string (WireGuard INI config text)",
  "private_key": "string (base64, WireGuard private key)",
  "public_key": "string (base64, WireGuard public key)"
}
```

| Статус | Описание |
|--------|----------|
| `200`  | Конфиг сгенерирован |
| `404`  | Устройство не найдено |
| `422`  | Нет доступного активного шлюза |

### DELETE /api/v1/devices/{id}

Удаление устройства.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Устройство удалено |
| `404`  | Устройство не найдено |

### POST /api/v1/devices/{id}/approve

Одобрение устройства (устанавливает `is_approved = true`).

**Ответ:** `200 OK`

```json
{
  "status": "approved"
}
```

| Статус | Описание |
|--------|----------|
| `200`  | Устройство одобрено |
| `404`  | Устройство не найдено |

### POST /api/v1/devices/{id}/revoke

Отзыв устройства (устанавливает `is_approved = false`).

**Ответ:** `200 OK`

```json
{
  "status": "revoked"
}
```

| Статус | Описание |
|--------|----------|
| `200`  | Устройство отозвано |
| `404`  | Устройство не найдено |

---

## Шлюзы

Все эндпоинты шлюзов требуют JWT-аутентификации.

### GET /api/v1/gateways

Список всех шлюзов.

**Ответ:** `200 OK`

```json
[
  {
    "id": "uuid",
    "network_id": "uuid",
    "name": "string",
    "public_ip": "string | null",
    "wireguard_pubkey": "string (base64)",
    "endpoint": "vpn.example.com:51820",
    "is_active": true,
    "priority": 0,
    "last_seen": "2026-03-13T12:00:00Z | null",
    "created_at": "2026-03-13T12:00:00Z",
    "updated_at": "2026-03-13T12:00:00Z"
  }
]
```

### POST /api/v1/gateways

Создание нового шлюза. Пара ключей WireGuard и токен аутентификации генерируются на сервере. Открытый текст токена и приватный ключ возвращаются **только в этом ответе** и никогда не хранятся и не возвращаются повторно.

**Тело запроса:**

```json
{
  "name": "string (required)",
  "network_id": "uuid (required)",
  "endpoint": "string (required, e.g. vpn.example.com:51820)",
  "public_ip": "string | null (optional)",
  "priority": "int | null (optional, default 0)"
}
```

**Ответ:** `201 Created`

```json
{
  "id": "uuid",
  "network_id": "uuid",
  "name": "string",
  "public_ip": "string | null",
  "wireguard_pubkey": "string (base64)",
  "endpoint": "string",
  "is_active": true,
  "priority": 0,
  "last_seen": null,
  "created_at": "2026-03-13T12:00:00Z",
  "updated_at": "2026-03-13T12:00:00Z",
  "token": "string (hex, 64 chars -- save this!)",
  "private_key": "string (base64, WireGuard private key -- save this!)"
}
```

| Статус | Описание |
|--------|----------|
| `201`  | Шлюз создан |
| `400`  | Отсутствуют обязательные поля или некорректный network_id |
| `409`  | Шлюз с таким именем или эндпоинтом уже существует |

### GET /api/v1/gateways/{id}

Получение шлюза по ID.

**Ответ:** `200 OK` -- объект шлюза (без токена и приватного ключа).

| Статус | Описание |
|--------|----------|
| `200`  | Шлюз найден |
| `404`  | Шлюз не найден |

### DELETE /api/v1/gateways/{id}

Удаление шлюза.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Шлюз удалён |
| `404`  | Шлюз не найден |

---

## S2S-туннели

Управление site-to-site туннелями. Все эндпоинты требуют JWT-аутентификации.

### GET /api/v1/s2s-tunnels

Список всех S2S-туннелей.

**Ответ:** `200 OK`

```json
[
  {
    "id": "uuid",
    "name": "string",
    "description": "string",
    "topology": "mesh | hub_spoke",
    "hub_gateway_id": "uuid | null",
    "is_active": true,
    "created_at": "2026-03-13T12:00:00Z",
    "updated_at": "2026-03-13T12:00:00Z"
  }
]
```

### POST /api/v1/s2s-tunnels

Создание нового S2S-туннеля.

**Тело запроса:**

```json
{
  "name": "string (required)",
  "description": "string",
  "topology": "string (required, 'mesh' or 'hub_spoke')",
  "hub_gateway_id": "uuid | null (required for hub_spoke)"
}
```

**Ответ:** `201 Created` -- объект туннеля.

| Статус | Описание |
|--------|----------|
| `201`  | Туннель создан |
| `400`  | Отсутствует имя/топология или некорректное значение топологии |
| `409`  | Туннель с таким именем уже существует |

### GET /api/v1/s2s-tunnels/{id}

Получение туннеля по ID.

**Ответ:** `200 OK` -- объект туннеля.

| Статус | Описание |
|--------|----------|
| `200`  | Туннель найден |
| `404`  | Туннель не найден |

### DELETE /api/v1/s2s-tunnels/{id}

Удаление туннеля.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Туннель удалён |
| `404`  | Туннель не найден |

### GET /api/v1/s2s-tunnels/{id}/members

Список участников туннеля.

**Ответ:** `200 OK`

```json
[
  {
    "tunnel_id": "uuid",
    "gateway_id": "uuid",
    "gateway_name": "string",
    "local_subnets": ["192.168.1.0/24"]
  }
]
```

### POST /api/v1/s2s-tunnels/{id}/members

Добавление шлюза как участника туннеля. Если участник уже существует, его `local_subnets` обновляются.

**Тело запроса:**

```json
{
  "gateway_id": "uuid (required)",
  "local_subnets": ["192.168.1.0/24"]
}
```

**Ответ:** `201 Created` (empty body)

| Статус | Описание |
|--------|----------|
| `201`  | Участник добавлен |
| `400`  | Некорректный gateway_id или шлюз/туннель не найдены |

### DELETE /api/v1/s2s-tunnels/{id}/members/{gatewayId}

Удаление участника из туннеля.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Участник удалён |
| `404`  | Участник не найден |

### GET /api/v1/s2s-tunnels/{id}/routes

Список маршрутов туннеля, отсортированных по метрике.

**Ответ:** `200 OK`

```json
[
  {
    "id": "uuid",
    "tunnel_id": "uuid",
    "destination": "10.10.0.0/16",
    "via_gateway": "uuid",
    "gateway_name": "string",
    "metric": 100,
    "is_active": true,
    "created_at": "2026-03-13T12:00:00Z"
  }
]
```

### POST /api/v1/s2s-tunnels/{id}/routes

Добавление маршрута в туннель.

**Тело запроса:**

```json
{
  "destination": "string (required, CIDR)",
  "via_gateway": "uuid (required)",
  "metric": "int (optional, default 100)"
}
```

**Ответ:** `201 Created` -- объект маршрута.

| Статус | Описание |
|--------|----------|
| `201`  | Маршрут добавлен |
| `400`  | Отсутствуют поля, некорректный CIDR или туннель/шлюз не найдены |

### DELETE /api/v1/s2s-tunnels/{id}/routes/{routeId}

Удаление маршрута из туннеля.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Маршрут удалён |
| `404`  | Маршрут не найден |

### GET /api/v1/s2s-tunnels/{id}/config/{gatewayId}

Скачивание конфигурации WireGuard для конкретного шлюза в туннеле. Возвращает конфиг как текстовый файл `.conf` с заголовком `Content-Disposition`.

**Ответ:** `200 OK` (`Content-Type: text/plain; charset=utf-8`)

Тело ответа -- конфигурационный INI-файл WireGuard. Плейсхолдер `PrivateKey` должен быть заполнен оператором.

| Статус | Описание |
|--------|----------|
| `200`  | Конфиг сгенерирован |
| `404`  | Туннель не найден или шлюз не является участником |

---

## MFA

Управление многофакторной аутентификацией. Все эндпоинты требуют JWT-аутентификации.

### GET /api/v1/mfa/status

Получение статуса MFA для аутентифицированного пользователя.

**Ответ:** `200 OK`

```json
{
  "mfa_enabled": true,
  "totp_configured": true,
  "totp_verified": true,
  "webauthn_count": 2,
  "backup_codes_left": 8
}
```

### POST /api/v1/mfa/totp/setup

Начало настройки TOTP -- генерация секрета и QR-кода.

**Тело запроса:**

```json
{
  "issuer": "string (optional, default 'Outpost VPN')"
}
```

**Ответ:** `200 OK`

```json
{
  "secret": "string (base32 TOTP secret)",
  "qr_url": "string (otpauth:// URI)",
  "qr_image": "string (base64-encoded PNG)"
}
```

### POST /api/v1/mfa/totp/verify

Проверка TOTP-кода. При первой успешной проверке MFA активируется для аккаунта.

**Тело запроса:**

```json
{
  "code": "string (required, 6-digit code)"
}
```

**Ответ:** `200 OK`

```json
{
  "valid": true
}
```

### DELETE /api/v1/mfa/totp

Отключение TOTP для текущего пользователя.

**Ответ:** `204 No Content`

### POST /api/v1/mfa/backup-codes

Генерация нового набора резервных кодов. Заменяет существующие коды.

**Ответ:** `200 OK`

```json
{
  "codes": ["string", "string", "..."]
}
```

### POST /api/v1/mfa/backup-codes/verify

Проверка одноразового резервного кода.

**Тело запроса:**

```json
{
  "code": "string (required)"
}
```

**Ответ:** `200 OK`

```json
{
  "valid": true
}
```

### GET /api/v1/mfa/webauthn/credentials

Список всех WebAuthn/FIDO2 учётных данных текущего пользователя.

**Ответ:** `200 OK`

```json
[
  {
    "id": "string",
    "user_id": "string",
    "credential_id": "base64",
    "public_key": "base64",
    "sign_count": 42,
    "name": "YubiKey 5",
    "created_at": "2026-03-13T12:00:00Z"
  }
]
```

### POST /api/v1/mfa/webauthn/credentials

Регистрация нового WebAuthn-ключа.

**Тело запроса:**

```json
{
  "credential_id": "base64 (required)",
  "public_key": "base64 (required)",
  "name": "string (optional, friendly name)"
}
```

**Ответ:** `201 Created` (empty body)

| Статус | Описание |
|--------|----------|
| `201`  | Ключ зарегистрирован |
| `400`  | Отсутствует credential_id или public_key |

### DELETE /api/v1/mfa/webauthn/credentials/{id}

Удаление WebAuthn-ключа.

**Ответ:** `204 No Content`

---

## Настройки

Управление настройками в формате ключ-значение. Все эндпоинты требуют JWT-аутентификации.

### GET /api/v1/settings

Получение всех настроек в формате ключ-значение.

**Ответ:** `200 OK`

```json
{
  "instance_name": "My Outpost",
  "session_timeout_minutes": 480,
  "enforce_mfa": true
}
```

### PUT /api/v1/settings

Массовое обновление нескольких настроек. Принимает JSON-объект пар ключ-значение. Все пары обновляются в одной транзакции.

**Тело запроса:**

```json
{
  "instance_name": "My Outpost",
  "enforce_mfa": true
}
```

**Ответ:** `200 OK` -- тот же объект в ответе.

| Статус | Описание |
|--------|----------|
| `200`  | Настройки обновлены |
| `400`  | Пустое тело или несериализуемое значение |

### GET /api/v1/settings/{key}

Получение настройки по ключу.

**Ответ:** `200 OK`

```json
{
  "key": "instance_name",
  "value": "My Outpost"
}
```

| Статус | Описание |
|--------|----------|
| `200`  | Настройка найдена |
| `404`  | Настройка не найдена |

### PUT /api/v1/settings/{key}

Установка настройки.

**Тело запроса:**

```json
{
  "value": "any JSON value"
}
```

**Ответ:** `200 OK`

```json
{
  "key": "string",
  "value": "any"
}
```

### DELETE /api/v1/settings/{key}

Удаление настройки.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Настройка удалена |
| `404`  | Настройка не найдена |

### POST /api/v1/settings/smtp/test

Отправка тестового email для проверки конфигурации SMTP.

**Тело запроса:**

```json
{
  "to": "string (required, email address)"
}
```

**Ответ:** `200 OK`

```json
{
  "status": "sent"
}
```

| Статус | Описание |
|--------|----------|
| `200`  | Тестовое письмо отправлено |
| `400`  | SMTP не настроен или отсутствует `to` |
| `500`  | Ошибка отправки через SMTP |

---

## Аудит

Эндпоинты аудиторского журнала. Все требуют JWT-аутентификации.

### GET /api/v1/audit

Список записей аудиторского журнала с пагинацией и фильтрацией.

**Параметры запроса:**

| Параметр  | Тип    | По умолчанию | Описание |
|-----------|--------|--------------|----------|
| `page`    | int    | 1       | Номер страницы |
| `per_page`| int    | 50      | Элементов на странице (макс. 1000) |
| `user_id` | uuid   |         | Фильтр по пользователю |
| `action`  | string |         | Фильтр по действию (например, `POST`, `DELETE`) |
| `resource`| string |         | Фильтр по пути ресурса |
| `from`    | string |         | Начало периода (RFC 3339) |
| `to`      | string |         | Конец периода (RFC 3339) |

**Ответ:** `200 OK`

```json
{
  "data": [
    {
      "id": 12345,
      "timestamp": "2026-03-13T12:00:00Z",
      "user_id": "uuid | null",
      "action": "POST",
      "resource": "/api/v1/users",
      "details": {},
      "ip_address": "192.168.1.1",
      "user_agent": "Mozilla/5.0..."
    }
  ],
  "total": 500,
  "page": 1,
  "per_page": 50,
  "total_pages": 10
}
```

### GET /api/v1/audit/export

Экспорт аудиторских логов в формате JSON или CSV. Поддерживает те же параметры фильтрации, что и список.

**Параметры запроса:**

| Параметр  | Тип    | По умолчанию | Описание |
|-----------|--------|--------------|----------|
| `format`  | string | json    | Формат экспорта: `json` или `csv` |

**Ответ:** `200 OK`

- JSON: `Content-Disposition: attachment; filename="audit_log.json"` -- массив записей аудита
- CSV: `Content-Type: text/csv`, `Content-Disposition: attachment; filename="audit_log.csv"` -- CSV с заголовками: `id,timestamp,user_id,action,resource,details,ip_address,user_agent`

### GET /api/v1/audit/stats

Агрегированные счётчики событий аудита, сгруппированные по действию и часовым интервалам. Поддерживает те же параметры фильтрации.

**Ответ:** `200 OK`

```json
[
  {
    "action": "POST",
    "bucket": "2026-03-13T12:00:00Z",
    "count": 42
  }
]
```

---

## Аналитика

Эндпоинты аналитики трафика. Все требуют JWT-аутентификации.

### GET /api/v1/analytics/bandwidth

Использование пропускной способности во времени, агрегированное по настраиваемым интервалам.

**Параметры запроса:**

| Параметр  | Тип    | По умолчанию | Описание |
|-----------|--------|--------------|----------|
| `from`    | string | -24h       | Начало периода (RFC 3339) |
| `to`      | string | now        | Конец периода (RFC 3339) |
| `bucket`  | string | `1h`       | Длительность интервала (формат Go duration: `1h`, `15m`, `24h`) |

**Ответ:** `200 OK`

```json
[
  {
    "bucket": "2026-03-13T12:00:00Z",
    "rx_bytes": 1048576,
    "tx_bytes": 524288
  }
]
```

### GET /api/v1/analytics/top-users

Топ пользователей по суммарному трафику.

**Параметры запроса:**

| Параметр  | Тип    | По умолчанию | Описание |
|-----------|--------|--------------|----------|
| `from`    | string | -24h    | Начало периода (RFC 3339) |
| `to`      | string | now     | Конец периода (RFC 3339) |
| `limit`   | int    | 10      | Количество пользователей (макс. 100) |

**Ответ:** `200 OK`

```json
[
  {
    "user_id": "uuid",
    "username": "string",
    "rx_bytes": 1048576,
    "tx_bytes": 524288,
    "total": 1572864
  }
]
```

### GET /api/v1/analytics/connections-heatmap

Количество подключений, сгруппированное по часу дня и дню недели.

**Параметры запроса:**

| Параметр  | Тип    | По умолчанию | Описание |
|-----------|--------|--------------|----------|
| `from`    | string | -24h    | Начало периода (RFC 3339) |
| `to`      | string | now     | Конец периода (RFC 3339) |

**Ответ:** `200 OK`

```json
[
  {
    "hour": 14,
    "day_of_week": 2,
    "count": 127
  }
]
```

### GET /api/v1/analytics/summary

Сводная статистика за указанный период.

**Параметры запроса:**

| Параметр  | Тип    | По умолчанию | Описание |
|-----------|--------|--------------|----------|
| `from`    | string | -24h    | Начало периода (RFC 3339) |
| `to`      | string | now     | Конец периода (RFC 3339) |

**Ответ:** `200 OK`

```json
{
  "total_rx_bytes": 10485760,
  "total_tx_bytes": 5242880,
  "total_flows": 1500,
  "unique_users": 42,
  "unique_devices": 67
}
```

---

## Соответствие стандартам

Эндпоинты отчётов о соответствии стандартам. Все требуют JWT-аутентификации.

### GET /api/v1/compliance/report

Запуск всех проверок соответствия (SOC2 + ISO 27001 + GDPR) и получение полного отчёта.

**Ответ:** `200 OK`

```json
{
  "overall_score": 18,
  "max_score": 24,
  "percentage": 75,
  "mfa_adoption": 85.5,
  "encryption_rate": 100.0,
  "posture_rate": 92.3,
  "audit_log_enabled": true,
  "password_policy": true,
  "session_timeout": true,
  "checks": [
    {
      "id": "soc2-mfa",
      "framework": "SOC2",
      "name": "MFA Enforcement",
      "description": "All users should have MFA enabled",
      "status": "passed | failed | warning",
      "details": "85.5% of users have MFA enabled"
    }
  ]
}
```

### GET /api/v1/compliance/soc2

Запуск только проверок SOC2.

**Ответ:** `200 OK` -- массив объектов `ComplianceCheck`.

```json
[
  {
    "id": "string",
    "framework": "SOC2",
    "name": "string",
    "description": "string",
    "status": "passed | failed | warning",
    "details": "string"
  }
]
```

### GET /api/v1/compliance/iso27001

Запуск только проверок ISO 27001.

**Ответ:** `200 OK` -- массив объектов `ComplianceCheck` (аналогичная структура, `framework` = `"ISO27001"`).

### GET /api/v1/compliance/gdpr

Запуск только проверок GDPR.

**Ответ:** `200 OK` -- массив объектов `ComplianceCheck` (аналогичная структура, `framework` = `"GDPR"`).

---

## Smart Routes

Селективная маршрутизация и правила обхода прокси. Все эндпоинты требуют JWT-аутентификации.

### GET /api/v1/smart-routes

Список всех групп smart-маршрутов.

**Ответ:** `200 OK`

```json
[
  {
    "id": "uuid",
    "name": "string",
    "description": "string | null",
    "is_active": true,
    "created_at": "2026-03-13T12:00:00Z",
    "updated_at": "2026-03-13T12:00:00Z"
  }
]
```

### POST /api/v1/smart-routes

Создание группы smart-маршрутов.

**Тело запроса:**

```json
{
  "name": "string (required)",
  "description": "string | null"
}
```

**Ответ:** `201 Created` -- smart route object.

| Статус | Описание |
|--------|----------|
| `201`  | Группа маршрутов создана |
| `400`  | Имя обязательно |
| `409`  | Smart-маршрут с таким именем уже существует |

### GET /api/v1/smart-routes/{id}

Получение группы smart-маршрутов с записями.

**Ответ:** `200 OK`

```json
{
  "id": "uuid",
  "name": "string",
  "description": "string | null",
  "is_active": true,
  "created_at": "2026-03-13T12:00:00Z",
  "updated_at": "2026-03-13T12:00:00Z",
  "entries": [
    {
      "id": "uuid",
      "smart_route_id": "uuid",
      "entry_type": "domain | cidr | domain_suffix",
      "value": "example.com",
      "action": "proxy | direct | block",
      "proxy_id": "uuid | null",
      "proxy_name": "string | null",
      "priority": 100,
      "created_at": "2026-03-13T12:00:00Z"
    }
  ]
}
```

### PUT /api/v1/smart-routes/{id}

Обновление группы smart-маршрутов.

**Тело запроса:**

```json
{
  "name": "string | null",
  "description": "string | null",
  "is_active": "bool | null"
}
```

**Ответ:** `200 OK` -- обновлённый объект (без записей).

| Статус | Описание |
|--------|----------|
| `200`  | Группа маршрутов обновлена |
| `404`  | Smart-маршрут не найден |
| `409`  | Smart-маршрут с таким именем уже существует |

### DELETE /api/v1/smart-routes/{id}

Удаление группы smart-маршрутов и всех её записей.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Группа маршрутов удалена |
| `404`  | Smart-маршрут не найден |

### POST /api/v1/smart-routes/{id}/entries

Добавление записи маршрутизации в группу.

**Тело запроса:**

```json
{
  "entry_type": "string (required: 'domain', 'cidr', or 'domain_suffix')",
  "value": "string (required, e.g. 'example.com' or '10.0.0.0/8')",
  "action": "string (required: 'proxy', 'direct', or 'block')",
  "proxy_id": "uuid (required when action is 'proxy')",
  "priority": "int (optional, default 100)"
}
```

**Ответ:** `201 Created`

```json
{
  "id": "uuid",
  "smart_route_id": "uuid",
  "entry_type": "domain",
  "value": "example.com",
  "action": "direct",
  "proxy_id": null,
  "priority": 100,
  "created_at": "2026-03-13T12:00:00Z"
}
```

| Статус | Описание |
|--------|----------|
| `201`  | Запись добавлена |
| `400`  | Отсутствуют/некорректные поля, прокси-сервер или smart-маршрут не найдены |
| `409`  | Дублирующая запись |

### DELETE /api/v1/smart-routes/{id}/entries/{entryId}

Удаление записи из группы smart-маршрутов.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Запись удалена |
| `404`  | Запись не найдена |

### GET /api/v1/smart-routes/proxy-servers

Список всех настроенных прокси-серверов.

**Ответ:** `200 OK`

```json
[
  {
    "id": "uuid",
    "name": "string",
    "type": "socks5 | http | shadowsocks | vless",
    "address": "string",
    "port": 1080,
    "username": "string | null",
    "password": "string | null",
    "extra_config": "string (JSON) | null",
    "is_active": true,
    "created_at": "2026-03-13T12:00:00Z",
    "updated_at": "2026-03-13T12:00:00Z"
  }
]
```

### POST /api/v1/smart-routes/proxy-servers

Добавление прокси-сервера.

**Тело запроса:**

```json
{
  "name": "string (required)",
  "type": "string (required: 'socks5', 'http', 'shadowsocks', or 'vless')",
  "address": "string (required)",
  "port": "int (required)",
  "username": "string | null",
  "password": "string | null",
  "extra_config": "string (JSON) | null"
}
```

**Ответ:** `201 Created` -- объект прокси-сервера.

| Статус | Описание |
|--------|----------|
| `201`  | Прокси-сервер создан |
| `400`  | Отсутствуют обязательные поля или некорректный тип |
| `409`  | Прокси-сервер уже существует |

### GET /api/v1/smart-routes/proxy-servers/{id}

Получение прокси-сервера по ID.

**Ответ:** `200 OK` -- объект прокси-сервера.

### PUT /api/v1/smart-routes/proxy-servers/{id}

Обновление прокси-сервера.

**Тело запроса:** JSON с полями `name`, `type`, `address`, `port`, `username`, `password`, `extra_config` (все опциональны).

**Ответ:** `200 OK` -- обновлённый объект прокси-сервера.

### DELETE /api/v1/smart-routes/proxy-servers/{id}

Удаление прокси-сервера. Ошибка, если прокси используется в записях маршрутов.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Прокси-сервер удалён |
| `404`  | Прокси-сервер не найден |
| `409`  | Прокси-сервер используется в записях маршрутов |

### POST /api/v1/smart-routes/{id}/networks/{networkId}

Привязка группы smart-маршрутов к сети.

**Ответ:** `201 Created`

### DELETE /api/v1/smart-routes/{id}/networks/{networkId}

Отвязка группы smart-маршрутов от сети.

**Ответ:** `204 No Content`

---

## Вебхуки

Управление подписками на вебхуки. Все эндпоинты требуют JWT-аутентификации.

### GET /api/v1/webhooks

Список всех подписок на вебхуки.

**Ответ:** `200 OK`

```json
[
  {
    "id": "uuid",
    "url": "https://example.com/webhook",
    "events": ["*"],
    "is_active": true,
    "created_at": "2026-03-13T12:00:00Z"
  }
]
```

Примечание: поле `secret` никогда не возвращается в ответах.

### POST /api/v1/webhooks

Создание подписки на вебхук.

**Тело запроса:**

```json
{
  "url": "string (required)",
  "secret": "string (required, used for HMAC-SHA256 signature)",
  "events": ["string (optional, default ['*'])"]
}
```

Типы событий: `user.created`, `user.deleted`, `device.created`, `device.approved`, `device.revoked`, `webhook.test` и `*` (все события).

**Ответ:** `201 Created`

```json
{
  "id": "uuid",
  "url": "string",
  "events": ["*"],
  "is_active": true,
  "created_at": "2026-03-13T12:00:00Z"
}
```

| Статус | Описание |
|--------|----------|
| `201`  | Подписка создана |
| `400`  | Отсутствует url или secret |

### GET /api/v1/webhooks/{id}

Получение подписки на вебхук.

**Ответ:** `200 OK` -- объект подписки.

| Статус | Описание |
|--------|----------|
| `200`  | Подписка найдена |
| `404`  | Подписка не найдена |

### DELETE /api/v1/webhooks/{id}

Удаление подписки на вебхук.

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Подписка удалена |
| `404`  | Подписка не найдена |

### POST /api/v1/webhooks/{id}/test

Отправка тестового события на эндпоинт вебхука.

**Ответ:** `200 OK`

```json
{
  "status": "delivered"
}
```

| Статус | Описание |
|--------|----------|
| `200`  | Тестовое событие доставлено |
| `404`  | Подписка не найдена |
| `502`  | Ошибка доставки (ошибка upstream) |

### Формат доставки вебхуков

При возникновении события Outpost отправляет HTTP POST на URL подписчика:

**Заголовки:**

| Заголовок | Описание |
|-----------|----------|
| `Content-Type` | `application/json` |
| `X-Outpost-Signature-256` | `sha256=<HMAC-SHA256 hex digest>` |
| `X-Outpost-Event-ID` | Уникальный ID события (UUID) |
| `X-Outpost-Event-Type` | Тип события |

**Тело:**

```json
{
  "id": "uuid",
  "type": "user.created",
  "timestamp": "2026-03-13T12:00:00Z",
  "data": {}
}
```

Доставка повторяется до 3 раз с экспоненциальной задержкой (1с, 2с, 4с). Ответ 2xx считается успешным.

---

## SCIM 2.0

Эндпоинты провижининга SCIM 2.0 (RFC 7643/7644) для автоматического создания пользователей и групп из внешних Identity Provider (Okta, Azure AD). SCIM-эндпоинты используют `Content-Type: application/scim+json`.

SCIM-эндпоинты используют аутентификацию по Bearer-токену (отдельно от JWT).

### GET /api/v1/scim/v2/Users

Список SCIM-пользователей с пагинацией.

**Параметры запроса:**

| Параметр     | Тип  | По умолчанию | Описание |
|-------------|------|--------------|----------|
| `startIndex`| int  | 1       | Начальный индекс (от 1) |
| `count`     | int  | 100     | Элементов на странице |

**Ответ:** `200 OK`

```json
{
  "schemas": ["urn:ietf:params:scim:api:messages:2.0:ListResponse"],
  "totalResults": 150,
  "startIndex": 1,
  "itemsPerPage": 100,
  "Resources": [
    {
      "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
      "id": "uuid",
      "externalId": "string",
      "userName": "string",
      "name": {
        "givenName": "string",
        "familyName": "string",
        "formatted": "string"
      },
      "emails": [
        {
          "value": "user@example.com",
          "type": "work",
          "primary": true
        }
      ],
      "active": true,
      "meta": {
        "resourceType": "User",
        "created": "2026-03-13T12:00:00Z",
        "lastModified": "2026-03-13T12:00:00Z"
      }
    }
  ]
}
```

### POST /api/v1/scim/v2/Users

Создание SCIM-пользователя. Если пароль не указан, генерируется случайный.

**Тело запроса:**

```json
{
  "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
  "userName": "string (required)",
  "externalId": "string",
  "name": {
    "givenName": "string",
    "familyName": "string"
  },
  "emails": [
    {
      "value": "user@example.com",
      "type": "work",
      "primary": true
    }
  ],
  "active": true,
  "password": "string (optional)"
}
```

**Ответ:** `201 Created` -- SCIM-ресурс пользователя.

| Статус | Описание |
|--------|----------|
| `201`  | Пользователь создан |
| `400`  | Отсутствует userName |
| `409`  | Пользователь уже существует |

### GET /api/v1/scim/v2/Users/{id}

Получение SCIM-пользователя по ID.

**Ответ:** `200 OK` -- SCIM-ресурс пользователя.

| Статус | Описание |
|--------|----------|
| `200`  | Пользователь найден |
| `404`  | Пользователь не найден |

### PUT /api/v1/scim/v2/Users/{id}

Замена SCIM-пользователя (полное обновление).

**Тело запроса:** аналогично POST /Users.

**Ответ:** `200 OK` -- обновлённый SCIM-ресурс пользователя.

| Статус | Описание |
|--------|----------|
| `200`  | Пользователь заменён |
| `404`  | Пользователь не найден |

### PATCH /api/v1/scim/v2/Users/{id}

Частичное обновление SCIM-пользователя через PATCH-операции (RFC 7644, раздел 3.5.2).

**Тело запроса:**

```json
{
  "schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
  "Operations": [
    {
      "op": "replace | add | remove",
      "path": "active | userName | name.givenName | name.familyName | emails | externalId",
      "value": "any"
    }
  ]
}
```

**Ответ:** `200 OK` -- обновлённый SCIM-ресурс пользователя.

| Статус | Описание |
|--------|----------|
| `200`  | Пользователь обновлён |
| `400`  | Некорректная операция или путь |
| `404`  | Пользователь не найден |

### DELETE /api/v1/scim/v2/Users/{id}

Деактивация SCIM-пользователя (мягкое удаление -- устанавливает `is_active = false`).

**Ответ:** `204 No Content`

| Статус | Описание |
|--------|----------|
| `204`  | Пользователь деактивирован |
| `404`  | Пользователь не найден |

### GET /api/v1/scim/v2/Groups

Список SCIM-групп.

**Ответ:** `200 OK` -- SCIM ListResponse с ресурсами групп.

### POST /api/v1/scim/v2/Groups

Создание SCIM-группы.

**Тело запроса:**

```json
{
  "schemas": ["urn:ietf:params:scim:schemas:core:2.0:Group"],
  "displayName": "string (required)",
  "members": [
    {
      "value": "user-uuid",
      "display": "username"
    }
  ]
}
```

**Ответ:** `201 Created` -- SCIM-ресурс группы.

### GET /api/v1/scim/v2/Groups/{id}

Получение SCIM-группы по ID.

### PUT /api/v1/scim/v2/Groups/{id}

Замена SCIM-группы.

### PATCH /api/v1/scim/v2/Groups/{id}

Частичное обновление SCIM-группы.

### DELETE /api/v1/scim/v2/Groups/{id}

Удаление SCIM-группы.

### GET /api/v1/scim/v2/ServiceProviderConfig

Эндпоинт обнаружения SCIM. Возвращает конфигурацию провайдера.

**Ответ:** `200 OK`

```json
{
  "schemas": ["urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"],
  "patch": { "supported": true },
  "bulk": { "supported": false },
  "filter": { "supported": true, "maxResults": 200 },
  "changePassword": { "supported": false },
  "sort": { "supported": false },
  "etag": { "supported": false },
  "authenticationSchemes": [
    {
      "name": "OAuth Bearer Token",
      "type": "oauthbearertoken"
    }
  ]
}
```

### GET /api/v1/scim/v2/Schemas

Эндпоинт обнаружения схем SCIM. Возвращает определения схем User и Group.

---

## Дашборд

### GET /api/v1/dashboard/stats

Получение сводной статистики для дашборда.

**Ответ:** `200 OK`

```json
{
  "active_users": 42,
  "total_users": 50,
  "active_devices": 67,
  "total_devices": 80,
  "active_gateways": 3,
  "total_gateways": 4,
  "active_networks": 2,
  "s2s_tunnels": 1
}
```

---

## ZTNA

Управление Zero Trust Network Access. Все эндпоинты требуют JWT-аутентификации.

### GET /api/v1/ztna/trust-scores

Получение оценок доверия всех устройств.

**Ответ:** `200 OK`

```json
{
  "trust_scores": [
    {
      "device_id": "uuid",
      "score": 85,
      "factors": {
        "disk_encryption": true,
        "antivirus": true,
        "os_version": true,
        "screen_lock": false,
        "firewall": true
      },
      "updated_at": "2026-03-15T12:00:00Z"
    }
  ]
}
```

### GET /api/v1/ztna/trust-scores/{deviceId}

Получение оценки доверия конкретного устройства.

**Ответ:** `200 OK`

### GET /api/v1/ztna/trust-history/{deviceId}

История изменения оценки доверия устройства.

**Ответ:** `200 OK`

```json
{
  "history": [
    {
      "score": 85,
      "recorded_at": "2026-03-15T12:00:00Z"
    }
  ]
}
```

### GET /api/v1/ztna/trust-config

Получение текущей конфигурации весов и порогов оценки доверия.

**Ответ:** `200 OK`

```json
{
  "weights": {
    "disk_encryption": 30,
    "antivirus": 20,
    "os_version": 20,
    "screen_lock": 10,
    "firewall": 20
  },
  "min_trust_score": 50
}
```

### PUT /api/v1/ztna/trust-config

Обновление конфигурации весов и порогов.

**Тело запроса:** JSON с полями `weights` и `min_trust_score`.

**Ответ:** `200 OK`

### GET /api/v1/ztna/policies

Список ZTNA-политик.

**Ответ:** `200 OK`

### POST /api/v1/ztna/policies

Создание ZTNA-политики.

**Тело запроса:**

```json
{
  "name": "require-encryption",
  "conditions": {
    "min_trust_score": 70,
    "require_disk_encryption": true
  },
  "action": "allow",
  "networks": ["uuid"]
}
```

**Ответ:** `201 Created`

### GET /api/v1/ztna/policies/{id}

Получение деталей политики.

### PUT /api/v1/ztna/policies/{id}

Обновление политики.

### DELETE /api/v1/ztna/policies/{id}

Удаление политики.

**Ответ:** `204 No Content`

### GET /api/v1/ztna/dns-rules

Список DNS-правил ZTNA.

### POST /api/v1/ztna/dns-rules

Создание DNS-правила.

**Тело запроса:**

```json
{
  "domain": "internal.example.com",
  "min_trust_score": 80,
  "action": "block"
}
```

**Ответ:** `201 Created`

### DELETE /api/v1/ztna/dns-rules

Удаление DNS-правила.

---

## NAT Traversal

Управление NAT traversal. Все эндпоинты требуют JWT-аутентификации.

### GET /api/v1/nat/status

Получение текущего статуса NAT.

**Ответ:** `200 OK`

### POST /api/v1/nat/check

Проверка типа NAT для текущего подключения.

**Ответ:** `200 OK`

```json
{
  "nat_type": "symmetric",
  "external_ip": "203.0.113.1",
  "external_port": 51820
}
```

### GET /api/v1/nat/relays

Список relay-серверов NAT traversal.

**Ответ:** `200 OK`

### POST /api/v1/nat/relays

Добавление relay-сервера.

**Тело запроса:**

```json
{
  "address": "relay.example.com:3478",
  "type": "stun"
}
```

**Ответ:** `201 Created`

### DELETE /api/v1/nat/relays/{id}

Удаление relay-сервера.

**Ответ:** `204 No Content`

---

## Сессии

Управление сессиями пользователей. Все эндпоинты требуют JWT-аутентификации.

### GET /api/v1/sessions

Список активных сессий текущего пользователя.

**Ответ:** `200 OK`

```json
{
  "sessions": [
    {
      "id": "uuid",
      "ip_address": "192.168.1.100",
      "user_agent": "Mozilla/5.0...",
      "created_at": "2026-03-15T10:00:00Z",
      "expires_at": "2026-03-16T10:00:00Z"
    }
  ]
}
```

### DELETE /api/v1/sessions/{id}

Завершение конкретной сессии.

**Ответ:** `204 No Content`

### DELETE /api/v1/sessions

Завершение всех сессий текущего пользователя (кроме текущей).

**Ответ:** `204 No Content`

---

## Тенанты

Управление тенантами (мультитенантность / MSP-режим). Все эндпоинты требуют JWT-аутентификации с правами администратора.

### GET /api/v1/tenants

Список тенантов.

**Ответ:** `200 OK`

```json
{
  "tenants": [
    {
      "id": "uuid",
      "name": "Acme Corp",
      "slug": "acme",
      "created_at": "2026-03-01T00:00:00Z"
    }
  ]
}
```

### POST /api/v1/tenants

Создание тенанта.

**Тело запроса:**

```json
{
  "name": "Acme Corp",
  "slug": "acme",
  "admin_email": "admin@acme.com"
}
```

**Ответ:** `201 Created`

### GET /api/v1/tenants/{id}

Получение деталей тенанта.

### PUT /api/v1/tenants/{id}

Обновление тенанта.

### DELETE /api/v1/tenants/{id}

Удаление тенанта.

**Ответ:** `204 No Content`

### GET /api/v1/tenants/{id}/stats

Статистика использования тенанта.

**Ответ:** `200 OK`

```json
{
  "users": 42,
  "devices": 67,
  "networks": 3,
  "gateways": 2,
  "bandwidth_bytes": 1073741824
}
```

---

## Email

Эндпоинты email-уведомлений. Требуют JWT-аутентификации.

### POST /api/v1/mail/test

Отправка тестового email-сообщения для проверки конфигурации SMTP.

**Тело запроса:**

```json
{
  "to": "admin@example.com"
}
```

**Ответ:** `200 OK`

```json
{
  "status": "ok",
  "message": "Тестовое письмо отправлено"
}
```

| Статус | Описание |
|--------|----------|
| `200`  | Письмо отправлено успешно |
| `400`  | Отсутствует адрес получателя |
| `500`  | Ошибка SMTP (не настроен или недоступен) |
