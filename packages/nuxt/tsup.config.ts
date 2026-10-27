import { defineConfig } from "tsup";

export default defineConfig([
  {
    entry: { module: "src/module.ts" },
    format: ["esm"],
    dts: true,
    clean: true,
    sourcemap: true,
    target: "es2022",
    external: ["@nuxt/kit", "@nuxt/schema"],
  },
  {
    // Runtime files are added to the consuming app by the module and
    // processed by Nuxt's bundler, so they are transpiled one-to-one (no
    // bundling) and keep their "#imports" and relative imports intact.
    entry: ["src/runtime/**/*.ts", "!src/runtime/**/*.d.ts"],
    outDir: "dist/runtime",
    bundle: false,
    format: ["esm"],
    dts: false,
    sourcemap: true,
    target: "es2022",
  },
]);
