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
# Production Compose requires explicit production values.
cp .env.production.example .env
docker compose up

# Or build individually
docker build -f docker/Dockerfile.api -t sender-api .
docker build -f docker/Dockerfile.worker -t sender-worker .
```

The production Compose file keeps host-run `DATABASE_URL` and `REDIS_URL`
separate from container service addresses. Set `COMPOSE_DATABASE_URL` and
`COMPOSE_REDIS_URL` explicitly when using managed production services; leave
them empty for the local `db` and `redis` services.

For local development use `docker-compose.dev.yml`; the main Compose file is
fixed to `ENV=production` and fails closed when its public URLs or metrics token
are missing.

## API Endpoints

Base URL: `http://localhost:8080/api/v1`

Machine-readable contract: [`openapi.yaml`](openapi.yaml). Runtime probes are
available at `/health`, `/readyz`, and `/metrics`. Set `METRICS_TOKEN` outside
local development; protected metrics require the `X-Metrics-Token` header.

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
verified and owned by the active team. Each team also has a configurable
`DAILY_RECIPIENT_LIMIT`; exhausted quotas return `429` and queue/database
failures release the reservation.

`POST /emails/batch` requires an `Idempotency-Key` header. The API derives a
stable key for each item, so retrying a partially completed batch does not queue
already accepted items again. A partial result returns HTTP `207` with accepted
item IDs and the failed item error.

### Emails

Email requests are `transactional` by default. Set `category` to `marketing`
for consent-based campaigns; those messages must have exactly one recipient,
the sender domain must have a valid DMARC policy, and the API adds RFC 2369 / RFC
8058 `List-Unsubscribe` and one-click headers. Configure `PUBLIC_API_URL` and
`UNSUBSCRIBE_SIGNING_SECRET` to enable public unsubscribe links. The GET link
only shows a confirmation page; the POST action performs the unsubscribe.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET/POST` | `/unsubscribe/:token` | Public confirmation and RFC 8058 one-click unsubscribe |
| `POST` | `/emails` | Send email |
| `POST` | `/emails/batch` | Batch send (max 100) |
| `GET` | `/emails/dead-letters` | List failed queue items for the team |
| `POST` | `/emails/dead-letters/:id/replay` | Replay a failed queue item |
| `GET` | `/emails` | List emails |
| `GET` | `/emails/:id` | Get email |
| `GET` | `/emails/:id/events` | Get email events |
| `DELETE` | `/emails/:id` | Cancel scheduled email |

### Overview

`GET /api/v1/dashboard/summary` returns a team-scoped overview for the
Dashboard: actions that need attention, sender setup, recent delivery numbers,
audience protection, app connections, and recent activity. It intentionally
returns customer-facing explanations instead of DNS, SMTP, AWS, or provider
error terminology.

### Teams

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/teams` | Create team |
| `GET` | `/teams` | List teams |
| `GET` | `/teams/:id` | Get team |
| `PATCH` | `/teams/:id` | Update team |
| `DELETE` | `/teams/:id` | Delete team |
| `POST` | `/teams/:id/invite` | Invite member |
| `GET` | `/teams/:id/members` | List members |
| `PATCH` | `/teams/:id/members/:userId/role` | Change member role |
| `DELETE` | `/teams/:id/members/:userId` | Remove member |
| `GET` | `/teams/:id/invitations` | List invitations |
| `DELETE` | `/teams/:id/invitations/:invitationId` | Revoke invitation |
| `POST` | `/teams/invitations/accept` | Accept invitation token |

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
| `GET` | `/webhooks/:id/deliveries` | List delivery attempts |
| `POST` | `/webhooks/:id/test` | Queue a test delivery |

### Inbound

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/inbound/ses` | SES/SNS inbound notification endpoint; legacy direct payloads require `X-Inbound-Token` |
| `GET` | `/inbound` | Authenticated, team-scoped inbound message list |
| `GET` | `/inbound/:id` | Get parsed message, headers, and attachment metadata |

### Billing

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/billing` | Get current plan and subscription state |
| `POST` | `/billing/checkout` | Create a Stripe Checkout session |
| `POST` | `/billing/portal` | Create a Stripe customer portal session |

SES/SNS `Notification` payloads are verified against the AWS SNS signing
certificate and a five-minute timestamp window. Configure
`INBOUND_S3_BUCKET`, `INBOUND_SQS_QUEUE_URL`, and the exact
`INBOUND_SNS_TOPIC_ARN` to enable the worker path that verifies SNS, downloads
raw messages from S3, and acknowledges SQS only after a successful database
write. Direct raw-payload calls are retained for local or legacy integrations
and require `X-Inbound-Token`; they are not a substitute for SNS verification
in production. S3-backed notifications must use the SQS worker path and are
rejected by the direct HTTP endpoint instead of being acknowledged as empty.

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
Set `REQUIRE_OUTBOUND_SES_EVENTS=true` when delivery, bounce, and complaint
callbacks are a production requirement; readiness then fails closed until the
topic is configured.

Bounce and complaint callbacks create a suppression for the affected recipient
within the owning team. Suppressed recipients are rejected before quota
reservation and queueing. Stripe billing is optional; when enabled, free/pro/
scale limits are selected from the verified subscription status rather than
from a user-controlled plan field.

SES/AWS failures are retained in failed/retrying event payloads with normalized
`smtp_code`, `enhanced_status_code`, `provider_code`, and `retryable` fields.
These are RFC 3463-style provider metadata; SES itself returns AWS error codes,
not SMTP wire responses.

In production, user webhooks must use HTTPS. HTTP webhook URLs remain available
only for local development, and private/link-local targets are rejected in all
environments. Delivery is at-least-once: consumers must deduplicate using
`X-Webhook-Delivery-ID` or the `id` field in the signed payload.

The web app includes Supabase password recovery at `/forgot-password`. Add the
public `/callback` and `/reset-password` URLs to the Supabase redirect allowlist
before enabling recovery for users.

### Low-volume profile

For local or low-volume outbound-only workloads, keep only SES, the application
host, and the database/authentication service enabled. The values below reduce
idle resource usage without changing delivery retries or email validation:

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
WEB_MEM_LIMIT=256m
WEB_CPUS=0.25
```

The worker interval trades up to a few seconds of idle webhook/scheduled-email
latency for fewer database and Redis polls. Set it to `1s` when latency matters
more than idle cost. Error reporting remains available with `SENTRY_DSN`; only
distributed tracing is disabled by the low-volume default. The web container
uses a lightweight `/healthz` endpoint that does not call Supabase, so health
checks do not create authentication traffic.

### AWS inbound messaging

The optional SES/SNS/SQS/S3 inbound transport is described in
[`infra/aws/README.md`](infra/aws/README.md). It provisions an encrypted,
private S3 bucket, SNS-to-SQS delivery with a dead-letter queue, and the
least-privilege policies required by the SES receipt rule. The receipt rule is
kept as an explicit rollout step because it changes mail routing for a verified
domain.

### Migrations

The first migration creates a fresh database schema. `002_delivery` is kept as
a compatibility migration, `003_hardening` adds tenant ownership,
idempotency, indexes, inbound deduplication, sending recovery timestamps, and
durable webhook deliveries, and `004_inbound_mx` adds the SES inbound MX
verification state. `005_public_access_hardening` removes application-table
privileges from Supabase's `anon` and `authenticated` roles because the Go API
is the public data boundary. `006_suppressions` adds team-scoped bounce and
complaint suppression, `007_team_invitations` adds expiring single-use team
invites, `008_ses_domain_identity` stores SES verification and DKIM records,
and `009_billing_state` stores Stripe subscription state and plan limits.
`010_billing_event_ordering` adds Stripe event deduplication and monotonic
state application; unknown Stripe prices fail closed to the free plan.
`011_batch_manifest` prevents a batch idempotency key from being reused with a
different payload. `012_marketing_unsubscribe` adds email categories and
unsubscribe suppressions.
Apply all migrations with:

```bash
DATABASE_URL=postgresql://supabase_admin:postgres@localhost:5432/sender_api make migrate-up
```

The migration command is the single upgrade path for existing and new
databases. Compose initializes a fresh volume through migration 012 and marks
it clean at version 12; existing installations must run the command during
deployment. Compose initialization only runs for a new database volume.
Existing installations with
duplicate normalized domains or nullable tenant IDs must be reviewed before
applying `003_hardening`; the migration fails rather than silently rewriting
them.

Stripe billing is optional. Set `STRIPE_SECRET_KEY`,
`STRIPE_WEBHOOK_SECRET`, the two recurring price IDs, and success/cancel/return
URLs to enable checkout and subscription callbacks. In production those URLs
must use HTTPS. Without those values the API keeps the free plan and does not
register the public Stripe callback.

For a release gate and backup/restore procedure, see
[`docs/production-readiness.md`](docs/production-readiness.md).

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
