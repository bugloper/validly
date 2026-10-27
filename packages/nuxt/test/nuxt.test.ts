import { type ChildProcess, spawn } from "node:child_process";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { buildNuxt, loadNuxt } from "@nuxt/kit";
import { afterAll, beforeAll, describe, expect, it } from "vitest";

const fixture = fileURLToPath(new URL("./fixture", import.meta.url));
const API_KEY = "nuxt-fixture-secret-key-5d1c";

let upstream: Server;
let app: ChildProcess;
let baseURL: string;
const seenAuth: Array<string | undefined> = [];

function listen(server: Server): Promise<number> {
  return new Promise((resolve) => server.listen(0, "127.0.0.1", () => resolve((server.address() as AddressInfo).port)));
}

beforeAll(async () => {
  // A stand-in for the Validly API.
  upstream = createServer((req, res) => {
    seenAuth.push(req.headers.authorization);
    let raw = "";
    req.on("data", (c) => (raw += c));
    req.on("end", () => {
      const { email } = JSON.parse(raw) as { email: string };
      const bad = email.endsWith("@nomail.example");
      res.setHeader("content-type", "application/json");
      res.end(
        JSON.stringify({
          email,
          syntax: { status: "valid", reason: null },
          domain: {
            normalized: email.split("@")[1],
            status: bad ? "undeliverable" : "deliverable",
            reason: null,
            mx_found: null,
            fallback_address_found: null,
          },
          disposable: { status: "not_disposable" },
          role: { status: "not_role" },
          suggestion: null,
          mailbox: { status: "unverified" },
          overall_status: bad ? "undeliverable" : "domain_deliverable",
        }),
      );
    });
  });
  const upstreamPort = await listen(upstream);

  // The key is set at build time too, so the bundle check below is meaningful.
  const nuxt = await loadNuxt({
    cwd: fixture,
    dev: false,
    // Nitro's preset typing is not exposed through @nuxt/kit's override type.
    overrides: { nitro: { preset: "node-server" }, validly: { apiKey: API_KEY } } as NonNullable<
      NonNullable<Parameters<typeof loadNuxt>[0]>["overrides"]
    >,
  });
  await buildNuxt(nuxt);
  await nuxt.close();

  const probe = createServer();
  const port = await listen(probe);
  await new Promise((r) => probe.close(r));
  baseURL = `http://127.0.0.1:${port}`;
  app = spawn(process.execPath, [join(fixture, ".output/server/index.mjs")], {
    env: {
      ...process.env,
      PORT: String(port),
      HOST: "127.0.0.1",
      NUXT_VALIDLY_BASE_URL: `http://127.0.0.1:${upstreamPort}`,
      NUXT_VALIDLY_API_KEY: API_KEY,
    },
    stdio: "inherit",
  });
  for (let i = 0; i < 100; i++) {
    try {
      await fetch(baseURL);
      return;
    } catch {
      await new Promise((r) => setTimeout(r, 100));
    }
  }
  throw new Error("Nuxt server did not start");
}, 180_000);

afterAll(() => {
  app?.kill();
  upstream?.close();
});

const post = (path: string, body: unknown) =>
  fetch(baseURL + path, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body),
  });

describe("@validly/nuxt", () => {
  it("proxies validation through the server route with the private key", async () => {
    const res = await post("/api/validly/validate", { email: "a@example.com" });
    expect(res.status).toBe(200);
    expect((await res.json()).overall_status).toBe("domain_deliverable");
    expect(seenAuth.at(-1)).toBe(`Bearer ${API_KEY}`);
  });

  it("rejects malformed proxy requests", async () => {
    expect((await post("/api/validly/validate", { nope: 1 })).status).toBe(400);
  });

  it("auto-imports useValidly() in server routes", async () => {
    expect((await post("/api/register", { email: "a@example.com" })).status).toBe(200);
    const bad = await post("/api/register", { email: "a@nomail.example" });
    expect(bad.status).toBe(422);
    expect(await bad.json()).toEqual({ error: "undeliverable_domain" });
  });

  it("renders a page that uses the auto-imported composable", async () => {
    const html = await (await fetch(baseURL)).text();
    expect(html).toContain("validly-nuxt-fixture");
    expect(html).toContain("/api/validly/validate");
    expect(html).toContain("describeResult:function");
  });

  it("never ships the API key to the browser", async () => {
    const html = await (await fetch(baseURL)).text();
    expect(html).not.toContain(API_KEY);
    const walk = (dir: string): string[] =>
      readdirSync(dir).flatMap((f) => (statSync(join(dir, f)).isDirectory() ? walk(join(dir, f)) : [join(dir, f)]));
    for (const file of walk(join(fixture, ".output/public"))) {
      expect(readFileSync(file, "utf8"), file).not.toContain(API_KEY);
    }
  });
});
