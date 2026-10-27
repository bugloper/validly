import { createValidly } from "@validly/server";

export const VALIDLY = Symbol("VALIDLY");

/** Provider for a ServerValidly configured from the environment. */
export const validlyProvider = {
  provide: VALIDLY,
  useFactory: () =>
    createValidly({
      baseUrl: process.env.VALIDLY_URL ?? "http://localhost:8080",
      ...(process.env.VALIDLY_API_KEY ? { apiKey: process.env.VALIDLY_API_KEY } : {}),
    }),
};
