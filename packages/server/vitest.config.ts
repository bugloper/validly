import { defineConfig } from "vitest/config";

export default defineConfig({
  // The NestJS test declares a controller with legacy decorators.
  oxc: { decorator: { legacy: true, emitDecoratorMetadata: true } },
  test: { environment: "node" },
});
