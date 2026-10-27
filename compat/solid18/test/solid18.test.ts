import { createRequire } from "node:module";
import { expect, it } from "vitest";

it("runs against Solid 1.8", () => {
  const pkg = createRequire(import.meta.url)("solid-js/package.json") as { version: string };
  expect(pkg.version).toMatch(/^1\.8\./);
});

import "../../../packages/solid/test/solid.test.ts";
