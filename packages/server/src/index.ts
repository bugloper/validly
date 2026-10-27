import {
  type Checks,
  type ClientOptions,
  checkSyntax,
  createClient,
  evaluate,
  isValidlyError,
  localInvalidResult,
  type Policy,
  type RejectionReason,
  type ValidationResult,
  type ValidlyClient,
} from "@validly/core";

export * from "@validly/core";

export interface ServerOptions extends ClientOptions {
  /** Which results to reject. Defaults reject invalid, undeliverable and disposable. */
  policy?: Policy;
  /**
   * What check() does when Validly cannot be reached: `allow` (default) lets
   * the address through so an outage does not block sign-ups; `reject`
   * refuses it.
   */
  whenUnavailable?: "allow" | "reject";
  /** Checks to request (all by default). */
  checks?: Partial<Checks>;
  /**
   * Requests per minute each client may make through handleValidateRequest
   * (default 30; 0 disables). The proxy shares your API key between all of
   * your users, so without a per-client limit one script could use up the
   * key's quota for everyone. Applies only when a clientId is passed.
   */
  proxyRequestsPerMinute?: number;
}

export type EmailCheck =
  | { accepted: true; result: ValidationResult | null; unavailable: boolean }
  | {
      accepted: false;
      reason: RejectionReason | "unavailable";
      message: string;
      result: ValidationResult | null;
      unavailable: boolean;
    };

export interface ProxyResponse {
  status: number;
  headers: Record<string, string>;
  body: unknown;
}

export interface ServerValidly {
  client: ValidlyClient;
  /**
   * Validates email and applies the policy. When Validly is unavailable
   * (network failure, timeout, 429 or 5xx) the result follows
   * `whenUnavailable`. Configuration errors, such as a rejected API key,
   * throw a ValidlyError instead of silently accepting every address.
   */
  check(email: string, options?: { signal?: AbortSignal }): Promise<EmailCheck>;
  /**
   * Handles a browser's validation request on your server, so the API key
   * stays server-side. Accepts `{ "email": string }` and returns the Validly
   * result (or an error) ready to send as JSON. Pass `clientId` (e.g. the
   * client's IP address) to apply the per-client limit.
   */
  handleValidateRequest(body: unknown, options?: { signal?: AbortSignal; clientId?: string }): Promise<ProxyResponse>;
}

/** Longest address the proxy forwards; the API applies its own limit too. */
const MAX_EMAIL_LENGTH = 512;

/**
 * Creates server-side Validly helpers. Reads nothing from the environment:
 * pass the API key explicitly, e.g. `apiKey: process.env.VALIDLY_API_KEY`.
 */
export function createValidly(options: ServerOptions = {}): ServerValidly {
  const { policy, whenUnavailable = "allow", checks, proxyRequestsPerMinute = 30, ...clientOptions } = options;
  const client = createClient(clientOptions);
  const proxyLimiter = createLimiter(proxyRequestsPerMinute);

  async function check(email: string, opts: { signal?: AbortSignal } = {}): Promise<EmailCheck> {
    // Malformed input is rejected locally, without a network call.
    const local = checkSyntax(email);
    if (local.status === "invalid") {
      const result = localInvalidResult(email, local);
      const decision = evaluate(result, policy);
      if (!decision.accepted) {
        return { accepted: false, reason: decision.reason, message: decision.message, result, unavailable: false };
      }
    }
    let result: ValidationResult;
    try {
      result = await client.validate(email, {
        ...(checks ? { checks } : {}),
        ...(opts.signal ? { signal: opts.signal } : {}),
      });
    } catch (err) {
      if (!isUnavailable(err)) throw err;
      if (whenUnavailable === "allow") return { accepted: true, result: null, unavailable: true };
      return {
        accepted: false,
        reason: "unavailable",
        message: "We could not check this email address right now. Please try again.",
        result: null,
        unavailable: true,
      };
    }
    const decision = evaluate(result, policy);
    if (decision.accepted) return { accepted: true, result, unavailable: false };
    return { accepted: false, reason: decision.reason, message: decision.message, result, unavailable: false };
  }

  async function handleValidateRequest(
    body: unknown,
    opts: { signal?: AbortSignal; clientId?: string } = {},
  ): Promise<ProxyResponse> {
    if (opts.clientId !== undefined) {
      const wait = proxyLimiter(opts.clientId);
      if (wait > 0) {
        return errorResponse(429, "rate_limited", "Too many validation requests. Please try again shortly.", {
          "retry-after": String(Math.ceil(wait / 1000)),
        });
      }
    }
    const email = (body as { email?: unknown } | null)?.email;
    if (typeof email !== "string" || email.length > MAX_EMAIL_LENGTH) {
      return errorResponse(400, "invalid_request", "Send a JSON body with an email string.");
    }
    try {
      const result = await client.validate(email, {
        ...(checks ? { checks } : {}),
        ...(opts.signal ? { signal: opts.signal } : {}),
      });
      return { status: 200, headers: { "cache-control": "no-store" }, body: result };
    } catch (err) {
      return proxyError(err);
    }
  }

  return { client, check, handleValidateRequest };
}

/** Network failures, timeouts, throttling and server errors: Validly is unavailable. */
function isUnavailable(err: unknown): boolean {
  if (!isValidlyError(err)) return true;
  switch (err.kind) {
    case "network":
    case "timeout":
    case "decode":
      return true;
    case "api":
      return err.status === 429 || (err.status ?? 0) >= 500;
    default:
      return false;
  }
}

const MAX_TRACKED_CLIENTS = 10_000;

/**
 * A token-bucket limiter per client. Returns 0 when allowed, otherwise the
 * milliseconds until the next request is allowed. Memory is bounded: the
 * least recently seen clients are forgotten first.
 */
function createLimiter(perMinute: number): (clientId: string) => number {
  if (!(perMinute > 0)) return () => 0;
  const ratePerMs = perMinute / 60_000;
  const buckets = new Map<string, { tokens: number; last: number }>();
  return (clientId) => {
    const now = Date.now();
    let b = buckets.get(clientId);
    if (b) {
      buckets.delete(clientId); // re-insert to keep Map order = recency
      b.tokens = Math.min(perMinute, b.tokens + (now - b.last) * ratePerMs);
      b.last = now;
    } else {
      b = { tokens: perMinute, last: now };
      if (buckets.size >= MAX_TRACKED_CLIENTS) {
        const oldest = buckets.keys().next().value;
        if (oldest !== undefined) buckets.delete(oldest);
      }
    }
    buckets.set(clientId, b);
    if (b.tokens >= 1) {
      b.tokens -= 1;
      return 0;
    }
    return (1 - b.tokens) / ratePerMs;
  };
}

/**
 * The client address as seen by your nearest reverse proxy: the right-most
 * X-Forwarded-For entry, else X-Real-IP. Right-most because earlier entries
 * are supplied by the client and can be forged.
 */
export function forwardedClientId(header: (name: string) => string | null | undefined): string | undefined {
  const xff = header("x-forwarded-for");
  if (xff) {
    const last = xff.split(",").at(-1)?.trim();
    if (last) return last;
  }
  return header("x-real-ip")?.trim() || undefined;
}

function errorResponse(
  status: number,
  code: string,
  message: string,
  headers: Record<string, string> = {},
): ProxyResponse {
  return { status, headers: { "cache-control": "no-store", ...headers }, body: { error: { code, message } } };
}

/** Maps upstream failures without leaking upstream details to browsers. */
function proxyError(err: unknown): ProxyResponse {
  if (isValidlyError(err) && err.kind === "api" && err.status === 429) {
    return errorResponse(
      429,
      "rate_limited",
      "Too many validation requests. Please try again shortly.",
      err.retryAfterSeconds ? { "retry-after": String(err.retryAfterSeconds) } : {},
    );
  }
  if (isValidlyError(err) && err.kind === "api" && err.status === 400) {
    return errorResponse(400, "invalid_request", "The validation request was rejected.");
  }
  return errorResponse(502, "internal_error", "Validation service temporarily unavailable.");
}

/** Response body the framework adapters send when an address is rejected. */
export function rejectionBody(check: Extract<EmailCheck, { accepted: false }>) {
  return { error: { code: "email_rejected", reason: check.reason, message: check.message } };
}
