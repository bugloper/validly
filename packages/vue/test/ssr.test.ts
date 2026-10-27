// @vitest-environment node
import { expect, it, vi } from "vitest";
import { createSSRApp, defineComponent, h, ref } from "vue";
import { renderToString } from "vue/server-renderer";
import { useEmailValidation } from "../src/index.js";

it("does not validate during server-side rendering", async () => {
  const validate = vi.fn();
  const App = defineComponent({
    setup() {
      const email = ref("prefilled@example.com");
      const { status } = useEmailValidation({ client: { validate }, email, debounceMs: 0 });
      return () => h("p", status.value);
    },
  });
  const html = await renderToString(createSSRApp(App));
  await new Promise((r) => setTimeout(r, 20));
  expect(html).toBe("<p>idle</p>");
  expect(validate).not.toHaveBeenCalled();
});
