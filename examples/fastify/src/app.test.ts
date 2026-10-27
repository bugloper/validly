import { createValidly } from "@validly/server";
import { expect, it } from "vitest";
import { fakeValidly } from "../../shared/fake-validly.ts";
import { createApp } from "./app.ts";

const app = createApp(createValidly({ fetch: fakeValidly }));
const register = (email: string) => app.inject({ method: "POST", url: "/register", payload: { email } });

it("registers deliverable addresses", async () => {
  expect((await register("person@example.com")).statusCode).toBe(201);
});

it("refuses undeliverable and disposable addresses", async () => {
  expect((await register("person@nomail.example")).json().error.reason).toBe("undeliverable_domain");
  expect((await register("person@mailinator.com")).statusCode).toBe(422);
});

it("proxies validation for browsers", async () => {
  const res = await app.inject({ method: "POST", url: "/api/validate", payload: { email: "person@example.com" } });
  expect(res.json().overall_status).toBe("domain_deliverable");
});
