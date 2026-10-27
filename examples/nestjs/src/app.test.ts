// Runs against the tsc output, which carries the decorator metadata Nest needs.
import "reflect-metadata";
import type { INestApplication } from "@nestjs/common";
import { Test } from "@nestjs/testing";
import { createValidly } from "@validly/server";
import request from "supertest";
import { afterAll, beforeAll, expect, it } from "vitest";
import { fakeValidly } from "../../shared/fake-validly.ts";

const { AppModule } = (await import("../dist/app.module.js")) as { AppModule: new () => unknown };
const { VALIDLY } = (await import("../dist/validly.js")) as { VALIDLY: symbol };

let app: INestApplication;
beforeAll(async () => {
  const moduleRef = await Test.createTestingModule({ imports: [AppModule] })
    .overrideProvider(VALIDLY)
    .useValue(createValidly({ fetch: fakeValidly }))
    .compile();
  app = moduleRef.createNestApplication({ logger: false });
  await app.init();
});
afterAll(() => app.close());

it("registers deliverable addresses", async () => {
  const res = await request(app.getHttpServer()).post("/register").send({ email: "person@example.com" });
  expect(res.status).toBe(201);
});

it("refuses undeliverable and disposable addresses", async () => {
  const res = await request(app.getHttpServer()).post("/register").send({ email: "person@nomail.example" });
  expect(res.status).toBe(422);
  expect(res.body.error.reason).toBe("undeliverable_domain");
});

it("proxies validation for browsers", async () => {
  const res = await request(app.getHttpServer()).post("/api/validate").send({ email: "person@example.com" });
  expect(res.status).toBe(200);
  expect(res.body.overall_status).toBe("domain_deliverable");
});
