import { createValidly } from "@validly/server";
import request from "supertest";
import { expect, it } from "vitest";
import { fakeValidly } from "../../shared/fake-validly.ts";
import { createApp } from "./app.ts";

const app = createApp(createValidly({ fetch: fakeValidly }));

it("registers deliverable addresses", async () => {
  const res = await request(app).post("/register").send({ email: "person@example.com" });
  expect(res.status).toBe(201);
});

it("refuses undeliverable, disposable and malformed addresses", async () => {
  for (const [email, reason] of [
    ["person@nomail.example", "undeliverable_domain"],
    ["person@mailinator.com", "disposable"],
    ["person@", "invalid_syntax"],
  ]) {
    const res = await request(app).post("/register").send({ email });
    expect(res.status).toBe(422);
    expect(res.body.error.reason).toBe(reason);
  }
});

it("proxies validation for browsers", async () => {
  const res = await request(app).post("/api/validate").send({ email: "person@example.com" });
  expect(res.body.overall_status).toBe("domain_deliverable");
});
