// A stand-in for the Validly API used by the server examples' tests.
// "@nomail.example" is undeliverable and "@mailinator.com" is disposable.
export const fakeValidly: typeof fetch = async (_url, init) => {
  const { email } = JSON.parse(String(init?.body)) as { email: string };
  const undeliverable = email.endsWith("@nomail.example");
  return Response.json({
    email,
    syntax: { status: "valid", reason: null },
    domain: {
      normalized: email.split("@")[1],
      status: undeliverable ? "undeliverable" : "deliverable",
      reason: undeliverable ? "null_mx" : "mx_records",
      mx_found: true,
      fallback_address_found: false,
    },
    disposable: { status: email.endsWith("@mailinator.com") ? "disposable" : "not_disposable" },
    role: { status: "not_role" },
    suggestion: null,
    mailbox: { status: "unverified" },
    overall_status: undeliverable ? "undeliverable" : "domain_deliverable",
  });
};
