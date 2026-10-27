import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import * as types from "../src/types.js";

const spec = JSON.parse(readFileSync(new URL("../../../openapi/openapi.json", import.meta.url), "utf8")) as {
  components: { schemas: Record<string, { enum?: string[] }> };
};

const pairs: Array<[string, readonly string[]]> = [
  ["SyntaxStatus", types.SYNTAX_STATUSES],
  ["SyntaxReason", types.SYNTAX_REASONS],
  ["DomainStatus", types.DOMAIN_STATUSES],
  ["DomainReason", types.DOMAIN_REASONS],
  ["DisposableStatus", types.DISPOSABLE_STATUSES],
  ["RoleStatus", types.ROLE_STATUSES],
  ["MailboxStatus", types.MAILBOX_STATUSES],
  ["OverallStatus", types.OVERALL_STATUSES],
  ["ErrorCode", types.ERROR_CODES],
];

describe("TypeScript types match the OpenAPI contract", () => {
  it.each(pairs)("%s", (schema, values) => {
    expect([...values].sort()).toEqual([...(spec.components.schemas[schema]?.enum ?? [])].sort());
  });
});
