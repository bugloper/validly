import type { ErrorCode } from "./types.js";

export type ValidlyErrorKind =
  /** The API responded with an error status. */
  | "api"
  /** The request did not complete (offline, DNS failure, CORS, ...). */
  | "network"
  /** The request exceeded the configured timeout. */
  | "timeout"
  /** The caller aborted the request. */
  | "aborted"
  /** The response body was not what the API contract promises. */
  | "decode";

/**
 * The single error type thrown by Validly clients. Inspect `kind` (and `code`
 * for API errors) rather than relying on `instanceof`, which can fail when
 * two copies of the package are bundled.
 */
export class ValidlyError extends Error {
  override readonly name = "ValidlyError";
  readonly kind: ValidlyErrorKind;
  /** HTTP status for `api` errors. */
  readonly status: number | undefined;
  /** API error code for `api` errors; `unknown` if the body was not a Validly error. */
  readonly code: ErrorCode | "unknown" | undefined;
  /** Seconds to wait before retrying, from Retry-After on 429 responses. */
  readonly retryAfterSeconds: number | undefined;

  constructor(
    kind: ValidlyErrorKind,
    message: string,
    details: {
      status?: number;
      code?: ErrorCode | "unknown";
      retryAfterSeconds?: number;
      cause?: unknown;
    } = {},
  ) {
    super(message, details.cause === undefined ? undefined : { cause: details.cause });
    this.kind = kind;
    this.status = details.status;
    this.code = details.code;
    this.retryAfterSeconds = details.retryAfterSeconds;
  }
}

/** Reports whether value is a ValidlyError, even across package copies. */
export function isValidlyError(value: unknown): value is ValidlyError {
  return value instanceof Error && value.name === "ValidlyError" && "kind" in value;
}
