import type { SyntaxReason, ValidationResult } from "./types.js";

/**
 * Result of the local syntax pre-check.
 *
 * - `valid`: the address passes every rule the server applies.
 * - `invalid`: the server is guaranteed to reject it with the same reason.
 * - `unchecked`: the domain is internationalized (or punycode). IDNA rules are
 *   applied by the server only, so the address must be sent to it.
 */
export type LocalSyntaxResult =
  | { status: "valid"; local: string; domain: string }
  | { status: "invalid"; reason: SyntaxReason; domain: string | null }
  | { status: "unchecked" };

const MAX_INPUT_BYTES = 512;
const MAX_ADDRESS = 254;
const MAX_LOCAL = 64;
const MAX_DOMAIN = 253;
const MAX_LABEL = 63;

// Matches Go's unicode.IsSpace, which the server uses. (JavaScript's \s also
// matches U+FEFF, which Go does not treat as space.)
const WHITESPACE = /[\t\n\v\f\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]/;
const ATEXT = /^[A-Za-z0-9!#$%&'*+\-/=?^_`{|}~.]+$/;
const LDH = /^[a-z0-9-]+$/;
const encoder = new TextEncoder();

/**
 * Checks email syntax locally, without any network access. It applies the
 * same rules as the Validly server, so it never rejects an address the server
 * would accept. It does not, and cannot, check whether the domain exists.
 */
export function checkSyntax(email: string): LocalSyntaxResult {
  const invalid = (reason: SyntaxReason, domain: string | null = null): LocalSyntaxResult => ({
    status: "invalid",
    reason,
    domain,
  });

  if (email === "") return invalid("empty");
  if (encoder.encode(email).length > MAX_INPUT_BYTES) return invalid("too_long");
  if (hasLoneSurrogate(email)) return { status: "unchecked" };
  if (WHITESPACE.test(email)) return invalid("contains_whitespace");
  if (email.startsWith('"')) return invalid("quoted_local_part_unsupported");

  const at = email.indexOf("@");
  if (at < 0) return invalid("missing_at_sign");
  if (email.indexOf("@", at + 1) >= 0) return invalid("multiple_at_signs");
  const local = email.slice(0, at);
  const rawDomain = email.slice(at + 1);

  const domain = parseDomain(rawDomain);
  if (domain === "unchecked") return { status: "unchecked" };
  if (typeof domain !== "string") return invalid(domain.reason);

  const localReason = checkLocal(local);
  if (localReason) return invalid(localReason, domain);
  if (local.length + 1 + domain.length > MAX_ADDRESS) return invalid("too_long", domain);
  return { status: "valid", local, domain };
}

function checkLocal(local: string): SyntaxReason | null {
  if (local === "") return "local_part_empty";
  // biome-ignore lint/suspicious/noControlCharactersInRegex: matching the ASCII range is the point
  if (/[^\x00-\x7f]/.test(local)) return "non_ascii_local_part_unsupported";
  if (local.length > MAX_LOCAL) return "local_part_too_long";
  if (!ATEXT.test(local)) return "local_part_invalid_character";
  if (local.startsWith(".") || local.endsWith(".") || local.includes("..")) {
    return "local_part_dot_placement";
  }
  return null;
}

function parseDomain(raw: string): string | { reason: SyntaxReason } | "unchecked" {
  if (raw === "") return { reason: "domain_empty" };
  if (raw.startsWith("[")) return { reason: "domain_literal_unsupported" };
  if (raw.endsWith(".")) return { reason: "domain_invalid" };
  // biome-ignore lint/suspicious/noControlCharactersInRegex: matching the ASCII range is the point
  if (/[^\x00-\x7f]/.test(raw)) return "unchecked";

  const domain = raw.toLowerCase();
  const labels = domain.split(".");

  // Mirrors the server's IDNA step, which runs before length checks: it
  // rejects non-LDH characters and bad hyphens but tolerates empty labels.
  for (const label of labels) {
    if (label === "") continue;
    if (label.startsWith("xn--")) return "unchecked";
    if (!LDH.test(label)) return { reason: "domain_invalid" };
    if (label.startsWith("-") || label.endsWith("-")) return { reason: "domain_invalid" };
    if (label.length >= 4 && label[2] === "-" && label[3] === "-") return { reason: "domain_invalid" };
  }

  if (domain.length > MAX_DOMAIN) return { reason: "domain_too_long" };
  for (const label of labels) {
    if (label === "") return { reason: "domain_invalid" };
    if (label.length > MAX_LABEL) return { reason: "domain_label_too_long" };
  }
  if (labels.length < 2) return { reason: "domain_not_fully_qualified" };
  if (/^[0-9]+$/.test(labels[labels.length - 1] ?? "")) return { reason: "domain_invalid" };
  return domain;
}

function hasLoneSurrogate(s: string): boolean {
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    if (c >= 0xd800 && c <= 0xdbff) {
      const next = s.charCodeAt(i + 1);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return true;
      i++;
    } else if (c >= 0xdc00 && c <= 0xdfff) {
      return true;
    }
  }
  return false;
}

/**
 * Builds the result the server would return for an address that failed the
 * local syntax check, so UIs can render local and remote results uniformly.
 */
export function localInvalidResult(
  email: string,
  check: Extract<LocalSyntaxResult, { status: "invalid" }>,
): ValidationResult {
  return {
    email,
    syntax: { status: "invalid", reason: check.reason },
    domain: {
      normalized: check.domain,
      status: "skipped",
      reason: "syntax_invalid",
      mx_found: null,
      fallback_address_found: null,
    },
    disposable: { status: "skipped" },
    role: { status: "skipped" },
    suggestion: null,
    mailbox: { status: "unverified" },
    overall_status: "invalid",
  };
}
