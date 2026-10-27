import { Hono } from "hono";
import { describe, expect, it } from "vitest";
import { requireValidEmail, type ValidlyVariables, validateHandler } from "../src/hono.js";
import { validly } from "./helpers.js";

function app() {
  const v = validly();
  const h = new Hono<{ Variables: ValidlyVariables }>();
  h.post("/api/validate", validateHandler(v));
  h.post("/register", requireValidEmail(v), async (c) => {
    const body = await c.req.json<{ email: string }>();
    return c.json({ ok: true, email: body.email, accepted: c.get("validly").accepted }, 201);
  });
  return h;
}

const post = (path: string, body: unknown) =>
  app().request(path, { method: "POST", body: JSON.stringify(body), headers: { "content-type": "application/json" } });

describe("hono", () => {
  it("accepts a good address and leaves the body readable", async () => {
    const res = await post("/register", { email: "a@example.com" });
    expect(res.status).toBe(201);
    expect(await res.json()).toEqual({ ok: true, email: "a@example.com", accepted: true });
  });

  it("rejects bad addresses", async () => {
    const res = await post("/register", { email: "a@nomail.example" });
    expect(res.status).toBe(422);
    expect((await post("/register", {})).status).toBe(400);
  });

  it("proxies validation requests", async () => {
    const res = await post("/api/validate", { email: "a@busy.example" });
    expect(res.status).toBe(429);
    expect(res.headers.get("retry-after")).toBe("3");
  });
});
