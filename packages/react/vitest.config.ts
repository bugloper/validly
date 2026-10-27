import { defineConfig } from "vitest/config";

// React 18 is covered by compat/react18, which runs this suite against it.
export default defineConfig({
  test: { environment: "happy-dom" },
});
