# Security Policy

## Supported versions

| Version | Supported |
| --- | --- |
| `main` | Yes |

Older snapshots may contain known defects and should not be used for
production workloads without review.

## Reporting a vulnerability

Please do not open a public issue for a security vulnerability. Use GitHub's
private vulnerability reporting or a private contact channel for the
repository maintainers. Include a concise description, affected paths or
versions, reproduction steps, and the potential impact.

Please avoid sending credentials, personal data, or full production
configuration in a report. Redact secrets and use a minimal proof of concept.

We will acknowledge receipt when possible, validate the report, coordinate a
fix, and publish a short advisory when disclosure is appropriate.

## Operational notes

- Treat `.env` and `web/.env.local` as secret material.
- Rotate AWS, Supabase, webhook, and inbound tokens if they may have been
  exposed.
- Keep API keys and webhook secrets out of logs, issues, pull requests, and
  screenshots.
