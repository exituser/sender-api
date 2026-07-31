# Sender API

> Email API for developers — open-source alternative to Resend, built on Amazon SES.

[![CI](../../actions/workflows/ci.yml/badge.svg)](../../actions/workflows/ci.yml)
[![License](LICENSE)](LICENSE)

[Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Support](SUPPORT.md)

## Quick Start

### Prerequisites

- Go 1.25+
- Node.js 22+ (for the web app)
- Docker & Docker Compose
- PostgreSQL (or Supabase)
- Redis
- AWS account with SES access

### Local Development

```bash
# Clone and setup
cp .env.example .env
# Edit .env with your credentials

# Start the containerized development stack (PostgreSQL, Redis, API, worker, web)
docker compose -f docker-compose.dev.yml up -d

# Or run only infrastructure and start Go/Next processes on the host:
docker compose -f docker-compose.dev.yml up -d db redis
go run ./cmd/api

# Run worker (separate terminal)
go run ./cmd/worker

# Run the web app (separate terminal)
cd web && npm ci && npm run dev
```

### Docker

```bash
# Build and run everything
docker compose up

# Or build individually
docker build -f docker/Dockerfile.api -t sender-api .
docker build -f docker/Dockerfile.worker -t sender-worker .
```

## API Endpoints

Base URL: `http://localhost:8080/api/v1`

### Authentication

Two auth methods:

| Type | Format | Use Case |
|------|--------|----------|
| JWT (Supabase) | `Bearer eyJhbGci...` | Web/mobile apps |
| API Key | `Bearer re_xxxxx` | Server-side, SDK |

JWT requests to team-scoped endpoints must include `X-Team-ID`. API keys are
bound to one team and use the permissions stored with the key (`send`, `read`,
or `*`). Requests without a verified team context are rejected.

### Emails

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/emails` | Send email |
| `POST` | `/emails/batch` | Batch send (max 100) |
| `GET` | `/emails` | List emails |
| `GET` | `/emails/:id` | Get email |
| `GET` | `/emails/:id/events` | Get email events |
| `DELETE` | `/emails/:id` | Cancel scheduled email |

### Teams

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/teams` | Create team |
| `GET` | `/teams` | List teams |
| `GET` | `/teams/:id` | Get team |
| `PATCH` | `/teams/:id` | Update team |
| `DELETE` | `/teams/:id` | Delete team |
| `POST` | `/teams/:id/invite` | Invite member |
| `DELETE` | `/teams/:id/members/:userId` | Remove member |

### Contacts

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/contacts` | Create contact |
| `GET` | `/contacts` | List contacts |
| `GET` | `/contacts/:id` | Get contact |
| `PATCH` | `/contacts/:id` | Update contact |
| `DELETE` | `/contacts/:id` | Delete contact |
| `POST` | `/contacts/import` | Import CSV |

### Domains

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/domains` | Add domain |
| `GET` | `/domains` | List domains |
| `GET` | `/domains/:id` | Get domain |
| `POST` | `/domains/:id/verify` | Verify domain |
| `DELETE` | `/domains/:id` | Delete domain |

### API Keys

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api-keys` | Create key |
| `GET` | `/api-keys` | List keys |
| `DELETE` | `/api-keys/:id` | Delete key |

### Webhooks

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/webhooks` | Create webhook |
| `GET` | `/webhooks` | List webhooks |
| `GET` | `/webhooks/:id` | Get webhook |
| `PATCH` | `/webhooks/:id` | Update webhook |
| `DELETE` | `/webhooks/:id` | Delete webhook |

### Inbound

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/inbound/ses` | SES notification endpoint; requires `X-Inbound-Token` |
| `GET` | `/inbound` | Authenticated, team-scoped inbound message list |

Set `INBOUND_WEBHOOK_TOKEN` before configuring SES/SNS to call the POST
endpoint. The endpoint fails closed with `503` when the token is not
configured; it does not accept a JWT in place of the webhook token.
The current endpoint accepts an SES/SNS notification containing the raw
message; `INBOUND_S3_BUCKET` and `INBOUND_SQS_QUEUE_URL` are reserved settings
and are not consumed by this version. Native SNS signature verification and
S3/SQS ingestion are therefore not claimed as implemented.

### Migrations

The first migration creates a fresh database schema. `002_delivery` adds the
delivery fields to an existing installation. Apply both with:

```bash
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/sender_api make migrate-up
```

The Compose database is initialized from `migrations/001_initial.up.sql`; use
the migration command for an already-existing database.

## Quality checks

The backend and frontend checks used by CI can be run locally:

```bash
make test
go vet ./cmd/... ./internal/... ./pkg/...
go build ./cmd/...

cd web
npm ci
npm run lint
npx tsc --noEmit
npm run build
npm audit --omit=dev --audit-level=high
```

## Project Structure

```
sender-api/
├── cmd/
│   ├── api/main.go          # API server entry
│   └── worker/main.go       # Queue worker entry
├── internal/
│   ├── auth/                 # JWT + API key auth
│   ├── config/               # Configuration
│   ├── domain/               # Entities + interfaces
│   ├── handler/              # HTTP handlers
│   ├── mailer/               # SES email sender
│   ├── queue/                # Redis queue
│   ├── repository/           # PostgreSQL repos
│   ├── service/              # Business logic
│   └── worker/               # Queue worker
├── migrations/               # SQL migrations
├── web/                      # Next.js frontend
├── docker/                   # Dockerfiles
├── docker-compose.yml
└── Makefile
```

## Tech Stack

- **Backend**: Go + Chi router
- **Database**: PostgreSQL (Supabase)
- **Queue**: Redis
- **Email**: Amazon SES v2
- **Auth**: Supabase Auth (JWT)
- **Frontend**: Next.js + Tailwind CSS v4
- **Deploy**: Docker + Docker Compose

## License

This project is available under the [MIT License](LICENSE).
