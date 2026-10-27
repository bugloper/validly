import { createClient, createValidationController, describeResult, type ValidationState } from "@validly/core";

// Calls the Validly API directly. Browser requests are anonymous (no API key)
// and need the page's origin in VALIDLY_CORS_ORIGINS on the server.
const client = createClient({ baseUrl: import.meta.env.VITE_VALIDLY_URL ?? "http://localhost:8080" });
const validation = createValidationController({ client, debounceMs: 400 });

const form = document.querySelector<HTMLFormElement>("#signup")!;
const input = document.querySelector<HTMLInputElement>("#email")!;
const status = document.querySelector<HTMLParagraphElement>("#email-status")!;

function render(state: ValidationState) {
  input.setAttribute("aria-busy", String(state.status === "validating"));
  const invalid = state.result?.syntax.status === "invalid" || state.result?.domain.status === "undeliverable";
  input.setAttribute("aria-invalid", String(invalid));

  if (state.status === "validating") status.textContent = "Checking email...";
  else if (state.status === "error") status.textContent = "Validation service temporarily unavailable.";
  else if (state.result) {
    const suggestion = state.result.suggestion;
    status.textContent = describeResult(state.result)?.text ?? "";
    if (suggestion) {
      const use = document.createElement("button");
      use.type = "button";
      use.textContent = `Use ${suggestion.email}`;
      // Only change the address when the user asks to.
      use.onclick = () => {
        input.value = suggestion.email;
        validation.validate(input.value);
      };
      status.append(" ", use);
    }
  } else status.textContent = "";
}

validation.subscribe(render);
input.addEventListener("input", () => validation.validate(input.value));

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const state = await validation.validateNow(input.value);
  const result = state.result;
  // Block only what is clearly wrong; let users through if the service is down.
  if (result && (result.syntax.status === "invalid" || result.domain.status === "undeliverable")) {
    input.focus();
    return;
  }
  status.textContent = "Thanks! Check your inbox to confirm your address.";
});
