create or replace function memory_assertion_events_append_only() returns trigger
language plpgsql as $$
begin
    raise exception 'memory_assertion_events is append-only: update rejected on assertion %',
        old.assertion_id
        using errcode = 'restrict_violation';
end;
$$;

create trigger memory_assertion_events_no_update
    before update on memory_assertion_events
    for each row execute function memory_assertion_events_append_only();
