import {
  type Checks,
  createClient,
  createValidationController,
  describeResult,
  type EmailValidator,
  IDLE_STATE,
  type ResultMessage,
  type ValidationState,
} from "@validly/core";
import { getContext, setContext } from "svelte";
import { type Readable, readable } from "svelte/store";

export * from "@validly/core";

const CONTEXT_KEY = Symbol("validly");

/** Makes a validator available to createEmailValidation() in child components. Call during component init. */
export function setValidlyClient(client: EmailValidator): void {
  setContext(CONTEXT_KEY, client);
}

/** Reads the validator set by setValidlyClient(). Call during component init. */
export function getValidlyClient(): EmailValidator | undefined {
  return getContext<EmailValidator | undefined>(CONTEXT_KEY);
}

export interface EmailValidationValue extends ValidationState {
  /** True while a request is in flight. */
  loading: boolean;
  /** True while waiting for the debounce or a response. */
  pending: boolean;
  /** True when the address is invalid or its domain cannot receive mail. */
  invalid: boolean;
  message: ResultMessage | null;
}

export interface EmailValidationStore extends Readable<EmailValidationValue> {
  validate(email: string): void;
  validateNow(email: string): Promise<ValidationState>;
  reset(): void;
  /**
   * Svelte action for an <input>: validates on input and keeps aria-invalid
   * and aria-busy in sync. `<input type="email" use:validation.action />`
   */
  action(node: HTMLInputElement): { destroy(): void };
}

export interface CreateEmailValidationOptions {
  /** Validator to use. Defaults to a same-origin client posting to /api/v1/validate. */
  client?: EmailValidator;
  debounceMs?: number;
  checks?: Partial<Checks>;
  localSyntaxCheck?: boolean;
}

let defaultClient: EmailValidator | undefined;

/** The same-origin client used when no client is configured. */
function sharedClient(): EmailValidator {
  defaultClient ??= createClient();
  return defaultClient;
}

function enrich(state: ValidationState): EmailValidationValue {
  const r = state.result;
  return {
    ...state,
    loading: state.status === "validating",
    pending: state.status === "pending" || state.status === "validating",
    invalid: r?.syntax.status === "invalid" || r?.domain.status === "undeliverable",
    message: r ? describeResult(r) : null,
  };
}

/**
 * Creates a validation store with debouncing, cancellation of superseded
 * requests and protection against out-of-order responses. When the last
 * subscriber unsubscribes (e.g. the component using `$validation` is
 * destroyed), pending work is cancelled.
 */
export function createEmailValidation(options: CreateEmailValidationOptions = {}): EmailValidationStore {
  const controller = createValidationController({
    client: options.client ?? sharedClient(),
    ...(options.debounceMs !== undefined ? { debounceMs: options.debounceMs } : {}),
    ...(options.checks ? { checks: options.checks } : {}),
    ...(options.localSyntaxCheck !== undefined ? { localSyntaxCheck: options.localSyntaxCheck } : {}),
  });

  const store = readable<EmailValidationValue>(enrich(IDLE_STATE), (set) => {
    set(enrich(controller.getState()));
    const unsubscribe = controller.subscribe((s) => set(enrich(s)));
    return () => {
      unsubscribe();
      controller.reset();
    };
  });

  return {
    subscribe: store.subscribe,
    validate: controller.validate,
    validateNow: controller.validateNow,
    reset: controller.reset,
    action(node) {
      const onInput = () => controller.validate(node.value);
      node.addEventListener("input", onInput);
      const unsubscribe = store.subscribe((v) => {
        node.setAttribute("aria-invalid", String(v.invalid));
        node.setAttribute("aria-busy", String(v.loading));
      });
      return {
        destroy() {
          node.removeEventListener("input", onInput);
          unsubscribe();
        },
      };
    },
  };
}
