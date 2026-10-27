import { createServerValidly } from "@validly/nextjs/server";

// Server-only module: "@validly/nextjs/server" imports "server-only", so a
// client component importing this file fails the build instead of leaking the key.
export const validly = createServerValidly({
  baseUrl: process.env.VALIDLY_URL ?? "http://localhost:8080",
  ...(process.env.VALIDLY_API_KEY ? { apiKey: process.env.VALIDLY_API_KEY } : {}),
  policy: { rejectDisposable: true },
});
