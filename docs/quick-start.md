# Quick start

## 1. Run the API

With Docker (API, PostgreSQL, Redis and Mailpit for verification emails):

```sh
docker compose up -d --build
curl -s localhost:8080/healthz          # {"status":"ok"}
```

If a port is taken, override it, e.g. `VALIDLY_PORT=18080 POSTGRES_PORT=15432 docker compose up -d`.

Or run just the API, which needs no other services:

```sh
go run ./cmd/validly                    # listens on :8080, metrics on :9090
```

## 2. Validate an address

```sh
curl -s localhost:8080/api/v1/validate \
  -H 'content-type: application/json' \
  -d '{"email":"person@gmail.com"}'
```

Decide with `overall_status`:

| `overall_status`     | Meaning                                                                   | Typical UI                                     |
| -------------------- | ------------------------------------------------------------------------- | ---------------------------------------------- |
| `invalid`            | Malformed; see `syntax.reason`                                            | "Please enter a valid email address."          |
| `undeliverable`      | The domain cannot receive mail (no such domain, null MX, no mail records) | "This domain does not appear to accept email." |
| `unknown`            | DNS could not answer reliably (timeout, SERVFAIL)                         | Let the user continue                          |
| `domain_deliverable` | The domain routes mail. The mailbox is **not** verified                   | Accept                                         |
| `syntax_valid`       | Well formed; DNS was not requested                                        | Accept                                         |

`disposable`, `role` and `suggestion` never change `overall_status`; apply
your own policy (the `evaluate()` helper in `@validly/core` has sensible
defaults).

From the command line, without a server:

```sh
go run ./cmd/validly check jane@gmial.com
```

## 3. Use it from your stack

```sh
pnpm add @validly/react        # or vue, angular, svelte, solid, nextjs, nuxt, server, core
```

```tsx
import { useEmailValidation } from "@validly/react";

export function EmailField() {
  const { validate, result, loading, error } = useEmailValidation({ debounceMs: 400 });
  return (
    <div>
      <input type="email" autoComplete="email" onChange={(e) => validate(e.target.value)} />
      {loading && <p>Checking email...</p>}
      {result?.syntax.status === "invalid" && <p>Please enter a valid email address.</p>}
      {result?.domain.status === "undeliverable" && <p>This domain does not appear to accept email.</p>}
      {result?.domain.status === "unknown" && <p>We could not check the domain right now.</p>}
      {error && <p>Validation service temporarily unavailable.</p>}
    </div>
  );
}
```

Browsers call Validly anonymously (add your origin to `VALIDLY_CORS_ORIGINS`)
or, better, through a route on your own server that holds the API key: see
[`@validly/nextjs`](../packages/nextjs), [`@validly/nuxt`](../packages/nuxt)
and [`@validly/server`](../packages/server).

Go:

```go
client := validly.NewClient(validly.WithBaseURL("http://localhost:8080"))
result, err := client.Validate(ctx, "person@example.com")
```

## 4. Next steps

- Create API keys and set limits: [configuration](configuration.md)
- Prove ownership with an emailed link: [verification](verification.md)
- Deploy: [deployment](deployment.md)
