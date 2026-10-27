#!/usr/bin/env node
// End-to-end check: runs the real Validly binary against a fake DNS server
// (no public DNS), then exercises the HTTP API, the Go example and every
// server-side example against it.
//
//   node scripts/e2e.mjs
//
// Prerequisites: `pnpm build` (packages), plus `pnpm --filter
// validly-example-nestjs build`, `... nextjs build` and `... nuxt build`.
// Set VALIDLY_E2E_DATABASE_URL to a disposable PostgreSQL database to also
// test ownership verification end to end.

import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
const work = mkdtempSync(join(tmpdir(), "validly-e2e-"));
const children = [];
const API_KEY = "e2e-api-key-0123456789";
const DNS_ADDR = "127.0.0.1:15354";
const BASE = "http://127.0.0.1:18080";
let failures = 0;

function cleanup() {
  for (const c of children) c.kill("SIGTERM");
  rmSync(work, { recursive: true, force: true });
}
process.on("exit", cleanup);
process.on("SIGINT", () => process.exit(130));

function start(name, cmd, args, opts = {}) {
  const child = spawn(cmd, args, {
    cwd: opts.cwd ?? root,
    env: { ...process.env, ...opts.env },
    stdio: ["ignore", "pipe", "pipe"],
  });
  let log = "";
  child.stdout.on("data", (d) => (log += d));
  child.stderr.on("data", (d) => (log += d));
  child.on("exit", (code) => {
    if (code && code !== 143 && !child.killed) console.error(`[${name}] exited with ${code}\n${log}`);
  });
  children.push(child);
  child.logs = () => log;
  return child;
}

async function waitFor(url, method = "GET") {
  for (let i = 0; i < 150; i++) {
    try {
      const r = await fetch(url, { method });
      if (r.status < 500) return;
    } catch {}
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`timed out waiting for ${url}`);
}

function check(name, ok, detail = "") {
  if (ok) console.log(`  ✓ ${name}`);
  else {
    failures++;
    console.log(`  ✗ ${name} ${detail}`);
  }
}

async function post(url, body, headers = {}) {
  const r = await fetch(url, {
    method: "POST",
    headers: { "content-type": "application/json", ...headers },
    body: JSON.stringify(body),
  });
  const text = await r.text();
  let json = null;
  try {
    json = JSON.parse(text);
  } catch {}
  return { status: r.status, json, text };
}

const auth = { authorization: `Bearer ${API_KEY}` };
const email = (sub) => `person@${sub}.validly-e2e.net`;

// ---------------------------------------------------------------- servers
console.log("Building validly and fakedns");
execFileSync("go", ["build", "-o", join(work, "validly"), "./cmd/validly"], { cwd: root, stdio: "inherit" });
execFileSync("go", ["build", "-o", join(work, "fakedns"), "./tools/fakedns"], { cwd: root, stdio: "inherit" });
start("fakedns", join(work, "fakedns"), ["-addr", DNS_ADDR]);

const smtp = await startSMTPSink();
const dbURL = process.env.VALIDLY_E2E_DATABASE_URL;
const validly = start("validly", join(work, "validly"), ["serve"], {
  env: {
    VALIDLY_ADDR: "127.0.0.1:18080",
    VALIDLY_METRICS_ADDR: "127.0.0.1:19090",
    VALIDLY_DNS_SERVERS: DNS_ADDR,
    VALIDLY_API_KEYS: `e2e:${API_KEY}`,
    VALIDLY_RATE_LIMIT_IP: "0",
    VALIDLY_LOG_LEVEL: "warn",
    ...(dbURL
      ? {
          VALIDLY_DATABASE_URL: dbURL,
          VALIDLY_SMTP_HOST: "127.0.0.1",
          VALIDLY_SMTP_PORT: String(smtp.port),
          VALIDLY_SMTP_FROM: "Validly <no-reply@validly.test>",
          VALIDLY_SMTP_TLS: "opportunistic",
          VALIDLY_VERIFICATION_URL: "https://app.example.com/verify?token={token}",
        }
      : {}),
  },
});
await waitFor(`${BASE}/healthz`);

// ---------------------------------------------------------------- API
console.log("API");
const cases = [
  ["deliverable", "domain_deliverable", "mx_records"],
  ["fallback", "domain_deliverable", "address_fallback"],
  ["nullmx", "undeliverable", "null_mx"],
  ["private", "undeliverable", "non_public_address"],
  ["missing", "undeliverable", "nxdomain"],
  ["servfail", "unknown", "servfail"],
];
for (const [sub, overall, reason] of cases) {
  const r = await post(`${BASE}/api/v1/validate`, { email: email(sub) }, auth);
  check(`${sub}: ${overall}/${reason}`, r.json?.overall_status === overall && r.json?.domain.reason === reason, r.text);
}
const anon = await post(`${BASE}/api/v1/validate`, { email: email("servfail") });
check("anonymous callers get redacted resolver errors", anon.json?.domain.reason === "dns_unavailable", anon.text);
const disposable = await post(`${BASE}/api/v1/validate`, { email: "x@mailinator.com", checks: { dns: false } });
check("disposable detection", disposable.json?.disposable.status === "disposable", disposable.text);
const typo = await post(`${BASE}/api/v1/validate`, { email: "jane@gmial.com", checks: { dns: false } });
check("typo suggestion", typo.json?.suggestion?.email === "jane@gmail.com", typo.text);
const invalid = await post(`${BASE}/api/v1/validate`, { email: "jane@@example.com" });
check("invalid syntax", invalid.json?.syntax.reason === "multiple_at_signs", invalid.text);
check("openapi.json served", (await fetch(`${BASE}/openapi.json`)).status === 200);
const metrics = await (await fetch("http://127.0.0.1:19090/metrics")).text();
check("metrics exported", metrics.includes("validly_dns_lookups_total"));

// ---------------------------------------------------------------- verification
if (dbURL) {
  console.log("Ownership verification (PostgreSQL + SMTP)");
  const send = await post(`${BASE}/api/v1/verification/send`, { email: "Owner@example.com" }, auth);
  check("send accepted", send.status === 202, send.text);
  const message = await smtp.next();
  const token = /token=([A-Za-z0-9_-]+)/.exec(message)?.[1];
  check("email delivered with a link", Boolean(token), message.slice(0, 200));
  const confirm = await post(`${BASE}/api/v1/verification/confirm`, { token }, auth);
  check("token confirms the address", confirm.json?.email === "Owner@example.com", confirm.text);
  const replay = await post(`${BASE}/api/v1/verification/confirm`, { token }, auth);
  check("token cannot be replayed", replay.status === 400 && replay.json?.error.code === "invalid_token", replay.text);
} else {
  console.log("Ownership verification: skipped (set VALIDLY_E2E_DATABASE_URL)");
}

// ---------------------------------------------------------------- Go example
console.log("Go example");
const goOut = execFileSync("go", ["run", ".", email("deliverable"), email("nullmx"), "jane@gmial.com"], {
  cwd: join(root, "examples/go"),
  env: { ...process.env, VALIDLY_URL: BASE },
}).toString();
check("deliverable", goOut.includes(`${email("deliverable")}: domain_deliverable`), goOut);
check("undeliverable", goOut.includes(`${email("nullmx")}: undeliverable`), goOut);

// ---------------------------------------------------------------- Node examples
const serverEnv = { VALIDLY_URL: BASE, VALIDLY_API_KEY: API_KEY };
const nodeExamples = [
  ["express", 3101, ["src/server.ts"], "/api/validate"],
  ["fastify", 3102, ["src/server.ts"], "/api/validate"],
  ["hono", 3103, ["src/server.ts"], "/api/validate"],
  ["nestjs", 3104, ["dist/main.js"], "/api/validate"],
];
for (const [name, port, args, proxy] of nodeExamples) {
  console.log(`${name} example`);
  const url = `http://127.0.0.1:${port}`;
  const child = start(name, process.execPath, args, {
    cwd: join(root, "examples", name),
    env: { ...serverEnv, PORT: String(port) },
  });
  try {
    await waitFor(`${url}/register`, "POST");
  } catch {
    check("started", false, child.logs());
    continue;
  }
  const good = await post(`${url}/register`, { email: email("deliverable") });
  check("registers a deliverable address", good.status === 201, good.text);
  const bad = await post(`${url}/register`, { email: email("nullmx") });
  check("refuses an undeliverable address", bad.status === 422, bad.text);
  const proxied = await post(`${url}${proxy}`, { email: email("fallback") });
  check("proxies validation", proxied.json?.overall_status === "domain_deliverable", proxied.text);
  child.kill("SIGTERM");
}

// ---------------------------------------------------------------- Next.js
console.log("nextjs example");
{
  const port = 3105;
  const child = start(
    "nextjs",
    process.execPath,
    ["node_modules/next/dist/bin/next", "start", "-p", String(port), "-H", "127.0.0.1"],
    {
      cwd: join(root, "examples/nextjs"),
      env: serverEnv,
    },
  );
  const url = `http://127.0.0.1:${port}`;
  try {
    await waitFor(url);
    const r = await post(`${url}/api/validate`, { email: email("deliverable") });
    check("route handler proxies with the server key", r.json?.overall_status === "domain_deliverable", r.text);
    const s = await post(`${url}/api/validate`, { email: email("servfail") });
    check("authenticated proxy sees resolver detail", s.json?.domain.reason === "servfail", s.text);
    const page = await (await fetch(url)).text();
    check("page renders", page.includes("Validly — Next.js"));
  } catch (e) {
    check("started", false, String(e) + child.logs());
  }
  child.kill("SIGTERM");
}

// ---------------------------------------------------------------- Nuxt
console.log("nuxt example");
{
  const port = 3106;
  const child = start("nuxt", process.execPath, [".output/server/index.mjs"], {
    cwd: join(root, "examples/nuxt"),
    env: { PORT: String(port), HOST: "127.0.0.1", NUXT_VALIDLY_BASE_URL: BASE, NUXT_VALIDLY_API_KEY: API_KEY },
  });
  const url = `http://127.0.0.1:${port}`;
  try {
    await waitFor(url);
    const r = await post(`${url}/api/validly/validate`, { email: email("deliverable") });
    check("module route proxies with the server key", r.json?.overall_status === "domain_deliverable", r.text);
    const good = await post(`${url}/api/register`, { email: email("deliverable") });
    check("registers a deliverable address", good.status === 200, good.text);
    const bad = await post(`${url}/api/register`, { email: email("nullmx") });
    check("refuses an undeliverable address", bad.status === 422, bad.text);
  } catch (e) {
    check("started", false, String(e) + child.logs());
  }
  child.kill("SIGTERM");
}

// ---------------------------------------------------------------- shutdown
validly.kill("SIGTERM");
const exitCode = await new Promise((r) => validly.once("exit", r));
check(
  "validly shut down gracefully on SIGTERM",
  exitCode === 0,
  `exit code ${exitCode}\n${validly.logs().slice(-500)}`,
);

console.log(failures ? `\n${failures} check(s) failed` : "\nAll end-to-end checks passed");
process.exit(failures ? 1 : 0);

/** A minimal SMTP server that records message bodies. */
async function startSMTPSink() {
  const queue = [];
  const waiters = [];
  const server = createServer((socket) => {
    let data = false;
    let body = "";
    let buffer = "";
    socket.write("220 sink ESMTP\r\n");
    socket.on("data", (chunk) => {
      buffer += chunk;
      for (let i = buffer.indexOf("\r\n"); i >= 0; i = buffer.indexOf("\r\n")) {
        const line = buffer.slice(0, i);
        buffer = buffer.slice(i + 2);
        if (data) {
          if (line === ".") {
            data = false;
            const w = waiters.shift();
            w ? w(body) : queue.push(body);
            body = "";
            socket.write("250 OK\r\n");
          } else body += `${line}\n`;
          continue;
        }
        const cmd = line.toUpperCase();
        if (cmd.startsWith("EHLO") || cmd.startsWith("HELO")) socket.write("250 sink\r\n");
        else if (cmd === "DATA") {
          data = true;
          socket.write("354 go ahead\r\n");
        } else if (cmd === "QUIT") socket.end("221 bye\r\n");
        else socket.write("250 OK\r\n");
      }
    });
  });
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  server.unref();
  return {
    port: server.address().port,
    next: () =>
      new Promise((resolve, reject) => {
        if (queue.length) return resolve(queue.shift());
        waiters.push(resolve);
        setTimeout(() => reject(new Error("no email received")), 5000).unref();
      }),
  };
}
