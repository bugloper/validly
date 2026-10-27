import { playwright } from "@vitest/browser-playwright";
import { defineConfig } from "vitest/config";

// The node project runs everything. The browser project re-runs the
// environment-sensitive tests in a real Chromium to catch accidental use of
// Node-only APIs. Set VALIDLY_BROWSER_CHANNEL=chrome to use an installed
// Chrome instead of Playwright's bundled Chromium.
const channel = process.env.VALIDLY_BROWSER_CHANNEL;

export default defineConfig({
  test: {
    projects: [
      { test: { name: "node", include: ["test/**/*.test.ts"], exclude: ["test/**/*.browser.test.ts"] } },
      {
        test: {
          name: "browser",
          include: ["test/**/*.browser.test.ts"],
          browser: {
            enabled: true,
            headless: true,
            provider: playwright(channel ? { launchOptions: { channel } } : {}),
            instances: [{ browser: "chromium" }],
          },
        },
      },
    ],
  },
});
