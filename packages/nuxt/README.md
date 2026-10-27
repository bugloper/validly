# @validly/nuxt

**Validly — Simple email validation for every stack.** Nuxt module for the [Validly](../../README.md) API. Nuxt 4.

```sh
pnpm add @validly/nuxt
```

```ts
// nuxt.config.ts
export default defineNuxtConfig({
  modules: ["@validly/nuxt"],
  validly: {
    baseUrl: "https://validly.example.com",
    // apiKey: set NUXT_VALIDLY_API_KEY instead of committing it
  },
});
```

The module:

- keeps `baseUrl` and `apiKey` in **private** runtime config (override at
  runtime with `NUXT_VALIDLY_BASE_URL` and `NUXT_VALIDLY_API_KEY`); they never
  reach the browser;
- adds a server route, `POST /api/validly/validate` (option `route`, `false`
  to disable), that proxies browser requests to Validly with the key;
- auto-imports `useEmailValidation()` in the app, which calls that route
  (same API as [`@validly/vue`](../vue));
- auto-imports `useValidly()` in server routes for policy checks.

```vue
<script setup lang="ts">
const email = ref("");
const { loading, message } = useEmailValidation({ email, debounceMs: 400 });
</script>
```

```ts
// server/api/register.post.ts — always re-check on the server
export default defineEventHandler(async (event) => {
  const { email } = await readBody(event);
  const check = await useValidly().check(email);
  if (!check.accepted) throw createError({ statusCode: 422, message: check.message });
  return { registered: true };
});
```

The package's test builds a Nuxt app with the module, runs it against a fake
Validly and checks the key appears in no browser asset. Example:
[`examples/nuxt`](../../examples/nuxt).
