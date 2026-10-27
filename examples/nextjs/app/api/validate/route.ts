import { createValidateRoute } from "@validly/nextjs/server";
import { validly } from "../../../lib/validly";

// Browser components post here; this handler calls Validly with the server-side key.
export const { POST } = createValidateRoute(validly);
