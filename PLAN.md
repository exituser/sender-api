# Sender API — План проекта

> Email API для разработчиков на базе Amazon SES. Open-source альтернатива Resend.

> **Статус реализации:** документ сохраняет целевую архитектуру и проектные
> примеры. Фактический текущий контракт зафиксирован в [README.md](README.md):
> Go API использует прямой `pgx`, SES v2 и Redis; inbound поддерживает SNS с
> проверкой подписи, S3/SQS worker path и защищённый legacy token endpoint.
> `go-mail`, `sqlc` и Go-библиотека миграций по-прежнему не подключены.

> **Фактическое состояние на июль 2026:** локальный backend, worker, frontend,
> durable queues/webhooks, inbound SNS/S3/SQS path, OpenAPI, Docker hardening и
> GitHub CI уже реализованы. Неподтверждёнными остаются только внешние
> production-операции: AWS/Supabase credentials, SES receipt rules, DNS,
> Hetzner/Cloudflare и фактический live rollout.

---

## Содержание

1. [Принятые решения](#принятые-решения)
2. [Стек технологий](#стек-технологий)
3. [Архитектура](#архитектура)
4. [Мульти-тенантность (Teams)](#мульти-тенантность-teams)
5. [Схема БД](#схема-бд)
6. [Структура проекта](#структура-проекта)
7. [API Endpoints](#api-endpoints)
8. [Аутентификация и авторизация](#аутентификация-и-авторизация)
9. [Отправка писем](#отправка-писем)
10. [Получение писем (Inbound)](#получение-писем-inbound)
11. [Верификация доменов](#верификация-доменов)
12. [Webhooks](#webhooks)
13. [Rate Limits и лимиты](#rate-limits-и-лимиты)
14. [Фронтенд](#фронтенд)
15. [Инфраструктура](#инфраструктура)
16. [Этапы реализации](#этапы-реализации)

---

## Принятые решения

| Решение | Выбор | Обоснование |
|---------|-------|-------------|
| **Backend** | Go + Chi | Простой, идиоматичный фреймворк, стандартный net/http |
| **БД** | Supabase (PostgreSQL) | Managed PostgreSQL + Auth + RLS + Realtime |
| **Auth** | Supabase Auth | Email/password + GitHub OAuth, JWT токены |
| **Email** | AWS SES v2 | Дешево, масштабируемо, надежно |
| **MIME** | SES v2 Simple message + стандартная библиотека | Конструирование писем без лишнего runtime-пакета |
| **Очередь** | Redis | Async отправка, буферизация |
| **Frontend** | Next.js + TS + Tailwind v4 + shadcn | Современный DX, компоненты |
| **Мульти-тенантность** | Teams | Разделение данных по командам |
| **Inbound** | SES → S3 → SQS → Go Worker | Получение входящих писем |
| **Хранилище писем** | S3 | Raw email и вложения в S3 |
| **Домены** | Автоматическая верификация через DNS | TXT/CNAME записи |
| **Деплой** | Hetzner + Cloudflare | Собственный сервер, CDN/proxy |
| **Мониторинг** | Sentry + structured logs | Ошибки и логирование |
| **Шаблоны** | ❌ v2 | Отложено |
| **Планировщик** | ❌ v2 | Отложено |
| **Open/Click Tracking** | ❌ v2 | Отложено |
| **Оплата** | ❌ v2 | Пока внутренний инструмент |
| **Тесты** | ✅ реализовано | Unit-тесты, race checks и migration smoke check в CI |
| **CI/CD** | GitHub Actions | Проверки и CodeQL; production deploy остаётся ручным |

---

## Стек технологий

### Backend (Go)

| Компонент | Пакет | Назначение |
|-----------|-------|------------|
| Веб-фреймворк | `github.com/go-chi/chi/v5` | HTTP роутинг |
| AWS SDK | `github.com/aws/aws-sdk-go-v2` | SES v2, S3, SQS, SNS |
| MIME | SES v2 Simple message | Конструирование писем |
| PostgreSQL | `github.com/jackc/pgx/v5` | Direct SQL запросы |
| SQL | `pgx/v5` + repository SQL | Прямые параметризованные запросы |
| JWT | `github.com/golang-jwt/jwt/v5` | Верификация Supabase JWT |
| JWKS | `github.com/MicahParks/keyfunc/v3` | Асинхронная верификация ключей |
| Redis | `github.com/redis/go-redis/v9` | Очередь сообщений |
| Config | `github.com/joho/godotenv` | ENV переменные |
| Logger | `log/slog` (stdlib) | Структурированное логирование |
| UUID | `github.com/google/uuid` | Генерация UUID |
| Migrations | `golang-migrate` CLI | Версионируемые SQL-миграции |

### Frontend (Next.js)

| Компонент | Пакет | Назначение |
|-----------|-------|------------|
| Framework | `next` | React framework |
| UI | `shadcn/ui` | Компоненты |
| CSS | `tailwindcss` v4 | Утилитарный CSS |
| Auth | `@supabase/ssr` | Supabase auth для Next.js |
| Supabase | `@supabase/supabase-js` | Supabase клиент |
| Charts | `recharts` | Графики для дашборда |
| Forms | `react-hook-form` + `zod` | Валидация форм |

### Инфраструктура

| Компонент | Технология | Назначение |
|-----------|------------|------------|
| Сервер | Hetzner (VPS/Cloud) | Хостинг бэкенда + Worker |
| CDN/Proxy | Cloudflare | SSL, кеширование, DNS |
| Контейнеризация | Docker + Docker Compose | Локальная + продакшн |
| Мониторинг | Sentry | Ошибки (Go + Next.js) |
| Логирование | slog + Sentry | Структурированные логи |
| CI/CD | GitHub Actions | Test, race, vet, build, frontend checks, Compose и CodeQL |

---

## Архитектура

### Общая схема

```
┌─────────────────────────────────────────────────────────────────┐
│                        Frontend (Next.js)                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│  │  Login   │  │ Dashboard│  │ Contacts │  │ Settings │       │
│  │  Signup  │  │  Emails  │  │ Domains  │  │ API Keys │       │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘       │
│       │              │              │              │              │
└───────┼──────────────┼──────────────┼──────────────┼────────────┘
        │              │              │              │
        ▼              ▼              ▼              ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Go Backend API                              │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                    Middleware                             │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐              │   │
│  │  │ JWT Auth │  │ API Key  │  │Rate Limit│              │   │
│  │  └──────────┘  └──────────┘  └──────────┘              │   │
│  └─────────────────────────────────────────────────────────┘   │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│  │  Emails  │  │ Contacts │  │ Domains  │  │  Teams   │       │
│  │ Handlers │  │ Handlers │  │ Handlers │  │ Handlers │       │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘       │
│       │              │              │              │              │
│  ┌────▼──────────────▼──────────────▼──────────────▼────┐       │
│  │                    Services                           │       │
│  └────┬──────────────┬──────────────┬──────────────┬────┘       │
│       │              │              │              │              │
│  ┌────▼─────┐  ┌─────▼────┐  ┌─────▼────┐  ┌─────▼────┐       │
│  │ SES v2   │  │ Supabase │  │  Redis   │  │  S3/SQS  │       │
│  │ (mailer) │  │  (DB)    │  │ (queue)  │  │ (inbound)│       │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘       │
└─────────────────────────────────────────────────────────────────┘
        │                                    │
        ▼                                    ▼
┌──────────────────┐              ┌──────────────────┐
│   Amazon SES     │              │   Supabase       │
│  (отправка)      │              │  (PostgreSQL +   │
│                  │              │   Auth + RLS)    │
└──────────────────┘              └──────────────────┘
```

### Flow: Отправка письма

```
1. Клиент → POST /api/v1/emails
2. Middleware: проверка JWT/API ключа → определение team_id
3. Handler: валидация параметров
4. Service: проверка подписки (free/pro/scale), rate limit
5. Repository: сохранение в БД (status: queued)
6. Queue: добавление в Redis
7. Worker: извлечение из очереди
8. Mailer: отправка через SES v2 Simple message
9. Repository: обновление статуса (sent/failed)
10. Webhook: уведомление клиента о статусе
```

### Flow: Получение письма (Inbound)

```
1. Входящее письмо → Amazon SES
2. SES Receipt Rule → S3 Bucket (raw email)
3. SES → SQS Queue (уведомление)
4. Go Worker: извлечение из SQS
5. Worker: скачивание raw email из S3
6. Worker: парсинг MIME (net/mail)
7. Worker: определение team_id по recipient domain
8. Repository: сохранение в БД
9. Webhook: уведомление клиента
```

---

## Мульти-тенантность (Teams)

### Модель данных

```
User (Supabase Auth)
  │
  ├── team_members ──┐
  │                  │
  └──────────────────▼
                   Team
                     │
                     ├── api_keys
                     ├── domains
                     ├── emails
                     ├── contacts
                     └── webhooks
```

### Роли в команде

| Роль | Права |
|------|-------|
| `owner` | Полный доступ, удаление команды, управление подпиской |
| `admin` | Управление участниками, API ключами, доменами |
| `member` | Отправка писем, просмотр контактов |

### Подписки ( plans)

| План | Лимиты |
|------|--------|
| **Free** | 3,000 писем/мес, 1 домен, 100 контактов |
| **Pro** | 50,000 писем/мес, 10 доменов, 10,000 контактов |
| **Scale** | 500,000 писем/мес, 100 доменов, 100,000 контактов |

---

## Схема БД

### Tables

```sql
-- ============================================
-- TEAMS
-- ============================================
CREATE TABLE teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    plan VARCHAR(50) DEFAULT 'free' CHECK (plan IN ('free', 'pro', 'scale')),
    stripe_customer_id VARCHAR(255),
    stripe_subscription_id VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- TEAM MEMBERS
-- ============================================
CREATE TABLE team_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
    role VARCHAR(50) DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(team_id, user_id)
);

-- ============================================
-- API KEYS
-- ============================================
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    key_hash VARCHAR(255) NOT NULL,
    key_prefix VARCHAR(10) NOT NULL,
    permissions JSONB DEFAULT '["send"]',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

-- ============================================
-- DOMAINS
-- ============================================
CREATE TABLE domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) DEFAULT 'pending' CHECK (status IN ('pending', 'verified', 'failed')),
    verification_token VARCHAR(255),
    spf_status VARCHAR(50) DEFAULT 'pending',
    dkim_status VARCHAR(50) DEFAULT 'pending',
    dmarc_status VARCHAR(50) DEFAULT 'pending',
    dkim_dns_record TEXT,
    spf_dns_record TEXT,
    dmarc_dns_record TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(team_id, name)
);

-- ============================================
-- EMAILS
-- ============================================
CREATE TABLE emails (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    api_key_id UUID REFERENCES api_keys(id),
    from_addr VARCHAR(255) NOT NULL,
    to_addr JSONB NOT NULL,
    cc JSONB,
    bcc JSONB,
    subject TEXT NOT NULL,
    html TEXT,
    text TEXT,
    status VARCHAR(50) DEFAULT 'queued' CHECK (status IN (
        'queued', 'sending', 'sent', 'delivered', 'opened',
        'clicked', 'bounced', 'complained', 'failed', 'cancelled'
    )),
    tags JSONB DEFAULT '[]',
    metadata JSONB DEFAULT '{}',
    headers JSONB DEFAULT '{}',
    scheduled_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_emails_team_id ON emails(team_id);
CREATE INDEX idx_emails_status ON emails(status);
CREATE INDEX idx_emails_created_at ON emails(created_at DESC);

-- ============================================
-- EMAIL EVENTS
-- ============================================
CREATE TABLE email_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_id UUID REFERENCES emails(id) ON DELETE CASCADE,
    event VARCHAR(50) NOT NULL,
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    data JSONB DEFAULT '{}',
    ip_address INET,
    user_agent TEXT
);

CREATE INDEX idx_email_events_email_id ON email_events(email_id);
CREATE INDEX idx_email_events_event ON email_events(event);

-- ============================================
-- CONTACTS
-- ============================================
CREATE TABLE contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    subscribed BOOLEAN DEFAULT true,
    properties JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(team_id, email)
);

CREATE INDEX idx_contacts_team_id ON contacts(team_id);

-- ============================================
-- INBOUND EMAILS
-- ============================================
CREATE TABLE inbound_emails (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    message_id VARCHAR(255),
    from_addr VARCHAR(255) NOT NULL,
    to_addr JSONB NOT NULL,
    subject TEXT,
    text TEXT,
    html TEXT,
    attachments JSONB DEFAULT '[]',
    raw_s3_key VARCHAR(1024),
    headers JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_inbound_emails_team_id ON inbound_emails(team_id);

-- ============================================
-- WEBHOOKS
-- ============================================
CREATE TABLE webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    events TEXT[] NOT NULL,
    secret VARCHAR(255) NOT NULL,
    active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- RLS POLICIES
-- ============================================
ALTER TABLE teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE domains ENABLE ROW LEVEL SECURITY;
ALTER TABLE emails ENABLE ROW LEVEL SECURITY;
ALTER TABLE email_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE contacts ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbound_emails ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhooks ENABLE ROW LEVEL SECURITY;

-- Пользователь видит только свои команды
CREATE POLICY "Users can view own teams" ON teams
    FOR SELECT USING (
        id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
    );

-- Пользователь видит только участников своих команд
CREATE POLICY "Users can view team members" ON team_members
    FOR SELECT USING (
        team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
    );

-- Пользователь видит только email'ы своей команды
CREATE POLICY "Users can view team emails" ON emails
    FOR SELECT USING (
        team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
    );

-- Аналогично для всех таблиц...
```

---

## Структура проекта

```
sender-api/
├── cmd/
│   ├── api/
│   │   └── main.go              # Точка входа API сервера
│   └── worker/
│       └── main.go              # Worker для очереди
├── internal/
│   ├── domain/
│   │   ├── email.go             # Сущность Email
│   │   ├── team.go              # Сущность Team
│   │   ├── contact.go           # Сущность Contact
│   │   ├── apikey.go            # Сущность API Key
│   │   ├── domain.go            # Сущность Domain
│   │   ├── inbound.go           # Сущность InboundEmail
│   │   └── ports.go             # Интерфейсы (репозитории, сервисы)
│   ├── service/
│   │   ├── email_service.go     # Бизнес-логика отправки
│   │   ├── team_service.go      # Управление командами
│   │   ├── contact_service.go   # Управление контактами
│   │   ├── domain_service.go    # Верификация доменов
│   │   └── inbound_service.go   # Обработка входящих
│   ├── handler/
│   │   ├── email_handler.go     # POST /emails, GET /emails
│   │   ├── team_handler.go      # CRUD команд
│   │   ├── contact_handler.go   # CRUD контактов
│   │   ├── domain_handler.go    # CRUD доменов
│   │   ├── apikey_handler.go    # CRUD API ключей
│   │   ├── webhook_handler.go   # Клиентские webhooks
│   │   └── inbound_handler.go   # SES inbound webhook
│   ├── repository/
│   │   ├── email_repo.go        # Supabase/Postgres
│   │   ├── team_repo.go
│   │   ├── contact_repo.go
│   │   ├── domain_repo.go
│   │   └── apikey_repo.go
│   ├── mailer/
│   │   ├── ses.go               # Реализация EmailSender
│   │   └── mime.go              # Конструирование MIME
│   ├── queue/
│   │   └── redis.go             # Redis очередь
│   ├── worker/
│   │   └── email_worker.go      # Обработка очереди
│   ├── auth/
│   │   ├── middleware.go         # JWT + API Key верификация
│   │   └── jwt.go               # JWKS интеграция
│   └── config/
│       └── config.go            # Конфигурация из env
├── pkg/
│   ├── middleware/
│   │   ├── auth.go              # Аутентификация
│   │   ├── ratelimit.go         # Rate limiting
│   │   └── cors.go              # CORS
│   ├── apikey/
│   │   └── apikey.go            # Генерация, хеширование
│   └── validator/
│       └── validator.go         # Валидация данных
├── migrations/
│   ├── 001_create_teams.up.sql
│   ├── 001_create_teams.down.sql
│   ├── 002_create_team_members.up.sql
│   └── ...
├── queries/
│   ├── emails.sql               # SQL для sqlc
│   ├── teams.sql
│   ├── contacts.sql
│   ├── domains.sql
│   └── api_keys.sql
├── web/                          # Next.js frontend
│   ├── src/
│   │   ├── app/
│   │   │   ├── (auth)/
│   │   │   │   ├── login/page.tsx
│   │   │   │   ├── signup/page.tsx
│   │   │   │   └── layout.tsx
│   │   │   ├── (dashboard)/
│   │   │   │   ├── layout.tsx
│   │   │   │   ├── emails/
│   │   │   │   │   ├── page.tsx
│   │   │   │   │   └── [id]/page.tsx
│   │   │   │   ├── contacts/page.tsx
│   │   │   │   ├── inbound/page.tsx
│   │   │   │   ├── domains/page.tsx
│   │   │   │   ├── api-keys/page.tsx
│   │   │   │   ├── webhooks/page.tsx
│   │   │   │   └── settings/
│   │   │   │       ├── team/page.tsx
│   │   │   │       └── billing/page.tsx
│   │   │   ├── api/
│   │   │   │   └── auth/
│   │   │   │       └── callback/
│   │   │   │           └── route.ts
│   │   │   └── layout.tsx
│   │   ├── components/
│   │   │   ├── ui/              # shadcn компоненты
│   │   │   ├── emails/
│   │   │   │   ├── email-list.tsx
│   │   │   │   ├── email-detail.tsx
│   │   │   │   └── send-email-dialog.tsx
│   │   │   ├── contacts/
│   │   │   ├── domains/
│   │   │   └── settings/
│   │   └── lib/
│   │       ├── supabase/
│   │       │   ├── client.ts    # Browser client
│   │       │   └── server.ts    # Server client
│   │       ├── api.ts           # API клиент к Go бэкенду
│   │       └── utils.ts
│   ├── package.json
│   ├── next.config.ts
│   └── tailwind.config.ts
├── supabase/
│   ├── config.toml
│   └── migrations/
│       └── 20240101000000_initial_schema.sql
├── docker/
│   ├── Dockerfile.api
│   ├── Dockerfile.worker
│   └── Dockerfile.web
├── docker-compose.yml
├── docker-compose.dev.yml
├── Makefile
├── go.mod
├── go.sum
├── .env.example
└── README.md
```

---

## API Endpoints

### Базовый URL
```
http://localhost:8080/api/v1
```

### Аутентификация

Два типа ключей:

| Тип | Использование | Формат |
|-----|---------------|--------|
| **JWT (Supabase)** | Веб-интерфейс, мобильные приложения | `Bearer eyJhbGciOi...` |
| **API Key** | Серверные запросы, SDK | `Bearer re_xxxxxxxxx` |

### Teams

| Метод | Endpoint | Описание | Роль |
|-------|----------|----------|------|
| `POST` | `/teams` | Создать команду | Auth user |
| `GET` | `/teams` | Список команд пользователя | Auth user |
| `GET` | `/teams/:id` | Детали команды | member+ |
| `PATCH` | `/teams/:id` | Обновить команду | owner/admin |
| `DELETE` | `/teams/:id` | Удалить команду | owner |
| `POST` | `/teams/:id/invite` | Пригласить участника | owner/admin |
| `DELETE` | `/teams/:id/members/:userId` | Удалить участника | owner/admin |
| `PATCH` | `/teams/:id/members/:userId/role` | Изменить роль | owner |

### Emails

| Метод | Endpoint | Описание |
|-------|----------|----------|
| `POST` | `/emails` | Отправить письмо |
| `POST` | `/emails/batch` | Batch отправка (до 100) |
| `GET` | `/emails` | Список писем (пагинация) |
| `GET` | `/emails/:id` | Детали письма |
| `GET` | `/emails/:id/events` | События письма |
| `DELETE` | `/emails/:id` | Отменить запланированное |

### Contacts

| Метод | Endpoint | Описание |
|-------|----------|----------|
| `POST` | `/contacts` | Добавить контакт |
| `GET` | `/contacts` | Список контактов |
| `GET` | `/contacts/:id` | Детали контакта |
| `PATCH` | `/contacts/:id` | Обновить контакт |
| `DELETE` | `/contacts/:id` | Удалить контакт |
| `POST` | `/contacts/import` | Импорт CSV |

### Domains

| Метод | Endpoint | Описание |
|-------|----------|----------|
| `POST` | `/domains` | Добавить домен |
| `GET` | `/domains` | Список доменов |
| `GET` | `/domains/:id` | Детали домена |
| `POST` | `/domains/:id/verify` | Верифицировать домен |
| `DELETE` | `/domains/:id` | Удалить домен |

### API Keys

| Метод | Endpoint | Описание |
|-------|----------|----------|
| `POST` | `/api-keys` | Создать ключ |
| `GET` | `/api-keys` | Список ключей |
| `DELETE` | `/api-keys/:id` | Удалить ключ |

### Webhooks (клиентские)

| Метод | Endpoint | Oписание |
|-------|----------|----------|
| `POST` | `/webhooks` | Создать webhook |
| `GET` | `/webhooks` | Список webhook'ов |
| `GET` | `/webhooks/:id` | Детали webhook |
| `PATCH` | `/webhooks/:id` | Обновить webhook |
| `DELETE` | `/webhooks/:id` | Удалить webhook |

### Inbound (SES)

| Метод | Endpoint | Описание |
|-------|----------|----------|
| `POST` | `/inbound/ses` | SES notification endpoint |

---

## Аутентификация и авторизация

### Supabase Auth

```typescript
// web/src/lib/supabase/client.ts
import { createBrowserClient } from '@supabase/ssr'

export function createClient() {
  return createBrowserClient(
    process.env.NEXT_PUBLIC_SUPABASE_URL!,
    process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY!
  )
}
```

### JWT верификация в Go

```go
// internal/auth/middleware.go
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        authHeader := r.Header.Get("Authorization")

        // Проверяем JWT (Supabase Auth)
        if strings.HasPrefix(authHeader, "Bearer ey") {
            claims, err := verifySupabaseJWT(authHeader)
            if err != nil {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }

            // Определяем team_id из claims
            teamID := getTeamFromClaims(claims)
            ctx := context.WithValue(r.Context(), "team_id", teamID)
            ctx = context.WithValue(ctx, "user_id", claims.Sub)
            ctx = context.WithValue(ctx, "role", "user")
            next.ServeHTTP(w, r.WithContext(ctx))
            return
        }

        // Проверяем API ключ
        if strings.HasPrefix(authHeader, "Bearer re_") {
            teamID, err := verifyAPIKey(authHeader)
            if err != nil {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), "team_id", teamID)
            ctx = context.WithValue(ctx, "role", "api_key")
            next.ServeHTTP(w, r.WithContext(ctx))
            return
        }

        http.Error(w, "Unauthorized", http.StatusUnauthorized)
    })
}
```

### Supabase JWT верификация

```go
// internal/auth/jwt.go
import (
    "github.com/golang-jwt/jwt/v5"
    "github.com/MicahParks/keyfunc/v3"
)

type SupabaseClaims struct {
    Sub   string `json:"sub"`
    Email string `json:"email"`
    Role  string `json:"role"`
    jwt.RegisteredClaims
}

var jwks *keyfunc.JWKS

func InitJWT(supabaseURL string) error {
    jwksURL := supabaseURL + "/auth/v1/.well-known/jwks.json"
    var err error
    jwks, err = keyfunc.Get(jwksURL, keyfunc.Options{
        Client:    http.DefaultClient,
        RefreshInterval: 10 * time.Minute,
    })
    return err
}

func verifySupabaseJWT(tokenString string) (*SupabaseClaims, error) {
    claims := &SupabaseClaims{}
    token, err := jwt.ParseWithClaims(tokenString, claims, jwks.Keyfunc)
    if err != nil || !token.Valid {
        return nil, fmt.Errorf("invalid token")
    }
    return claims, nil
}
```

### API Key генерация

```go
// pkg/apikey/apikey.go
func GenerateAPIKey() (raw string, hash string, prefix string, err error) {
    // Генерируем случайные 32 байта
    bytes := make([]byte, 32)
    _, err = rand.Read(bytes)
    if err != nil {
        return "", "", "", err
    }

    // Формат: re_<base64>
    raw = "re_" + base64.RawURLEncoding.EncodeToString(bytes)

    // Хеш для хранения в БД
    hash = sha256Hex(raw)

    // Префикс для отображения
    prefix = raw[:10] + "..."

    return raw, hash, prefix, nil
}
```

---

## Amazon SES: Sandbox vs Production

### Что такое SES Sandbox?

Когда вы впервые создаете аккаунт SES, он находится в **Sandbox** (песочнице):

| Режим | Ограничения |
|-------|-------------|
| **Sandbox** | Отправка только на **verified emails** (макс 200). Лимит: 200 писем/день. |
| **Production** | Отправка на **любые email**. Лимит: 50,000+/день (растет со временем). |

### Как перейти в Production

1. Зайти в AWS SES Console
2. Request production access
3. Указать use case (transactional emails)
4. Подтвердить email для уведомлений
5. Ждать 24-48 часов (обычно одобряют быстро)

### Рекомендация

Для начала используем **Sandbox** — подходит для разработки и тестов. Когда будем деплоить на прод, запросим Production access.

---

## Отправка писем

### SES v2 отправка

```go
// internal/mailer/ses.go
import (
    "github.com/aws/aws-sdk-go-v2/service/sesv2"
    "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

type SESMailer struct {
    client *sesv2.Client
}

func (m *SESMailer) Send(ctx context.Context, email *domain.Email) error {
    input := &sesv2.SendEmailInput{
        FromEmailAddress: aws.String(email.From),
        Destination: &types.Destination{
            ToAddresses: email.To,
            CcAddresses: email.CC,
            BccAddresses: email.BCC,
        },
        Content: &types.EmailContent{
            Simple: &types.Message{
                Subject: &types.Content{
                    Data: aws.String(email.Subject),
                },
                Body: &types.Body{
                    Html: &types.Content{
                        Data: aws.String(email.HTML),
                    },
                    Text: &types.Content{
                        Data: aws.String(email.Text),
                    },
                },
            },
        },
        // ConfigurationSetName: aws.String("tracking"),  // для open/click tracking
    }

    _, err := m.client.SendEmail(ctx, input)
    return err
}
```

### MIME и вложения

Текущая реализация использует `sesv2.SendEmail` с `Simple`-сообщением:
текстовая и HTML-части, вложения, кастомные заголовки и Configuration Set
собираются в `internal/mailer/ses.go`. Отдельный `go-mail` runtime-пакет не
нужен; ограничения размера и имена файлов проверяются до постановки письма
в очередь.

### Redis очередь

```go
// internal/queue/redis.go
import "github.com/redis/go-redis/v9"

type EmailQueue struct {
    client *redis.Client
}

func (q *EmailQueue) Enqueue(ctx context.Context, emailID string) error {
    return q.client.LPush(ctx, "emails:pending", emailID).Err()
}

func (q *EmailQueue) Dequeue(ctx context.Context) (string, error) {
    return q.client.BRPop(ctx, 0, "emails:pending").Result()
}
```

---

## Получение писем (Inbound)

### Архитектура

```
Входящее письмо
       │
       ▼
┌──────────────┐
│ Amazon SES   │ ← MX запись: inbound-smtp.us-east-1.amazonaws.com
└──────┬───────┘
       │ Receipt Rule
       ▼
┌──────────────┐     ┌──────────────────┐
│ S3 Bucket    │────▶│ SQS Queue        │
│ (raw email)  │     │ (уведомление)    │
└──────────────┘     └────────┬─────────┘
                              │
                              ▼
                     ┌──────────────────┐
                     │ Go Worker        │
                     │ (парсинг + store)│
                     └──────────────────┘
```

### SES Receipt Rule (AWS)

```json
{
  "Name": "inbound-emails",
  "Enabled": true,
  "Recipients": ["*@yourdomain.com"],
  "Actions": [
    {
      "S3Action": {
        "BucketName": "sender-api-inbound",
        "ObjectKeyPrefix": "raw/"
      }
    },
    {
      "SqsAction": {
        "QueueUrl": "https://sqs.us-east-1.amazonaws.com/ACCOUNT/inbound-emails",
        "TopicArn": "arn:aws:sns:us-east-1:ACCOUNT:inbound-notifications"
      }
    }
  ],
  "ScanEnabled": true
}
```

### Go Worker

```go
// internal/worker/inbound_worker.go
import (
    "github.com/aws/aws-sdk-go-v2/service/s3"
    "net/mail"
)

func (w *InboundWorker) ProcessMessage(ctx context.Context, msg *sqs.Message) error {
    // 1. Парсим SES notification
    var notification SESNotification
    json.Unmarshal([]byte(msg.Body), &notification)

    // 2. Скачиваем raw email из S3
    rawEmail, err := w.s3Client.GetObject(ctx, &s3.GetObjectInput{
        Bucket: aws.String("sender-api-inbound"),
        Key:    aws.String(notification.ObjectKey),
    })
    if err != nil {
        return err
    }

    // 3. Парсим MIME
    msg, err := mail.ReadMessage(rawEmail.Body)
    if err != nil {
        return err
    }

    // 4. Определяем team_id по recipient domain
    teamID, err := w.domainRepo.GetTeamByDomain(ctx, extractDomain(msg.Header.Get("To")))

    // 5. Сохраняем в БД
    inbound := &domain.InboundEmail{
        TeamID:    teamID,
        From:      msg.Header.Get("From"),
        To:        parseAddressList(msg.Header.Get("To")),
        Subject:   msg.Header.Get("Subject"),
        Headers:   parseHeaders(msg.Header),
        RawS3Key:  notification.ObjectKey,
    }

    return w.inboundRepo.Create(ctx, inbound)
}
```

---

## Верификация доменов

### Автоматическая верификация

При добавлении домена генерируются DNS записи:

```json
{
  "domain": "example.com",
  "verification_token": "sender-api-verify-abc123",
  "dns_records": [
    {
      "type": "TXT",
      "host": "@",
      "value": "v=spf1 include:amazonses.com ~all",
      "status": "pending"
    },
    {
      "type": "CNAME",
      "host": "selector._domainkey",
      "value": "dkim.amazonses.com",
      "status": "pending"
    },
    {
      "type": "MX",
      "host": "feedback-smtp",
      "value": "feedback-smtp.us-east-1.amazonses.com",
      "priority": 10,
      "status": "pending"
    },
    {
      "type": "TXT",
      "host": "_dmarc",
      "value": "v=DMARC1; p=none; rua=mailto:dmarc@example.com",
      "status": "pending"
    }
  ]
}
```

### Верификация через DNS

```go
// internal/service/domain_service.go
func (s *DomainService) Verify(ctx context.Context, domainID string) error {
    domain, err := s.domainRepo.GetByID(ctx, domainID)
    if err != nil {
        return err
    }

    // Проверяем TXT запись (SPF)
    txtRecords, err := net.LookupTXT(domain.Name)
    if err == nil && containsSPF(txtRecords) {
        domain.SPFStatus = "verified"
    }

    // Проверяем CNAME (DKIM)
    cnameRecords, err := net.LookupCNAME(domain.DKIMDNSRecord)
    if err == nil {
        domain.DKIMStatus = "verified"
    }

    // Проверяем MX (для inbound)
    mxRecords, err := net.LookupMX(domain.Name)
    if err == nil && containsSES MX(mxRecords) {
        domain.MXStatus = "verified"
    }

    // Общий статус
    if domain.SPFStatus == "verified" && domain.DKIMStatus == "verified" {
        domain.Status = "verified"
    }

    return s.domainRepo.Update(ctx, domain)
}
```

### API ответ при добавлении домена

```json
{
  "id": "dom_abc123",
  "name": "example.com",
  "status": "pending",
  "dns_records": [
    {
      "type": "TXT",
      "host": "@",
      "value": "v=spf1 include:amazonses.com ~all",
      "ttl": 300
    },
    {
      "type": "CNAME",
      "host": "em123._domainkey",
      "value": "em123.dkim.amazonses.com",
      "ttl": 300
    },
    {
      "type": "TXT",
      "host": "em123._domainkey",
      "value": "p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQ...",
      "ttl": 300
    }
  ],
  "instructions": "Добавьте указанные DNS записи в настройках вашего домена. Верификация произойдет автоматически в течение 72 часов.",
  "created_at": "2024-01-15T10:30:00Z"
}
```

---

## Webhooks

### Клиентские webhooks

Пользователь настраивает webhook для получения событий:

```go
// internal/handler/webhook_handler.go
func (h *WebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
    var req struct {
        URL    string   `json:"url"`
        Events []string `json:"events"`
    }

    // Валидация events
    validEvents := map[string]bool{
        "email.sent": true, "email.delivered": true,
        "email.opened": true, "email.clicked": true,
        "email.bounced": true, "email.complained": true,
        "inbound.received": true,
    }

    // Сохранение с secret для верификации
    secret := generateSecret()
    webhook := &domain.Webhook{
        TeamID:  teamID,
        URL:     req.URL,
        Events:  req.Events,
        Secret:  secret,
        Active:  true,
    }
}
```

### Верификация webhook'ов

```go
// pkg/webhook/verify.go
func VerifyWebhook(payload []byte, signature string, secret string) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(payload)
    expected := hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(expected), []byte(signature))
}
```

---

## Rate Limits и лимиты

### По планам

| План | Писем/мес | Писем/день | Rate Limit |
|------|-----------|------------|------------|
| Free | 3,000 | 100 | 10 req/s |
| Pro | 50,000 | ∞ | 50 req/s |
| Scale | 500,000 | ∞ | 200 req/s |

### Реализация

```go
// pkg/middleware/ratelimit.go
func RateLimit(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        teamID := getTeamID(r)

        // Проверяем лимит в Redis
        key := fmt.Sprintf("ratelimit:%s", teamID)
        count, err := redis.Incr(ctx, key).Result()

        if count > getLimit(teamID) {
            http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
            return
        }

        // TTL для окна (1 секунда)
        if count == 1 {
            redis.Expire(ctx, key, time.Second)
        }

        next.ServeHTTP(w, r)
    })
}
```

---

## Фронтенд

### Страницы

| Путь | Описание |
|------|----------|
| `/login` | Вход (email/password) |
| `/signup` | Регистрация |
| `/auth/callback` | Supabase OAuth callback |
| `/` | Dashboard (список писем) |
| `/emails` | Все письма |
| `/emails/:id` | Детали письма |
| `/inbound` | Входящие письма |
| `/contacts` | Контакты |
| `/domains` | Домены |
| `/api-keys` | API ключи |
| `/webhooks` | Webhooks |
| `/settings/team` | Настройки команды |
| `/settings/billing` | Подписка |

### Компоненты (shadcn)

- `Button`, `Input`, `Card`, `Dialog`, `DropdownMenu`
- `Table`, `DataTable` (с пагинацией)
- `Tabs`, `Badge`, `Toast`, `Alert`
- `Form` (react-hook-form + zod)

---

## Инфраструктура

### Docker Compose ( development)

```yaml
services:
  api:
    build:
      context: .
      dockerfile: docker/Dockerfile.api
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgresql://supabase_admin:postgres@db:5432/sender_api
      - REDIS_URL=redis:6379
      - AWS_REGION=us-east-1
      - SUPABASE_URL=http://localhost:54321
      - SUPABASE_ANON_KEY=...
    depends_on:
      - db
      - redis

  worker:
    build:
      context: .
      dockerfile: docker/Dockerfile.worker
    environment:
      - DATABASE_URL=postgresql://supabase_admin:postgres@db:5432/sender_api
      - REDIS_URL=redis:6379
      - AWS_REGION=us-east-1
    depends_on:
      - db
      - redis

  web:
    build:
      context: web
      dockerfile: ../docker/Dockerfile.web
    ports:
      - "3000:3000"
    environment:
      - NEXT_PUBLIC_SUPABASE_URL=http://localhost:54321
      - NEXT_PUBLIC_SUPABASE_ANON_KEY=...

  db:
    image: supabase/postgres:17.6.1.136
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_PASSWORD=postgres
      - POSTGRES_DB=sender_api

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
```

---

## Этапы реализации

Галочка означает реализованный и проверенный локальный код. Пункты о
внешних аккаунтах и production-инфраструктуре остаются открытыми до
фактической настройки и live-проверки.

### Этап 1: Инфраструктура (1-2 дня)

- [x] Инициализация Go модуля
- [x] Настройка Docker Compose
- [x] Supabase-совместимая схема + миграции
- [x] Redis подключение
- [x] Конфигурация (env)
- [x] Sentry интеграция

### Этап 2: Auth + Teams (2-3 дня)

- [x] Supabase Auth contract (email/password + GitHub — внешний Supabase setup)
- [x] JWT middleware в Go
- [x] API Key генерация
- [x] CRUD команд
- [x] Участники команд

### Этап 3: Domains (1-2 дня)

- [x] CRUD доменов
- [x] Генерация DNS записей
- [x] Автоматическая верификация
- [ ] SES domain setup

### Этап 4: Email Sending (3-4 дня)

- [x] SES v2 интеграция
- [x] Redis очередь
- [x] Worker для отправки
- [x] Статусы писем
- [x] Теги и метаданные

### Этап 5: Contacts (1-2 дня)

- [x] CRUD контактов
- [x] Импорт CSV
- [x] Подписки (subscribed)

### Этап 6: Inbound (2-3 дня)

- [ ] SES Receipt Rules (внешняя AWS настройка)
- [x] S3 bucket adapter
- [x] SQS очередь adapter
- [x] Go worker для парсинга
- [x] Сохранение в БД

### Этап 7: Webhooks (1-2 дня)

- [x] CRUD клиентских webhook'ов
- [x] Верификация подписи
- [x] Durable отправка событий
- [x] Retries

### Этап 8: Frontend (5-7 дней)

- [x] Layout + навигация
- [x] Auth страницы
- [x] Dashboard (список писем)
- [x] Страницы: emails, contacts, domains, api-keys
- [x] Настройки команды

### Этап 9: Деплой (2-3 дня)

- [ ] Hetzner сервер настройка (внешняя операция)
- [x] Docker Compose production
- [ ] Cloudflare proxy + SSL
- [ ] Sentry production настройка

### Этап 10: Polish (2-3 дня)

- [x] Error handling
- [x] Logging
- [x] Documentation

---

*План актуален: июль 2026*
