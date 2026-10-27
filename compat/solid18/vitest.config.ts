import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: { conditions: ["browser", "development"], dedupe: ["solid-js"] },
  test: { environment: "happy-dom", include: ["test/**/*.test.ts"], server: { deps: { inline: [/solid-js/] } } },
});
