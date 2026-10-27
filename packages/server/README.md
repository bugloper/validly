# @validly/server

**Validly — Simple email validation for every stack.** Server-side helpers for Node.js, with adapters for Express, Fastify, Hono and NestJS.

```sh
pnpm add @validly/server
```

```ts
import { createValidly } from "@validly/server";

const validly = createValidly({
  baseUrl: process.env.VALIDLY_URL,
  apiKey: process.env.VALIDLY_API_KEY,
  policy: { rejectDisposable: true, rejectRole: false, rejectUnknownDomain: false },
  whenUnavailable: "allow", // let sign-ups through if Validly is down
});

const check = await validly.check("person@example.com");
if (!check.accepted) console.log(check.reason, check.message);
```

`check()` runs the local syntax check first (no request for malformed
input). When Validly is unavailable (network error, timeout, 429, 5xx) it
follows `whenUnavailable` and reports `unavailable: true`; configuration
errors such as a rejected API key throw instead, so a misconfiguration is
loud rather than silently accepting everything.

`handleValidateRequest(body, { clientId })` implements a browser-facing proxy
route that keeps the API key on your server. Because every user shares that
key, it limits each `clientId` to `proxyRequestsPerMinute` (default 30). The
adapters pass the client IP for you.

## Adapters

Each adapter offers `requireValidEmail(validly, { field })`, which answers 422
(or 400/503) when an address is rejected, and `validateHandler(validly)` for a
proxy route. Express and Fastify identify clients by `req.ip` /
`request.ip`, so configure their trust-proxy settings behind a load
balancer. Hono uses the right-most `X-Forwarded-For` entry (or `X-Real-IP`);
override it with `validateHandler(validly, { clientId: (c) => ... })`.

```ts
// Express
import { getEmailCheck, requireValidEmail, validateHandler } from "@validly/server/express";
app.post("/api/validate", express.json(), validateHandler(validly));
app.post("/register", express.json(), requireValidEmail(validly), (req, res) => { /* getEmailCheck(res) */ });

// Fastify
import { requireValidEmail } from "@validly/server/fastify";
app.post("/register", { preHandler: requireValidEmail(validly) }, handler);

// Hono
import { requireValidEmail, type ValidlyVariables } from "@validly/server/hono";
app.post("/register", requireValidEmail(validly), (c) => c.json({ ok: c.get("validly").accepted }));

// NestJS
import { ValidEmailPipe } from "@validly/server/nestjs";
@Post("register") register(@Body("email", new ValidEmailPipe(validly)) email: string) {}
```

Rejections look like
`{ "error": { "code": "email_rejected", "reason": "disposable", "message": "Please use a permanent email address." } }`.

Tested with Express 5.3, Fastify 5.12, Hono 4.13 and NestJS 12.2. Examples:
[`examples/express`](../../examples/express), [`fastify`](../../examples/fastify),
[`hono`](../../examples/hono), [`nestjs`](../../examples/nestjs).
