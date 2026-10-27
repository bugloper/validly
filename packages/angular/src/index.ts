import {
  computed,
  DestroyRef,
  type EnvironmentProviders,
  InjectionToken,
  inject,
  makeEnvironmentProviders,
  type Signal,
  signal,
} from "@angular/core";
import type { AbstractControl, AsyncValidatorFn, ValidationErrors } from "@angular/forms";
import {
  type Checks,
  type ClientOptions,
  checkSyntax,
  createClient,
  createValidationController,
  describeResult,
  type EmailValidator,
  evaluate,
  IDLE_STATE,
  localInvalidResult,
  type Policy,
  type RejectionReason,
  type ResultMessage,
  type ValidationResult,
  type ValidationState,
  type ValidationStatus,
  type ValidlyError,
} from "@validly/core";
import { catchError, map, Observable, of, switchMap, timer } from "rxjs";

export * from "@validly/core";

/** The validator used by EmailValidationService. */
export const VALIDLY_CLIENT = new InjectionToken<EmailValidator>("VALIDLY_CLIENT");

/**
 * Registers Validly with the application injector.
 *
 *   bootstrapApplication(App, { providers: [provideValidly({ baseUrl: "https://validly.example.com" })] });
 *
 * Pass `client` to supply your own validator, e.g. one that calls your
 * backend. Never put API keys in browser code.
 */
export function provideValidly(options: ClientOptions & { client?: EmailValidator } = {}): EnvironmentProviders {
  const { client, ...clientOptions } = options;
  return makeEnvironmentProviders([
    { provide: VALIDLY_CLIENT, useValue: client ?? createClient(clientOptions) },
    { provide: EmailValidationService, useFactory: () => new EmailValidationService(inject(VALIDLY_CLIENT)) },
  ]);
}

/** Error object set on a form control under the `validly` key. */
export interface ValidlyValidationError {
  reason: RejectionReason | "unavailable";
  message: string;
  /** The full result, or null when the service was unavailable. */
  result: ValidationResult | null;
}

export interface EmailValidatorOptions {
  /** Delay before calling the API after the value changes (default 400ms). */
  debounceMs?: number;
  /** Which results reject the control (see Policy in @validly/core). */
  policy?: Policy;
  checks?: Partial<Checks>;
  /**
   * What to do when the API cannot be reached or errors:
   * `allow` (default) treats the value as valid so an outage does not block
   * your form; `reject` sets a `validly` error with reason `unavailable`.
   */
  whenUnavailable?: "allow" | "reject";
  /**
   * Called with every result (null when the service was unavailable), for
   * example to show a "Did you mean ...?" suggestion on an accepted address.
   */
  onResult?: (result: ValidationResult | null) => void;
}

/**
 * Validates email addresses. Provided by provideValidly(); inject it with
 * `inject(EmailValidationService)`.
 */
export class EmailValidationService {
  constructor(private readonly client: EmailValidator) {}

  /**
   * Validates one address. Unsubscribing aborts the HTTP request.
   */
  validate(email: string, options: { checks?: Partial<Checks> } = {}): Observable<ValidationResult> {
    return new Observable<ValidationResult>((subscriber) => {
      const controller = new AbortController();
      this.client
        .validate(email, { signal: controller.signal, ...(options.checks ? { checks: options.checks } : {}) })
        .then(
          (result) => {
            subscriber.next(result);
            subscriber.complete();
          },
          (err: unknown) => {
            if (!controller.signal.aborted) subscriber.error(err);
          },
        );
      return () => controller.abort();
    });
  }

  /**
   * An async validator for Reactive Forms. Angular unsubscribes from the
   * previous validation when the value changes, which cancels the debounce
   * timer and aborts any in-flight request.
   *
   *   email = new FormControl("", { validators: [Validators.required], asyncValidators: [service.validator()] });
   */
  validator(options: EmailValidatorOptions = {}): AsyncValidatorFn {
    const debounceMs = options.debounceMs ?? 400;
    return (control: AbstractControl): Observable<ValidationErrors | null> => {
      const value: unknown = control.value;
      if (typeof value !== "string" || value === "") return of(null);

      const local = checkSyntax(value);
      if (local.status === "invalid") {
        const result = localInvalidResult(value, local);
        options.onResult?.(result);
        return of(toErrors(evaluate(result, options.policy), result));
      }
      return timer(debounceMs).pipe(
        switchMap(() => this.validate(value, options.checks ? { checks: options.checks } : {})),
        map((result) => {
          options.onResult?.(result);
          return toErrors(evaluate(result, options.policy), result);
        }),
        catchError(() => {
          options.onResult?.(null);
          if (options.whenUnavailable !== "reject") return of(null);
          const error: ValidlyValidationError = {
            reason: "unavailable",
            message: "Validation service temporarily unavailable.",
            result: null,
          };
          return of({ validly: error });
        }),
      );
    };
  }
}

function toErrors(decision: ReturnType<typeof evaluate>, result: ValidationResult): ValidationErrors | null {
  if (decision.accepted) return null;
  const error: ValidlyValidationError = { reason: decision.reason, message: decision.message, result };
  return { validly: error };
}

export interface EmailValidationSignals {
  state: Signal<ValidationState>;
  status: Signal<ValidationStatus>;
  result: Signal<ValidationResult | null>;
  error: Signal<ValidlyError | null>;
  loading: Signal<boolean>;
  pending: Signal<boolean>;
  message: Signal<ResultMessage | null>;
  validate: (email: string) => void;
  validateNow: (email: string) => Promise<ValidationState>;
  reset: () => void;
}

/**
 * Signal-based validation state for templates, e.g. to show a "Did you mean"
 * suggestion. Must be called in an injection context (a constructor or field
 * initializer); pending work is cancelled when that context is destroyed.
 */
export function injectEmailValidation(
  options: { debounceMs?: number; checks?: Partial<Checks>; client?: EmailValidator } = {},
): EmailValidationSignals {
  const client = options.client ?? inject(VALIDLY_CLIENT);
  const controller = createValidationController({
    client,
    ...(options.debounceMs !== undefined ? { debounceMs: options.debounceMs } : {}),
    ...(options.checks ? { checks: options.checks } : {}),
  });
  const state = signal<ValidationState>(IDLE_STATE);
  controller.subscribe((s) => state.set(s));
  inject(DestroyRef).onDestroy(() => controller.destroy());

  const result = computed(() => state().result);
  return {
    state: state.asReadonly(),
    status: computed(() => state().status),
    result,
    error: computed(() => state().error),
    loading: computed(() => state().status === "validating"),
    pending: computed(() => state().status === "pending" || state().status === "validating"),
    message: computed(() => {
      const r = result();
      return r ? describeResult(r) : null;
    }),
    validate: controller.validate,
    validateNow: controller.validateNow,
    reset: controller.reset,
  };
}
