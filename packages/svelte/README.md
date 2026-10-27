# @validly/svelte

**Validly — Simple email validation for every stack.** Svelte stores and an input action for the [Validly](../../README.md) API. Svelte 4 and 5.

```sh
pnpm add @validly/svelte
```

```svelte
<script lang="ts">
  import { createClient, createEmailValidation } from "@validly/svelte";

  const validation = createEmailValidation({
    client: createClient({ baseUrl: "https://validly.example.com" }),
    debounceMs: 400,
  });
</script>

<!-- validates on input and keeps aria-invalid / aria-busy in sync -->
<input type="email" autocomplete="email" use:validation.action />

{#if $validation.loading}
  <p>Checking email...</p>
{:else if $validation.error}
  <p>Validation service temporarily unavailable.</p>
{:else if $validation.message}
  <p>{$validation.message.text}</p>
{/if}
```

`createEmailValidation()` returns a readable store of
`{ status, email, result, error, loading, pending, invalid, message }` plus
`validate`, `validateNow`, `reset` and `action`. When the last subscriber
leaves (for example, the component using `$validation` is destroyed),
pending work is cancelled. `setValidlyClient(client)` and `getValidlyClient()`
share a client through Svelte context.

Example: [`examples/svelte`](../../examples/svelte).
