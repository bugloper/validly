# Security and privacy

Validly is designed to run as a public-facing service that people will try
to abuse.

## Abuse controls

- **Request limits:** bodies over `VALIDLY_MAX_BODY_BYTES` (8 KiB) are
  rejected; raw input over 512 bytes is not parsed further; header size and
  read/write timeouts are bounded.
- **Rate limits:** per client IP for anonymous callers and per API key for
  authenticated ones (token bucket, `Retry-After` on 429). Client IPs come
  from `X-Forwarded-For` only when the peer is in `VALIDLY_TRUSTED_PROXIES`,
  so the header cannot be spoofed to dodge limits, and IPv6 clients are
  grouped by /64 so rotating addresses within a subscriber's prefix does not
  yield fresh limits. A full limiter evicts the least recently used tenth at
  once, so floods of new addresses cannot make it slow. There are no per-account
  quotas: Validly has API keys, not accounts.
- **Bounded DNS work:** a timeout per lookup, a global concurrency cap,
  coalescing of identical lookups and caching (see [dns-validation.md](dns-validation.md)).
- **No open resolver or fetcher:** DNS is only used for the domain of a
  syntactically valid email address, and only for MX/A/AAAA. Validly never
  connects to resolved addresses and never fetches caller-supplied URLs (the
  disposable sync URL and verification link are operator configuration). That
  rules out DNS rebinding and SSRF against your network. Special-use names
  such as `.internal` and `.local` are never queried.
- **No resolver detail for anonymous callers:** they see `dns_unavailable`
  instead of `timeout`/`servfail`/`resolver_error`.
- **Admin endpoints** exist only when `VALIDLY_ADMIN_TOKEN` is set, use a
  separate token (an API key does not work), and compare in constant time.
- **Verification** cannot be used for enumeration, spam or phishing; see
  [verification.md](verification.md).
- **CORS** is off unless `VALIDLY_CORS_ORIGINS` lists origins; preflights
  allow only `Content-Type`, so browsers cannot send API keys cross-origin.
- **Responses** carry `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`
  and `Referrer-Policy: no-referrer`.

## Credentials

- API keys are configured server-side and stored only as SHA-256 hashes in
  memory; lookups compare every key in constant time.
- **Never ship an API key to a browser or mobile app.** Browser code should
  either call Validly anonymously (rate-limited by IP) or call a route on your
  server that holds the key: `createValidateRoute` (Next.js), the Nuxt
  module's route, or `validateHandler` (Express/Fastify/Hono). The Next.js
  and Nuxt examples have tests that fail if the key appears in browser assets,
  and `@validly/nextjs/server` imports `server-only` so a client component
  that imports it fails to build.
- A proxy route forwards all of your users through one key, so the proxy
  helpers limit each client separately (`proxyRequestsPerMinute`, 30 by
  default). Express and Fastify use their own client IP (configure "trust
  proxy" / `trustProxy` behind a load balancer); Hono, Next.js and Nuxt use
  the right-most `X-Forwarded-For` entry or `X-Real-IP`, which your platform
  or proxy sets. Earlier `X-Forwarded-For` entries are ignored because
  clients can forge them. Pass `clientId` to use something else, such as a
  user ID.
- `check()` treats only real outages (network errors, timeouts, 429, 5xx)
  as "unavailable". A rejected API key or other configuration error throws,
  so a misconfiguration cannot silently turn validation off.
- SMTP credentials live only in Validly's environment.

## Privacy

- Logs never contain request bodies, email addresses or tokens. Request logs
  include method, route, status, duration and request ID. Delivery failures
  log the recipient's domain only.
- Validation stores nothing about the address. The DNS cache is keyed by
  domain.
- Ownership verification stores the address and token hash until a day after
  expiry (see [verification.md](verification.md#data-retention)).
- Metrics carry no addresses or domains.

## Client-side validation is not validation

The TypeScript packages run a local syntax check before calling the API. It
mirrors the server's rules (they share test vectors) and can only skip
requests for input that is certainly malformed. It cannot check DNS and can be
bypassed by anyone, so always re-check on your server before you rely on an
address; `@validly/server`, `@validly/nextjs/server` and the Nuxt module's
`useValidly()` exist for that.

## Reporting vulnerabilities

Please report security issues privately to the maintainers instead of opening
a public issue.
