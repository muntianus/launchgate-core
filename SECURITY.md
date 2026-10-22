# Security Policy

launchgate-core is an internal PerfOps microservice (default port 4020).

- **Report a vulnerability:** open a private security advisory on this repo
  (GitHub → Security → Advisories) or contact the owner listed in
  [`muntianus/.github`](https://github.com/muntianus/.github/blob/main/SECURITY.md).
- **Secrets:** none are required. Never commit `.env`; only `.env.example`
  (placeholder values) is tracked. CI runs gitleaks and govulncheck via the org security gate.
- The API is unauthenticated and sends `Access-Control-Allow-Origin: *`. `POST /api/v1/probe`
  makes outbound requests, so `target_url` must be http(s) with a host in
  `LAUNCHGATE_PROBE_ALLOWED_HOSTS` (default: the portfolio's public hosts) and redirects are
  not followed. Never add internal hosts or IPs; keep the service on loopback / behind Caddy.
