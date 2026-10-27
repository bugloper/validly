export default defineNuxtConfig({
  modules: ["@validly/nuxt"],
  // Server-only. Set NUXT_VALIDLY_BASE_URL and NUXT_VALIDLY_API_KEY at runtime;
  // they are never exposed to the browser.
  validly: {
    baseUrl: "http://localhost:8080",
  },
  compatibilityDate: "2026-10-01",
  devtools: { enabled: false },
  telemetry: false,
});
