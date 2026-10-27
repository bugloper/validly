import type { EmailValidator } from "./client.js";
import { isValidlyError, ValidlyError } from "./errors.js";
import { checkSyntax, localInvalidResult } from "./syntax.js";
import type { Checks, ValidationResult } from "./types.js";

export type ValidationStatus =
  /** Nothing to validate (empty input or after reset). */
  | "idle"
  /** Input changed; waiting for the debounce delay to pass. */
  | "pending"
  /** A request to the API is in flight. */
  | "validating"
  /** `result` holds the outcome for `email`. */
  | "done"
  /** The API could not be reached or returned an error; see `error`. */
  | "error";

export interface ValidationState {
  status: ValidationStatus;
  /** The address the state refers to. */
  email: string;
  result: ValidationResult | null;
  error: ValidlyError | null;
}

export interface ControllerOptions {
  /** A Validly client or any object with a compatible `validate` method. */
  client: EmailValidator;
  /** Delay after the last input before calling the API (default 300ms). */
  debounceMs?: number;
  /** Checks to request from the API. */
  checks?: Partial<Checks>;
  /**
   * Run the local syntax check first and skip the network for addresses that
   * are certainly invalid (default true).
   */
  localSyntaxCheck?: boolean;
}

export interface ValidationController {
  getState(): ValidationState;
  /** Calls listener after every state change. Returns an unsubscribe function. */
  subscribe(listener: (state: ValidationState) => void): () => void;
  /** Debounced validation, for input events. */
  validate(email: string): void;
  /**
   * Immediate validation, for form submission. Resolves with the outcome for
   * this email, even if the input changes before the response arrives.
   */
  validateNow(email: string): Promise<ValidationState>;
  /** Cancels pending work and returns to idle. */
  reset(): void;
  /** Cancels pending work and drops all listeners. */
  destroy(): void;
}

export const IDLE_STATE: ValidationState = Object.freeze({
  status: "idle",
  email: "",
  result: null,
  error: null,
}) as ValidationState;

/**
 * Framework-neutral validation state machine with debouncing, cancellation of
 * superseded requests and protection against out-of-order responses. The
 * framework packages are thin wrappers around it.
 */
export function createValidationController(options: ControllerOptions): ValidationController {
  const debounceMs = options.debounceMs ?? 300;
  const localCheck = options.localSyntaxCheck ?? true;
  const listeners = new Set<(s: ValidationState) => void>();

  let state: ValidationState = IDLE_STATE;
  let timer: ReturnType<typeof setTimeout> | undefined;
  // The debounced request; aborted as soon as the input changes.
  let inflight: AbortController | undefined;
  // validateNow() requests; they run to completion for their caller even if
  // the input changes meanwhile, and are aborted only by destroy().
  const submitted = new Set<AbortController>();
  // Incremented on every new input; results for older generations never
  // become the shared state.
  let generation = 0;

  function set(next: ValidationState) {
    state = next;
    for (const l of listeners) l(state);
  }

  function cancel() {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
    inflight?.abort();
    inflight = undefined;
  }

  /** Handles everything that can be decided without the network. */
  function begin(email: string): boolean {
    cancel();
    generation++;
    if (email === "") {
      set(IDLE_STATE);
      return false;
    }
    if (localCheck) {
      const local = checkSyntax(email);
      if (local.status === "invalid") {
        set({ status: "done", email, result: localInvalidResult(email, local), error: null });
        return false;
      }
    }
    return true;
  }

  /**
   * Validates email and returns the outcome for that email. The outcome
   * becomes the shared state only if no newer input arrived meanwhile.
   */
  async function run(email: string, gen: number, kind: "debounced" | "submitted"): Promise<ValidationState> {
    const controller = new AbortController();
    if (kind === "debounced") inflight = controller;
    else submitted.add(controller);
    set({ status: "validating", email, result: null, error: null });

    let outcome: ValidationState;
    try {
      const result = await options.client.validate(email, {
        signal: controller.signal,
        ...(options.checks ? { checks: options.checks } : {}),
      });
      outcome = { status: "done", email, result, error: null };
    } catch (err) {
      const error = isValidlyError(err) ? err : new ValidlyError("network", "Validation failed", { cause: err });
      outcome = { status: "error", email, result: null, error };
    } finally {
      if (inflight === controller) inflight = undefined;
      submitted.delete(controller);
    }
    if (gen === generation && !controller.signal.aborted) set(outcome);
    return outcome;
  }

  return {
    getState: () => state,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    validate(email) {
      if (!begin(email)) return;
      const gen = generation;
      set({ status: "pending", email, result: null, error: null });
      timer = setTimeout(() => {
        timer = undefined;
        void run(email, gen, "debounced");
      }, debounceMs);
    },
    validateNow(email) {
      if (!begin(email)) return Promise.resolve(state);
      return run(email, generation, "submitted");
    },
    reset() {
      cancel();
      generation++;
      set(IDLE_STATE);
    },
    destroy() {
      cancel();
      for (const c of submitted) c.abort();
      generation++;
      listeners.clear();
    },
  };
}
