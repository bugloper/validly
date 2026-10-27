<script setup lang="ts">
const email = ref("");
const submitted = ref<string | null>(null);
const serverError = ref<string | null>(null);

// Auto-imported by @validly/nuxt. Validates through /api/validly/validate,
// a server route that holds the API key.
const { loading, message, error, invalid, result } = useEmailValidation({ email, debounceMs: 400 });

async function onSubmit() {
  serverError.value = null;
  try {
    await $fetch("/api/register", { method: "POST", body: { email: email.value } });
    submitted.value = email.value;
  } catch (err) {
    serverError.value = (err as { data?: { message?: string } }).data?.message ?? "Registration failed.";
  }
}
</script>

<template>
  <main>
    <h1>Validly — Nuxt</h1>
    <form novalidate @submit.prevent="onSubmit">
      <label for="email">Email</label>
      <input
        id="email"
        v-model="email"
        type="email"
        autocomplete="email"
        aria-describedby="email-status"
        :aria-invalid="invalid"
        :aria-busy="loading"
      />
      <p id="email-status" role="status" aria-live="polite">
        <template v-if="loading">Checking email...</template>
        <template v-else-if="error">Validation service temporarily unavailable.</template>
        <template v-else>{{ message?.text }}</template>
        <!-- Only change the address when the user asks to. -->
        <button v-if="result?.suggestion" type="button" @click="email = result.suggestion.email">
          Use {{ result.suggestion.email }}
        </button>
      </p>
      <button type="submit">Create account</button>
      <p v-if="serverError" role="alert">{{ serverError }}</p>
      <p v-if="submitted">Thanks! We sent a confirmation link to {{ submitted }}.</p>
    </form>
  </main>
</template>
