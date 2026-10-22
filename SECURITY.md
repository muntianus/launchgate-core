# Security Policy

launchgate-core is an internal PerfOps microservice (default port 4020).

- **Report a vulnerability:** open a private security advisory on this repo
  (GitHub → Security → Advisories) or contact the owner listed in
  [`muntianus/.github`](https://github.com/muntianus/.github/blob/main/SECURITY.md).
- **Secrets:** none are required. Never commit `.env`; only `.env.example`
  (placeholder values) is tracked. GitGuardian scans every pull request for
  secrets.
- The API is unauthenticated and sends `Access-Control-Allow-Origin: *`. `POST /api/v1/probe`
  makes outbound HTTP requests to a caller-supplied `target_url`; keep the service on
  loopback / behind Caddy and never expose it publicly.
