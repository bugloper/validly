import { createClient, type EmailValidator } from "@validly/core";

// Re-exported so the auto-import resolves from this package's dependencies,
// not the app's.
export { describeResult } from "@validly/core";

import { type UseEmailValidationOptions, useEmailValidation as useVueEmailValidation } from "@validly/vue";
import { useRuntimeConfig } from "#imports";

const clients = new Map<string, EmailValidator>();

/**
 * Auto-imported composable. Validates through this app's server route, so no
 * credentials are needed in the browser.
 */
export function useEmailValidation(options: UseEmailValidationOptions = {}) {
  const route = useRuntimeConfig().public.validly?.route || "/api/validly/validate";
  let client = options.client ?? clients.get(route);
  if (!client) {
    client = createClient({ validateUrl: route });
    clients.set(route, client);
  }
  return useVueEmailValidation({ ...options, client });
}
