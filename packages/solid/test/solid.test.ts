import { createRoot, createSignal } from "solid-js";
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
  it("exposes accessors", async () => {
    const validate = vi.fn(async (email: string) => deliverable(email, "undeliverable"));
    await createRoot(async (dispose) => {
      const v = createEmailValidation({ client: { validate }, debounceMs: 100 });
      v.validate("a@nomail.example");
      expect(v.pending()).toBe(true);
      await vi.advanceTimersByTimeAsync(100);
      expect(validate).toHaveBeenCalledTimes(1);
      expect(v.status()).toBe("done");
      expect(v.invalid()).toBe(true);
      expect(v.message()?.text).toBe("This domain does not appear to accept email.");
      dispose();
    });
  });

  it("tracks an email signal", async () => {
    const validate = vi.fn(async (email: string) => deliverable(email));
    await createRoot(async (dispose) => {
      const [email, setEmail] = createSignal("");
      const v = createEmailValidation({ client: { validate }, email, debounceMs: 10 });
      setEmail("bad");
      await vi.advanceTimersByTimeAsync(0);
      expect(v.invalid()).toBe(true);
      setEmail("a@example.com");
      await vi.advanceTimersByTimeAsync(10);
      expect(validate).toHaveBeenCalledWith("a@example.com", expect.anything());
      dispose();
    });
  });

  it("cancels in-flight work on dispose", async () => {
    let signal: AbortSignal | undefined;
    const client: EmailValidator = {
      validate: (_e, o) => {
        signal = o?.signal;
        return new Promise(() => {});
      },
    };
    await createRoot(async (dispose) => {
      const v = createEmailValidation({ client, debounceMs: 0 });
      v.validate("a@example.com");
      await vi.advanceTimersByTimeAsync(0);
      expect(v.loading()).toBe(true);
      dispose();
    });
    expect(signal?.aborted).toBe(true);
  });

  it("ignores stale responses and reports errors", async () => {
    const pending: Array<(r: ValidationResult) => void> = [];
    await createRoot(async (dispose) => {
      const v = createEmailValidation({
        client: { validate: () => new Promise((r) => pending.push(r)) },
        debounceMs: 0,
      });
      v.validate("old@example.com");
      await vi.advanceTimersByTimeAsync(0);
      v.validate("new@example.com");
      await vi.advanceTimersByTimeAsync(0);
      pending[1]?.(deliverable("new@example.com"));
      await vi.runAllTimersAsync();
      pending[0]?.(deliverable("old@example.com"));
      await vi.runAllTimersAsync();
      expect(v.result()?.email).toBe("new@example.com");
      dispose();
    });

    await createRoot(async (dispose) => {
      const v = createEmailValidation({
        client: {
          validate: async () => {
            throw new ValidlyError("network", "offline");
          },
        },
        debounceMs: 0,
      });
      v.validate("a@example.com");
      await vi.runAllTimersAsync();
      expect(v.error()?.kind).toBe("network");
      dispose();
    });
  });
});
