import "reflect-metadata";
import type { INestApplication } from "@nestjs/common";
import { Body, Controller, HttpCode, Module, Post } from "@nestjs/common";
import { NestFactory } from "@nestjs/core";
import request from "supertest";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { ValidEmailPipe } from "../src/nestjs.js";
import { validly } from "./helpers.js";

const v = validly();

@Controller()
class RegistrationController {
  @Post("register")
  @HttpCode(201)
  register(@Body("email", new ValidEmailPipe(v)) email: string) {
    return { ok: true, email };
  }
}

@Module({ controllers: [RegistrationController] })
class AppModule {}

describe("nestjs", () => {
  let app: INestApplication;
  beforeAll(async () => {
    app = await NestFactory.create(AppModule, { logger: false });
    await app.init();
  });
  afterAll(() => app.close());

  it("accepts a good address", async () => {
    const res = await request(app.getHttpServer()).post("/register").send({ email: "a@example.com" });
    expect(res.status).toBe(201);
    expect(res.body).toEqual({ ok: true, email: "a@example.com" });
  });

  it("rejects bad addresses with 422", async () => {
    const res = await request(app.getHttpServer()).post("/register").send({ email: "a@mailinator.com" });
    expect(res.status).toBe(422);
    expect(res.body.error).toMatchObject({ code: "email_rejected", reason: "disposable" });
  });

  it("requires the field", async () => {
    expect((await request(app.getHttpServer()).post("/register").send({})).status).toBe(400);
  });
});
