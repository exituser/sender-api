# Sender API

> Email API for developers — open-source alternative to Resend, built on Amazon SES.

[![License](LICENSE)](LICENSE)

[Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Support](SUPPORT.md)

## Quick Start

### Prerequisites

- Go 1.26+
- Node.js 24+ (for the web app)
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

Machine-readable contract: [`openapi.yaml`](openapi.yaml). Runtime probes are
available at `/health`, `/readyz`, and `/metrics`.

### Authentication

Two auth methods:

| Type | Format | Use Case |
|------|--------|----------|
| JWT (Supabase) | `Bearer eyJhbGci...` | Web/mobile apps |
| API Key | `Bearer re_xxxxx` | Server-side, SDK |

JWT requests to team-scoped endpoints must include `X-Team-ID`. API keys are
bound to one team and use the permissions stored with the key (`send`, `read`,
or `*`). Requests without a verified team context are rejected.

`POST /emails` accepts an optional `Idempotency-Key` header. Reusing a key with
different request data returns `409`; reusing it with the same data returns the
original email ID without queuing a second message. The `From` domain must be
verified and owned by the active team.

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
| `POST` | `/inbound/ses` | SES/SNS inbound notification endpoint; legacy direct payloads require `X-Inbound-Token` |
| `GET` | `/inbound` | Authenticated, team-scoped inbound message list |

SES/SNS `Notification` payloads are verified against the AWS SNS signing
certificate and a five-minute timestamp window. Configure
`INBOUND_S3_BUCKET`, `INBOUND_SQS_QUEUE_URL`, and the exact
`INBOUND_SNS_TOPIC_ARN` to enable the worker path that verifies SNS, downloads
raw messages from S3, and acknowledges SQS only after a successful database
write. Direct raw-payload calls are retained for local or legacy integrations
and require `X-Inbound-Token`; they are not a substitute for SNS verification
in production.

The inbound adapter is optional. Leave `INBOUND_SQS_QUEUE_URL` and
`INBOUND_SNS_TOPIC_ARN` empty for an outbound-only deployment; the API does not
register the inbound callback unless the SQS adapter or direct webhook token is
configured.

Outbound SES event publishing can use `POST /api/v1/webhooks/ses`. Configure an
SES Configuration Set SNS destination and set `OUTBOUND_SES_TOPIC_ARN`; the
endpoint verifies the SNS signature and correlates the SES `mail.messageId`
returned by SES with the stored email. SES publishes event types such as Send,
Delivery, Bounce, Complaint, Open, and Click through event publishing.
This integration is optional. When `OUTBOUND_SES_TOPIC_ARN` is empty, the SES
event callback is not registered. When it is set, the exact topic ARN is still
checked, so disabling the integration does not weaken verification for enabled
callbacks.

### Low-volume profile

For a small outbound workload, keep only SES, the application host, and the
database/authentication service enabled. The values below reduce idle resource
usage without changing delivery retries or email validation:

```dotenv
AWS_SES_CONFIGSET=
OUTBOUND_SES_TOPIC_ARN=
INBOUND_SQS_QUEUE_URL=
INBOUND_SNS_TOPIC_ARN=
SENTRY_TRACES_SAMPLE_RATE=0
DB_MAX_CONNS=4
DB_MIN_CONNS=0
REDIS_POOL_SIZE=4
WORKER_POLL_INTERVAL=5s
```

The worker interval trades up to a few seconds of idle webhook/scheduled-email
latency for fewer database and Redis polls. Set it to `1s` when latency matters
more than idle cost. Error reporting remains available with `SENTRY_DSN`; only
distributed tracing is disabled by the low-volume default.

### Migrations

The first migration creates a fresh database schema. `002_delivery` is kept as
a compatibility migration, and `003_hardening` adds tenant ownership,
idempotency, indexes, inbound deduplication, sending recovery timestamps, and
durable webhook deliveries.
Apply all migrations with:

```bash
DATABASE_URL=postgresql://supabase_admin:postgres@localhost:5432/sender_api make migrate-up
```

The Compose database is initialized from `migrations/001_initial.up.sql` and
`migrations/003_hardening.up.sql`; its `schema_migrations` marker is set to
version 3. Use the migration command for an already-existing database. Compose
initialization only runs for a new database volume. Existing installations with
duplicate normalized domains or nullable tenant IDs must be reviewed before
applying `003_hardening`; the migration fails rather than silently rewriting
them.

## Quality checks

The backend and frontend checks used by CI can be run locally:

The frontend `npm run lint` command uses Oxlint with TypeScript, React,
Next.js, JSX accessibility, and import checks enabled.

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
