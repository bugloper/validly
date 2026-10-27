<script setup lang="ts">
import { evaluate, useEmailValidation } from "@validly/vue";
import { ref } from "vue";

const email = ref("");
const submitted = ref<string | null>(null);
// Validates automatically whenever the v-model changes.
const { loading, message, error, invalid, result, validateNow } = useEmailValidation({ email, debounceMs: 400 });

async function onSubmit() {
  const state = await validateNow(email.value);
  // If the service is unavailable, let the user through rather than block sign-ups.
  if (state.result && !evaluate(state.result).accepted) return;
  submitted.value = email.value;
}

function useSuggestion(address: string) {
  // Only change the address when the user asks to.
  email.value = address;
}
</script>

<template>
  <main>
    <h1>Validly — Vue</h1>
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
        <button v-if="result?.suggestion" type="button" @click="useSuggestion(result.suggestion.email)">
          Use {{ result.suggestion.email }}
        </button>
      </p>
      <button type="submit">Create account</button>
      <p v-if="submitted">Thanks! We sent a confirmation link to {{ submitted }}.</p>
    </form>
  </main>
</template>
