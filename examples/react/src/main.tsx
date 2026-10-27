import { createClient, ValidlyProvider } from "@validly/react";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { ControlledSignup } from "./ControlledSignup";
import { EmailField } from "./EmailField";

// Browser requests are anonymous: never put an API key in frontend code.
const client = createClient({ baseUrl: import.meta.env.VITE_VALIDLY_URL ?? "http://localhost:8080" });

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ValidlyProvider client={client}>
      <h1>Validly — React</h1>
      <h2>Uncontrolled</h2>
      <EmailField />
      <h2>Controlled, validated on submit</h2>
      <ControlledSignup />
    </ValidlyProvider>
  </StrictMode>,
);
