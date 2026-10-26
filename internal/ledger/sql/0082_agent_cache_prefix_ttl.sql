-- The agent's stable-prefix cache window, chosen in its definition. Empty is
-- the provider default; '1h' pays the longer write so an hourly agent reads
-- its prefix back instead of re-writing it every run.
alter table agent_specs
    add column cache_prefix_ttl text not null default '';
