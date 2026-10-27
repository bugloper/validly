import { ValidlyError } from "./errors.js";
import { type Checks, ERROR_CODES, type ErrorCode, type ValidationResult } from "./types.js";

export interface ClientOptions {
  /**
   * Validly API origin, e.g. "https://validly.example.com". Defaults to ""
   * (same origin), which suits a reverse proxy in front of the API.
   */
  baseUrl?: string;
  /**
   * Full URL (or path) for validation requests, overriding baseUrl. Use it to
   * point browser code at your own server-side proxy, e.g. "/api/validate".
   */
  validateUrl?: string;
  /**
   * API key. Server-side only: anything shipped to a browser is public.
   */
  apiKey?: string;
  /** Per-request timeout in milliseconds (default 10000). 0 or Infinity disables it. */
  timeoutMs?: number;
  /** Custom fetch, e.g. for testing or instrumentation. */
  fetch?: typeof globalThis.fetch;
  /** Extra headers sent with every request. */
  headers?: Record<string, string>;
}

export interface RequestOptions {
  signal?: AbortSignal;
}

export interface ValidateOptions extends RequestOptions {
  checks?: Partial<Checks>;
}

/** Anything that can validate an address. Framework adapters accept this. */
export interface EmailValidator {
  validate(email: string, options?: ValidateOptions): Promise<ValidationResult>;
}

export interface ValidlyClient extends EmailValidator {
  /** Emails a single-use verification link. Requires an API key (server-side). */
  sendVerification(email: string, options?: RequestOptions): Promise<void>;
  /** Consumes a verification token and returns the verified address. */
  confirmVerification(token: string, options?: RequestOptions): Promise<{ email: string }>;
}

const DEFAULT_TIMEOUT_MS = 10_000;
const MAX_TIMER_MS = 2_147_483_647;

/** Creates a client for the Validly HTTP API. */
export function createClient(options: ClientOptions = {}): ValidlyClient {
  const base = (options.baseUrl ?? "").replace(/\/+$/, "");
  const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  const doFetch = options.fetch ?? ((...args: Parameters<typeof fetch>) => globalThis.fetch(...args));

  async function post<T>(url: string, body: unknown, signal: AbortSignal | undefined): Promise<T> {
    const headers: Record<string, string> = {
      "content-type": "application/json",
      accept: "application/json",
      ...options.headers,
    };
    if (options.apiKey) headers.authorization = `Bearer ${options.apiKey}`;

    const { signal: combined, cleanup, timedOut } = linkSignals(signal, timeoutMs);
    let response: Response;
    try {
      response = await doFetch(url, {
        method: "POST",
        headers,
        body: JSON.stringify(body),
        signal: combined,
      });
    } catch (cause) {
      cleanup();
      if (timedOut()) throw new ValidlyError("timeout", `Validly request timed out after ${timeoutMs}ms`, { cause });
      if (signal?.aborted) throw new ValidlyError("aborted", "Validly request was aborted", { cause });
      throw new ValidlyError("network", "Validly request failed", { cause });
    }

    let text: string;
    try {
      text = await response.text();
    } catch (cause) {
      if (timedOut()) throw new ValidlyError("timeout", `Validly request timed out after ${timeoutMs}ms`, { cause });
      if (signal?.aborted) throw new ValidlyError("aborted", "Validly request was aborted", { cause });
      throw new ValidlyError("network", "Validly response could not be read", { cause });
    } finally {
      cleanup();
    }

    if (!response.ok) throw apiError(response, text);
    try {
      return JSON.parse(text) as T;
    } catch (cause) {
      throw new ValidlyError("decode", "Validly response was not valid JSON", { status: response.status, cause });
    }
  }

  return {
    async validate(email, opts = {}) {
      const result = await post<ValidationResult>(
        options.validateUrl ?? `${base}/api/v1/validate`,
        opts.checks ? { email, checks: opts.checks } : { email },
        opts.signal,
      );
      if (!isValidationResult(result)) {
        throw new ValidlyError("decode", "Validly response did not match the validation contract");
      }
      return result;
    },
    async sendVerification(email, opts = {}) {
      await post(`${base}/api/v1/verification/send`, { email }, opts.signal);
    },
    async confirmVerification(token, opts = {}) {
      const r = await post<{ email: string }>(`${base}/api/v1/verification/confirm`, { token }, opts.signal);
      return { email: r.email };
    },
  };
}

function apiError(response: Response, text: string): ValidlyError {
  let code: ErrorCode | "unknown" = "unknown";
  let message = `Validly API responded with HTTP ${response.status}`;
  try {
    const body = JSON.parse(text) as { error?: { code?: string; message?: string } };
    if (body.error?.code && (ERROR_CODES as readonly string[]).includes(body.error.code)) {
      code = body.error.code as ErrorCode;
      message = body.error.message || message;
    }
  } catch {
    // Not a Validly error body (e.g. a proxy error page).
  }
  const retryAfter = Number.parseInt(response.headers.get("retry-after") ?? "", 10);
  return new ValidlyError("api", message, {
    status: response.status,
    code,
    ...(retryAfter > 0 ? { retryAfterSeconds: retryAfter } : {}),
  });
}

function isValidationResult(v: unknown): v is ValidationResult {
  if (typeof v !== "object" || v === null) return false;
  const r = v as Record<string, unknown>;
  return (
    typeof r.email === "string" &&
    typeof r.overall_status === "string" &&
    typeof r.syntax === "object" &&
    typeof r.domain === "object" &&
    typeof r.mailbox === "object"
  );
}

/**
 * Combines a caller's signal with a timeout. Implemented by hand rather than
 * with AbortSignal.any/timeout to support older browsers.
 */
function linkSignals(signal: AbortSignal | undefined, timeoutMs: number) {
  const controller = new AbortController();
  let timedOut = false;
  const onAbort = () => controller.abort(signal?.reason);
  if (signal?.aborted) controller.abort(signal.reason);
  else signal?.addEventListener("abort", onAbort, { once: true });
  // setTimeout fires immediately for delays above 2^31-1 ms, so treat
  // Infinity as "no timeout" and clamp other large values.
  const timer =
    timeoutMs > 0 && Number.isFinite(timeoutMs)
      ? setTimeout(
          () => {
            timedOut = true;
            controller.abort();
          },
          Math.min(timeoutMs, MAX_TIMER_MS),
        )
      : undefined;
  return {
    signal: controller.signal,
    timedOut: () => timedOut,
    cleanup: () => {
      if (timer !== undefined) clearTimeout(timer);
      signal?.removeEventListener("abort", onAbort);
    },
  };
}
