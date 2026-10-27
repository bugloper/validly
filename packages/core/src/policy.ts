import type { ValidationResult } from "./types.js";

export interface Policy {
  /** Reject addresses at disposable providers (default true). */
  rejectDisposable?: boolean;
  /** Reject role addresses such as support@ (default false). */
  rejectRole?: boolean;
  /**
   * Reject addresses whose domain could not be checked (default false).
   * Rejecting them turns a DNS outage into a sign-up outage.
   */
  rejectUnknownDomain?: boolean;
}

export type RejectionReason = "invalid_syntax" | "undeliverable_domain" | "unknown_domain" | "disposable" | "role";

export type PolicyDecision =
  | { accepted: true; reason: null; message: null }
  | { accepted: false; reason: RejectionReason; message: string };

const MESSAGES: Record<RejectionReason, string> = {
  invalid_syntax: "Please enter a valid email address.",
  undeliverable_domain: "This domain does not appear to accept email.",
  unknown_domain: "We could not check this email domain right now. Please try again.",
  disposable: "Please use a permanent email address.",
  role: "Please use a personal email address.",
};

/**
 * Applies an acceptance policy to a validation result. Acceptance never means
 * the mailbox exists: use ownership verification for that.
 */
export function evaluate(result: ValidationResult, policy: Policy = {}): PolicyDecision {
  const reject = (reason: RejectionReason): PolicyDecision => ({
    accepted: false,
    reason,
    message: MESSAGES[reason],
  });
  if (result.syntax.status === "invalid") return reject("invalid_syntax");
  if (result.domain.status === "undeliverable") return reject("undeliverable_domain");
  if (result.domain.status === "unknown" && policy.rejectUnknownDomain) return reject("unknown_domain");
  if (result.disposable.status === "disposable" && (policy.rejectDisposable ?? true)) return reject("disposable");
  if (result.role.status === "role" && policy.rejectRole) return reject("role");
  return { accepted: true, reason: null, message: null };
}
