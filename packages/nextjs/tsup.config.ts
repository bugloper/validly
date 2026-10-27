import { defineConfig } from "tsup";

const external = ["@validly/core", "@validly/react", "@validly/server", "server-only", "react", "next"];

export default defineConfig([
  {
    entry: ["src/server.ts"],
    format: ["esm", "cjs"],
    dts: true,
    clean: true,
    sourcemap: true,
    target: "es2022",
    external,
  },
  {
    entry: ["src/client.ts"],
    format: ["esm", "cjs"],
    dts: true,
    sourcemap: true,
    target: "es2020",
    external,
    banner: { js: '"use client";' },
  },
]);
