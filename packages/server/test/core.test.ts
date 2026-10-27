import { describe, expect, it } from "vitest";
import { validly } from "./helpers.js";

describe("check", () => {
  it("applies the policy", async () => {
    const v = validly();
    expect((await v.check("a@example.com")).accepted).toBe(true);
    expect(await v.check("a@nomail.example")).toMatchObject({ accepted: false, reason: "undeliverable_domain" });
    expect(await v.check("a@mailinator.com")).toMatchObject({ accepted: false, reason: "disposable" });
    expect((await validly({ policy: { rejectDisposable: false } }).check("a@mailinator.com")).accepted).toBe(true);
  });

  it("lets addresses through when Validly is down unless told otherwise", async () => {
    expect(await validly().check("a@down.example")).toMatchObject({ accepted: true, unavailable: true, result: null });
    expect(await validly({ whenUnavailable: "reject" }).check("a@down.example")).toMatchObject({
      accepted: false,
      reason: "unavailable",
    });
  });

  it("throws on configuration errors instead of accepting everything", async () => {
    const { createValidly } = await import("../src/index.js");
    const v = createValidly({
      whenUnavailable: "allow",
      fetch: async () =>
        Response.json({ error: { code: "unauthorized", message: "Invalid API key." } }, { status: 401 }),
    });
    await expect(v.check("a@example.com")).rejects.toMatchObject({ kind: "api", status: 401 });
  });

  it("sends the API key upstream", async () => {
    let auth: string | null = null;
    const { createValidly } = await import("../src/index.js");
    const v = createValidly({
      apiKey: "server-secret",
      fetch: async (_u, init) => {
        auth = new Headers(init?.headers).get("authorization");
        return new Response("{}", { status: 500 });
      },
    });
    await v.check("a@example.com");
    expect(auth).toBe("Bearer server-secret");
  });
});

describe("handleValidateRequest", () => {
  it("returns the result", async () => {
    const r = await validly().handleValidateRequest({ email: "a@example.com" });
    expect(r.status).toBe(200);
    expect(r.headers["cache-control"]).toBe("no-store");
    expect((r.body as { overall_status: string }).overall_status).toBe("domain_deliverable");
  });

  it("rejects malformed bodies", async () => {
    for (const body of [null, {}, { email: 1 }, { email: "a".repeat(600) }, "a@b.co"]) {
      expect((await validly().handleValidateRequest(body)).status).toBe(400);
    }
  });

  it("limits each client", async () => {
    const v = validly({ proxyRequestsPerMinute: 2 });
    const req = () => v.handleValidateRequest({ email: "a@example.com" }, { clientId: "198.51.100.7" });
    expect((await req()).status).toBe(200);
    expect((await req()).status).toBe(200);
    const limited = await req();
    expect(limited.status).toBe(429);
    expect(Number(limited.headers["retry-after"])).toBeGreaterThan(0);
    // Other clients are unaffected.
    expect((await v.handleValidateRequest({ email: "a@example.com" }, { clientId: "203.0.113.1" })).status).toBe(200);
  });

  it("finds the client behind a reverse proxy", async () => {
    const { forwardedClientId } = await import("../src/index.js");
    const headers = (h: Record<string, string>) => (n: string) => h[n];
    expect(forwardedClientId(headers({ "x-forwarded-for": "6.6.6.6, 203.0.113.9" }))).toBe("203.0.113.9");
    expect(forwardedClientId(headers({ "x-real-ip": "203.0.113.5" }))).toBe("203.0.113.5");
    expect(forwardedClientId(headers({}))).toBeUndefined();
  });

  it("maps upstream failures without leaking details", async () => {
    const down = await validly().handleValidateRequest({ email: "a@down.example" });
    expect(down).toMatchObject({ status: 502, body: { error: { code: "internal_error" } } });
    const busy = await validly().handleValidateRequest({ email: "a@busy.example" });
    expect(busy).toMatchObject({ status: 429, headers: { "retry-after": "3" } });
  });
});
