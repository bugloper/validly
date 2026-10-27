import { type EmailCheck, rejectionBody, type ServerValidly } from "./index.js";

// Structural types keep this adapter free of a dependency on @types/express.
interface Request {
  body?: unknown;
  /** Express's client IP, which honours its "trust proxy" setting. */
  ip?: string | undefined;
}
interface Response {
  locals: Record<string, unknown>;
  status(code: number): Response;
  setHeader(name: string, value: string): unknown;
  json(body: unknown): unknown;
}
type Next = (err?: unknown) => void;

export interface RequireValidEmailOptions {
  /** Body field holding the address (default "email"). */
  field?: string;
}

/**
 * Express middleware that validates `req.body[field]` (parse JSON or form
 * bodies first) and applies the policy. Rejected addresses get a 422 response
 * (503 if Validly was unreachable and `whenUnavailable` is "reject"). The
 * check is stored on `res.locals.validly` for later handlers.
 */
export function requireValidEmail(validly: ServerValidly, options: RequireValidEmailOptions = {}) {
  const field = options.field ?? "email";
  return async (req: Request, res: Response, next: Next): Promise<void> => {
    const email = (req.body as Record<string, unknown> | undefined)?.[field];
    if (typeof email !== "string") {
      res.status(400).json({ error: { code: "invalid_request", message: `The ${field} field is required.` } });
      return;
    }
    let check: EmailCheck;
    try {
      check = await validly.check(email);
    } catch (err) {
      next(err);
      return;
    }
    res.locals.validly = check;
    if (!check.accepted) {
      res.status(check.reason === "unavailable" ? 503 : 422).json(rejectionBody(check));
      return;
    }
    next();
  };
}

/** Reads the check stored by requireValidEmail. */
export function getEmailCheck(res: { locals: Record<string, unknown> }): EmailCheck | undefined {
  return res.locals.validly as EmailCheck | undefined;
}

/**
 * Route handler that proxies browser validation requests to Validly, keeping
 * the API key on the server: `app.post("/api/validate", express.json(), validateHandler(validly))`.
 * Clients are rate-limited per `req.ip`; set Express's "trust proxy" when
 * running behind a reverse proxy.
 */
export function validateHandler(validly: ServerValidly) {
  return async (req: Request, res: Response): Promise<void> => {
    const r = await validly.handleValidateRequest(req.body, req.ip ? { clientId: req.ip } : {});
    for (const [k, v] of Object.entries(r.headers)) res.setHeader(k, v);
    res.status(r.status).json(r.body);
  };
}
