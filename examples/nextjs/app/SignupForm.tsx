"use client";

import { useEmailValidation } from "@validly/nextjs/client";
import { useActionState, useId, useState } from "react";
import { type SignupState, signup } from "./actions";

const initial: SignupState = { error: null, registered: null };

export function SignupForm() {
  const [email, setEmail] = useState("");
  const [state, action, submitting] = useActionState(signup, initial);
  // Posts to /api/validate (our Route Handler), never to Validly directly.
  const { validate, message, loading, error, result, getInputProps } = useEmailValidation({ debounceMs: 400 });
  const statusId = useId();

  return (
    <form action={action} noValidate>
      <label htmlFor="email">Email</label>
      <input
        {...getInputProps({ describedBy: statusId })}
        id="email"
        name="email"
        value={email}
        onChange={(e) => {
          setEmail(e.target.value);
          validate(e.target.value);
        }}
      />
      <p id={statusId} role="status" aria-live="polite">
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
      <button type="submit" disabled={submitting}>
        Create account
      </button>
      {state.error && <p role="alert">{state.error}</p>}
      {state.registered && <p>Thanks! We sent a confirmation link to {state.registered}.</p>}
    </form>
  );
}
