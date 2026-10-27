import { describe, expect, it } from "vitest";
import { describeResult } from "../src/describe.js";
import { evaluate } from "../src/policy.js";
import type { ValidationResult } from "../src/types.js";
import { deliverable } from "./fixtures.js";

function withChanges(changes: (r: ValidationResult) => void): ValidationResult {
  const r = deliverable("a@example.com");
  changes(r);
  return r;
}

describe("evaluate", () => {
  it("accepts a deliverable address", () => {
    expect(evaluate(deliverable("a@example.com"))).toEqual({ accepted: true, reason: null, message: null });
  });

  it("rejects by default only what is clearly bad", () => {
    expect(evaluate(withChanges((r) => (r.syntax.status = "invalid"))).reason).toBe("invalid_syntax");
    expect(evaluate(withChanges((r) => (r.domain.status = "undeliverable"))).reason).toBe("undeliverable_domain");
    expect(evaluate(withChanges((r) => (r.disposable.status = "disposable"))).reason).toBe("disposable");
    expect(evaluate(withChanges((r) => (r.domain.status = "unknown"))).accepted).toBe(true);
    expect(evaluate(withChanges((r) => (r.role.status = "role"))).accepted).toBe(true);
  });

  it("applies opt-in rules", () => {
    const policy = { rejectUnknownDomain: true, rejectRole: true, rejectDisposable: false };
    expect(
      evaluate(
        withChanges((r) => (r.domain.status = "unknown")),
        policy,
      ).reason,
    ).toBe("unknown_domain");
    expect(
      evaluate(
        withChanges((r) => (r.role.status = "role")),
        policy,
      ).reason,
    ).toBe("role");
    expect(
      evaluate(
        withChanges((r) => (r.disposable.status = "disposable")),
        policy,
      ).accepted,
    ).toBe(true);
  });
});

describe("describeResult", () => {
  it("prioritises syntax, then suggestions, then domain, then disposable", () => {
    expect(describeResult(deliverable("a@example.com"))).toBeNull();
    expect(describeResult(withChanges((r) => (r.syntax.status = "invalid")))?.tone).toBe("error");
    expect(
      describeResult(withChanges((r) => (r.suggestion = { domain: "gmail.com", email: "a@gmail.com" })))?.text,
    ).toBe("Did you mean a@gmail.com?");
    expect(describeResult(withChanges((r) => (r.domain.status = "undeliverable")))?.tone).toBe("error");
    expect(describeResult(withChanges((r) => (r.domain.status = "unknown")))?.tone).toBe("warning");
    expect(describeResult(withChanges((r) => (r.disposable.status = "disposable")))?.tone).toBe("warning");
  });
});
