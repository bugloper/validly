import { describe, expect, it } from "vitest";
import shared from "../../../testdata/syntax-cases.json";
import { checkSyntax, createClient, createValidationController, isValidlyError } from "../src/index.js";
import { deliverable, hangingFetch, jsonResponse } from "./fixtures.js";

describe("browser compatibility", () => {
  it("runs in a real browser", () => {
    expect(typeof window).toBe("object");
    expect(typeof process === "undefined" || !("versions" in process && process.versions.node)).toBe(true);
  });

  it("agrees with the server on the shared syntax vectors", () => {
    for (const tc of shared.cases) {
      const got = checkSyntax(tc.email);
      if ("clientDefers" in tc && tc.clientDefers) {
        expect(got.status, tc.email).toBe("unchecked");
      } else if (tc.reason === null) {
        expect(got.status, tc.email).toBe("valid");
      } else {
        expect(got, tc.email).toMatchObject({ status: "invalid", reason: tc.reason });
      }
    }
  });

  it("validates through fetch and supports abort", async () => {
    const client = createClient({ fetch: async () => jsonResponse(deliverable("a@example.com")) });
    expect((await client.validate("a@example.com")).overall_status).toBe("domain_deliverable");

    const controller = new AbortController();
    const pending = createClient({ fetch: hangingFetch }).validate("a@example.com", { signal: controller.signal });
    controller.abort();
    const err = await pending.catch((e: unknown) => e);
    expect(isValidlyError(err) && err.kind).toBe("aborted");
  });

  it("times out with real timers", async () => {
    const err = await createClient({ fetch: hangingFetch, timeoutMs: 20 })
      .validate("a@example.com")
      .catch((e: unknown) => e);
    expect(isValidlyError(err) && err.kind).toBe("timeout");
  });

  it("debounces with real timers", async () => {
    let calls = 0;
    const controller = createValidationController({
      debounceMs: 20,
      client: {
        validate: async (email) => {
          calls++;
          return deliverable(email);
        },
      },
    });
    controller.validate("a@example.co");
    controller.validate("a@example.com");
    await new Promise((r) => setTimeout(r, 60));
    expect(calls).toBe(1);
    expect(controller.getState()).toMatchObject({ status: "done", email: "a@example.com" });
  });
});
