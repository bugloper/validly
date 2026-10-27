import { defineConfig } from "vitest/config";

export default defineConfig({
  // The tests declare a component with Angular's (legacy) decorators.
  oxc: { decorator: { legacy: true } },
  test: { environment: "happy-dom", setupFiles: ["test/setup.ts"] },
});
