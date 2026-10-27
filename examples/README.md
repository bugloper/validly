# Examples

Each example is a small working app built against the packages in this
repository. `pnpm examples:test` builds and tests all of them, and
`pnpm e2e` runs the Go example and every server example against a real
Validly server (see [CONTRIBUTING.md](../CONTRIBUTING.md#end-to-end-tests)).

| Example                                                                   | Shows                                                                        | Run                                             |
| ------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | ----------------------------------------------- |
| [vanilla](vanilla)                                                        | `@validly/core` controller with plain DOM code                               | `pnpm --filter validly-example-vanilla dev`     |
| [react](react)                                                            | Controlled and uncontrolled fields, accessible messages, submit-time check   | `pnpm --filter validly-example-react dev`       |
| [vue](vue)                                                                | `v-model` + composable                                                       | `pnpm --filter validly-example-vue dev`         |
| [angular](angular)                                                        | Reactive Forms async validator, zoneless                                     | `pnpm --filter validly-example-angular start`   |
| [svelte](svelte)                                                          | Store + `use:` action (Svelte 5)                                             | `pnpm --filter validly-example-svelte dev`      |
| [solid](solid)                                                            | Signal-driven validation                                                     | `pnpm --filter validly-example-solid dev`       |
| [nextjs](nextjs)                                                          | Route Handler proxy, client hook, Server Action                              | `pnpm --filter validly-example-nextjs dev`      |
| [nuxt](nuxt)                                                              | Module proxy route, composable, server re-check                              | `pnpm --filter validly-example-nuxt dev`        |
| [express](express) · [fastify](fastify) · [hono](hono) · [nestjs](nestjs) | Registration endpoint that rejects bad addresses, plus a browser proxy route | `pnpm --filter validly-example-express start`   |
| [go](go)                                                                  | Go SDK command-line tool                                                     | `cd examples/go && go run . person@example.com` |

Start the API first (`docker compose up -d` or `go run ./cmd/validly`).
Browser examples call `http://localhost:8080` directly (set `VITE_VALIDLY_URL`
to change it); the compose file already allows the Vite, Next.js and Angular
dev-server origins in `VALIDLY_CORS_ORIGINS`. Server examples read
`VALIDLY_URL` and `VALIDLY_API_KEY` (Nuxt: `NUXT_VALIDLY_BASE_URL` and
`NUXT_VALIDLY_API_KEY`).
