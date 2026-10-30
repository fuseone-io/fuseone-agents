-- Logo and icon images an installation serves itself (issue #1). An
-- air-gapped installation cannot reach an external URL, and an image pasted
-- into settings as a data URI makes every read of the brand carry it.
--
-- Addressed by content: the key is the SHA-256 of the bytes, so the same
-- upload twice is one row, a reference cannot silently start meaning other
-- bytes, and a URL built from it can be cached for ever. In Postgres rather
-- than on a volume so every replica serves it and a backup includes it.
--
-- The checks restate what the upload path enforces, so a row that skipped it
-- still cannot be a type the public route would serve unsafely or a size the
-- sign-in screen should not download.
create table branding_assets (
    sha256       text primary key check (sha256 ~ '^[0-9a-f]{64}$'),
    content_type text not null
        check (content_type in ('image/png', 'image/jpeg', 'image/webp')),
    body         bytea not null check (octet_length(body) between 1 and 524288),
    created_by   text not null,
    created_at   timestamptz not null default now()
);
