import { Component, inject, provideZonelessChangeDetection } from "@angular/core";
import { TestBed } from "@angular/core/testing";
import { FormControl } from "@angular/forms";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  EmailValidationService,
  type EmailValidator,
  injectEmailValidation,
  provideValidly,
  type ValidationResult,
  ValidlyError,
  type ValidlyValidationError,
} from "../src/index.js";

function result(
  email: string,
  changes: Partial<Pick<ValidationResult, "overall_status">> & {
    domain?: ValidationResult["domain"]["status"];
    disposable?: ValidationResult["disposable"]["status"];
  } = {},
): ValidationResult {
  return {
    email,
    syntax: { status: "valid", reason: null },
    domain: {
      normalized: "example.com",
      status: changes.domain ?? "deliverable",
      reason: "mx_records",
      mx_found: true,
      fallback_address_found: false,
    },
    disposable: { status: changes.disposable ?? "not_disposable" },
    role: { status: "not_role" },
    suggestion: null,
    mailbox: { status: "unverified" },
    overall_status: changes.overall_status ?? "domain_deliverable",
  };
}

function setup(client: EmailValidator) {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [provideZonelessChangeDetection(), provideValidly({ client })],
  });
  return TestBed.inject(EmailValidationService);
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

async function settle(control: FormControl, ms = 400) {
  await vi.advanceTimersByTimeAsync(ms);
  await vi.runAllTimersAsync();
  return control;
}

describe("EmailValidationService.validator", () => {
  it("accepts deliverable addresses", async () => {
    const service = setup({ validate: async (e) => result(e) });
    const control = new FormControl("a@example.com", { asyncValidators: [service.validator()] });
    expect(control.status).toBe("PENDING");
    await settle(control);
    expect(control.status).toBe("VALID");
  });

  it("rejects undeliverable and disposable addresses with a validly error", async () => {
    const service = setup({
      validate: async (e) =>
        e.startsWith("d@") ? result(e, { disposable: "disposable" }) : result(e, { domain: "undeliverable" }),
    });
    const control = new FormControl("u@nomail.example", { asyncValidators: [service.validator()] });
    await settle(control);
    expect((control.errors?.["validly"] as ValidlyValidationError | undefined)?.reason).toBe("undeliverable_domain");

    control.setValue("d@mailinator.com");
    await settle(control);
    expect((control.errors?.["validly"] as ValidlyValidationError | undefined)?.message).toBe(
      "Please use a permanent email address.",
    );
  });

  it("rejects bad syntax without calling the API", async () => {
    const validate = vi.fn();
    const service = setup({ validate });
    const control = new FormControl("not-an-email", { asyncValidators: [service.validator()] });
    expect(control.errors?.validly.reason).toBe("invalid_syntax");
    expect(validate).not.toHaveBeenCalled();
  });

  it("debounces and cancels superseded requests", async () => {
    const signals: AbortSignal[] = [];
    const validate = vi.fn((email: string, opts?: { signal?: AbortSignal }) => {
      if (opts?.signal) signals.push(opts.signal);
      return new Promise<ValidationResult>((resolve) => setTimeout(() => resolve(result(email)), 1000));
    });
    const service = setup({ validate });
    const control = new FormControl("", { asyncValidators: [service.validator({ debounceMs: 300 })] });

    control.setValue("a@example.co");
    await vi.advanceTimersByTimeAsync(100);
    control.setValue("a@example.com");
    await vi.advanceTimersByTimeAsync(300);
    expect(validate).toHaveBeenCalledTimes(1);

    control.setValue("b@example.com");
    expect(signals[0]?.aborted).toBe(true);
    await settle(control, 2000);
    expect(validate).toHaveBeenCalledTimes(2);
    expect(control.status).toBe("VALID");
  });

  it("allows the value when the API is unavailable, unless configured to reject", async () => {
    const service = setup({
      validate: async () => {
        throw new ValidlyError("network", "offline");
      },
    });
    const lenient = new FormControl("a@example.com", { asyncValidators: [service.validator()] });
    await settle(lenient);
    expect(lenient.status).toBe("VALID");

    const strict = new FormControl("a@example.com", {
      asyncValidators: [service.validator({ whenUnavailable: "reject" })],
    });
    await settle(strict);
    expect(strict.errors?.validly.reason).toBe("unavailable");
  });

  it("applies policy options", async () => {
    const service = setup({ validate: async (e) => result(e, { domain: "unknown", overall_status: "unknown" }) });
    const control = new FormControl("a@example.com", {
      asyncValidators: [service.validator({ policy: { rejectUnknownDomain: true } })],
    });
    await settle(control);
    expect(control.errors?.validly.reason).toBe("unknown_domain");
  });
});

describe("onResult", () => {
  it("reports accepted results so suggestions can be shown", async () => {
    const service = setup({
      validate: async (e) => ({ ...result(e), suggestion: { domain: "gmail.com", email: "a@gmail.com" } }),
    });
    const seen: Array<ValidationResult | null> = [];
    const control = new FormControl("a@gmial.com", {
      asyncValidators: [service.validator({ onResult: (r) => seen.push(r) })],
    });
    await settle(control);
    expect(control.status).toBe("VALID");
    expect(seen[0]?.suggestion?.email).toBe("a@gmail.com");
  });
});

describe("EmailValidationService.validate", () => {
  it("emits a result and aborts on unsubscribe", async () => {
    let signal: AbortSignal | undefined;
    const service = setup({
      validate: (_e, opts) => {
        signal = opts?.signal;
        return new Promise(() => {});
      },
    });
    const sub = service.validate("a@example.com").subscribe();
    sub.unsubscribe();
    expect(signal?.aborted).toBe(true);

    const ok = setup({ validate: async (e) => result(e) });
    const values: ValidationResult[] = [];
    ok.validate("a@example.com").subscribe((v) => values.push(v));
    await vi.runAllTimersAsync();
    expect(values[0]?.overall_status).toBe("domain_deliverable");
  });
});

describe("injectEmailValidation", () => {
  it("exposes signals and cleans up with the component", async () => {
    let signal: AbortSignal | undefined;
    const client: EmailValidator = {
      validate: (_e, opts) => {
        signal = opts?.signal;
        return new Promise(() => {});
      },
    };
    @Component({ selector: "test-email", template: "" })
    class EmailComponent {
      readonly validation = injectEmailValidation({ debounceMs: 0 });
      readonly service = inject(EmailValidationService);
    }
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({ providers: [provideZonelessChangeDetection(), provideValidly({ client })] });
    const fixture = TestBed.createComponent(EmailComponent);
    const v = fixture.componentInstance.validation;

    v.validate("bad");
    expect(v.message()?.text).toBe("Please enter a valid email address.");

    v.validate("a@example.com");
    await vi.advanceTimersByTimeAsync(0);
    expect(v.loading()).toBe(true);
    fixture.destroy();
    expect(signal?.aborted).toBe(true);
  });
});
