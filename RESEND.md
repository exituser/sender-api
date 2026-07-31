# Resend — Полная документация

> Email API для разработчиков. Отправка транзакционных и маркетинговых писем в масштабе через простой, современный API.

---

## Содержание

1. [Обзор продукта](#обзор-продукта)
2. [Ключевые возможности](#ключевые-возможности)
3. [Тарифы](#тарифы)
4. [Быстрый старт](#быстрый-старт)
5. [API Reference](#api-reference)
6. [SDK и инструменты](#sdk-и-инструменты)
7. [Webhooks](#webhooks)
8. [Безопасность и соответствие](#безопасность-и-соответствие)

---

## Обзор продукта

**Resend** — это современный email API, ориентированный на разработчиков. Сервис предоставляет простой REST API для отправки, получения и управления электронной почтой.

### Позиционирование
- "Email API for developers"
- Альтернатива SendGrid, Mailgun, Amazon SES
- Фокус на developer experience (DX)
- Поддержка AI-агентов и автоматизации

### Базовый URL
```
https://api.resend.com
```

---

## Ключевые возможности

### Типы писем
- **Транзакционные письма** — персонализированные, событие-управляемые сообщения
- **Маркетинговые рассылки** — массовые Broadcast для контактных списков

### Основные функции

| Функция | Описание |
|---------|----------|
| Отправка писем | Транзакционные и маркетинговые |
| Входящая почта | Получение писем через webhooks |
| Шаблоны | HTML-шаблоны с переменными |
| Контакты | Управление контактами и сегментами |
| Топики | Подписки пользователей на темы |
| Batch отправка | До 100 писем за один запрос |
| Планирование | Отправка по расписанию |
| Webhooks | Отслеживание событий |
| Вложения | Файлы до 40MB (после Base64) |
| Кастомные заголовки | Свои заголовки для писем |
| Idempotency | Защита от дублирования |
| Теги | Метки для категоризации писем |

### Технические возможности
- **DKIM/SPF/DMARC** — аутентификация домена
- **Open & Click Tracking** — отслеживание открытий и кликов
- **Мультирегион** — отправка из разных регионов
- **React Email** — шаблоны как React-компоненты
- **BIMI** — отображение логотипа в почтовых клиентах
- **Apple Branded Mail** — брендирование для Apple Mail
- **Dedicated IPs** — выделенные IP-адреса (от $30/мес)
- **SMTP relay** — альтернативный способ отправки
- **OAuth 2.1 + PKCE** — авторизация для third-party приложений

---

## Тарифы

### Транзакционные письма

| План | Цена | Писем/мес | Доплата за 1000 | Лимит в день |
|------|------|-----------|-----------------|--------------|
| **Free** | $0 | 3,000 | — | 100 |
| **Pro** | $20 | 50,000 | $0.90 | Без лимита |
| **Pro** | $35 | 100,000 | $0.90 | Без лимита |
| **Scale** | $90 | 100,000 | $0.90 | Без лимита |
| **Scale** | $160 | 200,000 | $0.80 | Без лимита |
| **Scale** | $350 | 500,000 | $0.70 | Без лимита |
| **Scale** | $650 | 1,000,000 | $0.65 | Без лимита |
| **Scale** | $825 | 1,500,000 | $0.52 | Без лимита |
| **Scale** | $1,150 | 2,500,000 | $0.46 | Без лимита |
| **Enterprise** | Custom | Custom | Custom | Без лимита |

### Маркетинговые письма

| План | Цена | Контактов |
|------|------|-----------|
| **Free** | $0 | 1,000 |
| **Pro marketing** | $40 | 5,000 |
| **Pro marketing** | $80 | 10,000 |
| **Pro marketing** | $120 | 15,000 |
| **Pro marketing** | $180 | 25,000 |
| **Pro marketing** | $250 | 50,000 |
| **Pro marketing** | $450 | 100,000 |
| **Pro marketing** | $650 | 150,000 |
| **Enterprise** | Custom | Custom |

### Примеры стоимости при высоких объемах

| Объем | План | Цена/мес | Эффективная ставка за 1000 |
|-------|------|----------|---------------------------|
| 500K писем | Scale | $350 | $0.70 |
| 1M писем | Scale | $650 | $0.65 |
| 2.5M писем | Scale | $1,150 | $0.46 |

### Дополнительные возможности

| Опция | Цена |
|-------|------|
| Dedicated IPs | $30/мес (Scale план, >3000 писем/день) |
| Automations | 10,000 runs/мес включено, далее $0.0015/run |
| AI Credits | 5-500/мес в зависимости от плана |

### AI Credits

| План | AI Credits/мес |
|------|----------------|
| Free | 5 |
| Pro | 100 |
| Scale | 500 |
| Enterprise | Flexible |

---

## Быстрый старт

### Требования
1. Собственный домен, верифицированный в Resend
2. API ключ Resend

### Создание API ключа
1. Перейдите в [Dashboard → API Keys](https://resend.com/api-keys)
2. Нажмите "Create API Key"
3. Скопируйте ключ (отображается только один раз)

### Установка SDK

**Node.js:**
```bash
npm install resend
```

**Python:**
```bash
pip install resend
```

**Go:**
```bash
go get github.com/resend/resend-go/v3
```

**Ruby:**
```bash
gem install resend
```

**PHP:**
```bash
composer require resend/resend-php
```

### Первое письмо

**Node.js:**
```typescript
import { Resend } from 'resend';

const resend = new Resend('re_xxxxxxxxx');

const { data, error } = await resend.emails.send({
  from: 'Acme <onboarding@resend.dev>',
  to: ['user@example.com'],
  subject: 'hello world',
  html: '<p>it works!</p>',
});
```

**Python:**
```python
import resend

resend.api_key = "re_xxxxxxxxx"

params: resend.Emails.SendParams = {
    "from": "Acme <onboarding@resend.dev>",
    "to": ["user@example.com"],
    "subject": "hello world",
    "html": "<p>it works!</p>"
}

email = resend.Emails.send(params)
```

**Go:**
```go
package main

import (
    "context"
    "github.com/resend/resend-go/v3"
)

func main() {
    ctx := context.TODO()
    client := resend.NewClient("re_xxxxxxxxx")

    params := &resend.SendEmailRequest{
        From:    "Acme <onboarding@resend.dev>",
        To:      []string{"user@example.com"},
        Subject: "hello world",
        Html:    "<p>it works!</p>",
    }

    sent, err := client.Emails.SendWithContext(ctx, params)
    if err != nil {
        panic(err)
    }
}
```

**cURL:**
```bash
curl -X POST 'https://api.resend.com/emails' \
     -H 'Authorization: Bearer re_xxxxxxxxx' \
     -H 'Content-Type: application/json' \
     -d '{
  "from": "Acme <onboarding@resend.dev>",
  "to": ["user@example.com"],
  "subject": "hello world",
  "html": "<p>it works!</p>"
}'
```

**CLI:**
```bash
resend emails send \
  --from "Acme <onboarding@resend.dev>" \
  --to user@example.com \
  --subject "hello world" \
  --html "<p>it works!</p>"
```

---

## API Reference

### Аутентификация

Все запросы требуют заголовок авторизации:

```
Authorization: Bearer re_xxxxxxxxx
User-Agent: my-app/1.0
```

**Важно:** Заголовок `User-Agent` обязателен. Без него запрос вернет `403`.

### Rate Limits

- **10 запросов в секунду** на команду
- Лимит applies ко всем API ключам команды
- Может быть увеличен для доверенных отправителей по запросу

### HTTP Status Codes

| Код | Описание |
|-----|----------|
| `200` | Успешный запрос |
| `400` | Неверные параметры |
| `401` | API ключ отсутствует |
| `403` | API ключ невалиден |
| `404` | Ресурс не найден |
| `429` | Превышен rate limit |
| `5xx` | Ошибка сервера Resend |

---

### Emails

#### Отправка письма

```
POST /emails
```

**Body Parameters:**

| Параметр | Тип | Обязательный | Описание |
|----------|-----|--------------|----------|
| `from` | string | ✅ | Адрес отправителя (формат: `Name <email>`) |
| `to` | string \| string[] | ✅ | Адрес получателя (макс. 50) |
| `subject` | string | ✅ | Тема письма |
| `html` | string | — | HTML-версия |
| `text` | string | — | Текстовая версия |
| `react` | ReactNode | — | React-компонент (только Node.js SDK) |
| `cc` | string \| string[] | — | Копия |
| `bcc` | string \| string[] | — | Скрытая копия |
| `reply_to` | string \| string[] | — | Адрес для ответа |
| `scheduled_at` | string | — | Время отправки (ISO 8601 или natural language) |
| `headers` | object | — | Кастомные заголовки |
| `attachments` | array | — | Вложения (макс. 40MB) |
| `tags` | array | — | Теги для категоризации |
| `template` | object | — | Использование шаблона |
| `topic_id` | string | — | ID топика |

**Headers:**

| Заголовок | Описание |
|-----------|----------|
| `Idempotency-Key` | Уникальный ключ для предотвращения дублирования (макс. 256 символов, истекает через 24ч) |

**Пример ответа:**
```json
{
  "id": "49a3999c-0ce1-4ea6-ab68-afcd6dc2e794"
}
```

#### Пакетная отправка

```
POST /emails/batch
```

Отправка до 100 писем за один запрос. Тело запроса — массив объектов писем.

#### Список писем

```
GET /emails
```

Возвращает список отправленных писем с пагинацией.

#### Детали письма

```
GET /emails/{email_id}
```

#### Обновление письма

```
PATCH /emails/{email_id}
```

Обновление запланированного письма.

#### Отмена письма

```
DELETE /emails/{email_id}/schedule
```

Отмена запланированного письма.

---

### Domains

#### Создание домена

```
POST /domains
```

#### Список доменов

```
GET /domains
```

#### Детали домена

```
GET /domains/{domain_id}
```

#### Верификация домена

```
POST /domains/{domain_id}/verify
```

#### Удаление домена

```
DELETE /domains/{domain_id}
```

---

### Contacts

#### Создание контакта

```
POST /contacts
```

**Body:**
- `email` — email адрес (обязательный)
- `first_name` — имя
- `last_name` — фамилия
- `subscribed` — статус подписки (true/false)
- `audience_id` — ID аудитории

#### Список контактов

```
GET /contacts
```

#### Детали контакта

```
GET /contacts/{contact_id}
```

#### Обновление контакта

```
PATCH /contacts/{contact_id}
```

#### Удаление контакта

```
DELETE /contacts/{contact_id}
```

---

### Broadcasts (Маркетинговые рассылки)

#### Создание рассылки

```
POST /broadcasts
```

**Body:**
- `name` — название рассылки
- `audience_id` — ID аудитории
- `from` — адрес отправителя
- `subject` — тема
- `html` — содержимое

#### Список рассылок

```
GET /broadcasts
```

#### Детали рассылки

```
GET /broadcasts/{broadcast_id}
```

#### Отправка рассылки

```
POST /broadcasts/{broadcast_id}/send
```

#### Метрики рассылки

```
GET /broadcasts/{broadcast_id}/metrics
```

---

### Webhooks

#### Создание webhook

```
POST /webhooks
```

**Body:**
- `url` — URL для接收ия событий
- `events` — массив событий для отслеживания

#### Список webhook'ов

```
GET /webhooks
```

#### Детали webhook

```
GET /webhooks/{webhook_id}
```

#### Обновление webhook

```
PATCH /webhooks/{webhook_id}
```

#### Удаление webhook

```
DELETE /webhooks/{webhook_id}
```

**События webhook'ов:**
- `email.sent` — письмо отправлено
- `email.delivered` — письмо доставлено
- `email.opened` — письмо открыто
- `email.clicked` — клик по ссылке
- `email.bounced` — письмо вернулось
- `email.complained` — жалоба на спам
- `email.failed` — ошибка отправки
- `email.delivery_delayed` — задержка доставки
- `email.received` — входящее письмо
- `email.scheduled` — письмо запланировано
- `email.suppressed` — письмо подавлено
- `contact.created` — контакт создан
- `contact.updated` — контакт обновлен
- `contact.deleted` — контакт удален
- `domain.created` — домен создан
- `domain.updated` — домен обновлен
- `domain.deleted` — домен удален
- `suppression.added` — email добавлен в suppression list
- `suppression.removed` — email удален из suppression list

---

### Templates

#### Создание шаблона

```
POST /templates
```

**Body:**
- `name` — название шаблона
- `html` — HTML содержимое
- `variables` — переменные шаблона

#### Список шаблонов

```
GET /templates
```

#### Детали шаблона

```
GET /templates/{template_id}
```

#### Публикация шаблона

```
POST /templates/{template_id}/publish
```

#### Обновление шаблона

```
PATCH /templates/{template_id}
```

#### Удаление шаблона

```
DELETE /templates/{template_id}
```

#### Дублирование шаблона

```
POST /templates/{template_id}/duplicate
```

---

### API Keys

#### Создание API ключа

```
POST /api-keys
```

#### Список API ключей

```
GET /api-keys
```

#### Удаление API ключа

```
DELETE /api-keys/{api_key_id}
```

---

### Suppressions

#### Добавление в suppression list

```
POST /suppressions
```

#### Список suppressions

```
GET /suppressions
```

#### Детали suppression

```
GET /suppressions/{id}
```

#### Удаление из suppression list

```
DELETE /suppressions/{id}
```

#### Batch добавление

```
POST /suppressions/batch
```

Макс. 100 email за раз.

#### Batch удаление

```
DELETE /suppressions/batch
```

Макс. 100 email за раз.

---

### Topics

#### Создание топика

```
POST /topics
```

#### Список топиков

```
GET /topics
```

#### Детали топика

```
GET /topics/{topic_id}
```

#### Обновление топика

```
PATCH /topics/{topic_id}
```

#### Удаление топика

```
DELETE /topics/{topic_id}
```

---

### Segments

#### Создание сегмента

```
POST /segments
```

#### Список сегментов

```
GET /segments
```

#### Детали сегмента

```
GET /segments/{segment_id}
```

#### Удаление сегмента

```
DELETE /segments/{segment_id}
```

#### Контакты в сегменте

```
GET /segments/{segment_id}/contacts
```

---

### Logs

#### Список логов

```
GET /logs
```

#### Детали лога

```
GET /logs/{log_id}
```

---

### OAuth

#### Регистрация клиента

```
POST /oauth/register
```

RFC 7591 — динамическая регистрация OAuth клиента.

#### Авторизация

```
GET /oauth/authorize
```

Старт PKCE flow.

#### Токен

```
POST /oauth/token
```

Обмен authorization code на токены.

#### Ревокация токена

```
POST /oauth/revoke
```

#### Список grants

```
GET /oauth/grants
```

#### Ревокация grant

```
DELETE /oauth/grants/{grant_id}
```

---

### Contact Properties

#### Создание свойства

```
POST /contact-properties
```

#### Список свойств

```
GET /contact-properties
```

#### Детали свойства

```
GET /contact-properties/{property_id}
```

#### Обновление свойства

```
PATCH /contact-properties/{property_id}
```

#### Удаление свойства

```
DELETE /contact-properties/{property_id}
```

---

## SDK и инструменты

### Официальные SDK

| Язык | Репозиторий |
|------|-------------|
| Node.js | [resend/resend-node](https://github.com/resend/resend-node) |
| Python | [resend/resend-python](https://github.com/resend/resend-python) |
| PHP | [resend/resend-php](https://github.com/resend/resend-php) |
| Ruby | [resend/resend-ruby](https://github.com/resend/resend-ruby) |
| Go | [resend/resend-go](https://github.com/resend/resend-go) |
| Java | [resend/resend-java](https://github.com/resend/resend-java) |
| Rust | [resend/resend-rust](https:///github.com/resend/resend-rust) |
| .NET | [resend/resend-dotnet](https://github.com/resend/resend-dotnet) |
| Elixir | [resend/resend-elixir](https://github.com/resend/resend-elixir) |
| Laravel | [resend/resend-laravel](https://github.com/resend/resend-laravel) |

### CLI

Официальная командная строка для Resend:

```bash
# Установка
curl -s https://resend.com/install.sh | sh

# Отправка письма
resend emails send \
  --from "Acme <onboarding@resend.dev>" \
  --to user@example.com \
  --subject "Hello" \
  --html "<p>Hello World</p>"
```

### MCP Server

Подключение Resend к AI-агентам:

```bash
# Remote MCP Server
https://mcp.resend.com/mcp

# Self-hosted
https://github.com/resend/resend-mcp
```

### React Email

Создание HTML-шаблонов как React-компонентов:

```tsx
import { Html, Head, Body, Container, Text } from '@react-email/components';

export default function Email() {
  return (
    <Html>
      <Head />
      <Body>
        <Container>
          <Text>Hello World!</Text>
        </Container>
      </Body>
    </Html>
  );
}
```

### OpenAPI Specification

- YAML: https://resend.com/openapi.yaml
- JSON: https://resend.com/openapi.json

---

## Webhooks

### Настройка

1. Создайте endpoint в вашем приложении
2. Добавьте webhook в Resend Dashboard или через API
3. Укажите URL и события для отслеживания
4. Настройте верификацию подписи

### Верификация запросов

Используйте signing secret для проверки подлинности webhook'ов:

```python
import hmac
import hashlib

def verify_webhook(payload, signature, secret):
    expected = hmac.new(
        secret.encode(),
        payload.encode(),
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(expected, signature)
```

### Хранение событий

Рекомендации по хранению данных webhook'ов:
- Используйте базу данных для хранения событий
- Индексируйте по `email_id` для быстрого поиска
- Храните метаданные (время, тип события)

---

## Безопасность и соответствие

### Сертификаты
- **SOC 2 Type II** — контроль безопасности
- **GDPR** — соответствие регламенту защиты данных

### Безопасность API
- API ключи: храните безопасно, не коммитьте в код
- Используйте environment variables
- Настройте permissions для API ключей
- Используйте MFA для аккаунта

### Рекомендации
- Не храните секреты в коде
- Используйте dedicated IPs для production
- Настройте DMARC для вашего домена
- МонITORьте deliverability

---

## Интеграции

### Поддерживаемые платформы
- Vercel
- Next.js
- React
- Astro
- SvelteKit
- Nuxt
- Django
- Flask
- FastAPI
- Laravel
- Rails
- Go (net/http, Gin, Echo)
- и многие другие

### AI-агенты
- Cursor
- Claude
- Devin.ai
- OpenClaw
- и другие MCP-совместимые клиенты

---

## Полезные ссылки

- **Документация:** https://resend.com/docs
- **API Reference:** https://resend.com/docs/api-reference
- **Dashboard:** https://resend.com
- **GitHub:** https://github.com/resend
- **Pricing:** https://resend.com/pricing
- **Blog:** https://resend.com/blog
- **Handbook:** https://resend.com/handbook
- **Security:** https://resend.com/security
- **Contact:** https://resend.com/contact

---

## Примеры кода

### Отправка с React Email шаблоном (Node.js)

```typescript
import { Resend } from 'resend';
import WelcomeEmail from './emails/welcome';

const resend = new Resend(process.env.RESEND_API_KEY);

const { data, error } = await resend.emails.send({
  from: 'Acme <onboarding@resend.dev>',
  to: ['user@example.com'],
  subject: 'Welcome!',
  react: WelcomeEmail({ firstName: 'John' }),
});
```

### Отправка с вложениями (Python)

```python
import resend

resend.api_key = "re_xxxxxxxxx"

params: resend.Emails.SendParams = {
    "from": "Acme <onboarding@resend.dev>",
    "to": ["user@example.com"],
    "subject": "Report attached",
    "html": "<p>Please find the report attached.</p>",
    "attachments": [
        {
            "filename": "report.pdf",
            "content": base64_encoded_content
        }
    ]
}

email = resend.Emails.send(params)
```

### Планирование отправки

```typescript
await resend.emails.send({
  from: 'Acme <onboarding@resend.dev>',
  to: ['user@example.com'],
  subject: 'Scheduled email',
  html: '<p>This will be sent later!</p>',
  scheduled_at: '2026-08-05T11:52:01.858Z'
});
```

### Использование шаблонов

```typescript
await resend.emails.send({
  from: 'Acme <onboarding@resend.dev>',
  to: ['user@example.com'],
  subject: 'Welcome!',
  template: {
    id: 'template_id_here',
    variables: {
      FIRST_NAME: 'John',
      CTA_LINK: 'https://example.com'
    }
  }
});
```

### Batch отправка

```python
import resend

resend.api_key = "re_xxxxxxxxx"

emails = [
    {
        "from": "Acme <onboarding@resend.dev>",
        "to": [f"user{i}@example.com"],
        "subject": f"Hello User {i}",
        "html": f"<p>Hello User {i}!</p>"
    }
    for i in range(100)
]

result = resend.Batch.send(emails)
```

---

*Документация актуальна на основе данных с https://resend.com (июль 2026)*
