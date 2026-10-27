# @validly/nextjs

**Validly — Simple email validation for every stack.** Next.js App Router integration for the [Validly](../../README.md) API. Tested with Next.js 16.4.

```sh
pnpm add @validly/nextjs
```

Browser code never talks to Validly directly. It calls a Route Handler in your
app, which calls Validly with the API key from your server environment.

```ts
// lib/validly.ts (server-only)
import { createServerValidly } from "@validly/nextjs/server";

export const validly = createServerValidly({
  baseUrl: process.env.VALIDLY_URL,
  apiKey: process.env.VALIDLY_API_KEY, // never NEXT_PUBLIC_
});
```

```ts
// app/api/validate/route.ts
import { createValidateRoute } from "@validly/nextjs/server";
import { validly } from "@/lib/validly";

export const { POST } = createValidateRoute(validly);
```

```tsx
// app/SignupForm.tsx
"use client";
import { useEmailValidation } from "@validly/nextjs/client";

export function SignupForm() {
  const { getInputProps, message, loading } = useEmailValidation({ debounceMs: 400 }); // posts to /api/validate
  return (
    <>
      <input name="email" {...getInputProps({ describedBy: "email-msg" })} />
      <p id="email-msg">{loading ? "Checking email..." : message?.text}</p>
    </>
  );
}
```

```ts
// app/actions.ts — always re-check on the server
"use server";
import { emailFieldError } from "@validly/nextjs/server";
import { validly } from "@/lib/validly";

export async function signup(_: unknown, formData: FormData) {
  const error = await emailFieldError(validly, formData.get("email"));
  if (error) return { error };
  // ...create the account, then send a verification email
}
```

## Client/server boundary

- `@validly/nextjs/server` imports `server-only`. Importing it from a client
  component fails the build, so the key cannot leak into the bundle by
  accident.
- `@validly/nextjs/client` is marked `"use client"` and only knows your route's
  URL (`route`, default `/api/validate`).
- The route accepts `{ "email": string }`, forwards it, and maps upstream
  failures to generic errors (429 with `Retry-After`, otherwise 502) without
  leaking details.
- Everyone's requests go through your one key, so the route limits each
  client (`proxyRequestsPerMinute`, default 30). Clients are identified by the
  right-most `X-Forwarded-For` entry or `X-Real-IP`, as set by your platform
  or proxy; pass `createValidateRoute(validly, { clientId: (req) => ... })` to
  use something else.
- `@validly/nextjs/client` re-exports types only. In Server Components and
  Server Actions, import runtime helpers such as `evaluate` or
  `describeResult` from `@validly/core`.

Full example with a Server Action: [`examples/nextjs`](../../examples/nextjs).
Its test builds the app with a marker key and fails if the key appears in any
browser asset.
