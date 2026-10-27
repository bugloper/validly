import { createClient, ValidlyContext } from "@validly/solid";
import { render } from "solid-js/web";
import { App } from "./App";

// Browser requests are anonymous: never put an API key in frontend code.
const client = createClient({ baseUrl: import.meta.env.VITE_VALIDLY_URL ?? "http://localhost:8080" });

render(
  () => (
    <ValidlyContext.Provider value={client}>
      <App />
    </ValidlyContext.Provider>
  ),
  document.getElementById("root")!,
);
