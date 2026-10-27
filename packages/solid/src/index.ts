import {
  type Checks,
  createClient,
  createValidationController,
  describeResult,
  type EmailValidator,
  type ResultMessage,
  type ValidationResult,
  type ValidationState,
  type ValidationStatus,
  type ValidlyError,
} from "@validly/core";
import {
  type Accessor,
  createContext,
  createEffect,
  createMemo,
  createSignal,
  getOwner,
  on,
  onCleanup,
  useContext,
} from "solid-js";

export * from "@validly/core";

/** Provide a validator to createEmailValidation() calls below it. */
export const ValidlyContext = createContext<EmailValidator>();

export interface CreateEmailValidationOptions {
  /** Validator to use. Defaults to ValidlyContext, then a same-origin client. */
  client?: EmailValidator;
  /** An accessor (e.g. a signal) to validate automatically whenever it changes. */
  email?: Accessor<string>;
  debounceMs?: number;
  checks?: Partial<Checks>;
  localSyntaxCheck?: boolean;
}

export interface EmailValidation {
  state: Accessor<ValidationState>;
  status: Accessor<ValidationStatus>;
  result: Accessor<ValidationResult | null>;
  error: Accessor<ValidlyError | null>;
  /** True while a request is in flight. */
  loading: Accessor<boolean>;
  /** True while waiting for the debounce or a response. */
  pending: Accessor<boolean>;
  /** True when the address is invalid or its domain cannot receive mail. */
  invalid: Accessor<boolean>;
  message: Accessor<ResultMessage | null>;
  validate: (email: string) => void;
  validateNow: (email: string) => Promise<ValidationState>;
  reset: () => void;
}

let defaultClient: EmailValidator | undefined;

/** The same-origin client used when no client is configured. */
function sharedClient(): EmailValidator {
  defaultClient ??= createClient();
  return defaultClient;
}

/**
 * Reactive email validation with debouncing, cancellation of superseded
 * requests and protection against out-of-order responses. Pending work is
 * cancelled when the owning component or root is disposed.
 */
export function createEmailValidation(options: CreateEmailValidationOptions = {}): EmailValidation {
  const client = options.client ?? useContext(ValidlyContext) ?? sharedClient();
  const controller = createValidationController({
    client,
    ...(options.debounceMs !== undefined ? { debounceMs: options.debounceMs } : {}),
    ...(options.checks ? { checks: options.checks } : {}),
    ...(options.localSyntaxCheck !== undefined ? { localSyntaxCheck: options.localSyntaxCheck } : {}),
  });

  const [state, setState] = createSignal<ValidationState>(controller.getState(), { equals: false });
  controller.subscribe(setState);
  if (getOwner()) onCleanup(() => controller.destroy());

  const source = options.email;
  if (source) createEffect(on(source, (email) => controller.validate(email)));

  const result = createMemo(() => state().result);
  return {
    state,
    status: () => state().status,
    result,
    error: () => state().error,
    loading: () => state().status === "validating",
    pending: () => state().status === "pending" || state().status === "validating",
    invalid: () => result()?.syntax.status === "invalid" || result()?.domain.status === "undeliverable",
    message: createMemo(() => {
      const r = result();
      return r ? describeResult(r) : null;
    }),
    validate: controller.validate,
    validateNow: controller.validateNow,
    reset: controller.reset,
  };
}
