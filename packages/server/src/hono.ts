import type { Context, Handler, MiddlewareHandler } from "hono";
import { type EmailCheck, forwardedClientId, rejectionBody, type ServerValidly } from "./index.js";

export type ValidlyVariables = { validly: EmailCheck };

async function jsonBody(c: Context): Promise<unknown> {
  try {
    return await c.req.json();
  } catch {
    return null;
  }
}

/**
 * Middleware that validates the JSON body's `field` and applies the policy.
 * Rejected addresses get a 422 response (503 if Validly was unreachable and
 * `whenUnavailable` is "reject"). The check is available as `c.get("validly")`.
 */
export function requireValidEmail(
  validly: ServerValidly,
  options: { field?: string } = {},
): MiddlewareHandler<{ Variables: ValidlyVariables }> {
  const field = options.field ?? "email";
  return async (c, next) => {
    const body = await jsonBody(c);
    const email = (body as Record<string, unknown> | null)?.[field];
    if (typeof email !== "string") {
      return c.json({ error: { code: "invalid_request", message: `The ${field} field is required.` } }, 400);
    }
    const check = await validly.check(email, { signal: c.req.raw.signal });
    c.set("validly", check);
    if (!check.accepted) return c.json(rejectionBody(check), check.reason === "unavailable" ? 503 : 422);
    await next();
  };
}

/**
 * Route handler that proxies browser validation requests to Validly. Clients
 * are rate-limited by `clientId(c)`, which defaults to the right-most
 * X-Forwarded-For entry (or X-Real-IP) set by your reverse proxy.
 */
export function validateHandler(
  validly: ServerValidly,
  options: { clientId?: (c: Context) => string | undefined } = {},
): Handler {
  const clientIdOf = options.clientId ?? ((c: Context) => forwardedClientId((n) => c.req.header(n)));
  return async (c) => {
    const clientId = clientIdOf(c);
    const r = await validly.handleValidateRequest(await jsonBody(c), {
      signal: c.req.raw.signal,
      ...(clientId ? { clientId } : {}),
    });
    return new Response(JSON.stringify(r.body), {
      status: r.status,
      headers: { "content-type": "application/json", ...r.headers },
    });
  };
}
