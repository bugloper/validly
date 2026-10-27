import { addImports, addServerHandler, addServerImports, createResolver, defineNuxtModule } from "@nuxt/kit";
import type { NuxtModule } from "@nuxt/schema";

export interface ModuleOptions {
  /** Validly API origin, e.g. "https://validly.example.com". Server-only. */
  baseUrl?: string;
  /**
   * Validly API key. Server-only: it is stored in private runtime config and
   * never sent to the browser. Prefer setting NUXT_VALIDLY_API_KEY.
   */
  apiKey?: string;
  /**
   * Server route that proxies browser validation requests to Validly
   * (default "/api/validly/validate"). Set to false to disable it.
   */
  route?: string | false;
}

const module: NuxtModule<ModuleOptions> = defineNuxtModule<ModuleOptions>({
  meta: {
    name: "@validly/nuxt",
    configKey: "validly",
    compatibility: { nuxt: ">=4.0.0" },
  },
  defaults: {
    baseUrl: "",
    apiKey: "",
    route: "/api/validly/validate",
  },
  setup(options, nuxt) {
    const { resolve } = createResolver(import.meta.url);
    const route = options.route === false ? "" : (options.route ?? "/api/validly/validate");

    // Private (server-only) config. Values set in nuxt.config's runtimeConfig
    // or through NUXT_VALIDLY_* environment variables take precedence.
    const existing = (nuxt.options.runtimeConfig.validly ?? {}) as Record<string, unknown>;
    nuxt.options.runtimeConfig.validly = {
      baseUrl: options.baseUrl ?? "",
      apiKey: options.apiKey ?? "",
      ...existing,
    };
    nuxt.options.runtimeConfig.public.validly = { route };

    if (route) {
      addServerHandler({ route, method: "post", handler: resolve("./runtime/server/validate") });
    }
    addImports([
      { name: "useEmailValidation", from: resolve("./runtime/composables") },
      { name: "describeResult", from: resolve("./runtime/composables") },
    ]);
    addServerImports([{ name: "useValidly", from: resolve("./runtime/server/utils") }]);
    nuxt.options.build.transpile.push(resolve("./runtime"));
  },
});

export default module;
