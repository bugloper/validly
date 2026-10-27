# Deployment

Validly is a single static binary. The Docker image is ~45 MB, based on
`gcr.io/distroless/static-debian12:nonroot`: no shell, a non-root user, and CA
certificates for SMTP TLS and list syncing.

## Image

```sh
docker build --build-arg VERSION=1.0.0 -t validly:1.0.0 .
docker run -p 8080:8080 -e VALIDLY_API_KEYS=web:$(openssl rand -hex 24) validly:1.0.0
```

The image runs `validly serve`, exposes 8080 (API) and 9090 (metrics), and
declares a `HEALTHCHECK` using the built-in `validly healthcheck` command.

## Minimal production setup

Basic validation needs nothing but the container. Add the rest as needed:

| Need                                                                            | Add                                             |
| ------------------------------------------------------------------------------- | ----------------------------------------------- |
| Several replicas sharing DNS results                                            | Redis (`VALIDLY_REDIS_URL`)                     |
| Admin blocklist/allowlist that survives restarts and is shared between replicas | PostgreSQL (`VALIDLY_DATABASE_URL`)             |
| Ownership verification                                                          | PostgreSQL, SMTP and `VALIDLY_VERIFICATION_URL` |

Recommended settings for a public deployment:

```sh
VALIDLY_API_KEYS=web-backend:<random>,billing:<random>
VALIDLY_TRUSTED_PROXIES=10.0.0.0/8          # your load balancer's range
VALIDLY_CORS_ORIGINS=https://app.example.com # only if browsers call Validly directly
VALIDLY_METRICS_ADDR=:9090                   # scrape from inside the cluster only
```

Put Validly behind TLS (a load balancer or reverse proxy); it serves plain
HTTP.

## Health and shutdown

- Liveness: `GET /healthz`.
- Readiness: `GET /readyz`. On SIGTERM it starts returning 503 first, so the
  load balancer stops sending traffic, then in-flight requests get up to
  `VALIDLY_SHUTDOWN_TIMEOUT` (15 s) to finish.
- An outage of PostgreSQL or Redis is reported in `/readyz` but does not make
  the instance unready: validation keeps working.

Kubernetes example:

```yaml
livenessProbe: { httpGet: { path: /healthz, port: 8080 } }
readinessProbe: { httpGet: { path: /readyz, port: 8080 } }
terminationGracePeriodSeconds: 20
```

## DNS

Validly queries the resolvers in `/etc/resolv.conf`, or `VALIDLY_DNS_SERVERS`.
Use resolvers you trust; a resolver that forges answers can make any domain
look deliverable. Outbound UDP and TCP port 53 must be allowed.

## Database

Migrations are embedded and applied on startup under an advisory lock, so
several replicas can start at once. If the database is unreachable at
startup, Validly starts anyway: validation works, the database-backed
features report errors, and migrations are retried in the background (with
backoff up to 30 s) until the database is back. Expired verification tokens are deleted
hourly, a day after they expire.

## Observability

- Logs: JSON on stdout, one line per request (method, route, status,
  duration, request ID). Bodies, addresses and tokens are never logged.
- Metrics (Prometheus, via OpenTelemetry): `validly_http_requests_total`,
  `validly_http_request_duration_seconds`, `validly_validation_duration_seconds`,
  `validly_dns_lookups_total{status,reason}`, `validly_dns_lookup_duration_seconds`,
  `validly_cache_requests_total{cache,result}`, `validly_api_errors_total{code}`,
  `validly_verification_events_total{event}`.
- Traces: set `OTEL_EXPORTER_OTLP_ENDPOINT`.
