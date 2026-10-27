import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: { dedupe: ["svelte"] },
  test: { environment: "happy-dom", include: ["test/**/*.test.ts"] },
});
