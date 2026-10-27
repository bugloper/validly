import { createClient, createValidly } from "@validly/vue";
import { createApp } from "vue";
import App from "./App.vue";

// Browser requests are anonymous: never put an API key in frontend code.
const client = createClient({ baseUrl: import.meta.env.VITE_VALIDLY_URL ?? "http://localhost:8080" });

createApp(App).use(createValidly({ client })).mount("#app");
