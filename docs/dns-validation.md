# DNS validation and its limits

## What Validly checks

For a syntactically valid address, Validly resolves the domain (in its
lowercase IDNA/ASCII form) following RFC 5321 section 5.1:

1. Query MX.
   - NXDOMAIN → `undeliverable` / `nxdomain`.
   - A single `MX 0 .` record → `undeliverable` / `null_mx` (RFC 7505). If a
     domain publishes a null MX alongside real exchangers, the real ones win.
   - Any other MX record → `deliverable` / `mx_records`.
2. If the domain exists but has no MX records (NODATA), query A and AAAA in
   parallel. A public address → `deliverable` / `address_fallback`; only
   private, loopback or reserved addresses → `undeliverable` /
   `non_public_address`; nothing → `undeliverable` / `no_mail_records`.
3. Timeouts, SERVFAIL, REFUSED and transport errors produce `unknown`. A
   lookup that did not finish is **never** reported as deliverable. If A
   failed while AAAA found nothing (or vice versa), the result is `unknown`.

Special-use names (`.localhost`, `.test`, `.invalid`, `.example`, `.local`,
`.onion`, `.alt`, `.internal`, `.arpa`) are reported as `reserved_domain`
without any query.

## What it cannot tell you

- **Whether a mailbox exists.** `deliverable` means the domain routes mail.
  Most providers accept mail for any address at SMTP time, or refuse to say,
  and SMTP "probing" is unreliable and looks like abuse. Validly does not
  probe. Use [ownership verification](verification.md) when it matters.
- **Whether the MX hosts work.** Validly does not resolve or connect to the
  exchangers named in MX records.
- **Anything beyond your resolver's honesty.** Results are only as good as
  the configured resolvers.
- **The future.** A deliverable domain can lapse; results are cached for at
  most an hour.

## Behaviour under load and failure

- Each domain lookup (MX plus any fallback) is bounded by
  `VALIDLY_DNS_TIMEOUT` (3 s), and each individual query attempt by a third
  of that, so a dead resolver or slow MX answer still leaves time to try the
  next resolver and the A/AAAA fallback. At most `VALIDLY_DNS_MAX_CONCURRENT`
  (64) lookups run at once.
- Concurrent requests for the same domain share one lookup.
- If a client disconnects, its request stops waiting, but a shared lookup
  still finishes (within the timeout) so other callers and the cache benefit.
- Results are cached for the DNS TTL (the SOA negative TTL for NXDOMAIN and
  NODATA), clamped to 30 s–1 h; indeterminate results for 10 s.
- Queries use UDP with EDNS (1232 bytes) and retry over TCP when truncated.
  Several resolvers are tried in order when one fails or answers SERVFAIL or
  REFUSED.

## Why not the standard library resolver?

Go's `net.Resolver` cannot distinguish NXDOMAIN from NODATA (both are "not
found"), and does not expose TTLs. Validly uses
[`github.com/miekg/dns`](https://github.com/miekg/dns) to build exact queries,
behind a small `Querier` interface; the tests substitute a deterministic fake.

## Safety

Validly only resolves domains of syntactically valid email addresses, only
the record types above, and never connects to the addresses it finds, so it
cannot be used to make requests to internal services. It is not a
general-purpose DNS lookup service.
