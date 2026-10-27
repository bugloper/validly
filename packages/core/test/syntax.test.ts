import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { checkSyntax, localInvalidResult } from "../src/syntax.js";
import type { SyntaxReason } from "../src/types.js";

interface SharedCase {
  email: string;
  reason: SyntaxReason | null;
  domain?: string;
  clientDefers?: boolean;
}

const { cases } = JSON.parse(readFileSync(new URL("../../../testdata/syntax-cases.json", import.meta.url), "utf8")) as {
  cases: SharedCase[];
};

describe("checkSyntax matches the server on shared vectors", () => {
  it.each(cases)("$email", (tc) => {
    const got = checkSyntax(tc.email);
    if (tc.clientDefers) {
      expect(got.status).toBe("unchecked");
      return;
    }
    if (tc.reason === null) {
      expect(got).toEqual({ status: "valid", local: tc.email.slice(0, tc.email.indexOf("@")), domain: tc.domain });
    } else {
      expect(got).toEqual({ status: "invalid", reason: tc.reason, domain: tc.domain ?? null });
    }
  });
});

describe("checkSyntax", () => {
  it("never modifies the input", () => {
    const r = checkSyntax("Mixed.Case@Example.COM");
    expect(r).toEqual({ status: "valid", local: "Mixed.Case", domain: "example.com" });
  });

  it("defers non-ASCII and punycode domains to the server", () => {
    expect(checkSyntax("a@bücher.example").status).toBe("unchecked");
    expect(checkSyntax("a@xn--bcher-kva.example").status).toBe("unchecked");
  });

  it("treats U+FEFF as a character, as the server does", () => {
    // Go's unicode.IsSpace does not include the BOM, so the server reaches the
    // domain stage; a non-ASCII domain is deferred.
    expect(checkSyntax("a@exa\ufeffmple.com").status).toBe("unchecked");
  });

  it("measures length in UTF-8 bytes", () => {
    expect(checkSyntax(`${"é".repeat(257)}@example.com`)).toMatchObject({ reason: "too_long" });
  });

  it("builds a server-shaped result for local failures", () => {
    const check = checkSyntax("a..b@example.com");
    if (check.status !== "invalid") throw new Error("expected invalid");
    expect(localInvalidResult("a..b@example.com", check)).toEqual({
      email: "a..b@example.com",
      syntax: { status: "invalid", reason: "local_part_dot_placement" },
      domain: {
        normalized: "example.com",
        status: "skipped",
        reason: "syntax_invalid",
        mx_found: null,
        fallback_address_found: null,
      },
      disposable: { status: "skipped" },
      role: { status: "skipped" },
      suggestion: null,
      mailbox: { status: "unverified" },
      overall_status: "invalid",
    });
  });
});
