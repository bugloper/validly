# Configuration

Validly reads environment variables only. Invalid values stop the server at
startup with a list of every problem. [`.env.example`](../.env.example) lists
them all.

## HTTP

| Variable                   | Default | Description                                                                                           |
| -------------------------- | ------- | ----------------------------------------------------------------------------------------------------- |
| `VALIDLY_ADDR`             | `:8080` | API listen address                                                                                    |
| `VALIDLY_METRICS_ADDR`     | `:9090` | Prometheus `/metrics` listener; empty disables it. Keep it off the public internet                    |
| `VALIDLY_LOG_LEVEL`        | `info`  | `debug`, `info`, `warn` or `error` (JSON logs on stdout)                                              |
| `VALIDLY_MAX_BODY_BYTES`   | `8192`  | Request body limit (minimum 512)                                                                      |
| `VALIDLY_SHUTDOWN_TIMEOUT` | `15s`   | Time allowed for in-flight requests on SIGTERM                                                        |
| `VALIDLY_CORS_ORIGINS`     | —       | Comma-separated origins allowed to call the API from browsers. Unset means no CORS headers            |
| `VALIDLY_TRUSTED_PROXIES`  | —       | IPs/CIDRs whose `X-Forwarded-For` is trusted for client IPs. Unset means the TCP peer address is used |

## Authentication and rate limits

| Variable                     | Default | Description                                                                                                                      |
| ---------------------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `VALIDLY_API_KEYS`           | —       | `name:secret` pairs, comma-separated. Secrets need at least 16 characters. Names appear in logs and limit keys, secrets never do |
| `VALIDLY_REQUIRE_API_KEY`    | `false` | Reject anonymous validation requests                                                                                             |
| `VALIDLY_ADMIN_TOKEN`        | —       | Enables the admin API; at least 32 characters                                                                                    |
| `VALIDLY_RATE_LIMIT_IP`      | `60`    | Anonymous requests per minute per client IP (burst equal to the limit); `0` disables                                             |
| `VALIDLY_RATE_LIMIT_API_KEY` | `600`   | Requests per minute per API key; `0` disables                                                                                    |

Limits are kept in memory per process: with N replicas the effective limit is
up to N times higher.

## DNS and caching

| Variable                     | Default          | Description                                                                                                                                            |
| ---------------------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `VALIDLY_DNS_SERVERS`        | system resolvers | Resolver IPs, optionally with `:port`, e.g. `1.1.1.1,8.8.8.8`                                                                                          |
| `VALIDLY_DNS_TIMEOUT`        | `3s`             | Bound for one complete domain lookup; each query attempt gets a third of it, leaving time to fail over to another resolver and for the A/AAAA fallback |
| `VALIDLY_DNS_MAX_CONCURRENT` | `64`             | Domain lookups in flight at once                                                                                                                       |
| `VALIDLY_CACHE_MAX_ENTRIES`  | `100000`         | In-memory DNS cache size                                                                                                                               |
| `VALIDLY_REDIS_URL`          | —                | Share the DNS cache between replicas, e.g. `redis://redis:6379/0`                                                                                      |

Results are cached for their DNS TTL, clamped to 30 s–1 h. Indeterminate
results are cached for 10 s. If Redis fails, Validly skips it for 5 s at a
time and keeps validating.

## Disposable domains and suggestions

| Variable                           | Default       | Description                                                                            |
| ---------------------------------- | ------------- | -------------------------------------------------------------------------------------- |
| `VALIDLY_DISPOSABLE_FILE`          | —             | Extra newline-separated domains, re-read when the file changes                         |
| `VALIDLY_DISPOSABLE_SYNC_URL`      | —             | HTTPS URL of a newline-separated list (max 16 MiB) fetched periodically                |
| `VALIDLY_DISPOSABLE_SYNC_INTERVAL` | `24h`         | How often to fetch the sync URL                                                        |
| `VALIDLY_SUGGESTION_DOMAINS`       | built-in list | Comma-separated domains that typo suggestions may point to (replaces the default list) |

## PostgreSQL

| Variable               | Default | Description                                                             |
| ---------------------- | ------- | ----------------------------------------------------------------------- |
| `VALIDLY_DATABASE_URL` | —       | Persists admin lists and verification tokens. Migrations run on startup |

Without a database, admin lists live in memory and verification is disabled.

## Ownership verification

Set all of the following to enable it (a partial configuration is rejected at startup).

| Variable                                         | Default    | Description                                                                                                                                                         |
| ------------------------------------------------ | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `VALIDLY_SMTP_HOST`                              | —          | SMTP relay host                                                                                                                                                     |
| `VALIDLY_SMTP_PORT`                              | `587`      | `465` uses implicit TLS; otherwise STARTTLS when offered                                                                                                            |
| `VALIDLY_SMTP_TLS`                               | `required` | `required` refuses a server that does not offer STARTTLS (so TLS cannot be stripped in transit); `opportunistic` allows plaintext, for local relays such as Mailpit |
| `VALIDLY_SMTP_USERNAME`, `VALIDLY_SMTP_PASSWORD` | —          | Optional; only sent over TLS (or to localhost)                                                                                                                      |
| `VALIDLY_SMTP_FROM`                              | —          | Sender, e.g. `Example <no-reply@example.com>`                                                                                                                       |
| `VALIDLY_VERIFICATION_URL`                       | —          | Link template containing `{token}`, e.g. `https://app.example.com/verify?token={token}`                                                                             |
| `VALIDLY_VERIFICATION_TOKEN_TTL`                 | `30m`      | Token lifetime                                                                                                                                                      |

Verification also requires `VALIDLY_DATABASE_URL` and at least one API key.

## Tracing

Standard OpenTelemetry variables: when `OTEL_EXPORTER_OTLP_ENDPOINT` (or
`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`) is set, traces are exported over
OTLP/HTTP, and incoming `traceparent` headers are honoured.
