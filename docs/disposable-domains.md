# Disposable domains and admin lists

Disposable-domain detection is a heuristic. New throwaway services appear
every day and no list is complete: `not_disposable` only means "not on any
configured list". Treat it as one signal, not a verdict.

## Sources

A domain, or any of its parent domains (`x.mailinator.com` matches
`mailinator.com`), is checked against, in order:

1. **Admin allowlist.** Always wins: listed domains are never disposable.
2. **Admin blocklist.**
3. **Bundled list:** ~9,900 domains from the CC0
   [disposable-email-domains](https://github.com/disposable-email-domains/disposable-email-domains)
   project, compiled into the binary. See
   [`internal/disposable/data/README.md`](../internal/disposable/data/README.md)
   for the snapshot date and how to refresh it.
4. **`VALIDLY_DISPOSABLE_FILE`:** a newline-separated file, `#` comments
   allowed. It is re-read when its modification time changes (checked every
   30 s), so updates need no restart.
5. **`VALIDLY_DISPOSABLE_SYNC_URL`:** an HTTPS URL of the same format, fetched
   on startup and every `VALIDLY_DISPOSABLE_SYNC_INTERVAL` (24 h). A failed
   or empty fetch keeps the previous list. Optional, and set by the operator,
   never by API callers.

Invalid lines are skipped.

## Admin API

Set `VALIDLY_ADMIN_TOKEN` (at least 32 characters) to enable it. With
`VALIDLY_DATABASE_URL`, entries are stored in PostgreSQL and every replica
picks up changes within 30 seconds; without it they live in memory on one
instance.

```sh
T="Authorization: Bearer $VALIDLY_ADMIN_TOKEN"

# Block a domain
curl -X PUT -H "$T" localhost:8080/api/v1/admin/disposable-domains/blocklist/burner.example.org

# Never treat a domain as disposable
curl -X PUT -H "$T" localhost:8080/api/v1/admin/disposable-domains/allowlist/partner.example.com

# List and remove
curl -H "$T" localhost:8080/api/v1/admin/disposable-domains
curl -X DELETE -H "$T" localhost:8080/api/v1/admin/disposable-domains/blocklist/burner.example.org
```

Domains are normalized (lowercase, IDNA) before storage. Admin changes are
logged with the list and domain.

## Role addresses

Local parts such as `admin`, `support`, `sales`, `noreply` and `postmaster`
(ignoring case and `+tag`) are reported as `role`. This is informational:
role addresses are valid, and neither the API nor the default client policy
rejects them.

## Typo suggestions

When a domain is one edit (two for domains of 12+ characters) from exactly
one domain in the curated list (Gmail, Outlook, Yahoo, iCloud, Proton, GMX
and ~45 others), Validly returns a suggestion. It counts adjacent swaps as one
edit (optimal string alignment distance). Ties produce no suggestion, exact
matches produce none, and suggestions only ever point at curated domains, so
a typo can't be "corrected" to an arbitrary or malicious domain. The submitted
address is never changed; show the suggestion and let the user choose.
Replace the list with `VALIDLY_SUGGESTION_DOMAINS`.
