// Provided by Nuxt and Nitro when the consuming app is built.
declare module "#imports" {
  export function useRuntimeConfig(): {
    validly?: { baseUrl?: string; apiKey?: string };
    public: { validly?: { route?: string } };
  };
  export {
    defineEventHandler,
    getRequestHeader,
    getRequestIP,
    readBody,
    setResponseHeaders,
    setResponseStatus,
  } from "h3";
}
