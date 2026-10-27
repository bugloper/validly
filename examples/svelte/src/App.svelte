<script lang="ts">
  import { createClient, createEmailValidation, evaluate } from "@validly/svelte";

  // Browser requests are anonymous: never put an API key in frontend code.
  const client = createClient({ baseUrl: import.meta.env.VITE_VALIDLY_URL ?? "http://localhost:8080" });
  const validation = createEmailValidation({ client, debounceMs: 400 });

  let email = $state("");
  let submitted = $state<string | null>(null);

  async function onSubmit(event: SubmitEvent) {
    event.preventDefault();
    const state = await validation.validateNow(email);
    // If the service is unavailable, let the user through rather than block sign-ups.
    if (state.result && !evaluate(state.result).accepted) return;
    submitted = email;
  }

  function useSuggestion(address: string) {
    // Only change the address when the user asks to.
    email = address;
    validation.validate(address);
  }
</script>

<main>
  <h1>Validly — Svelte</h1>
  <form novalidate onsubmit={onSubmit}>
    <label for="email">Email</label>
    <!-- use:validation.action validates on input and keeps aria-invalid/aria-busy in sync -->
    <input id="email" type="email" autocomplete="email" aria-describedby="email-status" bind:value={email} use:validation.action />
    <p id="email-status" role="status" aria-live="polite">
      {#if $validation.loading}
        Checking email...
      {:else if $validation.error}
        Validation service temporarily unavailable.
      {:else}
        {$validation.message?.text ?? ""}
      {/if}
      {#if $validation.result?.suggestion}
        {@const suggestion = $validation.result.suggestion}
        <button type="button" onclick={() => useSuggestion(suggestion.email)}>Use {suggestion.email}</button>
      {/if}
    </p>
    <button type="submit">Create account</button>
    {#if submitted}<p>Thanks! We sent a confirmation link to {submitted}.</p>{/if}
  </form>
</main>
