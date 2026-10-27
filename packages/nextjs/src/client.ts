import { createClient, type EmailValidator } from "@validly/core";
import { type UseEmailValidationOptions, useEmailValidation as useReactEmailValidation } from "@validly/react";

// Types only: this module is marked "use client", so re-exported functions
// would become client references in Server Components. Import runtime
// helpers such as evaluate() or describeResult() from @validly/core.
export type {
  ResultMessage,
  UseEmailValidationResult,
  ValidationResult,
  ValidationState,
} from "@validly/react";

const clients = new Map<string, EmailValidator>();

/** A validator that posts to your own route (default "/api/validate"). */
export function createRouteClient(route = "/api/validate"): EmailValidator {
  let client = clients.get(route);
  if (!client) {
    client = createClient({ validateUrl: route });
    clients.set(route, client);
  }
  return client;
}

export interface UseNextEmailValidationOptions extends Omit<UseEmailValidationOptions, "client"> {
  /** A validator to use instead of the route client. */
  client?: EmailValidator | undefined;
  /** Your createValidateRoute() endpoint (default "/api/validate"). */
  route?: string;
}

/**
 * useEmailValidation() wired to your Route Handler instead of the Validly
 * API, so no credentials are needed in the browser.
 */
export function useEmailValidation(options: UseNextEmailValidationOptions = {}) {
  const { route, client, ...rest } = options;
  return useReactEmailValidation({ ...rest, client: client ?? createRouteClient(route) });
}
