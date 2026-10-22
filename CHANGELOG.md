# Changelog

Notable changes to this service. Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [SemVer](https://semver.org/).

## [Unreleased]

### Security

- `POST /api/v1/probe` only probes `target_url`s whose host is in `LAUNCHGATE_PROBE_ALLOWED_HOSTS`
  (default: public globlex.* / perfops.* hosts) over http(s) — 400 for other schemes, 403 for
  other hosts — and probes no longer follow redirects. Previously any caller could make the gate
  probe internal addresses and report status codes and latency.

### Changed

- Adopted the muntianus Repository Standard v1: committed OpenAPI contract
  (`docs/api/openapi.json`) with a route/spec drift test, `AGENTS.md` / `CLAUDE.md` /
  `SECURITY.md`, `.env.example`, Dependabot, a non-root Docker image with a health check,
  and CI with SHA-pinned actions, least-privilege permissions, gofmt
  (secret scanning stays with GitGuardian).
