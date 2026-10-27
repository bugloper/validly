# Contributing to Validly

## Layout

```text
cmd/validly/        server binary (serve, check, healthcheck, version)
internal/           syntax, dns, cache, disposable, role, suggestions, validation,
                    ratelimit, verification, store (PostgreSQL), config,
                    observability, httpapi (Echo)
openapi/            OpenAPI 3.1 contract (embedded and served at /openapi.json)
sdk/go/             Go client (separate module)
packages/           npm packages (pnpm workspace)
compat/             re-run adapter test suites against older framework versions
examples/           example apps, all built and tested in CI
testdata/           syntax vectors shared by the Go server and the TypeScript client
tools/fakedns/      deterministic DNS server for end-to-end tests
scripts/e2e.mjs     end-to-end run against the real binary
```

## Requirements

Go 1.26, Node.js 24+, pnpm 12, and optionally Docker and PostgreSQL.

## Go

```sh
gofmt -l cmd internal openapi tools sdk examples/go   # must print nothing
go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
go test -race ./...
(cd sdk/go && go test -race ./...)
go test -run='^$' -fuzz=FuzzParse -fuzztime=30s ./internal/syntax
```

PostgreSQL tests run when `VALIDLY_TEST_DATABASE_URL` points at a disposable
database (for example `docker compose up -d postgres` and
`postgres://validly:validly@localhost:5432/validly?sslmode=disable`).
No test uses public DNS: the DNS tests use a fake `Querier` and a local
DNS server.

## TypeScript

```sh
pnpm install
pnpm lint                 # Biome
pnpm build
pnpm typecheck
pnpm test
pnpm test:compat          # React 18, Vue 3.4, Angular 20, Svelte 4, Solid 1.8
pnpm test:browser         # core in Chromium (pnpm --filter @validly/core exec playwright install chromium)
pnpm check:packages       # publint + are-the-types-wrong
pnpm examples:test
```

Set `VALIDLY_BROWSER_CHANNEL=chrome` to run the browser tests with an
installed Chrome instead of Playwright's Chromium.

## End-to-end tests

```sh
pnpm build
pnpm --filter validly-example-nestjs --filter validly-example-nextjs --filter validly-example-nuxt build
VALIDLY_E2E_DATABASE_URL=postgres://... pnpm e2e   # the variable is optional
```

The script builds the server, points it at `tools/fakedns`, checks every DNS
outcome over HTTP, optionally runs ownership verification through PostgreSQL
and an SMTP sink, and drives the Go and server examples against it.

## Changing the contract

1. Edit `openapi/openapi.json`.
2. Update the Go types in `internal/validation` and `sdk/go`, and the
   TypeScript types and enum arrays in `packages/core/src/types.ts`.
3. `go test ./internal/httpapi` validates real responses against the
   document; `pnpm --filter @validly/core test` checks the enums.

Syntax rule changes go into `testdata/syntax-cases.json` so that the Go and
TypeScript implementations stay identical.

## Renaming the npm scope

The packages use `@validly/*`, but ownership of the `validly` npm organisation
has not been verified. To switch scopes before publishing (example:
`@validly-dev`):

```sh
grep -rl --exclude-dir=node_modules --exclude-dir=dist --exclude-dir=.next --exclude-dir=.nuxt --exclude-dir=.output \
  '@validly/' packages examples compat docs .github README.md CONTRIBUTING.md package.json \
  | xargs sed -i '' 's#@validly/#@validly-dev/#g'
pnpm install   # regenerates pnpm-lock.yaml
```

(On Linux use `sed -i` without `''`.) Then rebuild and run the full test suite.

## Publishing

Not automated yet. For each package: `pnpm --filter <name> build`,
`pnpm --filter <name> check:package`, then `pnpm publish --access public`.
`workspace:^` ranges are rewritten to real versions by pnpm on publish.
