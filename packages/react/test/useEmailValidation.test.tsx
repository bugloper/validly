import { act, cleanup, fireEvent, render, renderHook, screen } from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  type EmailValidator,
  useEmailValidation,
  type ValidationResult,
  ValidlyError,
  ValidlyProvider,
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

function undeliverable(email: string): ValidationResult {
  const r = deliverable(email);
  r.domain = { ...r.domain, status: "undeliverable", reason: "null_mx" };
  r.overall_status = "undeliverable";
  return r;
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function EmailField({ client }: { client: EmailValidator }) {
  const { getInputProps, loading, message, error } = useEmailValidation({ client, debounceMs: 100 });
  return (
    <div>
      <label htmlFor="email">Email</label>
      <input id="email" {...getInputProps({ describedBy: "email-msg" })} />
      <p id="email-msg">{loading ? "Checking email..." : (message?.text ?? "")}</p>
      {error && <p role="alert">Validation service temporarily unavailable.</p>}
    </div>
  );
}

function Uncontrolled({ client }: { client: EmailValidator }) {
  const { getInputProps, status } = useEmailValidation({ client, debounceMs: 0 });
  return (
    <>
      <input aria-label="email" {...getInputProps()} />
      <output>{status}</output>
    </>
  );
}

describe("useEmailValidation", () => {
  it("validates after the debounce and exposes accessible state", async () => {
    const validate = vi.fn(async (email: string) => undeliverable(email));
    render(<EmailField client={{ validate }} />);
    const input = screen.getByLabelText("Email");
    expect(input.getAttribute("type")).toBe("email");
    expect(input.getAttribute("autocomplete")).toBe("email");
    expect(input.getAttribute("aria-describedby")).toBe("email-msg");

    fireEvent.change(input, { target: { value: "a@nomail.example" } });
    expect(validate).not.toHaveBeenCalled();
    await act(() => vi.advanceTimersByTimeAsync(100));
    expect(validate).toHaveBeenCalledTimes(1);

    expect(screen.getByText("This domain does not appear to accept email.")).toBeTruthy();
    expect(input.getAttribute("aria-invalid")).toBe("true");
  });

  it("shows loading while the request is in flight", async () => {
    let resolve!: (r: ValidationResult) => void;
    const client: EmailValidator = { validate: () => new Promise((r) => (resolve = r)) };
    render(<EmailField client={client} />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "a@example.com" } });
    await act(() => vi.advanceTimersByTimeAsync(100));
    expect(screen.getByText("Checking email...")).toBeTruthy();
    expect(screen.getByLabelText("Email").getAttribute("aria-busy")).toBe("true");
    await act(async () => resolve(deliverable("a@example.com")));
    expect(screen.queryByText("Checking email...")).toBeNull();
  });

  it("reports service errors", async () => {
    const client: EmailValidator = {
      validate: async () => {
        throw new ValidlyError("network", "offline");
      },
    };
    render(<EmailField client={client} />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "a@example.com" } });
    await act(() => vi.advanceTimersByTimeAsync(100));
    expect(screen.getByRole("alert").textContent).toContain("temporarily unavailable");
  });

  it("ignores stale responses", async () => {
    const pending: Array<{ email: string; resolve: (r: ValidationResult) => void }> = [];
    // Ignores abort signals so that the stale response really arrives.
    const client: EmailValidator = {
      validate: (email) => new Promise((resolve) => pending.push({ email, resolve })),
    };
    const { result } = renderHook(() => useEmailValidation({ client, debounceMs: 0 }));
    act(() => result.current.validate("old@example.com"));
    await act(() => vi.advanceTimersByTimeAsync(0));
    act(() => result.current.validate("new@example.com"));
    await act(() => vi.advanceTimersByTimeAsync(0));

    await act(async () => pending[1]?.resolve(deliverable("new@example.com")));
    await act(async () => pending[0]?.resolve(undeliverable("old@example.com")));
    expect(result.current.result?.email).toBe("new@example.com");
    expect(result.current.result?.domain.status).toBe("deliverable");
  });

  it("aborts the in-flight request on unmount", async () => {
    let signal: AbortSignal | undefined;
    const client: EmailValidator = {
      validate: (_email, opts) => {
        signal = opts?.signal;
        return new Promise(() => {});
      },
    };
    const { result, unmount } = renderHook(() => useEmailValidation({ client, debounceMs: 0 }));
    act(() => result.current.validate("a@example.com"));
    await act(() => vi.advanceTimersByTimeAsync(0));
    expect(signal?.aborted).toBe(false);
    unmount();
    expect(signal?.aborted).toBe(true);
  });

  it("decides invalid syntax locally", () => {
    const validate = vi.fn();
    const { result } = renderHook(() => useEmailValidation({ client: { validate } }));
    act(() => result.current.validate("nope"));
    expect(validate).not.toHaveBeenCalled();
    expect(result.current.result?.syntax.status).toBe("invalid");
    expect(result.current.message?.text).toBe("Please enter a valid email address.");
  });

  it("uses the client from ValidlyProvider and survives StrictMode", async () => {
    const validate = vi.fn(async (email: string) => deliverable(email));
    const { result } = renderHook(() => useEmailValidation({ debounceMs: 0 }), {
      wrapper: ({ children }) => (
        <StrictMode>
          <ValidlyProvider client={{ validate }}>{children}</ValidlyProvider>
        </StrictMode>
      ),
    });
    act(() => result.current.validate("a@example.com"));
    await act(() => vi.advanceTimersByTimeAsync(0));
    expect(validate).toHaveBeenCalledTimes(1);
    expect(result.current.status).toBe("done");
  });

  it("validateNow resolves for form submission", async () => {
    const client: EmailValidator = { validate: async (email) => deliverable(email) };
    const { result } = renderHook(() => useEmailValidation({ client, debounceMs: 10_000 }));
    let final: Awaited<ReturnType<typeof result.current.validateNow>> | undefined;
    await act(async () => {
      final = await result.current.validateNow("a@example.com");
    });
    expect(final?.result?.overall_status).toBe("domain_deliverable");
  });

  it("works uncontrolled through the returned onChange handler", async () => {
    const validate = vi.fn(async (email: string) => deliverable(email));
    render(<Uncontrolled client={{ validate }} />);
    fireEvent.change(screen.getByLabelText("email"), { target: { value: "a@example.com" } });
    await act(() => vi.advanceTimersByTimeAsync(0));
    expect(screen.getByText("done")).toBeTruthy();
  });
});
