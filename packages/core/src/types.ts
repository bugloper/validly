// Types mirroring the Validly API contract (openapi/openapi.json). The const
// arrays are checked against the OpenAPI enums in tests.

export const SYNTAX_STATUSES = ["valid", "invalid"] as const;
export type SyntaxStatus = (typeof SYNTAX_STATUSES)[number];

export const SYNTAX_REASONS = [
  "empty",
  "too_long",
  "contains_whitespace",
  "missing_at_sign",
  "multiple_at_signs",
  "local_part_empty",
  "local_part_too_long",
  "local_part_invalid_character",
  "local_part_dot_placement",
  "quoted_local_part_unsupported",
  "non_ascii_local_part_unsupported",
  "domain_empty",
  "domain_literal_unsupported",
  "domain_invalid",
  "domain_too_long",
  "domain_label_too_long",
  "domain_not_fully_qualified",
] as const;
export type SyntaxReason = (typeof SYNTAX_REASONS)[number];

/**
 * `deliverable` means the domain publishes mail routing (MX or an address
 * fallback). It never proves that a particular mailbox exists.
 */
export const DOMAIN_STATUSES = ["deliverable", "undeliverable", "unknown", "skipped"] as const;
export type DomainStatus = (typeof DOMAIN_STATUSES)[number];

export const DOMAIN_REASONS = [
  "mx_records",
  "address_fallback",
  "null_mx",
  "nxdomain",
  "no_mail_records",
  "non_public_address",
  "reserved_domain",
  "timeout",
  "servfail",
  "resolver_error",
  "dns_unavailable",
  "not_requested",
  "syntax_invalid",
] as const;
export type DomainReason = (typeof DOMAIN_REASONS)[number];

export const DISPOSABLE_STATUSES = ["disposable", "not_disposable", "skipped"] as const;
export type DisposableStatus = (typeof DISPOSABLE_STATUSES)[number];

export const ROLE_STATUSES = ["role", "not_role", "skipped"] as const;
export type RoleStatus = (typeof ROLE_STATUSES)[number];

export const MAILBOX_STATUSES = ["unverified", "verified"] as const;
export type MailboxStatus = (typeof MAILBOX_STATUSES)[number];

export const OVERALL_STATUSES = ["invalid", "undeliverable", "unknown", "domain_deliverable", "syntax_valid"] as const;
export type OverallStatus = (typeof OVERALL_STATUSES)[number];

export const ERROR_CODES = [
  "invalid_request",
  "invalid_email",
  "invalid_domain",
  "invalid_token",
  "unauthorized",
  "not_found",
  "method_not_allowed",
  "payload_too_large",
  "unsupported_media_type",
  "rate_limited",
  "verification_disabled",
  "internal_error",
] as const;
export type ErrorCode = (typeof ERROR_CODES)[number];

export interface ValidationResult {
  /** The submitted address, unmodified. */
  email: string;
  syntax: { status: SyntaxStatus; reason: SyntaxReason | null };
  domain: {
    /** Lowercase ASCII (IDNA) domain, or null if it could not be parsed. */
    normalized: string | null;
    status: DomainStatus;
    reason: DomainReason | null;
    /** null when DNS was skipped or could not give a reliable answer. */
    mx_found: boolean | null;
    fallback_address_found: boolean | null;
  };
  disposable: { status: DisposableStatus };
  /** Informational only: role addresses are not invalid. */
  role: { status: RoleStatus };
  /** A likely intended address. Never apply it without asking the user. */
  suggestion: { domain: string; email: string } | null;
  /** Always `unverified` in validation results. */
  mailbox: { status: MailboxStatus };
  overall_status: OverallStatus;
}

/** Optional validation stages. Each defaults to true. */
export interface Checks {
  dns: boolean;
  disposable: boolean;
  role: boolean;
  typos: boolean;
}
