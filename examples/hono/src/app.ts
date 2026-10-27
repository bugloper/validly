import type { ServerValidly } from "@validly/server";
import { requireValidEmail, type ValidlyVariables, validateHandler } from "@validly/server/hono";
import { Hono } from "hono";
import { bodyLimit } from "hono/body-limit";

export function createApp(validly: ServerValidly) {
  const app = new Hono<{ Variables: ValidlyVariables }>();
  app.use(bodyLimit({ maxSize: 8 * 1024 }));

  app.post("/api/validate", validateHandler(validly));

  app.post("/register", requireValidEmail(validly), (c) =>
    c.json({ registered: true, emailChecked: !c.get("validly").unavailable }, 201),
  );

  return app;
}
