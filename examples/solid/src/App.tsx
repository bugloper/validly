import { createEmailValidation, evaluate } from "@validly/solid";
import { createSignal, Show } from "solid-js";

export function App() {
  const [email, setEmail] = createSignal("");
  const [submitted, setSubmitted] = createSignal<string | null>(null);
  // Validates automatically whenever the email signal changes.
  const v = createEmailValidation({ email, debounceMs: 400 });

  async function onSubmit(event: SubmitEvent) {
    event.preventDefault();
    const state = await v.validateNow(email());
    // If the service is unavailable, let the user through rather than block sign-ups.
    if (state.result && !evaluate(state.result).accepted) return;
    setSubmitted(email());
  }

  return (
    <main>
      <h1>Validly — Solid</h1>
      <form novalidate onSubmit={onSubmit}>
        <label for="email">Email</label>
        <input
          id="email"
          type="email"
          autocomplete="email"
          aria-describedby="email-status"
          aria-invalid={v.invalid()}
          aria-busy={v.loading()}
          value={email()}
          onInput={(e) => setEmail(e.currentTarget.value)}
        />
        <p id="email-status" role="status" aria-live="polite">
          <Show when={!v.loading()} fallback="Checking email...">
            <Show when={!v.error()} fallback="Validation service temporarily unavailable.">
              {v.message()?.text ?? ""}
            </Show>
          </Show>
          <Show when={v.result()?.suggestion}>
            {(suggestion) => (
              // Only change the address when the user asks to.
              <button type="button" onClick={() => setEmail(suggestion().email)}>
                Use {suggestion().email}
              </button>
            )}
          </Show>
        </p>
        <button type="submit">Create account</button>
        <Show when={submitted()}>{(address) => <p>Thanks! We sent a confirmation link to {address()}.</p>}</Show>
      </form>
    </main>
  );
}
