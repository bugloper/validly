# @validly/vue

**Validly — Simple email validation for every stack.** Vue 3 composable for the [Validly](../../README.md) API. Vue 3.4+.

```sh
pnpm add @validly/vue
```

```ts
// main.ts
import { createClient, createValidly } from "@validly/vue";
app.use(createValidly({ client: createClient({ baseUrl: "https://validly.example.com" }) }));
```

```vue
<script setup lang="ts">
import { useEmailValidation } from "@validly/vue";
import { ref } from "vue";

const email = ref("");
// Pass your v-model ref to validate whenever it changes.
const { loading, message, error, invalid } = useEmailValidation({ email, debounceMs: 400 });
</script>

<template>
  <input v-model="email" type="email" autocomplete="email" :aria-invalid="invalid" :aria-busy="loading" />
  <p v-if="loading">Checking email...</p>
  <p v-else-if="error">Validation service temporarily unavailable.</p>
  <p v-else>{{ message?.text }}</p>
</template>
```

Returns computed refs `status`, `result`, `error`, `loading`, `pending`,
`invalid` and `message`, plus `validate`, `validateNow` and `reset`. Without
an `email` ref, call `validate(value)` yourself. Pending work is cancelled
when the component's scope is disposed.

Options: `client` (otherwise the plugin's, otherwise same-origin), `email`,
`debounceMs`, `checks`, `localSyntaxCheck`. For Nuxt, use
[`@validly/nuxt`](../nuxt). Example: [`examples/vue`](../../examples/vue).
