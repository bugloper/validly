import { describe, expect, it, vi } from "vitest";
import { createClient } from "../src/client.js";
import { isValidlyError, type ValidlyError } from "../src/errors.js";
import { deliverable, hangingFetch, jsonResponse } from "./fixtures.js";

async function catchError(p: Promise<unknown>): Promise<ValidlyError> {
  try {
    await p;
  } catch (err) {
    if (isValidlyError(err)) return err;
    throw err;
  }
  throw new Error("expected rejection");
}

describe("createClient", () => {
  it("posts to the validate endpoint with the API key", async () => {
    const fetch = vi.fn(async () => jsonResponse(deliverable("a@example.com")));
    const client = createClient({ baseUrl: "https://validly.test/", apiKey: "k", fetch });
    const result = await client.validate("a@example.com", { checks: { dns: false } });

    expect(result.overall_status).toBe("domain_deliverable");
    const [url, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("https://validly.test/api/v1/validate");
    expect(init.method).toBe("POST");
    expect((init.headers as Record<string, string>).authorization).toBe("Bearer k");
    expect(JSON.parse(init.body as string)).toEqual({ email: "a@example.com", checks: { dns: false } });
  });

  it("supports a proxy URL", async () => {
    const fetch = vi.fn(async (_url: string | URL | Request) => jsonResponse(deliverable("a@example.com")));
    await createClient({ validateUrl: "/api/validate", fetch }).validate("a@example.com");
    expect(fetch.mock.calls[0]?.[0]).toBe("/api/validate");
  });

  it("maps API errors", async () => {
    const fetch = async () =>
      jsonResponse({ error: { code: "rate_limited", message: "Slow down." } }, 429, { "retry-after": "7" });
    const err = await catchError(createClient({ fetch }).validate("a@example.com"));
    expect(err).toMatchObject({
      kind: "api",
      status: 429,
      code: "rate_limited",
      retryAfterSeconds: 7,
      message: "Slow down.",
    });
  });

  it("maps non-Validly error bodies to code unknown", async () => {
    const fetch = async () => new Response("<html>Bad gateway</html>", { status: 502 });
    const err = await catchError(createClient({ fetch }).validate("a@example.com"));
    expect(err).toMatchObject({ kind: "api", status: 502, code: "unknown" });
  });

  it("reports network failures", async () => {
    const fetch = async () => {
      throw new TypeError("Failed to fetch");
    };
    const err = await catchError(createClient({ fetch }).validate("a@example.com"));
    expect(err.kind).toBe("network");
    expect(err.cause).toBeInstanceOf(TypeError);
  });

  it("reports decode failures", async () => {
    const err1 = await catchError(createClient({ fetch: async () => new Response("{oops") }).validate("a@b.co"));
    expect(err1.kind).toBe("decode");
    const err2 = await catchError(createClient({ fetch: async () => jsonResponse({ ok: true }) }).validate("a@b.co"));
    expect(err2.kind).toBe("decode");
  });

  it("times out", async () => {
    vi.useFakeTimers();
    try {
      const p = catchError(createClient({ fetch: hangingFetch, timeoutMs: 1000 }).validate("a@b.co"));
      await vi.advanceTimersByTimeAsync(1000);
      expect((await p).kind).toBe("timeout");
    } finally {
      vi.useRealTimers();
    }
  });

  it("treats Infinity and huge timeouts as no timeout", async () => {
    for (const timeoutMs of [Number.POSITIVE_INFINITY, 2 ** 40]) {
      const fetch = async () => {
        await new Promise((r) => setTimeout(r, 20));
        return jsonResponse(deliverable("a@example.com"));
      };
      const r = await createClient({ fetch, timeoutMs }).validate("a@example.com");
      expect(r.overall_status).toBe("domain_deliverable");
    }
  });

  it("can be cancelled", async () => {
    const controller = new AbortController();
    const p = catchError(createClient({ fetch: hangingFetch }).validate("a@b.co", { signal: controller.signal }));
    controller.abort();
    expect((await p).kind).toBe("aborted");
  });

  it("rejects immediately with an already-aborted signal", async () => {
    const err = await catchError(
      createClient({ fetch: hangingFetch }).validate("a@b.co", { signal: AbortSignal.abort() }),
    );
    expect(err.kind).toBe("aborted");
  });

  it("sends and confirms verification", async () => {
    const fetch = vi.fn(async (url: string | URL | Request) =>
      String(url).endsWith("/send")
        ? jsonResponse({ status: "accepted" }, 202)
        : jsonResponse({ status: "verified", email: "a@b.co" }),
    );
    const client = createClient({ baseUrl: "http://x", apiKey: "k", fetch });
    await client.sendVerification("a@b.co");
    await expect(client.confirmVerification("tok")).resolves.toEqual({ email: "a@b.co" });
    expect(fetch.mock.calls.map((c) => c[0])).toEqual([
      "http://x/api/v1/verification/send",
      "http://x/api/v1/verification/confirm",
    ]);
  });

  it("works with the real global fetch", async () => {
    // Node and browsers both provide fetch; make sure binding is correct.
    const server = await import("node:http").then(
      ({ createServer }) =>
        new Promise<import("node:http").Server>((resolve) => {
          const s = createServer((_req, res) => {
            res.setHeader("content-type", "application/json");
            res.end(JSON.stringify(deliverable("a@example.com")));
          }).listen(0, () => resolve(s));
        }),
    );
    try {
      const { port } = server.address() as { port: number };
      const r = await createClient({ baseUrl: `http://127.0.0.1:${port}` }).validate("a@example.com");
      expect(r.domain.status).toBe("deliverable");
    } finally {
      server.close();
    }
  });
});
