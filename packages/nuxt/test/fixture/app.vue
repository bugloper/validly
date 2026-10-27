<script setup lang="ts">
const email = ref("");
const { message, loading } = useEmailValidation({ email, debounceMs: 300 });
</script>

<template>
  <main>
    <h1>validly-nuxt-fixture</h1>
    <p class="auto-import">describeResult:{{ typeof describeResult }}</p>
    <input v-model="email" type="email" autocomplete="email" />
    <p>{{ loading ? "Checking email..." : (message?.text ?? "") }}</p>
  </main>
</template>
