import type { ValidationResult } from "../src/types.js";

export function deliverable(email: string): ValidationResult {
  return {
    email,
    syntax: { status: "valid", reason: null },
    domain: {
      normalized: email.slice(email.indexOf("@") + 1).toLowerCase(),
      status: "deliverable",
      reason: "mx_records",
      mx_found: true,
      fallback_address_found: false,
    },
    disposable: { status: "not_disposable" },
    role: { status: "not_role" },
    suggestion: null,
    mailbox: { status: "unverified" },
    overall_status: "domain_deliverable",
  };
}

export function jsonResponse(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json", ...headers },
  });
}

/** A fetch that never resolves until its signal aborts, like a stalled server. */
export const hangingFetch: typeof fetch = (_input, init) =>
  new Promise((_resolve, reject) => {
    if (init?.signal?.aborted) {
      reject(new DOMException("aborted", "AbortError"));
      return;
    }
    init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
  });
