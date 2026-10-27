# Validly

**Validly — Simple email validation for every stack.**

Validly is a self-hostable email validation service (Go) with a family of
TypeScript packages for browsers, frontend frameworks and Node.js servers. It
needs no paid third-party validation API.

It answers four different questions, and keeps them apart:

| Question                          | How Validly answers it                                                        | Field                                   |
| --------------------------------- | ----------------------------------------------------------------------------- | --------------------------------------- |
| Is the address well formed?       | Local syntax rules (RFC 5321/5322 dot-atom, IDNA domains)                     | `syntax.status`                         |
| Can the domain receive mail?      | DNS: MX, RFC 7505 null MX, A/AAAA fallback                                    | `domain.status`                         |
| Does this mailbox exist?          | **Not answered.** DNS cannot tell, and SMTP probing is unreliable and abusive | `mailbox.status` is always `unverified` |
| Does someone control the address? | Optional ownership verification: a single-use emailed link                    | `/api/v1/verification/*`                |

A `deliverable` domain means the domain publishes mail routing. It never means
that `person@` that domain exists.

Validly also flags disposable providers, notes role addresses such as
`support@` (informational only), and suggests fixes for domain typos
(`gmial.com` → `gmail.com`) without ever rewriting the address itself.

## Quick start

```sh
docker compose up -d --build        # API on :8080, PostgreSQL, Redis, Mailpit
curl -s localhost:8080/api/v1/validate \
  -H 'content-type: application/json' \
  -d '{"email":"jane@gmial.com"}'
```

```json
{
  "email": "jane@gmial.com",
  "syntax": { "status": "valid", "reason": null },
  "domain": {
    "normalized": "gmial.com",
    "status": "undeliverable",
    "reason": "non_public_address",
    "mx_found": false,
    "fallback_address_found": true
  },
  "disposable": { "status": "disposable" },
  "role": { "status": "not_role" },
  "suggestion": { "domain": "gmail.com", "email": "jane@gmail.com" },
  "mailbox": { "status": "unverified" },
  "overall_status": "undeliverable"
}
```

Live DNS answers change over time, so your output may differ. Without
Docker: `go run ./cmd/validly` (Go 1.26). Basic validation needs no
database, cache or SMTP server. See [docs/quick-start.md](docs/quick-start.md).

## Packages

| Package                                              | What it provides                                                                        | Tested with                                       |
| ---------------------------------------------------- | --------------------------------------------------------------------------------------- | ------------------------------------------------- |
| [`@validly/core`](packages/core)                     | Types, HTTP client, local syntax check, debounced validation controller, policy helper  | Node 24/26, Chromium                              |
| [`@validly/react`](packages/react)                   | `useEmailValidation()`, `ValidlyProvider`                                               | React 18.3, 19.3                                  |
| [`@validly/vue`](packages/vue)                       | `useEmailValidation()` composable, plugin                                               | Vue 3.4, 3.5                                      |
| [`@validly/angular`](packages/angular)               | Injectable service, Reactive Forms async validator, signals                             | Angular 20.3, 22.2                                |
| [`@validly/svelte`](packages/svelte)                 | Store and input action                                                                  | Svelte 4.2, 5.57                                  |
| [`@validly/solid`](packages/solid)                   | `createEmailValidation()` primitive                                                     | Solid 1.8, 1.9                                    |
| [`@validly/nextjs`](packages/nextjs)                 | Route Handler proxy, Server Action helpers, client hook                                 | Next.js 16.4 (App Router)                         |
| [`@validly/nuxt`](packages/nuxt)                     | Nuxt module: proxy route, server-only config, auto-imports                              | Nuxt 4.6                                          |
| [`@validly/server`](packages/server)                 | Server-side policy checks and proxy handler; Express, Fastify, Hono and NestJS adapters | Express 5.3, Fastify 5.12, Hono 4.13, NestJS 12.2 |
| [Go SDK](sdk/go) `github.com/validly/validly/sdk/go` | Typed client with retries                                                               | Go 1.26                                           |

Every package has working examples in [`examples/`](examples), each built and
tested in CI, and run against the real server by `pnpm e2e`.

## How it fits together

```text
 Browser ── @validly/react|vue|angular|svelte|solid ──┐
            (anonymous; or via your server's proxy)   │
                                                      ▼
 Your server ─ @validly/server | nextjs | nuxt ─▶  Validly API (Go, Echo)
   (holds the API key)                              ├─ syntax   (pure Go, IDNA)
                                                    ├─ dns      (MX / null MX / A+AAAA, cached, coalesced)
 Go services ─ sdk/go ─────────────────────────────▶├─ disposable, role, suggestions
                                                    ├─ verification (optional: PostgreSQL + SMTP)
                                                    └─ cache (memory or Redis), rate limits, metrics
```

## Documentation

- [Quick start](docs/quick-start.md)
- [API reference](docs/api.md) · [OpenAPI 3.1](openapi/openapi.json) (also served at `/openapi.json`)
- [Configuration](docs/configuration.md) · [Docker deployment](docs/deployment.md)
- [JavaScript and TypeScript packages](docs/packages.md) · [Go SDK](sdk/go/README.md)
- [DNS validation and its limits](docs/dns-validation.md)
- [Disposable domains and admin lists](docs/disposable-domains.md)
- [Ownership verification](docs/verification.md)
- [Security and privacy](docs/security.md)
- [Troubleshooting](docs/troubleshooting.md) · [Contributing](CONTRIBUTING.md)

## Status and naming

Version 0.1.0, not yet published. Before publishing:

- **npm:** the unscoped name `validly` is already taken on npm. The packages
  use the `@validly` scope; no `@validly/*` package exists today, but whether
  the `validly` npm organisation can be registered has not been verified. If
  it cannot, rename the scope everywhere, e.g. to `@validly-dev`
  (see [CONTRIBUTING.md](CONTRIBUTING.md#renaming-the-npm-scope)).
- **Go:** the module path `github.com/validly/validly` is a placeholder until
  the repository has a home.
- **Brand:** domain and trademark availability for "Validly" have not been checked.

## License

[MIT](LICENSE)
