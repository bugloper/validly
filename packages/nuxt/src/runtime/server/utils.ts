import { createValidly, type ServerOptions, type ServerValidly } from "@validly/server";
import { useRuntimeConfig } from "#imports";

let cached: { key: string; validly: ServerValidly } | undefined;

/**
 * Server-side Validly helpers configured from private runtime config.
 * Auto-imported in server routes: `const check = await useValidly().check(email)`.
 */
export function useValidly(options: Omit<ServerOptions, "baseUrl" | "apiKey"> = {}): ServerValidly {
  const config = useRuntimeConfig().validly ?? {};
  const key = JSON.stringify([config.baseUrl, config.apiKey, options]);
  if (cached?.key !== key) {
    cached = {
      key,
      validly: createValidly({
        ...options,
        ...(config.baseUrl ? { baseUrl: config.baseUrl } : {}),
        ...(config.apiKey ? { apiKey: config.apiKey } : {}),
      }),
    };
  }
  return cached.validly;
}
