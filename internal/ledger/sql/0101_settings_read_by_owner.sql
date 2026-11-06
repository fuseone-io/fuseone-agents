-- A read by name finds what belongs to one channel connection.
--
-- A conversation names its connection inside the value, because rows written
-- before the connection joined the key are named by the id alone. An identity
-- names it at the front of its key, so its rows are a range. Neither scope is
-- known at that read, which is what the primary key starts with.
create index settings_kind_channel_idx on settings (kind, (value->>'channel'));
create index settings_kind_name_pattern_idx on settings (kind, name text_pattern_ops);
