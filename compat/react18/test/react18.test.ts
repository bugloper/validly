import { version } from "react";
import { expect, it } from "vitest";

it("runs against React 18", () => {
  expect(version).toMatch(/^18\./);
});

// Register the package's own suite.
import "../../../packages/react/test/useEmailValidation.test.tsx";
