# Production readiness

This document is the release gate for a real-user deployment. Local Docker
health is not production proof: a release is ready only when the external
provider, DNS, TLS, backups, and a real smoke test are verified together.

## Implemented in the repository

- Production configuration rejects debug mode, localhost defaults, wildcard or
  HTTP CORS origins, and missing operational integration settings.
- `/metrics` is protected by `METRICS_TOKEN` when configured; API and web
  responses include baseline security headers.
- Per-team daily recipient quotas are reserved atomically in Redis and released
  when persistence or queueing fails using the original reservation date. A
  rejected quota returns HTTP 429.
- Email `sending` records are checked periodically while the worker is running;
  stale unknown-outcome sends fail closed instead of being resubmitted, while
  provider-accepted sends are persisted atomically when the repository supports
  it.
- Production webhook creation, updates, and delivery require HTTPS and reject
  private, special-use, and link-local targets.
- Supabase `anon` and `authenticated` roles have no privileges on application
  tables; the Go API is the public data boundary.
- Cryptographically invalid or future-dated SNS messages are acknowledged
  instead of being retried forever. Signed notifications delayed by the
  asynchronous SQS transport are accepted and deduplicated by provider/message
  IDs. Unroutable inbound domains remain retryable and are eventually moved to
  the SQS DLQ for operator review.
- Bounce and complaint callbacks create team-scoped suppressions before a new
  send is queued; provider-accepted sends are acknowledged as terminal when
  persistence cannot be completed, preventing an automatic duplicate send.
- Unsubscribed contacts are rejected both when queued and again immediately
  before provider submission; CSV imports preserve an explicit `subscribed`
  column. Outbound domain verification does not require changing existing MX
  records; MX verification remains an inbound-only requirement.
- Marketing messages are single-recipient, require a verified DMARC policy, and
  receive RFC 2369/8058 one-click unsubscribe headers. The public GET endpoint
  is non-mutating to avoid link-scanner unsubscribes; POST records a durable
  team-scoped unsubscribe suppression.
- SES delivery failures expose RFC 3463-style normalized status metadata in
  event payloads while preserving the original AWS provider code.
- Email events and webhook deliveries are committed in one database
  transaction. Failed email queue items are bounded, team-scoped, and can be
  replayed by an owner or admin through the API.
- Batch manifests prevent reusing one batch idempotency key with a different
  payload. Production upgrades use `scripts/migrate.sh`, which applies pending
  migrations and fails closed on a dirty schema state.
- Team invitations are single-use, email-bound, seven-day tokens; Stripe
  checkout, portal sessions, webhook signature verification, and plan-specific
  recipient limits are implemented but remain optional integrations. Stripe
  events are deduplicated and stale events cannot overwrite newer billing state;
  unknown prices fail closed to the free plan.
- Batch sends require an idempotency key and return HTTP 207 with accepted item
  IDs when a later item fails. Webhook consumers must deduplicate delivery IDs
  because delivery is at-least-once.
- `make backup` creates a permission-restricted custom-format PostgreSQL dump
  from either `DATABASE_URL` or the running Compose `db` service, and validates
  the archive header before reporting success.
  Restore is deliberately gated by `CONFIRM_RESTORE=YES`.
- `infra/systemd/sender-api-backup.service` and `.timer` provide a daily local
  backup schedule with 14-day rotation under `/home/ubuntu/sender-api-backups`.
  Install them on the host, then copy
  the resulting dumps to separate storage and test a restore before launch.
  The backup host must not be the only copy.
- The worker purges terminal outbound messages after `EMAIL_RETENTION_DAYS`.
  For inbound mail it deletes the raw S3 object first and removes the database
  row only after S3 confirms deletion, so a transient object-store failure does
  not orphan an undiscoverable copy. Set either retention value to `0` only
  when an approved policy requires indefinite storage.
- Failed webhook deliveries can be replayed by an owner or admin after the
  endpoint has been repaired; delivery remains at-least-once.

## External release gates

These cannot be safely faked in source code and must be checked in the target
AWS/Supabase/hosting accounts:

1. SES production access is approved in `eu-west-1`; sandbox sending is not a
   real-user launch state.
2. The public web/API hostname has TLS termination, a recovery redirect URL,
   and CORS values containing only the real HTTPS web origin.
3. All production environment values pass `config.Validate`, including the
   metrics token and daily limit. If inbound or outbound SNS integrations are
   enabled, their exact queue/topic/bucket/configuration-set values must also
   be present and verified.
   For Compose, use `COMPOSE_DATABASE_URL` and `COMPOSE_REDIS_URL`; the
   host-run `DATABASE_URL` and `REDIS_URL` values must not be copied into
   containers when they point at `localhost`.
   With managed datastores, start only the application path so unused local
   containers are not created:

   ```bash
   docker compose up -d migrate db-role api worker web
   ```

   The migration preflight and application readiness checks connect to the
   configured URLs directly, so local `db` or `redis` health cannot block that
   rollout.
4. The sender domain is verified in SES and DNS. Inbound domains must also have
   the SES MX record and an active receipt rule set. Confirm the recipient before
   activating mail flow.
5. If outbound SES events are enabled, they have a configuration-set SNS
   destination and a public HTTPS callback. An outbound-only deployment may
   intentionally leave `OUTBOUND_SES_TOPIC_ARN` blank, but must keep
   `REQUIRE_OUTBOUND_SES_EVENTS=false`.
6. A backup is stored outside the application host, a restore has been tested
   into an isolated database, and the RPO/RTO are written down.
   The inbound S3 principal must have `s3:DeleteObject`, and the bucket should
   also enforce a lifecycle expiration matching the approved retention period
   as defense in depth.
7. Abuse controls are reviewed: daily quota, request rate limit, API-key
   rotation, bounce/complaint handling, and a support path. If Stripe is
   enabled, verify live price IDs, webhook delivery, cancellation behavior,
   and that unpaid subscriptions fall back to the free limit.
8. Run the end-to-end smoke test below with a verified sender and a disposable
   recipient, then confirm the SES message ID, worker status, and callback
   event if event publishing is enabled.

## Safe local checks

```bash
CORS_ORIGINS=https://app.example.com \
AWS_REGION=eu-west-1 \
SUPABASE_URL=https://project.supabase.co \
METRICS_TOKEN=local-check-token \
POSTGRES_PASSWORD=local-check-password \
APP_DB_PASSWORD=local-app-password \
NEXT_PUBLIC_API_URL=https://api.example.com \
NEXT_PUBLIC_SUPABASE_URL=https://project.supabase.co \
NEXT_PUBLIC_SUPABASE_ANON_KEY=local-check-anon \
docker compose config --quiet
docker compose -f docker-compose.dev.yml config --quiet
go test ./cmd/... ./internal/... ./pkg/...
go test -race ./cmd/... ./internal/... ./pkg/...
go vet ./cmd/... ./internal/... ./pkg/...
go build ./cmd/...

cd web
npm ci
npm run lint
npx tsc --noEmit
npm run build
npm audit --omit=dev --audit-level=high
```

For the local stack, verify `/health`, `/readyz`, `/healthz`, the worker logs,
and that the database migration marker is clean. Never use `docker compose down
-v` against a database containing data unless the volume deletion is the
explicit, reviewed operation.

## Compose backup and restore

With the production stack running, the backup path needs no host PostgreSQL
client or database password in the shell environment:

```bash
BACKUP_DIR=/var/backups/sender-api make backup
pg_restore --list /var/backups/sender-api/sender-api-<timestamp>.dump >/dev/null
```

Copy each dump to separate storage before rotating local files. For a restore,
use an isolated database or maintenance window and require the explicit guard:

```bash
CONFIRM_RESTORE=YES \
BACKUP_FILE=/var/backups/sender-api/sender-api-<timestamp>.dump \
make restore
```

After restore, rerun the migration/schema preflight, verify `13|false`, and
exercise `/health`, `/readyz`, one authenticated read, and one disposable send
before returning traffic. Periodically restore a copied dump into an isolated
PostgreSQL instance and record the restore duration and schema checks.

## Inbound error interpretation

- `inbound recipient domain is not configured: example.com` means the domain is
  absent, pending, or not MX-verified for a team. The message is not routed;
  check the domain record and the active SES receipt rule.
- `stale SNS notification` means the signed notification has a future-dated
  timestamp or arrived through a synchronous callback outside its freshness
  window. Queue-delivered notifications use the asynchronous verifier and are
  not discarded only because they waited in SQS; the original raw message
  should remain available in S3 when the receipt rule stored it.
