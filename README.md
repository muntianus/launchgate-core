# LaunchGate Core (`launchgate-core`)

Automated CI/CD release readiness gate evaluating k6 load metrics against YAML SLO policies (`GO` / `CONDITIONAL_GO` / `NO_GO`).

## Verdict Engine
- Evaluates `p95`, `p99`, `error_rate`, and regression delta against baseline.
- Integrates with GitHub Actions & VictoriaMetrics.
