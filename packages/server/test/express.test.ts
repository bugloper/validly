import express from "express";
import request from "supertest";
import { describe, expect, it } from "vitest";
import { getEmailCheck, requireValidEmail, validateHandler } from "../src/express.js";
import { validly } from "./helpers.js";

function app() {
  const v = validly();
  const a = express();
  a.use(express.json());
  a.post("/api/validate", validateHandler(v));
  a.post("/register", requireValidEmail(v), (_req, res) => {
    res.status(201).json({ ok: true, unavailable: getEmailCheck(res)?.unavailable });
  });
  return a;
}

describe("express", () => {
  it("accepts a good address", async () => {
    const res = await request(app()).post("/register").send({ email: "a@example.com" });
    expect(res.status).toBe(201);
    expect(res.body).toEqual({ ok: true, unavailable: false });
  });

  it("rejects bad addresses with 422", async () => {
    const res = await request(app()).post("/register").send({ email: "a@nomail.example" });
    expect(res.status).toBe(422);
    expect(res.body.error).toMatchObject({ code: "email_rejected", reason: "undeliverable_domain" });
    expect((await request(app()).post("/register").send({ email: "nope" })).body.error.reason).toBe("invalid_syntax");
  });

  it("requires the field", async () => {
    expect((await request(app()).post("/register").send({})).status).toBe(400);
  });

  it("proxies validation requests", async () => {
    const res = await request(app()).post("/api/validate").send({ email: "a@mailinator.com" });
    expect(res.status).toBe(200);
    expect(res.headers["cache-control"]).toBe("no-store");
    expect(res.body.disposable.status).toBe("disposable");
  });
});
