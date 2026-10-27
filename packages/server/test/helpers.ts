import type { ValidationResult } from "@validly/core";
import { createValidly, type ServerOptions } from "../src/index.js";

export function result(
  email: string,
  overrides: {
    domain?: ValidationResult["domain"]["status"];
    disposable?: ValidationResult["disposable"]["status"];
  } = {},
): ValidationResult {
  const domain = overrides.domain ?? "deliverable";
  return {
    email,
    syntax: { status: "valid", reason: null },
    domain: {
      normalized: email.split("@")[1] ?? null,
      status: domain,
      reason: null,
      mx_found: null,
      fallback_address_found: null,
    },
    disposable: { status: overrides.disposable ?? "not_disposable" },
    role: { status: "not_role" },
    suggestion: null,
    mailbox: { status: "unverified" },
    overall_status:
      domain === "deliverable" ? "domain_deliverable" : domain === "undeliverable" ? "undeliverable" : "unknown",
  };
}

/**
 * A fake upstream Validly: addresses at "nomail.example" are undeliverable,
 * "mailinator.com" is disposable, "down.example" makes the API fail and
 * "busy.example" returns 429.
 */
export const upstream: typeof fetch = async (_url, init) => {
  const { email } = JSON.parse(String(init?.body)) as { email: string };
  const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
    new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json", ...headers } });
  if (email.endsWith("@down.example")) return json({ error: { code: "internal_error", message: "x" } }, 500);
  if (email.endsWith("@busy.example"))
    return json({ error: { code: "rate_limited", message: "x" } }, 429, { "retry-after": "3" });
  if (email.endsWith("@nomail.example")) return json(result(email, { domain: "undeliverable" }));
  if (email.endsWith("@mailinator.com")) return json(result(email, { disposable: "disposable" }));
  return json(result(email));
};

export function validly(options: ServerOptions = {}) {
  return createValidly({ baseUrl: "http://validly.internal", apiKey: "server-secret", fetch: upstream, ...options });
}
