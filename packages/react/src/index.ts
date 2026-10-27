import {
  type Checks,
  type ControllerOptions,
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
  type ChangeEvent,
  createContext,
  createElement,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useSyncExternalStore,
} from "react";

export * from "@validly/core";

const ValidlyContext = createContext<EmailValidator | null>(null);

export interface ValidlyProviderProps {
  /** A client from createClient(), or any object with a compatible validate method. */
  client: EmailValidator;
  children?: ReactNode;
}

/** Provides a validator to every useEmailValidation() below it. */
export function ValidlyProvider({ client, children }: ValidlyProviderProps) {
  return createElement(ValidlyContext.Provider, { value: client }, children);
}

let defaultClient: EmailValidator | undefined;

/** The same-origin client used when no client is configured. */
function sharedClient(): EmailValidator {
  defaultClient ??= createClient();
  return defaultClient;
}

export interface UseEmailValidationOptions extends Omit<ControllerOptions, "client" | "checks"> {
  /**
   * Validator to use. Defaults to the nearest ValidlyProvider, then to a
   * same-origin client posting to /api/v1/validate.
   */
  client?: EmailValidator | undefined;
  checks?: Partial<Checks>;
}

export interface EmailInputProps {
  type: "email";
  autoComplete: "email";
  onChange: (event: ChangeEvent<HTMLInputElement>) => void;
  "aria-invalid": boolean;
  "aria-busy": boolean;
  "aria-describedby"?: string;
}

export interface UseEmailValidationResult {
  status: ValidationStatus;
  /** Result for the current input, or null while pending, on error or when idle. */
  result: ValidationResult | null;
  /** Set when the API was unreachable or returned an error. */
  error: ValidlyError | null;
  /** True while a request is in flight. */
  loading: boolean;
  /** True while waiting for the debounce or a response. */
  pending: boolean;
  /** A user-facing message for the current result, if any. */
  message: ResultMessage | null;
  /** Debounced validation, for input events. */
  validate: (email: string) => void;
  /** Immediate validation, for form submission. */
  validateNow: (email: string) => Promise<ValidationState>;
  reset: () => void;
  /**
   * Props for an <input>: type, autocomplete, change handler and ARIA state.
   * Pass the id of the element showing `message` as describedBy.
   */
  getInputProps: (options?: { describedBy?: string }) => EmailInputProps;
}

/**
 * Validates an email address as the user types. Requests are debounced,
 * superseded requests are cancelled, and out-of-order responses are ignored.
 */
export function useEmailValidation(options: UseEmailValidationOptions = {}): UseEmailValidationResult {
  const contextClient = useContext(ValidlyContext);
  const client = options.client ?? contextClient ?? sharedClient();
  // Callers often pass an inline client object; read it through a ref so a new
  // object identity on each render does not recreate the controller.
  const clientRef = useRef(client);
  clientRef.current = client;
  const { debounceMs, localSyntaxCheck } = options;
  const checksKey = JSON.stringify(options.checks ?? null);

  // biome-ignore lint/correctness/useExhaustiveDependencies: checks are compared by value via checksKey
  const controller = useMemo(
    () =>
      createValidationController({
        client: { validate: (email, opts) => clientRef.current.validate(email, opts) },
        ...(debounceMs !== undefined ? { debounceMs } : {}),
        ...(localSyntaxCheck !== undefined ? { localSyntaxCheck } : {}),
        ...(options.checks ? { checks: options.checks } : {}),
      }),
    [debounceMs, localSyntaxCheck, checksKey],
  );
  useEffect(() => () => controller.destroy(), [controller]);

  const state = useSyncExternalStore(controller.subscribe, controller.getState, controller.getState);

  const validate = useCallback((email: string) => controller.validate(email), [controller]);
  const validateNow = useCallback((email: string) => controller.validateNow(email), [controller]);
  const reset = useCallback(() => controller.reset(), [controller]);

  const flagged = state.result?.syntax.status === "invalid" || state.result?.domain.status === "undeliverable";
  const loading = state.status === "validating";

  const getInputProps = useCallback(
    (opts: { describedBy?: string } = {}): EmailInputProps => ({
      type: "email",
      autoComplete: "email",
      onChange: (event) => controller.validate(event.target.value),
      "aria-invalid": flagged,
      "aria-busy": loading,
      ...(opts.describedBy ? { "aria-describedby": opts.describedBy } : {}),
    }),
    [controller, flagged, loading],
  );

  return {
    status: state.status,
    result: state.result,
    error: state.error,
    loading,
    pending: state.status === "pending" || loading,
    message: state.result ? describeResult(state.result) : null,
    validate,
    validateNow,
    reset,
    getInputProps,
  };
}
