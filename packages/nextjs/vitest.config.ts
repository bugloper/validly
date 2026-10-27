import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "happy-dom",
    // "server-only" throws unless imported under the react-server condition,
    // which is how Next.js loads server code.
    server: { deps: { inline: ["server-only"] } },
    alias: { "server-only": new URL("./test/server-only-stub.ts", import.meta.url).pathname },
  },
});
