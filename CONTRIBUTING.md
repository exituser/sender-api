# Contributing

Thanks for helping improve Sender API. Small, focused pull requests are
easier to review and safer to deploy.

## Before you start

- Read the [README](README.md) and [security policy](SECURITY.md).
- Do not commit `.env`, `web/.env.local`, credentials, tokens, or production
  data.
- For security vulnerabilities, use the private reporting process in
  [SECURITY.md](SECURITY.md); do not open a public issue.

## Development workflow

1. Create a branch from `main`.
2. Keep the change focused and preserve existing tenant isolation boundaries.
3. Add or update regression tests for behavior changes.
4. Run the relevant checks before opening a pull request:

   ```bash
   gofmt -w ./cmd ./internal ./pkg
   go test ./cmd/... ./internal/... ./pkg/...
   go vet ./cmd/... ./internal/... ./pkg/...
   go build ./cmd/...

   cd web
   npm ci
   npm run lint
   npx tsc --noEmit
   npm run build
   npm audit --omit=dev --audit-level=high
   ```

5. Update documentation and migrations when the public contract changes.
6. Open a pull request using the checklist provided by the repository.

## Commits

Use short, imperative [Conventional Commits](https://www.conventionalcommits.org/)
with one logical change per commit. Examples:

- `feat(api): add domain verification endpoint`
- `fix(auth): reject team access without membership`
- `test(queue): cover recovery after worker restart`
- `docs(repo): add security reporting policy`

Keep the subject concise. Use the body to explain trade-offs or migration
notes when the title is not enough.

## Pull requests

A pull request should explain the user-visible or operational impact, list the
validation that was run, and call out any remaining risks. Changes affecting
authentication, tenant boundaries, inbound email, webhooks, Docker, or
migrations require especially clear test evidence.
