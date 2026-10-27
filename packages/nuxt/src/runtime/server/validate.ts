import { forwardedClientId } from "@validly/server";
import {
  defineEventHandler,
  getRequestHeader,
  getRequestIP,
  readBody,
  setResponseHeaders,
  setResponseStatus,
} from "#imports";
import { useValidly } from "./utils.js";

/** POST {route}: proxies a browser's validation request to Validly. */
export default defineEventHandler(async (event) => {
  let body: unknown = null;
  try {
    body = await readBody(event);
  } catch {
    // Invalid JSON: handleValidateRequest answers 400.
  }
  // Per-client limit: the address seen by the nearest reverse proxy, else
  // the TCP peer.
  const clientId = forwardedClientId((n) => getRequestHeader(event, n)) ?? getRequestIP(event);
  const r = await useValidly().handleValidateRequest(body, clientId ? { clientId } : {});
  setResponseStatus(event, r.status);
  setResponseHeaders(event, r.headers);
  return r.body;
});
