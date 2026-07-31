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
  when persistence or queueing fails. A rejected quota returns HTTP 429.
- Email `sending` records are checked periodically while the worker is running;
  stale unknown-outcome sends fail closed instead of being resubmitted, while
  provider-accepted sends are persisted atomically when the repository supports
  it.
- Production webhook creation, updates, and delivery require HTTPS and still
  reject private or link-local targets.
- Supabase `anon` and `authenticated` roles have no privileges on application
  tables; the Go API is the public data boundary.
- Stale or cryptographically invalid SNS messages are acknowledged instead of
  being retried forever. Unroutable inbound domains remain retryable and are
  eventually moved to the SQS DLQ for operator review.
- Bounce and complaint callbacks create team-scoped suppressions before a new
  send is queued; provider-accepted sends are acknowledged as terminal when
  persistence cannot be completed, preventing an automatic duplicate send.
- Team invitations are single-use, email-bound, seven-day tokens; Stripe
  checkout, portal sessions, webhook signature verification, and plan-specific
  recipient limits are implemented but remain optional integrations.
- `make backup` creates a permission-restricted custom-format PostgreSQL dump.
  Restore is deliberately gated by `CONFIRM_RESTORE=YES`.

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
4. The sender domain is verified in SES and DNS. Inbound domains must also have
   the SES MX record and an active receipt rule set. Confirm the recipient before
   activating mail flow.
5. Outbound SES events have a configuration-set SNS destination and a public
   HTTPS callback. Do not put a placeholder ARN or callback URL into production.
6. A backup is stored outside the application host, a restore has been tested
   into an isolated database, and the RPO/RTO are written down.
7. Abuse controls are reviewed: daily quota, request rate limit, API-key
   rotation, bounce/complaint handling, and a support path. If Stripe is
   enabled, verify live price IDs, webhook delivery, cancellation behavior,
   and that unpaid subscriptions fall back to the free limit.
8. Run the end-to-end smoke test below with a verified sender and a disposable
   recipient, then confirm the SES message ID, worker status, and callback
   event if event publishing is enabled.

## Safe local checks

```bash
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

## Inbound error interpretation

- `inbound recipient domain is not configured: example.com` means the domain is
  absent, pending, or not MX-verified for a team. The message is not routed;
  check the domain record and the active SES receipt rule.
- `stale SNS notification` means the message sat in SQS beyond the five-minute
  signature window. It is acknowledged as non-retryable; the original raw
  message should remain available in S3 when the receipt rule stored it.
