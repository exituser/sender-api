## Summary

<!-- What changed and why? Keep the scope focused. -->

## Validation

- [ ] `go test ./cmd/... ./internal/... ./pkg/...`
- [ ] `go vet ./cmd/... ./internal/... ./pkg/...`
- [ ] `go build ./cmd/...`
- [ ] Frontend lint, typecheck, build, and production dependency audit (when `web/` changes)
- [ ] Compose configuration validation (when Docker or environment wiring changes)

## Safety checklist

- [ ] No credentials, tokens, private data, or generated build artifacts are included.
- [ ] Tenant boundaries and authorization behavior were reviewed.
- [ ] Migrations are backward-aware and documented when needed.
- [ ] Remaining risks or follow-up work are described below.

## Notes

<!-- Add rollout, migration, compatibility, or residual-risk notes. -->
