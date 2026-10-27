import "server-only";
import {
  createValidly,
  type EmailCheck,
  forwardedClientId,
  type ServerOptions,
  type ServerValidly,
} from "@validly/server";

export type { EmailCheck, ServerOptions } from "@validly/server";

/**
 * Route Handler that lets browser code validate addresses through your
 * server, so the Validly API key never reaches the client:
 *
 *   // app/api/validate/route.ts
 *   export const { POST } = createValidateRoute({
 *     baseUrl: process.env.VALIDLY_URL,
 *     apiKey: process.env.VALIDLY_API_KEY,
 *   });
 */
export function createValidateRoute(
  options: ServerOptions | ServerValidly,
  route: {
    /**
     * Identifies the client for the per-client limit (see
     * `proxyRequestsPerMinute`). Defaults to the right-most X-Forwarded-For
     * entry, or X-Real-IP, as set by your hosting platform or reverse proxy.
     */
    clientId?: (request: Request) => string | undefined;
  } = {},
) {
  const validly = "handleValidateRequest" in options ? options : createValidly(options);
  const clientIdOf = route.clientId ?? ((req: Request) => forwardedClientId((n) => req.headers.get(n)));
  return {
    async POST(request: Request): Promise<Response> {
      let body: unknown = null;
      try {
        body = await request.json();
      } catch {
        // Fall through: handleValidateRequest rejects a missing body with 400.
      }
      const clientId = clientIdOf(request);
      const r = await validly.handleValidateRequest(body, {
        signal: request.signal,
        ...(clientId ? { clientId } : {}),
      });
      return Response.json(r.body, { status: r.status, headers: r.headers });
    },
  };
}

/**
 * Validates and applies the policy in Server Components and Server Actions:
 *
 *   const validly = createServerValidly({ apiKey: process.env.VALIDLY_API_KEY, ... });
 *   const check = await validly.check(formData.get("email"));
 */
export function createServerValidly(options: ServerOptions): ServerValidly {
  return createValidly(options);
}

/** Server Action helper: returns a form-friendly error message or null. */
export async function emailFieldError(
  validly: ServerValidly,
  value: FormDataEntryValue | null,
): Promise<string | null> {
  if (typeof value !== "string" || value === "") return "Please enter your email address.";
  const check: EmailCheck = await validly.check(value);
  return check.accepted ? null : check.message;
}
