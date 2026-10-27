import { defineConfig } from "vitest/config";

export default defineConfig({
  oxc: { decorator: { legacy: true } },
  resolve: {
    dedupe: [
      "@angular/core",
      "@angular/common",
      "@angular/compiler",
      "@angular/forms",
      "@angular/platform-browser",
      "rxjs",
    ],
  },
  test: {
    environment: "happy-dom",
    include: ["test/**/*.test.ts"],
    setupFiles: ["../../packages/angular/test/setup.ts"],
  },
});
