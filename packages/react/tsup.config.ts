import { defineConfig } from "tsup";

export default defineConfig({
  entry: ["src/index.ts"],
  format: ["esm", "cjs"],
  dts: true,
  clean: true,
  sourcemap: true,
  target: "es2020",
  external: ["react", "@validly/core"],
  // Hooks run on the client; mark the module for React Server Components.
  banner: { js: '"use client";' },
});
