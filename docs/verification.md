# Ownership verification

DNS can only say that a domain accepts mail. To know that a person controls
an address, send them a link and wait for them to use it. Validly can do this
for you; it is optional and off by default.

## Enable it

```sh
VALIDLY_DATABASE_URL=postgres://validly:secret@db:5432/validly
VALIDLY_SMTP_HOST=smtp.example.com
VALIDLY_SMTP_PORT=587
VALIDLY_SMTP_USERNAME=apikey
VALIDLY_SMTP_PASSWORD=...
VALIDLY_SMTP_FROM="Example <no-reply@example.com>"
VALIDLY_VERIFICATION_URL=https://app.example.com/verify?token={token}
VALIDLY_API_KEYS=app-backend:<random>
```

The verification endpoints require an API key, so call them from your
backend, never from the browser. `docker compose up` configures everything
with Mailpit; read the emails at <http://localhost:8025>.

## Flow

```text
your backend ── POST /api/v1/verification/send {"email"} ──▶ Validly ── email with link ──▶ user
user clicks https://app.example.com/verify?token=… ──▶ your page
your backend ── POST /api/v1/verification/confirm {"token"} ──▶ Validly ── {"status":"verified","email"} ──▶ mark verified
```

```ts
import { createValidly } from "@validly/server";

const validly = createValidly({ baseUrl: process.env.VALIDLY_URL, apiKey: process.env.VALIDLY_API_KEY });
await validly.client.sendVerification("person@example.com");
// later, in your /verify route:
const { email } = await validly.client.confirmVerification(tokenFromQuery);
```

```go
err := client.SendVerification(ctx, "person@example.com")
email, err := client.ConfirmVerification(ctx, token)
if validly.IsCode(err, validly.ErrInvalidToken) { /* expired, used or unknown */ }
```

## Guarantees

- **Unguessable tokens:** 256 bits from `crypto/rand`, base64url-encoded
  (43 characters).
- **Only hashes are stored:** the database holds SHA-256(token), so a
  database leak does not yield usable links.
- **Expiring and single-use:** tokens expire after
  `VALIDLY_VERIFICATION_TOKEN_TTL` (30 min). Confirmation is one atomic
  `UPDATE … WHERE used_at IS NULL AND expires_at > now()`, so concurrent or
  repeated confirmations succeed at most once.
- **No enumeration:** `send` returns the same `202 {"status":"accepted"}` for
  every well-formed address, whether a message was sent, suppressed by the
  rate limit, or failed. Sending happens in the background, so response
  timing does not reveal the outcome either. `confirm` returns the same
  `invalid_token` error for unknown, expired and used tokens.
- **Rate limits:** at most 3 tokens per address per hour, serialized per
  address in PostgreSQL. Case, `+tag` and Unicode-domain variants of an
  address share one limit, so they cannot be used to multiply it. The
  per-API-key request limit applies to both endpoints too.
- **The link is server configuration:** callers cannot choose where the
  emailed link points, so Validly cannot be used to send phishing links.
- **Secrets stay server-side:** SMTP credentials live only in Validly's
  environment; tokens and full addresses are never logged.

## Data retention

The `verification_tokens` table stores the address (needed to answer
`confirm`), both hashes and timestamps. Rows are deleted a day after they
expire.

## Delivery

The SMTP adapter uses implicit TLS on port 465 and STARTTLS elsewhere. By
default it refuses servers that do not offer STARTTLS, so an attacker on the
network path cannot downgrade the connection and read tokens; set
`VALIDLY_SMTP_TLS=opportunistic` only for local relays such as Mailpit.
Credentials are only sent over TLS (or to localhost). SMTP replies are never
logged verbatim, because servers often echo the recipient address in them;
only the failing step and reply code are. To use another
delivery mechanism, implement the `verification.Mailer` interface (one `Send`
method) and wire it in `cmd/validly`. Failed deliveries are logged (domain
only) and counted in `validly_verification_events_total{event="send_error"}`.
Requests beyond the 1,000-message queue are dropped and counted as
`dropped`.
