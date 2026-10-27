import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createRouteClient, useEmailValidation } from "../src/client.js";
import { createServerValidly, createValidateRoute, emailFieldError } from "../src/server.js";

function result(email: string, domain: "deliverable" | "undeliverable" = "deliverable") {
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

const upstream = vi.fn(async (_url: string | URL | Request, init?: RequestInit) => {
  const { email } = JSON.parse(String(init?.body)) as { email: string };
  return Response.json(result(email, email.endsWith("@nomail.example") ? "undeliverable" : "deliverable"));
});

const options = { baseUrl: "https://validly.internal", apiKey: "secret", fetch: upstream };

afterEach(() => upstream.mockClear());

describe("createValidateRoute", () => {
  it("proxies to Validly with the server-side key", async () => {
    const { POST } = createValidateRoute(options);
    const res = await POST(
      new Request("http://app/api/validate", { method: "POST", body: JSON.stringify({ email: "a@example.com" }) }),
    );
    expect(res.status).toBe(200);
    expect((await res.json()).overall_status).toBe("domain_deliverable");
    const [url, init] = upstream.mock.calls[0] ?? [];
    expect(url).toBe("https://validly.internal/api/v1/validate");
    expect(new Headers(init?.headers).get("authorization")).toBe("Bearer secret");
  });

  it("limits each client by its forwarded address", async () => {
    const { POST } = createValidateRoute({ ...options, proxyRequestsPerMinute: 1 });
    const req = (ip: string) =>
      new Request("http://app/api/validate", {
        method: "POST",
        headers: { "x-forwarded-for": `1.2.3.4, ${ip}` },
        body: JSON.stringify({ email: "a@example.com" }),
      });
    expect((await POST(req("203.0.113.1"))).status).toBe(200);
    expect((await POST(req("203.0.113.1"))).status).toBe(429);
    expect((await POST(req("203.0.113.2"))).status).toBe(200);
  });

  it("rejects bodies that are not JSON", async () => {
    const { POST } = createValidateRoute(options);
    const res = await POST(new Request("http://app/api/validate", { method: "POST", body: "nope" }));
    expect(res.status).toBe(400);
    expect(upstream).not.toHaveBeenCalled();
  });
});

describe("server actions", () => {
  it("returns a form error message", async () => {
    const validly = createServerValidly(options);
    expect(await emailFieldError(validly, "a@example.com")).toBeNull();
    expect(await emailFieldError(validly, "a@nomail.example")).toBe("This domain does not appear to accept email.");
    expect(await emailFieldError(validly, null)).toBe("Please enter your email address.");
  });
});

describe("client hook", () => {
  it("posts to the route, not to Validly", async () => {
    const browserFetch = vi.fn(async (_url: string | URL | Request, _init?: RequestInit) =>
      Response.json(result("a@example.com")),
    );
    vi.stubGlobal("fetch", browserFetch);
    try {
      const { result: hook } = renderHook(() => useEmailValidation({ debounceMs: 0 }));
      await act(async () => {
        await hook.current.validateNow("a@example.com");
      });
      expect(browserFetch.mock.calls[0]?.[0]).toBe("/api/validate");
      const init = browserFetch.mock.calls[0]?.[1] as RequestInit | undefined;
      expect(new Headers(init?.headers).get("authorization")).toBeNull();
      expect(hook.current.result?.overall_status).toBe("domain_deliverable");
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("ignores an explicitly undefined client", async () => {
    const browserFetch = vi.fn(async (_url: string | URL | Request, _init?: RequestInit) =>
      Response.json(result("a@example.com")),
    );
    vi.stubGlobal("fetch", browserFetch);
    try {
      const { result: hook } = renderHook(() => useEmailValidation({ client: undefined, route: "/api/check" }));
      await act(async () => {
        await hook.current.validateNow("a@example.com");
      });
      expect(browserFetch.mock.calls[0]?.[0]).toBe("/api/check");
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("caches route clients", () => {
    expect(createRouteClient("/x")).toBe(createRouteClient("/x"));
  });
});
