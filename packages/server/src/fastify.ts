import { type EmailCheck, rejectionBody, type ServerValidly } from "./index.js";

// Structural types keep this adapter free of a dependency on fastify's types.
interface Request {
  body?: unknown;
  /** Fastify's client IP, which honours its trustProxy option. */
  ip?: string;
}
interface Reply {
  code(statusCode: number): Reply;
  header(name: string, value: string): Reply;
  send(payload?: unknown): Reply;
}

const checks = new WeakMap<object, EmailCheck>();

/**
 * A preHandler hook that validates `request.body[field]` and applies the
 * policy. Rejected addresses get a 422 response (503 if Validly was
 * unreachable and `whenUnavailable` is "reject").
 *
 *   app.post("/register", { preHandler: requireValidEmail(validly) }, handler)
 */
export function requireValidEmail(validly: ServerValidly, options: { field?: string } = {}) {
  const field = options.field ?? "email";
  // Async hooks that send a reply must return it so Fastify stops the chain.
  return async (request: Request, reply: Reply): Promise<Reply | undefined> => {
    const email = (request.body as Record<string, unknown> | undefined)?.[field];
    if (typeof email !== "string") {
      return reply.code(400).send({ error: { code: "invalid_request", message: `The ${field} field is required.` } });
    }
    const check = await validly.check(email);
    checks.set(request, check);
    if (!check.accepted) {
      return reply.code(check.reason === "unavailable" ? 503 : 422).send(rejectionBody(check));
    }
    return undefined;
  };
}

/** Reads the check stored by requireValidEmail for this request. */
export function getEmailCheck(request: object): EmailCheck | undefined {
  return checks.get(request);
}

/**
 * Route handler that proxies browser validation requests to Validly:
 * `app.post("/api/validate", validateHandler(validly))`. Clients are
 * rate-limited per `request.ip`; configure Fastify's trustProxy behind a proxy.
 */
export function validateHandler(validly: ServerValidly) {
  return async (request: Request, reply: Reply): Promise<Reply> => {
    const r = await validly.handleValidateRequest(request.body, request.ip ? { clientId: request.ip } : {});
    for (const [k, v] of Object.entries(r.headers)) reply.header(k, v);
    return reply.code(r.status).send(r.body);
  };
}
