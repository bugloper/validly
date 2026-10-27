import { useEmailValidation } from "@validly/react";

/** The README example: an uncontrolled input validated as the user types. */
export function EmailField() {
  const { validate, result, loading, error } = useEmailValidation({ debounceMs: 400 });

  return (
    <div>
      <input type="email" autoComplete="email" aria-label="Email" onChange={(event) => validate(event.target.value)} />

      {loading && <p>Checking email...</p>}

      {result?.syntax.status === "invalid" && <p>Please enter a valid email address.</p>}

      {result?.domain.status === "undeliverable" && <p>This domain does not appear to accept email.</p>}

      {result?.domain.status === "unknown" && <p>We could not check the domain right now.</p>}

      {result?.suggestion && <p>Did you mean {result.suggestion.email}?</p>}

      {error && <p>Validation service temporarily unavailable.</p>}
    </div>
  );
}
