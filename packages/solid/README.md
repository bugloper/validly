# @validly/solid

**Validly — Simple email validation for every stack.** SolidJS primitives for the [Validly](../../README.md) API. Solid 1.8+.

```sh
pnpm add @validly/solid
```

```tsx
import { createClient, createEmailValidation, ValidlyContext } from "@validly/solid";
import { createSignal, Show } from "solid-js";

function EmailField() {
  const [email, setEmail] = createSignal("");
  const v = createEmailValidation({ email, debounceMs: 400 }); // tracks the signal

  return (
    <>
      <input
        type="email"
        autocomplete="email"
        aria-invalid={v.invalid()}
        onInput={(e) => setEmail(e.currentTarget.value)}
      />
      <Show when={!v.loading()} fallback={<p>Checking email...</p>}>
        <p>{v.error() ? "Validation service temporarily unavailable." : v.message()?.text}</p>
      </Show>
    </>
  );
}

<ValidlyContext.Provider value={createClient({ baseUrl: "https://validly.example.com" })}>
  <EmailField />
</ValidlyContext.Provider>;
```

Returns accessors `state`, `status`, `result`, `error`, `loading`, `pending`,
`invalid` and `message`, plus `validate`, `validateNow` and `reset`. Pending
work is cancelled when the owner is disposed. Example:
[`examples/solid`](../../examples/solid).
