# @validly/core

**Validly — Simple email validation for every stack.** Framework-neutral client, types and helpers for the [Validly](../../README.md) API.

```sh
pnpm add @validly/core
```

## Validate

```ts
import { createClient } from "@validly/core";

const validly = createClient({ baseUrl: "https://validly.example.com" });
const result = await validly.validate("person@example.com");

result.overall_status; // "invalid" | "undeliverable" | "unknown" | "domain_deliverable" | "syntax_valid"
result.domain.status; // "deliverable" means the domain routes mail, not that the mailbox exists
```

Options: `baseUrl`, `validateUrl` (e.g. your own proxy route), `apiKey`
(server-side only), `timeoutMs` (default 10 s), `fetch`, `headers`. Every call
accepts `{ signal }` for cancellation; `validate` also accepts `{ checks }`.

## Errors

Every failure is a `ValidlyError`. Check `kind`, not `instanceof`:

```ts
import { isValidlyError } from "@validly/core";

try {
  await validly.validate(email);
} catch (err) {
  if (isValidlyError(err)) {
    err.kind; // "api" | "network" | "timeout" | "aborted" | "decode"
    err.code; // for "api": "rate_limited", "unauthorized", ... or "unknown"
    err.retryAfterSeconds;
  }
}
```

## Local syntax check

```ts
import { checkSyntax } from "@validly/core";

checkSyntax("person@@example.com"); // { status: "invalid", reason: "multiple_at_signs", domain: null }
checkSyntax("person@münchen.de"); // { status: "unchecked" } — IDNA is left to the server
```

It applies the server's rules, verified against shared test vectors, so it
never rejects an address the server accepts. It performs **no** DNS checks and
is no substitute for server-side validation.

## Validation controller

`createValidationController({ client, debounceMs, checks, localSyntaxCheck })`
manages debouncing, cancellation of superseded requests and out-of-order
responses. The framework packages are thin wrappers around it; use it
directly in vanilla JavaScript:

```ts
const controller = createValidationController({ client: validly, debounceMs: 400 });
controller.subscribe((state) => render(state)); // { status, email, result, error }
input.addEventListener("input", () => controller.validate(input.value));
form.addEventListener("submit", async (e) => {
  e.preventDefault();
  const { result } = await controller.validateNow(input.value);
});
```

## Policy and messages

```ts
import { describeResult, evaluate } from "@validly/core";

evaluate(result); // rejects invalid, undeliverable, disposable
evaluate(result, { rejectRole: true }); // also role addresses
describeResult(result)?.text; // "Did you mean person@gmail.com?"
```

Runs in browsers and Node.js 20+. ESM and CommonJS.
