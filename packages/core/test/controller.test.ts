import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { EmailValidator, ValidateOptions } from "../src/client.js";
import { createValidationController, type ValidationState } from "../src/controller.js";
import { ValidlyError } from "../src/errors.js";
import type { ValidationResult } from "../src/types.js";
import { deliverable } from "./fixtures.js";

/** A validator whose responses are resolved manually by the test. */
function manualValidator() {
  const calls: Array<{
    email: string;
    options: ValidateOptions | undefined;
    resolve: (r: ValidationResult) => void;
    reject: (e: unknown) => void;
  }> = [];
  const validator: EmailValidator = {
    validate: (email, options) =>
      new Promise((resolve, reject) => {
        calls.push({ email, options, resolve, reject });
        options?.signal?.addEventListener("abort", () => reject(new ValidlyError("aborted", "aborted")));
      }),
  };
  return { validator, calls };
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("createValidationController", () => {
  it("debounces input", async () => {
    const { validator, calls } = manualValidator();
    const c = createValidationController({ client: validator, debounceMs: 300 });
    c.validate("a@example.co");
    c.validate("a@example.com");
    expect(c.getState()).toMatchObject({ status: "pending", email: "a@example.com" });
    await vi.advanceTimersByTimeAsync(299);
    expect(calls).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(1);
    expect(calls.map((x) => x.email)).toEqual(["a@example.com"]);
    expect(c.getState().status).toBe("validating");

    calls[0]?.resolve(deliverable("a@example.com"));
    await vi.runAllTimersAsync();
    expect(c.getState()).toMatchObject({ status: "done", email: "a@example.com", error: null });
    expect(c.getState().result?.overall_status).toBe("domain_deliverable");
  });

  it("cancels superseded requests and ignores stale responses", async () => {
    const { validator, calls } = manualValidator();
    const c = createValidationController({ client: validator, debounceMs: 10 });
    c.validate("first@example.com");
    await vi.advanceTimersByTimeAsync(10);
    c.validate("second@example.com");
    await vi.advanceTimersByTimeAsync(10);

    expect(calls[0]?.options?.signal?.aborted).toBe(true);
    // Even if the first response still arrives, it must not win.
    calls[0]?.resolve(deliverable("first@example.com"));
    calls[1]?.resolve(deliverable("second@example.com"));
    await vi.runAllTimersAsync();
    expect(c.getState().result?.email).toBe("second@example.com");
  });

  it("ignores a stale response that arrives after a newer one", async () => {
    // A validator that ignores abort signals, to simulate a response racing in.
    const resolvers: Array<(r: ValidationResult) => void> = [];
    const validator: EmailValidator = {
      validate: () => new Promise((resolve) => resolvers.push(resolve)),
    };
    const c = createValidationController({ client: validator, debounceMs: 0 });
    c.validate("old@example.com");
    await vi.advanceTimersByTimeAsync(0);
    c.validate("new@example.com");
    await vi.advanceTimersByTimeAsync(0);
    resolvers[1]?.(deliverable("new@example.com"));
    await vi.runAllTimersAsync();
    resolvers[0]?.(deliverable("old@example.com"));
    await vi.runAllTimersAsync();
    expect(c.getState().result?.email).toBe("new@example.com");
  });

  it("decides locally invalid addresses without the network", () => {
    const { validator, calls } = manualValidator();
    const c = createValidationController({ client: validator });
    c.validate("not-an-email");
    expect(calls).toHaveLength(0);
    expect(c.getState()).toMatchObject({ status: "done" });
    expect(c.getState().result?.syntax).toEqual({ status: "invalid", reason: "missing_at_sign" });
  });

  it("can skip the local check", async () => {
    const { validator, calls } = manualValidator();
    const c = createValidationController({ client: validator, localSyntaxCheck: false, debounceMs: 0 });
    c.validate("not-an-email");
    await vi.advanceTimersByTimeAsync(0);
    expect(calls).toHaveLength(1);
  });

  it("returns to idle on empty input and cancels pending work", async () => {
    const { validator, calls } = manualValidator();
    const c = createValidationController({ client: validator, debounceMs: 10 });
    c.validate("a@example.com");
    c.validate("");
    await vi.advanceTimersByTimeAsync(50);
    expect(calls).toHaveLength(0);
    expect(c.getState().status).toBe("idle");
  });

  it("surfaces errors", async () => {
    const validator: EmailValidator = {
      validate: async () => {
        throw new ValidlyError("network", "offline");
      },
    };
    const c = createValidationController({ client: validator, debounceMs: 0 });
    c.validate("a@example.com");
    await vi.runAllTimersAsync();
    expect(c.getState()).toMatchObject({ status: "error", result: null });
    expect(c.getState().error?.kind).toBe("network");
  });

  it("wraps foreign errors", async () => {
    const validator: EmailValidator = {
      validate: async () => {
        throw new Error("boom");
      },
    };
    const c = createValidationController({ client: validator, debounceMs: 0 });
    c.validate("a@example.com");
    await vi.runAllTimersAsync();
    expect(c.getState().error).toBeInstanceOf(ValidlyError);
  });

  it("validateNow skips the debounce and resolves with the final state", async () => {
    const { validator, calls } = manualValidator();
    const c = createValidationController({ client: validator, debounceMs: 1000 });
    const p = c.validateNow("a@example.com");
    expect(calls).toHaveLength(1);
    calls[0]?.resolve(deliverable("a@example.com"));
    await expect(p).resolves.toMatchObject({ status: "done", email: "a@example.com" });

    await expect(c.validateNow("bad")).resolves.toMatchObject({ status: "done" });
  });

  it("validateNow resolves with its own address even if the input changes", async () => {
    const { validator, calls } = manualValidator();
    const c = createValidationController({ client: validator, debounceMs: 10 });
    const submitted = c.validateNow("first@example.com");
    c.validate("second@example.com");
    await vi.advanceTimersByTimeAsync(10);

    // The submitted request is not aborted by later typing.
    expect(calls[0]?.options?.signal?.aborted).toBe(false);
    calls[1]?.resolve(deliverable("second@example.com"));
    await vi.runAllTimersAsync();
    calls[0]?.resolve(deliverable("first@example.com"));

    const outcome = await submitted;
    expect(outcome).toMatchObject({ status: "done", email: "first@example.com" });
    // The shared state still follows the latest input.
    expect(c.getState()).toMatchObject({ status: "done", email: "second@example.com" });
  });

  it("destroy aborts submitted requests", async () => {
    const { validator, calls } = manualValidator();
    const c = createValidationController({ client: validator });
    const submitted = c.validateNow("a@example.com");
    c.destroy();
    expect(calls[0]?.options?.signal?.aborted).toBe(true);
    await expect(submitted).resolves.toMatchObject({ status: "error", email: "a@example.com" });
  });

  it("passes requested checks to the client", async () => {
    const { validator, calls } = manualValidator();
    const c = createValidationController({ client: validator, checks: { dns: false }, debounceMs: 0 });
    c.validate("a@example.com");
    await vi.advanceTimersByTimeAsync(0);
    expect(calls[0]?.options?.checks).toEqual({ dns: false });
  });

  it("notifies subscribers and stops after unsubscribe/destroy", async () => {
    const { validator, calls } = manualValidator();
    const c = createValidationController({ client: validator, debounceMs: 0 });
    const seen: ValidationState["status"][] = [];
    const unsubscribe = c.subscribe((s) => seen.push(s.status));
    c.validate("a@example.com");
    await vi.advanceTimersByTimeAsync(0);
    calls[0]?.resolve(deliverable("a@example.com"));
    await vi.runAllTimersAsync();
    expect(seen).toEqual(["pending", "validating", "done"]);

    unsubscribe();
    c.reset();
    expect(seen).toHaveLength(3);

    c.validate("b@example.com");
    c.destroy();
    await vi.runAllTimersAsync();
    expect(calls).toHaveLength(1);
  });
});
