import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, effectScope, h, nextTick, ref } from "vue";
import {
  createValidly,
  type EmailValidator,
  useEmailValidation,
  type ValidationResult,
  ValidlyError,
} from "../src/index.js";

function deliverable(email: string): ValidationResult {
  return {
    email,
    syntax: { status: "valid", reason: null },
    domain: {
      normalized: "example.com",
      status: "deliverable",
      reason: "mx_records",
      mx_found: true,
      fallback_address_found: false,
    },
    disposable: { status: "not_disposable" },
    role: { status: "not_role" },
    suggestion: null,
    mailbox: { status: "unverified" },
    overall_status: "domain_deliverable",
  };
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("useEmailValidation", () => {
  it("validates with debouncing and exposes reactive state", async () => {
    const validate = vi.fn(async (email: string) => deliverable(email));
    const scope = effectScope();
    const v = scope.run(() => useEmailValidation({ client: { validate }, debounceMs: 100 }))!;

    v.validate("a@example.com");
    expect(v.pending.value).toBe(true);
    expect(v.loading.value).toBe(false);
    await vi.advanceTimersByTimeAsync(100);
    expect(validate).toHaveBeenCalledTimes(1);
    expect(v.status.value).toBe("done");
    expect(v.result.value?.overall_status).toBe("domain_deliverable");
    expect(v.invalid.value).toBe(false);
    scope.stop();
  });

  it("watches a model ref", async () => {
    const validate = vi.fn(async (email: string) => deliverable(email));
    const email = ref("");
    const scope = effectScope();
    const v = scope.run(() => useEmailValidation({ client: { validate }, email, debounceMs: 50 }))!;

    email.value = "bad";
    await nextTick();
    expect(v.invalid.value).toBe(true);
    expect(v.message.value?.text).toBe("Please enter a valid email address.");

    email.value = "a@example.com";
    await nextTick();
    await vi.advanceTimersByTimeAsync(50);
    expect(validate).toHaveBeenCalledWith("a@example.com", expect.anything());
    scope.stop();
  });

  it("cancels in-flight work when the scope is disposed", async () => {
    let signal: AbortSignal | undefined;
    const client: EmailValidator = {
      validate: (_e, opts) => {
        signal = opts?.signal;
        return new Promise(() => {});
      },
    };
    const scope = effectScope();
    const v = scope.run(() => useEmailValidation({ client, debounceMs: 0 }))!;
    v.validate("a@example.com");
    await vi.advanceTimersByTimeAsync(0);
    expect(v.loading.value).toBe(true);
    scope.stop();
    expect(signal?.aborted).toBe(true);
  });

  it("ignores stale responses", async () => {
    const pending: Array<(r: ValidationResult) => void> = [];
    const client: EmailValidator = { validate: () => new Promise((r) => pending.push(r)) };
    const scope = effectScope();
    const v = scope.run(() => useEmailValidation({ client, debounceMs: 0 }))!;
    v.validate("old@example.com");
    await vi.advanceTimersByTimeAsync(0);
    v.validate("new@example.com");
    await vi.advanceTimersByTimeAsync(0);
    pending[1]?.(deliverable("new@example.com"));
    await vi.runAllTimersAsync();
    pending[0]?.(deliverable("old@example.com"));
    await vi.runAllTimersAsync();
    expect(v.result.value?.email).toBe("new@example.com");
    scope.stop();
  });

  it("surfaces API errors", async () => {
    const client: EmailValidator = {
      validate: async () => {
        throw new ValidlyError("api", "down", { status: 503, code: "internal_error" });
      },
    };
    const scope = effectScope();
    const v = scope.run(() => useEmailValidation({ client, debounceMs: 0 }))!;
    v.validate("a@example.com");
    await vi.runAllTimersAsync();
    expect(v.status.value).toBe("error");
    expect(v.error.value?.status).toBe(503);
    scope.stop();
  });
});

describe("in a component with the plugin", () => {
  it("binds to v-model and renders messages", async () => {
    const validate = vi.fn(async (email: string) => {
      const r = deliverable(email);
      r.domain.status = "undeliverable";
      r.overall_status = "undeliverable";
      return r;
    });
    const EmailField = defineComponent({
      setup() {
        const email = ref("");
        const { loading, message, invalid } = useEmailValidation({ email, debounceMs: 10 });
        return () =>
          h("div", [
            h("input", {
              type: "email",
              autocomplete: "email",
              value: email.value,
              "aria-invalid": invalid.value,
              "aria-busy": loading.value,
              onInput: (e: Event) => {
                email.value = (e.target as HTMLInputElement).value;
              },
            }),
            h("p", { class: "msg" }, loading.value ? "Checking email..." : (message.value?.text ?? "")),
          ]);
      },
    });

    const wrapper = mount(EmailField, { global: { plugins: [createValidly({ client: { validate } })] } });
    await wrapper.find("input").setValue("a@nomail.example");
    await vi.advanceTimersByTimeAsync(10);
    await nextTick();
    expect(wrapper.find(".msg").text()).toBe("This domain does not appear to accept email.");
    expect(wrapper.find("input").attributes("aria-invalid")).toBe("true");

    wrapper.unmount();
  });
});
