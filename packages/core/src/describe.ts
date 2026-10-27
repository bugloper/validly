import type { ValidationResult } from "./types.js";

export interface ResultMessage {
  /** `error`: block or flag the input. `warning`: let the user decide. */
  tone: "error" | "warning";
  text: string;
}

/**
 * Returns a short, user-facing message for a result, or null when there is
 * nothing to say. Messages describe the domain only; they never claim that a
 * mailbox exists.
 */
export function describeResult(result: ValidationResult): ResultMessage | null {
  if (result.syntax.status === "invalid") {
    return { tone: "error", text: "Please enter a valid email address." };
  }
  if (result.suggestion) {
    return { tone: "warning", text: `Did you mean ${result.suggestion.email}?` };
  }
  switch (result.domain.status) {
    case "undeliverable":
      return { tone: "error", text: "This domain does not appear to accept email." };
    case "unknown":
      return { tone: "warning", text: "We could not check the domain right now." };
  }
  if (result.disposable.status === "disposable") {
    return { tone: "warning", text: "This looks like a temporary email address." };
  }
  return null;
}
