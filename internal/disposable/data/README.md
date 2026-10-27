# Bundled disposable-domain list

`disposable_domains.txt` is a snapshot of
`disposable_email_blocklist.conf` from
<https://github.com/disposable-email-domains/disposable-email-domains>,
released under CC0 1.0 (public domain). Fetched 2026-10-26, 9,927 domains,
SHA-256 `acb14963f88689009907aee11e2c9fab0df8355b6721859a0cc813ae03cdecb8`.

Refresh it with:

```sh
curl -fsSL https://raw.githubusercontent.com/disposable-email-domains/disposable-email-domains/main/disposable_email_blocklist.conf \
  -o internal/disposable/data/disposable_domains.txt
```

Deployments can add domains without a rebuild through `VALIDLY_DISPOSABLE_FILE`,
`VALIDLY_DISPOSABLE_SYNC_URL` or the admin API (see `docs/disposable-domains.md`).
