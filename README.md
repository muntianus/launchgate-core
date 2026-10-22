# LaunchGate Core (`launchgate-core`)

Automated CI/CD release readiness gate and HTTP microservice evaluating k6 load metrics, FinOps hourly burn, and Chaos resilience telemetry against YAML SLO policies (`GO` / `CONDITIONAL_GO` / `NO_GO`).

Built with **100% Go standard library** for zero-dependency reliability and deterministic sub-second builds.

## Architecture & Package Layout

- `internal/policy/policy.go` — YAML/JSON SLO policy parser and built-in presets (`globlex-b2b`, `perfops-gateway`, `aco-control-plane`).
- `internal/evaluator/evaluator.go` — Deterministic release readiness engine scoring `p95_ms`, `p99_ms`, `error_rate_max`, `min_rps`, `finops_max_hourly_usd`, baseline regressions, and Chaos recovery telemetry, plus `ProbeTarget` live HTTP latency prober.
- `internal/server/server.go` — HTTP API server (`PORT` default `4020`).
- `cmd/launchgate/main.go` — Unified CLI & microservice binary entrypoint.
- `policies/globlex-enterprise-slo.yaml` — Reference enterprise B2B SLO policy definition.

## CLI Usage

`cmd/launchgate` is the only binary entrypoint. There is deliberately no root `main.go`; always build the
package explicitly (CI enforces exactly one `main` package):

```bash
# Build binary
go build -o launchgate ./cmd/launchgate

# Evaluate release metrics against a preset SLO policy
./launchgate evaluate --p95 320 --error-rate 0.002 --rps 150 --policy globlex-b2b

# Evaluate against a YAML SLO policy file
./launchgate evaluate --p95 375 --error-rate 0.003 --rps 140 --policy policies/globlex-enterprise-slo.yaml

# Run a live multi-sample probe against a target endpoint
./launchgate probe --url https://api.globlex.pro/health --samples 5 --policy globlex-b2b
```

## HTTP Microservice API

Start the server (defaults to port `4020`):

```bash
./launchgate serve --port 4020
```

| Method | Endpoint | Description |
| --- | --- | --- |
| `GET` | `/health` | Returns `{"status":"ok","service":"launchgate-core","version":"1.0.0"}` |
| `GET` | `/api/v1/policies` | Lists built-in preset SLO policies (`globlex-b2b`, `perfops-gateway`, `aco-control-plane`) |
| `POST` | `/api/v1/evaluate` | Evaluates k6 summary or flat telemetry payload and returns `GO` / `CONDITIONAL_GO` / `NO_GO` report |
| `POST` | `/api/v1/probe` | Performs a live multi-sample HTTP probe against `target_url` and evaluates against the SLO policy |

### Example `POST /api/v1/evaluate`

```bash
curl -s -X POST http://localhost:4020/api/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "policy": "globlex-b2b",
    "service": "globlex-settlement-api",
    "release_sha": "9f3c2a1",
    "metrics": {
      "http_req_duration": { "avg": 110.4, "p(95)": 295.0, "p(99)": 540.0 },
      "http_req_failed": { "rate": 0.0015 },
      "http_reqs": { "rate": 165.0 }
    },
    "finops": { "hourly_cost_usd": 34.0 }
  }'
```

## Testing

```bash
go test -v ./...
```

## API contract, CI & deploy

- **Contract:** [`docs/api/openapi.json`](docs/api/openapi.json) (OpenAPI 3.1).
  `internal/server/contract_test.go` fails when `Routes` in `internal/server/server.go` and the
  spec disagree.
- **Tests:** `gofmt -l . && go vet ./... && go test ./...`. CI also builds the single
  `cmd/launchgate` entrypoint, runs a sample evaluation, and the org security gate
  (gitleaks + govulncheck).
- **Config:** every environment variable is listed in [`.env.example`](.env.example).
- **Deploy:** `docker build -t launchgate-core . && docker run -p 4020:4020 launchgate-core` —
  static binary on alpine, non-root, `HEALTHCHECK` on `GET /health`. The portfolio port map
  lives in [`muntianus/architecture`](https://github.com/muntianus/architecture).

## Contact / Support

PerfOps official Telegram contact: [@perfops_ai_manager](https://t.me/perfops_ai_manager).

For security vulnerabilities, use private vulnerability reporting rather than public Telegram messages.
