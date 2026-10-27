import { createValidly } from "@validly/server";
import { createApp } from "./app.ts";

const validly = createValidly({
  baseUrl: process.env.VALIDLY_URL ?? "http://localhost:8080",
  ...(process.env.VALIDLY_API_KEY ? { apiKey: process.env.VALIDLY_API_KEY } : {}),
  policy: { rejectDisposable: true },
});

const port = Number(process.env.PORT ?? 3001);
createApp(validly).listen(port, () => console.log(`express example on http://localhost:${port}`));
