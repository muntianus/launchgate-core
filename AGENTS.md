# AGENTS.md — launchgate-core

> Canon: `~/github/vault/personal/agent-config/AGENTS.md`
> Org policy: [`muntianus/.github/AGENT_POLICY.md`](https://github.com/muntianus/.github/blob/main/AGENT_POLICY.md) ·
> [Repository Standard](https://github.com/muntianus/.github/blob/main/docs/standards/REPOSITORY_STANDARD.md)

## Repo-local rules

- **Zero dependencies.** Go standard library only (see `go.mod`); adding a module needs an ADR.
- **One entrypoint.** `cmd/launchgate` is the only `main` package (CLI + `serve`); CI fails otherwise.
- **Contract first.** A route change updates `Routes` in `internal/server/server.go` and
  `docs/api/openapi.json` in the same commit; `internal/server/contract_test.go` fails on drift.
- **Verdicts are part of the API.** `GO` / `CONDITIONAL_GO` / `NO_GO` and the policy preset
  names are consumed by k6, locust, chaostoolkit and ACO — changing them is a breaking change.
- **Config is env-only.** Every variable the code reads is listed in `.env.example`; never commit `.env`.
- **`GET /health` stays dependency-free** (liveness only).
- Before pushing: `gofmt -l . && go vet ./... && go test ./...`.
- Conventional commits; branch `feat/*` / `fix/*` / `chore/*`; never push to `main`.
