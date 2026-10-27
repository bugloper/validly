import { VERSION } from "@angular/core";
import { expect, it } from "vitest";

it("runs against Angular 20", () => {
  expect(VERSION.major).toBe("20");
});

import "../../../packages/angular/test/angular.test.ts";
