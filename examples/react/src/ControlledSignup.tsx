import { evaluate, useEmailValidation } from "@validly/react";
import { type FormEvent, useId, useState } from "react";

/** A controlled input with accessible status messages and submit-time validation. */
export function ControlledSignup() {
  const [email, setEmail] = useState("");
  const [submitted, setSubmitted] = useState<string | null>(null);
  const { validate, validateNow, message, loading, error, result, getInputProps } = useEmailValidation();
  const messageId = useId();

  const inputProps = getInputProps({ describedBy: messageId });

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    const state = await validateNow(email);
    // If the service is unavailable, let the user through rather than block sign-ups.
    if (state.result && !evaluate(state.result).accepted) return;
    setSubmitted(email);
  }

  return (
    <form onSubmit={onSubmit} noValidate>
      <label htmlFor="signup-email">Email</label>
      <input
        {...inputProps}
        id="signup-email"
        value={email}
        onChange={(event) => {
          setEmail(event.target.value);
          validate(event.target.value);
        }}
      />
      <p id={messageId} role="status" aria-live="polite">
        {loading ? "Checking email..." : error ? "Validation service temporarily unavailable." : (message?.text ?? "")}
        {result?.suggestion && (
          <button
            type="button"
            onClick={() => {
              // Only change the address when the user asks to.
              setEmail(result.suggestion!.email);
              validate(result.suggestion!.email);
            }}
          >
            Use {result.suggestion.email}
          </button>
        )}
      </p>
      <button type="submit">Create account</button>
      {submitted && <p>Thanks! We sent a confirmation link to {submitted}.</p>}
    </form>
  );
}
