import {
  type Checks,
  createClient,
  createValidationController,
  describeResult,
  type EmailValidator,
  IDLE_STATE,
  type ResultMessage,
  type ValidationResult,
  type ValidationState,
  type ValidationStatus,
  type ValidlyError,
} from "@validly/core";
import {
  type App,
  type ComputedRef,
  computed,
  getCurrentScope,
  type InjectionKey,
  inject,
  onScopeDispose,
  type ShallowRef,
  shallowRef,
  toValue,
  type WatchSource,
  watch,
} from "vue";

export * from "@validly/core";

export const ValidlyKey: InjectionKey<EmailValidator> = Symbol("validly");

/**
 * Vue plugin that provides a validator to every useEmailValidation().
 *
 *   app.use(createValidly({ client: createClient({ baseUrl: "https://validly.example.com" }) }))
 */
export function createValidly(options: { client: EmailValidator }) {
  return {
    install(app: App) {
      app.provide(ValidlyKey, options.client);
    },
  };
}

let defaultClient: EmailValidator | undefined;

/** The same-origin client used when no client is configured. */
function sharedClient(): EmailValidator {
  defaultClient ??= createClient();
  return defaultClient;
}

export interface UseEmailValidationOptions {
  /** Validator to use. Defaults to the plugin's, then a same-origin client. */
  client?: EmailValidator;
  /** A ref (e.g. your v-model) to validate automatically whenever it changes. */
  email?: WatchSource<string>;
  debounceMs?: number;
  checks?: Partial<Checks>;
  localSyntaxCheck?: boolean;
}

export interface UseEmailValidation {
  state: Readonly<ShallowRef<ValidationState>>;
  status: ComputedRef<ValidationStatus>;
  result: ComputedRef<ValidationResult | null>;
  error: ComputedRef<ValidlyError | null>;
  /** True while a request is in flight. */
  loading: ComputedRef<boolean>;
  /** True while waiting for the debounce or a response. */
  pending: ComputedRef<boolean>;
  /** True when the address is invalid or its domain cannot receive mail. */
  invalid: ComputedRef<boolean>;
  message: ComputedRef<ResultMessage | null>;
  validate: (email: string) => void;
  validateNow: (email: string) => Promise<ValidationState>;
  reset: () => void;
}

/**
 * Reactive email validation with debouncing, cancellation of superseded
 * requests and protection against out-of-order responses. Pending work is
 * cancelled when the calling component (or effect scope) is disposed.
 */
export function useEmailValidation(options: UseEmailValidationOptions = {}): UseEmailValidation {
  const client = options.client ?? inject(ValidlyKey, null) ?? sharedClient();
  const controller = createValidationController({
    client,
    ...(options.debounceMs !== undefined ? { debounceMs: options.debounceMs } : {}),
    ...(options.checks ? { checks: options.checks } : {}),
    ...(options.localSyntaxCheck !== undefined ? { localSyntaxCheck: options.localSyntaxCheck } : {}),
  });

  const state = shallowRef<ValidationState>(IDLE_STATE);
  controller.subscribe((s) => {
    state.value = s;
  });

  // Never validate during server-side rendering: the request would outlive
  // the render, and relative URLs do not resolve on the server.
  if (options.email !== undefined && typeof window !== "undefined") {
    const source = options.email;
    watch(
      () => toValue(source),
      (email) => controller.validate(email),
      { immediate: true },
    );
  }
  if (getCurrentScope()) onScopeDispose(() => controller.destroy());

  const result = computed(() => state.value.result);
  return {
    state,
    status: computed(() => state.value.status),
    result,
    error: computed(() => state.value.error),
    loading: computed(() => state.value.status === "validating"),
    pending: computed(() => state.value.status === "pending" || state.value.status === "validating"),
    invalid: computed(
      () => result.value?.syntax.status === "invalid" || result.value?.domain.status === "undeliverable",
    ),
    message: computed(() => (result.value ? describeResult(result.value) : null)),
    validate: controller.validate,
    validateNow: controller.validateNow,
    reset: controller.reset,
  };
}
