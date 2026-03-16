# Security Policy

[Русская версия ниже / Russian version below](#политика-безопасности)

## Supported Versions

| Version | Supported |
|---|---|
| 0.1.x (latest) | Yes |

We provide security updates only for the latest minor release. We strongly recommend always running the latest version.

## Reporting a Vulnerability

**Please do not create a public GitHub issue for security vulnerabilities.**

### How to Report

Send an email to **romashqua@icloud.com** with the following information:

1. **Description** of the vulnerability
2. **Steps to reproduce** (or proof-of-concept)
3. **Impact assessment** — what can an attacker do?
4. **Affected component** (core, gateway, proxy, client, frontend, API)
5. **Suggested fix** (if any)

### What to Expect

| Timeline | Action |
|---|---|
| **24 hours** | We acknowledge receipt of your report |
| **72 hours** | We provide a preliminary assessment and severity rating |
| **7 days** | We aim to have a fix ready for critical/high vulnerabilities |
| **14 days** | We coordinate disclosure and release the patched version |
| **30 days** | Public disclosure (or earlier if the fix is released) |

### Severity Classification

We use CVSS v3.1 scoring to classify vulnerabilities:

- **Critical (9.0-10.0):** Remote code execution, authentication bypass, full data extraction
- **High (7.0-8.9):** Privilege escalation, IDOR with access to other users' data, WireGuard key leakage
- **Medium (4.0-6.9):** Information disclosure, CSRF, denial of service
- **Low (0.1-3.9):** Minor information leaks, UI-only issues

### What Qualifies

- Authentication or authorization bypass
- Remote code execution
- SQL injection, command injection
- IDOR (Insecure Direct Object References) on sensitive resources
- WireGuard private key leakage
- Token/session hijacking
- Cryptographic weaknesses
- Privilege escalation
- Sensitive data exposure in logs or API responses

### What Does Not Qualify

- Vulnerabilities in dependencies without a demonstrated exploitation path in Outpost
- Denial of service via resource exhaustion (unless trivially exploitable)
- Issues requiring physical access to the server
- Social engineering attacks
- Missing security headers that don't lead to a demonstrated exploit
- Vulnerabilities in third-party services (PostgreSQL, Redis, WireGuard kernel module)

## Bug Bounty

We do not currently have a formal bug bounty program. However, we deeply appreciate responsible disclosures and are happy to:

- Credit you in our security advisory and CHANGELOG (unless you prefer anonymity)
- Send you Outpost merchandise (when available)
- Include you in our Security Hall of Fame

We are considering launching a formal bounty program and will update this document if one is established.

## Security Best Practices for Operators

When deploying Outpost in production:

1. **Change default credentials** immediately after first login
2. **Enable MFA** for all admin accounts (TOTP or WebAuthn)
3. **Use TLS** for all HTTP and gRPC endpoints
4. **Restrict database access** — never expose PostgreSQL to the public internet
5. **Set a strong `JWT_SECRET`** — at least 32 bytes of random data
6. **Enable audit logging** and forward to your SIEM
7. **Run the compliance dashboard** to check for common misconfigurations
8. **Keep Outpost up to date** — run the latest version
9. **Use network segmentation** — deploy `outpost-proxy` in the DMZ
10. **Review ZTNA policies** — configure device posture checks appropriate for your environment

## Disclosure Policy

We follow a coordinated disclosure process:

1. Reporter sends vulnerability information privately
2. We confirm and assess severity
3. We develop and test a fix
4. We release the fix and publish a security advisory
5. Reporter receives credit (if desired)

We ask reporters to allow us a reasonable timeframe (up to 90 days) to address vulnerabilities before public disclosure.

## Contact

- **Email:** romashqua@icloud.com

---

Thank you for helping keep Outpost and its users safe.

---

# Политика безопасности

## Поддерживаемые версии

| Версия | Поддержка |
|---|---|
| 0.1.x (последняя) | Да |

Мы выпускаем обновления безопасности только для последнего минорного релиза. Настоятельно рекомендуем всегда использовать актуальную версию.

## Сообщение об уязвимости

**Пожалуйста, не создавайте публичный GitHub issue для уязвимостей безопасности.**

### Как сообщить

Отправьте письмо на **romashqua@icloud.com** со следующей информацией:

1. **Описание** уязвимости
2. **Шаги для воспроизведения** (или proof-of-concept)
3. **Оценка влияния** — что может сделать атакующий?
4. **Затронутый компонент** (core, gateway, proxy, client, frontend, API)
5. **Предлагаемое исправление** (если есть)

### Чего ожидать

| Срок | Действие |
|---|---|
| **24 часа** | Мы подтверждаем получение вашего отчёта |
| **72 часа** | Мы даём предварительную оценку и рейтинг серьёзности |
| **7 дней** | Мы стремимся подготовить исправление для критических/высоких уязвимостей |
| **14 дней** | Мы координируем раскрытие и выпускаем исправленную версию |
| **30 дней** | Публичное раскрытие (или раньше, если исправление уже выпущено) |

### Классификация серьёзности

Мы используем CVSS v3.1 для классификации уязвимостей:

- **Критическая (9.0-10.0):** Удалённое выполнение кода, обход аутентификации, полное извлечение данных
- **Высокая (7.0-8.9):** Повышение привилегий, IDOR с доступом к данным других пользователей, утечка ключей WireGuard
- **Средняя (4.0-6.9):** Раскрытие информации, CSRF, отказ в обслуживании
- **Низкая (0.1-3.9):** Незначительные утечки информации, проблемы только в UI

### Что считается уязвимостью

- Обход аутентификации или авторизации
- Удалённое выполнение кода
- SQL-инъекции, инъекции команд
- IDOR (небезопасные прямые ссылки на объекты) для чувствительных ресурсов
- Утечка приватных ключей WireGuard
- Перехват токенов/сессий
- Криптографические слабости
- Повышение привилегий
- Утечка чувствительных данных в логах или ответах API

### Что не считается уязвимостью

- Уязвимости в зависимостях без продемонстрированного пути эксплуатации в Outpost
- Отказ в обслуживании через исчерпание ресурсов (если не тривиально эксплуатируемо)
- Проблемы, требующие физического доступа к серверу
- Атаки социальной инженерии
- Отсутствующие заголовки безопасности без продемонстрированного эксплойта
- Уязвимости в сторонних сервисах (PostgreSQL, Redis, модуль ядра WireGuard)

## Программа вознаграждений

На данный момент у нас нет формальной программы bug bounty. Тем не менее, мы высоко ценим ответственное раскрытие и готовы:

- Указать вас в нашем security advisory и CHANGELOG (если вы не предпочитаете анонимность)
- Отправить мерч Outpost (когда будет доступен)
- Включить вас в наш Зал славы безопасности

Мы рассматриваем запуск формальной программы вознаграждений и обновим этот документ, если она будет создана.

## Лучшие практики безопасности для операторов

При развёртывании Outpost в продакшене:

1. **Смените учётные данные по умолчанию** сразу после первого входа
2. **Включите MFA** для всех учётных записей администраторов (TOTP или WebAuthn)
3. **Используйте TLS** для всех HTTP и gRPC эндпоинтов
4. **Ограничьте доступ к базе данных** — никогда не открывайте PostgreSQL в публичный интернет
5. **Установите надёжный `JWT_SECRET`** — минимум 32 байта случайных данных
6. **Включите аудит-логирование** и настройте пересылку в SIEM
7. **Запустите панель соответствия** для проверки распространённых ошибок конфигурации
8. **Обновляйте Outpost** — используйте последнюю версию
9. **Используйте сегментацию сети** — размещайте `outpost-proxy` в DMZ
10. **Проверьте политики ZTNA** — настройте проверки состояния устройств под ваше окружение

## Политика раскрытия

Мы следуем процессу координированного раскрытия:

1. Исследователь отправляет информацию об уязвимости конфиденциально
2. Мы подтверждаем и оцениваем серьёзность
3. Мы разрабатываем и тестируем исправление
4. Мы выпускаем исправление и публикуем security advisory
5. Исследователь получает благодарность (если желает)

Мы просим исследователей дать нам разумный срок (до 90 дней) для устранения уязвимостей до публичного раскрытия.

## Контакты

- **Email:** romashqua@icloud.com

---

Спасибо за помощь в обеспечении безопасности Outpost и его пользователей.
