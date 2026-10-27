import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: { dedupe: ["vue", "@vue/test-utils"] },
  test: { environment: "happy-dom", include: ["test/**/*.test.ts"] },
});
