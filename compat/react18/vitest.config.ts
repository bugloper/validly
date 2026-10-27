import { defineConfig } from "vitest/config";

export default defineConfig({
  // Resolve these from this package (React 18) even when imported from
  // packages/react, whose own node_modules hold React 19.
  resolve: { dedupe: ["react", "react-dom", "@testing-library/react"] },
  test: { environment: "happy-dom", include: ["test/**/*.test.ts"] },
});
