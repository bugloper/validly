import { defineConfig } from "vitest/config";

export default defineConfig({
  // Solid's node build is the non-reactive SSR build; tests need the browser build.
  resolve: { conditions: ["browser", "development"] },
  test: { environment: "happy-dom", server: { deps: { inline: [/solid-js/] } } },
});
