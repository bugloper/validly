import type { ServerValidly } from "@validly/server";
import { getEmailCheck, requireValidEmail, validateHandler } from "@validly/server/express";
import express from "express";

export function createApp(validly: ServerValidly) {
  const app = express();
  app.use(express.json({ limit: "8kb" }));

  // Browsers call this instead of Validly, so the API key stays on the server.
  app.post("/api/validate", validateHandler(validly));

  // A registration endpoint that refuses invalid, undeliverable and
  // disposable addresses. Note that acceptance does not prove the mailbox
  // exists: send a verification email for that.
  app.post("/register", requireValidEmail(validly), (_req, res) => {
    const check = getEmailCheck(res);
    res.status(201).json({ registered: true, emailChecked: !check?.unavailable });
  });

  return app;
}
