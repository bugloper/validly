import { expect, it } from "vitest";
import { version } from "vue";

it("runs against Vue 3.4", () => {
  expect(version).toMatch(/^3\.4\./);
});

import "../../../packages/vue/test/useEmailValidation.test.ts";
