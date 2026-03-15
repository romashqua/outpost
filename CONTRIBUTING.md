# Участие в проекте Outpost VPN

Спасибо за интерес к участию в проекте Outpost! Это руководство поможет вам настроить окружение и ознакомиться с процессом разработки.

## Содержание

- [Кодекс поведения](#кодекс-поведения)
- [С чего начать](#с-чего-начать)
- [Настройка окружения разработки](#настройка-окружения-разработки)
- [Сборка из исходников](#сборка-из-исходников)
- [Запуск тестов](#запуск-тестов)
- [Стиль кода и соглашения](#стиль-кода-и-соглашения)
- [Отправка изменений](#отправка-изменений)
- [Правила оформления Issue](#правила-оформления-issue)
- [Сообщество](#сообщество)

---

## Кодекс поведения

Участвуя в проекте, вы соглашаетесь поддерживать уважительную и инклюзивную среду. Будьте вежливы, конструктивны и профессиональны во всех взаимодействиях.

---

## С чего начать

1. **Сделайте форк** репозитория на GitHub
2. **Склонируйте** ваш форк локально:
   ```bash
   git clone https://github.com/<your-username>/outpost.git
   cd outpost
   ```
3. **Добавьте upstream** remote:
   ```bash
   git remote add upstream https://github.com/romashqua/outpost.git
   ```
4. **Создайте ветку** для вашей работы:
   ```bash
   git checkout -b feature/my-feature
   ```

---

## Настройка окружения разработки

### Требования

| Инструмент | Версия | Назначение |
|---|---|---|
| **Go** | 1.24+ | Компиляция бэкенда |
| **Node.js** | 22+ | Сборка фронтенда |
| **pnpm** | 9+ | Менеджер пакетов фронтенда |
| **Docker** | 24+ | Контейнерная среда выполнения |
| **Docker Compose** | v2+ | Локальный стек для разработки |
| **PostgreSQL** | 17 | База данных (или используйте Docker) |
| **Redis** | 7 | Кэш/сессии (или используйте Docker) |
| **golangci-lint** | последняя | Go-линтер |
| **buf** | последняя | Protobuf-кодогенерация (при изменении proto-файлов) |
| **sqlc** | последняя | SQL-кодогенерация (при изменении запросов) |

### Быстрая настройка через Docker

Самый быстрый способ получить работающее окружение для разработки:

```bash
# Запуск PostgreSQL и Redis
docker compose -f deploy/docker/docker-compose.yml up -d postgres redis

# Установка переменных окружения
export DATABASE_URL="postgres://outpost:outpost@localhost:5432/outpost?sslmode=disable"
export REDIS_URL="redis://localhost:6379"

# Выполнение миграций
make migrate-up

# Запуск бэкенда
go run ./cmd/outpost-core

# В отдельном терминале запустите dev-сервер фронтенда
cd web-ui && pnpm install && pnpm dev
```

### Полный стек через Docker Compose

```bash
docker compose -f deploy/docker/docker-compose.yml up -d --build
```

Это запустит все сервисы: core, gateway, proxy, PostgreSQL и Redis.

---

## Сборка из исходников

### Бэкенд

```bash
# Сборка всех бинарных файлов
make build

# Или сборка отдельных компонентов
go build -o bin/outpost-core ./cmd/outpost-core
go build -o bin/outpost-gateway ./cmd/outpost-gateway
go build -o bin/outpost-proxy ./cmd/outpost-proxy
go build -o bin/outpost-client ./cmd/outpost-client
go build -o bin/outpostctl ./cmd/outpostctl
```

### Фронтенд

```bash
cd web-ui
pnpm install
pnpm build        # Продакшен-сборка (результат встраивается через go:embed)
pnpm dev          # Dev-сервер с горячей перезагрузкой
```

### Protobuf (только при изменении .proto файлов)

```bash
make proto
```

### SQL-кодогенерация (только при изменении запросов)

```bash
make sqlc
```

---

## Запуск тестов

### Юнит-тесты

```bash
# Все тесты с детектором гонок
go test ./... -race -count=1

# Конкретный пакет
go test ./internal/auth/... -v

# С покрытием
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### End-to-End тесты

E2E-тесты требуют работающего PostgreSQL:

```bash
# Запуск тестовой базы данных
docker compose -f deploy/docker/docker-compose.yml up -d postgres

# Запуск E2E-тестов
TEST_DATABASE_URL="postgres://outpost:outpost@localhost:5432/outpost_test?sslmode=disable" \
  go test -v ./tests/e2e/
```

### Smoke-тесты

Smoke-тесты требуют работающего экземпляра Outpost:

```bash
OUTPOST_API_URL="http://localhost:8080" go test -v ./tests/e2e/ -run TestSmoke
```

### Фронтенд

```bash
cd web-ui
pnpm lint         # ESLint
npx tsc --noEmit  # Проверка типов TypeScript
```

### Линтинг

```bash
make lint          # golangci-lint (бэкенд)
make fmt           # gofmt
go vet ./...       # go vet
```

---

## Стиль кода и соглашения

### Go (бэкенд)

- Следуйте стандартным Go-соглашениям и идиомам
- Используйте `gofmt` / `goimports` для форматирования
- Все экспортируемые типы и функции должны иметь документирующие комментарии
- Для логирования используйте `slog` (не `log` и не `fmt.Println`)
- Обработка ошибок: оборачивайте ошибки с контекстом через `fmt.Errorf("...: %w", err)`
- Базы данных: используйте `sqlc` для типобезопасных запросов -- без ORM, без raw SQL в обработчиках
- HTTP-обработчики возвращают ошибки через хелпер `respondError()`
- Все API-эндпоинты документированы аннотациями swaggo

### TypeScript (фронтенд)

- `camelCase` для переменных и функций
- Только функциональные компоненты с хуками (без классовых компонентов)
- Zustand для глобального управления состоянием
- TanStack Query для всего серверного состояния (без ручных fetch в useEffect)
- Tailwind CSS для стилизации -- без CSS-in-JS, без CSS-модулей
- `lucide-react` для иконок, `recharts` для графиков
- i18n через `react-i18next` (английский и русский)

### SQL

- `snake_case` для всех идентификаторов
- Миграции в директории `migrations/` в формате golang-migrate
- Файлы миграций: `NNNNNN_description.up.sql` и `NNNNNN_description.down.sql`
- Каждая `up`-миграция должна иметь соответствующую `down`-миграцию

### Protobuf

- Proto-файлы в директории `proto/`, управляются через Buf
- Сгенерированный Go-код помещается в `pkg/pb/`

### Общие правила

- Весь код и комментарии на английском языке
- Никогда не коммитьте секреты, `.env`-файлы или учётные данные
- Делайте PR сфокусированными -- одна фича или фикс на один PR
- Пишите тесты для новой функциональности

---

## Отправка изменений

### Процесс Pull Request

1. **Синхронизируйтесь с upstream:**
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Убедитесь, что все проверки проходят:**
   ```bash
   make test && make lint
   cd web-ui && pnpm lint && npx tsc --noEmit
   ```

3. **Отправьте свою ветку:**
   ```bash
   git push origin feature/my-feature
   ```

4. **Откройте Pull Request** на GitHub в ветку `main`

5. **Заполните шаблон PR** с описанием изменений, проведённого тестирования и любого релевантного контекста

### Требования к PR

- Все проверки CI должны пройти (тесты, линтинг, сборка)
- Код должен быть отформатирован (`gofmt`, ESLint)
- Новые функции должны сопровождаться тестами
- Изменения схемы базы данных должны включать `up`- и `down`-миграции
- Изменения API должны обновлять аннотации swaggo и спецификацию OpenAPI
- Ломающие изменения должны быть явно описаны в PR

### Сообщения коммитов

Пишите ясные, описательные сообщения коммитов:

```
Add device posture check for screen lock status

Implements the screen lock detection for macOS and Windows in the
ZTNA posture engine. Updates the trust score calculation to include
the new check with configurable weight.
```

- Используйте повелительное наклонение ("Add feature", а не "Added feature")
- Первая строка: краткое описание (до 72 символов)
- Тело: объясняйте *почему*, а не только *что*

---

## Правила оформления Issue

### Баг-репорты

Используйте [шаблон баг-репорта](.github/ISSUE_TEMPLATE/bug_report.md) и включите:

- Шаги воспроизведения
- Ожидаемое и фактическое поведение
- Детали окружения (ОС, версия Go, версия Docker)
- Релевантные логи или сообщения об ошибках

### Запросы функций

Используйте [шаблон запроса функции](.github/ISSUE_TEMPLATE/feature_request.md) и включите:

- Ясное описание предлагаемой функции
- Сценарий использования / мотивация
- Рассмотренные альтернативные подходы

### Уязвимости безопасности

**Не создавайте публичный issue.** Смотрите [SECURITY.md](SECURITY.md) для инструкций по ответственному раскрытию.

---

## Сообщество

- **GitHub Issues** -- баг-репорты, запросы функций и обсуждения
- **Pull Requests** -- вклад в код и ревью
- **Discussions** -- вопросы, идеи и общение

---

Спасибо за участие в проекте Outpost! Каждый вклад помогает сделать self-hosted VPN лучше для всех.
