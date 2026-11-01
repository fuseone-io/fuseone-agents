-- A webhook that comes from GitHub (#202).
--
-- GitHub never sends the secret: it signs each delivery's body with it. So for
-- such a hook the secret is kept where it can be read again — sealed by the
-- vault, under a context naming the path, so a row copied to another path does
-- not open — beside the hash every hook keeps. An ordinary hook stores no
-- sealed copy; its secret remains unrecoverable from a dump.
--
-- `source` and `events` come from the published version, like the path: which
-- deliveries start a run is part of the definition, reviewed with it.
alter table webhook_triggers
    add column source text not null default ''
        check (source in ('', 'github')),
    add column events text[] not null default '{}',
    add column secret_sealed bytea,
    add column secret_nonce bytea;
