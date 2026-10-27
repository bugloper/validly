# JavaScript and TypeScript packages

All packages are ESM-first with CommonJS builds where practical (the Nuxt
module is ESM-only), ship TypeScript declarations, and keep dependencies to a
minimum: each other, plus `server-only` for Next.js and `@nuxt/kit` for Nuxt.
Frameworks are peer dependencies.

| Package            | Use it for                                                                                          | Guide                                   |
| ------------------ | --------------------------------------------------------------------------------------------------- | --------------------------------------- |
| `@validly/core`    | Any JavaScript runtime: the client, types, local syntax check, validation controller, policy helper | [README](../packages/core/README.md)    |
| `@validly/react`   | React components                                                                                    | [README](../packages/react/README.md)   |
| `@validly/vue`     | Vue 3 components                                                                                    | [README](../packages/vue/README.md)     |
| `@validly/angular` | Angular services, Reactive Forms and signals                                                        | [README](../packages/angular/README.md) |
| `@validly/svelte`  | Svelte 4 and 5                                                                                      | [README](../packages/svelte/README.md)  |
| `@validly/solid`   | SolidJS                                                                                             | [README](../packages/solid/README.md)   |
| `@validly/nextjs`  | Next.js App Router (client and server)                                                              | [README](../packages/nextjs/README.md)  |
| `@validly/nuxt`    | Nuxt 4                                                                                              | [README](../packages/nuxt/README.md)    |
| `@validly/server`  | Node.js servers: Express, Fastify, Hono, NestJS                                                     | [README](../packages/server/README.md)  |

The React, Vue, Angular, Svelte, Solid and server packages re-export
`@validly/core`, so one import is enough.

## Shared behaviour

All the frontend integrations wrap the same controller from `@validly/core`, so they behave
identically:

- **Debouncing:** input is validated after `debounceMs` (300 ms by default)
  of quiet.
- **Cancellation:** a new input aborts the previous request, and unmounting
  aborts whatever is in flight.
- **No stale results:** a late response for an older input is ignored.
- **Local syntax check first:** obviously malformed input is decided without
  a network request, using the same rules (and shared test vectors) as the
  server. Internationalized domains are always sent to the server.
- **States:** `idle`, `pending` (debouncing), `validating` (request in
  flight), `done` and `error`. `loading` means `validating`.
- **Errors:** a single `ValidlyError` type with `kind`: `api` (with `status`
  and `code`), `network`, `timeout`, `aborted` or `decode`.

When the service is unavailable you get `status: "error"` and `error` set;
the recommended UX is to tell the user and let them continue, then re-check
on the server.

## Where requests go

| Setup                           | Client configuration                                                                                         | Credentials                     |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------ | ------------------------------- |
| Browser → Validly               | `createClient({ baseUrl: "https://validly.example.com" })` and your origin in `VALIDLY_CORS_ORIGINS`         | None (anonymous, per-IP limits) |
| Browser → your server → Validly | Next.js: `@validly/nextjs/client`; Nuxt: automatic; others: `createClient({ validateUrl: "/api/validate" })` | API key on your server only     |
| Server → Validly                | `@validly/server`, `@validly/nextjs/server`, `useValidly()` in Nuxt                                          | API key from the environment    |

## Choosing what to reject

`evaluate(result, policy)` turns a result into accept/reject with a
user-facing message. Defaults: reject invalid syntax, undeliverable domains
and disposable addresses; accept role addresses and unknown domains (so a DNS
outage doesn't block sign-ups). `describeResult(result)` returns a short
message for display.

## Tested versions

Each adapter's suite runs against the newest release and, through
[`compat/`](../compat), the oldest supported one: React 18.3 and 19.3, Vue 3.4
and 3.5, Angular 20.3 and 22.2, Svelte 4.2 and 5.57, Solid 1.8 and 1.9. Next.js
16.4 and Nuxt 4.6 are covered by the examples and the end-to-end run. The core
client also runs in Chromium. The workspace uses TypeScript 6.0 (Angular 22
requires it).
