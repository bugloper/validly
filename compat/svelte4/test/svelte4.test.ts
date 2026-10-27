import { VERSION } from "svelte/compiler";
import { expect, it } from "vitest";

it("runs against Svelte 4", () => {
  expect(VERSION).toMatch(/^4\./);
});

import "../../../packages/svelte/test/svelte.test.ts";
