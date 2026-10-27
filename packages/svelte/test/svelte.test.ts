import { get } from "svelte/store";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createEmailValidation, type EmailValidator, type ValidationResult, ValidlyError } from "../src/index.js";

function deliverable(email: string, domain: "deliverable" | "undeliverable" = "deliverable"): ValidationResult {
  return {
    email,
    syntax: { status: "valid", reason: null },
    domain: { normalized: "example.com", status: domain, reason: null, mx_found: null, fallback_address_found: null },
    disposable: { status: "not_disposable" },
    role: { status: "not_role" },
    suggestion: null,
    mailbox: { status: "unverified" },
    overall_status: domain === "deliverable" ? "domain_deliverable" : "undeliverable",
  };
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("createEmailValidation", () => {
  it("is a readable store with derived fields", async () => {
    const validate = vi.fn(async (email: string) => deliverable(email, "undeliverable"));
    const v = createEmailValidation({ client: { validate }, debounceMs: 100 });
    const values: string[] = [];
    const unsubscribe = v.subscribe((s) => values.push(s.status));

    v.validate("a@nomail.example");
    expect(get(v).pending).toBe(true);
    await vi.advanceTimersByTimeAsync(100);
    expect(validate).toHaveBeenCalledTimes(1);
    const s = get(v);
    expect(s.invalid).toBe(true);
    expect(s.message?.text).toBe("This domain does not appear to accept email.");
    expect(values).toEqual(["idle", "pending", "validating", "done"]);
    unsubscribe();
  });

  it("cancels pending work when the last subscriber leaves", async () => {
    let signal: AbortSignal | undefined;
    const client: EmailValidator = {
      validate: (_e, o) => {
        signal = o?.signal;
        return new Promise(() => {});
      },
    };
    const v = createEmailValidation({ client, debounceMs: 0 });
    const unsubscribe = v.subscribe(() => {});
    v.validate("a@example.com");
    await vi.advanceTimersByTimeAsync(0);
    expect(get(v).loading).toBe(true);
    unsubscribe();
    expect(signal?.aborted).toBe(true);
  });

  it("ignores stale responses", async () => {
    const pending: Array<(r: ValidationResult) => void> = [];
    const v = createEmailValidation({ client: { validate: () => new Promise((r) => pending.push(r)) }, debounceMs: 0 });
    const unsubscribe = v.subscribe(() => {});
    v.validate("old@example.com");
    await vi.advanceTimersByTimeAsync(0);
    v.validate("new@example.com");
    await vi.advanceTimersByTimeAsync(0);
    pending[1]?.(deliverable("new@example.com"));
    await vi.runAllTimersAsync();
    pending[0]?.(deliverable("old@example.com"));
    await vi.runAllTimersAsync();
    expect(get(v).result?.email).toBe("new@example.com");
    unsubscribe();
  });

  it("reports errors", async () => {
    const v = createEmailValidation({
      client: {
        validate: async () => {
          throw new ValidlyError("timeout", "slow");
        },
      },
      debounceMs: 0,
    });
    const unsubscribe = v.subscribe(() => {});
    v.validate("a@example.com");
    await vi.runAllTimersAsync();
    expect(get(v).error?.kind).toBe("timeout");
    unsubscribe();
  });

  it("provides an input action", async () => {
    const validate = vi.fn(async (email: string) => deliverable(email));
    const v = createEmailValidation({ client: { validate }, debounceMs: 10 });
    const input = document.createElement("input");
    const action = v.action(input);

    input.value = "nope";
    input.dispatchEvent(new Event("input"));
    expect(input.getAttribute("aria-invalid")).toBe("true");

    input.value = "a@example.com";
    input.dispatchEvent(new Event("input"));
    await vi.advanceTimersByTimeAsync(10);
    expect(validate).toHaveBeenCalledWith("a@example.com", expect.anything());
    await vi.runAllTimersAsync();
    expect(input.getAttribute("aria-invalid")).toBe("false");

    action.destroy();
    input.value = "b@example.com";
    input.dispatchEvent(new Event("input"));
    await vi.runAllTimersAsync();
    expect(validate).toHaveBeenCalledTimes(1);
  });
});
