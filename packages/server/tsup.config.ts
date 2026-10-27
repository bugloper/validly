import { defineConfig } from "tsup";

export default defineConfig({
  entry: ["src/index.ts", "src/express.ts", "src/fastify.ts", "src/hono.ts", "src/nestjs.ts"],
  format: ["esm", "cjs"],
  dts: true,
  clean: true,
  sourcemap: true,
  target: "es2022",
  external: ["@validly/core", "hono", "@nestjs/common"],
});
