import type { ServerValidly } from "@validly/server";
import { getEmailCheck, requireValidEmail, validateHandler } from "@validly/server/fastify";
import Fastify from "fastify";

export function createApp(validly: ServerValidly) {
  const app = Fastify({ bodyLimit: 8192 });

  app.post("/api/validate", validateHandler(validly));

  app.post("/register", { preHandler: requireValidEmail(validly) }, async (request, reply) => {
    const check = getEmailCheck(request);
    return reply.code(201).send({ registered: true, emailChecked: !check?.unavailable });
  });

  return app;
}
