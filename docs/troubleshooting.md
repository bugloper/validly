# Troubleshooting

**Every domain comes back `unknown`.**
The resolvers are unreachable or failing. Call the API with an API key to see
the specific reason (`timeout`, `servfail`, `resolver_error`), check
`validly_dns_lookups_total{status="unknown"}`, make sure outbound UDP/TCP 53 is
allowed, or set `VALIDLY_DNS_SERVERS=1.1.1.1,8.8.8.8`.

**`domain.reason` is `dns_unavailable`, not the real error.**
That is deliberate for anonymous callers. Send an API key.

**A real domain is reported `non_public_address`.**
It has no MX records and its A/AAAA records point at private or reserved
addresses, often because a split-horizon resolver returns internal answers.
Point `VALIDLY_DNS_SERVERS` at a public resolver.

**`.local` / `.internal` / `.test` domains are `reserved_domain`.**
Special-use names are never queried. Validly validates public email
addresses only.

**An address with a quoted local part, an IP literal or non-ASCII before the `@` is invalid.**
Those forms are valid in the RFCs but rarely deliverable in practice, and
Validly reports them with dedicated `*_unsupported` reasons. Non-ASCII
_domains_ are supported.

**Leading or trailing spaces make an address invalid.**
Validly never modifies input. Trim it in your form if you want to.

**The browser gets CORS errors.**
Add your page's origin (scheme, host and port, no path) to
`VALIDLY_CORS_ORIGINS`, or call Validly through your own server route.

**429 `rate_limited`.**
Respect `Retry-After`. Behind a load balancer, set `VALIDLY_TRUSTED_PROXIES`
so users are not all counted as the balancer's IP. Server-to-server traffic
should use an API key, which has its own (higher) limit.

**401 `unauthorized` on `/api/v1/validate`.**
A key was sent but is not in `VALIDLY_API_KEYS` (a wrong key is never
silently treated as anonymous), or `VALIDLY_REQUIRE_API_KEY=true` and no key
was sent.

**`503 verification_disabled`.**
Ownership verification needs `VALIDLY_DATABASE_URL`, `VALIDLY_SMTP_HOST`,
`VALIDLY_SMTP_FROM` and `VALIDLY_VERIFICATION_URL`.

**`send` returns 202 but no email arrives.**
`202` is returned regardless of outcome, by design. Check the logs for
"sending verification email failed" and the metric
`validly_verification_events_total`: `rate_limited` (3 per address per hour),
`send_error`, `store_error` or `dropped`.

**The server refuses to start: "invalid configuration".**
The message lists every problem. Common ones: an API key secret shorter than
16 characters, an admin token shorter than 32, or a partial verification
setup.

**Redis is down and `/readyz` says `cache: error`.**
Validation continues without the shared cache; Validly retries Redis every
few seconds.

**`docker compose up` fails with "port is already allocated".**
Override host ports: `VALIDLY_PORT=18080 POSTGRES_PORT=15432 docker compose up -d`.
