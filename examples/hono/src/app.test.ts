import { createValidly } from "@validly/server";
import { expect, it } from "vitest";
import { fakeValidly } from "../../shared/fake-validly.ts";
import { createApp } from "./app.ts";

const app = createApp(createValidly({ fetch: fakeValidly }));
const post = (path: string, body: unknown) =>
  app.request(path, { method: "POST", body: JSON.stringify(body), headers: { "content-type": "application/json" } });

it("registers deliverable addresses", async () => {
  expect((await post("/register", { email: "person@example.com" })).status).toBe(201);
});

it("refuses undeliverable addresses", async () => {
  const res = await post("/register", { email: "person@nomail.example" });
  expect(res.status).toBe(422);
});

it("proxies validation for browsers", async () => {
  const res = await post("/api/validate", { email: "person@example.com" });
  expect((await res.json()).overall_status).toBe("domain_deliverable");
});
