# API reference

The machine-readable contract is [`openapi/openapi.json`](../openapi/openapi.json)
(OpenAPI 3.1), also served at `GET /openapi.json`. The server's handler tests
validate every response against it, and the TypeScript types are checked
against its enums.

All bodies are JSON. Requests must send `Content-Type: application/json`;
unknown request fields are ignored. Responses carry `Cache-Control: no-store`.

## Authentication

API keys come from `VALIDLY_API_KEYS`. Send one as
`Authorization: Bearer <key>` or `X-API-Key: <key>`.

- `POST /api/v1/validate` accepts anonymous requests unless
  `VALIDLY_REQUIRE_API_KEY=true`. Anonymous callers are rate-limited per IP
  and see `dns_unavailable` instead of specific resolver failures.
- Verification endpoints always require a key.
- Admin endpoints require `Authorization: Bearer <VALIDLY_ADMIN_TOKEN>` and
  exist only when that token is set.

A key that is presented but wrong is rejected with 401, even where keys are
optional.

## `POST /api/v1/validate`

```json
{
  "email": "person@example.com",
  "checks": { "dns": true, "disposable": true, "role": true, "typos": true }
}
```

`checks` and each of its fields are optional and default to `true`. Syntax is
always evaluated. The `email` value is validated exactly as sent: Validly does
not trim or rewrite it.

Response (200, also for invalid addresses):

```json
{
  "email": "person@example.com",
  "syntax": { "status": "valid", "reason": null },
  "domain": {
    "normalized": "example.com",
    "status": "deliverable",
    "reason": "mx_records",
    "mx_found": true,
    "fallback_address_found": false
  },
  "disposable": { "status": "not_disposable" },
  "role": { "status": "not_role" },
  "suggestion": null,
  "mailbox": { "status": "unverified" },
  "overall_status": "domain_deliverable"
}
```

### Statuses

| Field               | Values                                                                      |
| ------------------- | --------------------------------------------------------------------------- |
| `syntax.status`     | `valid`, `invalid`                                                          |
| `domain.status`     | `deliverable`, `undeliverable`, `unknown`, `skipped`                        |
| `disposable.status` | `disposable`, `not_disposable`, `skipped`                                   |
| `role.status`       | `role`, `not_role`, `skipped`                                               |
| `mailbox.status`    | `unverified` (always, in validation results)                                |
| `overall_status`    | `invalid`, `undeliverable`, `unknown`, `domain_deliverable`, `syntax_valid` |

`mx_found` and `fallback_address_found` are `null` whenever DNS was skipped or
could not give a reliable answer: Validly never reports `false` for something
it could not check.

### `syntax.reason`

`empty`, `too_long`, `contains_whitespace`, `missing_at_sign`,
`multiple_at_signs`, `local_part_empty`, `local_part_too_long`,
`local_part_invalid_character`, `local_part_dot_placement`,
`quoted_local_part_unsupported`, `non_ascii_local_part_unsupported`,
`domain_empty`, `domain_literal_unsupported`, `domain_invalid`,
`domain_too_long`, `domain_label_too_long`, `domain_not_fully_qualified`.

Limits: 64 octets for the local part, 253 for the (ASCII) domain, 63 per label
and 254 for the whole address (RFC 5321).

### `domain.reason`

| Reason                                  | Status        | Meaning                                                                     |
| --------------------------------------- | ------------- | --------------------------------------------------------------------------- |
| `mx_records`                            | deliverable   | The domain has MX records                                                   |
| `address_fallback`                      | deliverable   | No MX, but public A/AAAA records (RFC 5321 implicit MX)                     |
| `null_mx`                               | undeliverable | `MX 0 .`: the domain accepts no mail (RFC 7505)                             |
| `nxdomain`                              | undeliverable | The domain does not exist                                                   |
| `no_mail_records`                       | undeliverable | The domain exists but has no MX, A or AAAA records                          |
| `non_public_address`                    | undeliverable | Only private, loopback or reserved addresses                                |
| `reserved_domain`                       | undeliverable | A special-use name such as `.test`, `.local` or `.internal` (never queried) |
| `timeout`, `servfail`, `resolver_error` | unknown       | DNS could not answer (authenticated callers only)                           |
| `dns_unavailable`                       | unknown       | The same, for anonymous callers                                             |
| `not_requested`                         | skipped       | `checks.dns` was false                                                      |
| `syntax_invalid`                        | skipped       | The address failed syntax validation                                        |

## Ownership verification

Enabled when PostgreSQL, SMTP and `VALIDLY_VERIFICATION_URL` are configured.
See [verification.md](verification.md).

- `POST /api/v1/verification/send` `{"email": "..."}` → `202 {"status":"accepted"}`
- `POST /api/v1/verification/confirm` `{"token": "..."}` → `200 {"status":"verified","email":"..."}`

## Admin

- `GET /api/v1/admin/disposable-domains` → `{"entries":[{"list","domain","created_at"}]}`
- `PUT /api/v1/admin/disposable-domains/{blocklist|allowlist}/{domain}` → the entry
- `DELETE /api/v1/admin/disposable-domains/{blocklist|allowlist}/{domain}` → 204

## Operations

- `GET /healthz` — the process is up.
- `GET /readyz` — 200 while serving, 503 once shutdown begins. Reports
  `database` and `cache` as `ok`, `error` or `disabled`. They are informational:
  syntax and DNS validation keep working without them.
- `GET /openapi.json`
- `GET /metrics` on `VALIDLY_METRICS_ADDR` (default `:9090`), Prometheus format.

## Errors

```json
{ "error": { "code": "rate_limited", "message": "Rate limit exceeded. Retry later." } }
```

| HTTP | `code`                                                                |
| ---- | --------------------------------------------------------------------- |
| 400  | `invalid_request`, `invalid_email`, `invalid_domain`, `invalid_token` |
| 401  | `unauthorized`                                                        |
| 404  | `not_found`                                                           |
| 405  | `method_not_allowed`                                                  |
| 413  | `payload_too_large`                                                   |
| 415  | `unsupported_media_type`                                              |
| 429  | `rate_limited` (with `Retry-After`)                                   |
| 503  | `verification_disabled`                                               |
| 500  | `internal_error`                                                      |
