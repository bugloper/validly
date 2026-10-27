import Fastify from "fastify";
import { describe, expect, it } from "vitest";
import { getEmailCheck, requireValidEmail, validateHandler } from "../src/fastify.js";
import { validly } from "./helpers.js";

function app() {
  const v = validly({ whenUnavailable: "reject" });
  const f = Fastify();
  f.post("/api/validate", validateHandler(v));
  f.post("/register", { preHandler: requireValidEmail(v) }, async (req, reply) => {
    return reply.code(201).send({ ok: true, accepted: getEmailCheck(req)?.accepted });
  });
  return f;
}

describe("fastify", () => {
  it("accepts a good address", async () => {
    const res = await app().inject({ method: "POST", url: "/register", payload: { email: "a@example.com" } });
    expect(res.statusCode).toBe(201);
    expect(res.json()).toEqual({ ok: true, accepted: true });
  });

  it("rejects disposable addresses and reports outages", async () => {
    const bad = await app().inject({ method: "POST", url: "/register", payload: { email: "a@mailinator.com" } });
    expect(bad.statusCode).toBe(422);
    expect(bad.json().error.reason).toBe("disposable");
    const down = await app().inject({ method: "POST", url: "/register", payload: { email: "a@down.example" } });
    expect(down.statusCode).toBe(503);
  });

  it("proxies validation requests", async () => {
    const res = await app().inject({ method: "POST", url: "/api/validate", payload: { email: "a@example.com" } });
    expect(res.statusCode).toBe(200);
    expect(res.json().overall_status).toBe("domain_deliverable");
    const bad = await app().inject({ method: "POST", url: "/api/validate", payload: { nope: true } });
    expect(bad.statusCode).toBe(400);
  });
});
