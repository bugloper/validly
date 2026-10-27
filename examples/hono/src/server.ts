import { serve } from "@hono/node-server";
import { createValidly } from "@validly/server";
import { createApp } from "./app.ts";

const validly = createValidly({
  baseUrl: process.env.VALIDLY_URL ?? "http://localhost:8080",
  ...(process.env.VALIDLY_API_KEY ? { apiKey: process.env.VALIDLY_API_KEY } : {}),
});

const port = Number(process.env.PORT ?? 3003);
serve({ fetch: createApp(validly).fetch, port, hostname: "127.0.0.1" });
console.log(`hono example on http://localhost:${port}`);
