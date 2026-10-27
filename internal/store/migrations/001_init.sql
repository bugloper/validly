CREATE TABLE verification_tokens (
    token_hash  bytea       PRIMARY KEY,
    email_hash  bytea       NOT NULL,
    email       text        NOT NULL,
    created_at  timestamptz NOT NULL,
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz
);

CREATE INDEX verification_tokens_email_recent ON verification_tokens (email_hash, created_at);
CREATE INDEX verification_tokens_expires ON verification_tokens (expires_at);

CREATE TABLE disposable_list_entries (
    list        text        NOT NULL CHECK (list IN ('blocklist', 'allowlist')),
    domain      text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (list, domain)
);
