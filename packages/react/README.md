# @validly/react

**Validly — Simple email validation for every stack.** React hook for the [Validly](../../README.md) API. React 18 and 19.

```sh
pnpm add @validly/react
```

```tsx
import { createClient, useEmailValidation, ValidlyProvider } from "@validly/react";

const client = createClient({ baseUrl: "https://validly.example.com" });

export function App() {
  return (
    <ValidlyProvider client={client}>
      <EmailField />
    </ValidlyProvider>
  );
}

function EmailField() {
  const { validate, result, loading, error } = useEmailValidation({ debounceMs: 400 });

  return (
    <div>
      <input type="email" autoComplete="email" onChange={(event) => validate(event.target.value)} />
      {loading && <p>Checking email...</p>}
      {result?.syntax.status === "invalid" && <p>Please enter a valid email address.</p>}
      {result?.domain.status === "undeliverable" && <p>This domain does not appear to accept email.</p>}
      {result?.domain.status === "unknown" && <p>We could not check the domain right now.</p>}
      {error && <p>Validation service temporarily unavailable.</p>}
    </div>
  );
}
```

`useEmailValidation(options)` returns `status`, `result`, `error`, `loading`
(request in flight), `pending` (debouncing or loading), `message` (a
ready-made user-facing message), `validate`, `validateNow` (for submit
handlers), `reset` and `getInputProps({ describedBy })`. The last returns
`type`, `autoComplete`, `onChange`, `aria-invalid`, `aria-busy` and
`aria-describedby` for an accessible input:

```tsx
const { getInputProps, message } = useEmailValidation();
<input id="email" {...getInputProps({ describedBy: "email-msg" })} />
<p id="email-msg" role="status" aria-live="polite">{message?.text}</p>
```

Options: `client` (otherwise the provider's, otherwise a same-origin client),
`debounceMs` (300), `checks`, `localSyntaxCheck` (true). Inline client objects
are fine; they do not reset state on re-render.

Superseded requests are aborted, stale responses are ignored, and pending work
is cancelled on unmount. The module is marked `"use client"`, so in React
Server Components import helpers such as `evaluate` and `createClient` from
`@validly/core` rather than from this package. For Next.js, use
[`@validly/nextjs`](../nextjs), which routes requests through your server.

See [`examples/react`](../../examples/react) for controlled and uncontrolled forms.
