-- Event declarations now carry the small context contract that travels to a
-- listener. Existing rows only declared event names, so each name becomes an
-- object with that same event and no inferred context.
alter table agent_specs
    alter column emits drop default;

create or replace function agent_emits_text_array_to_jsonb(events text[])
returns jsonb
language sql
immutable
as $$
    select coalesce(
        jsonb_agg(jsonb_build_object('event', one.event) order by one.ord),
        '[]'::jsonb
    )
    from unnest(events) with ordinality as one(event, ord)
$$;

alter table agent_specs
    alter column emits type jsonb
    using agent_emits_text_array_to_jsonb(emits);

alter table agent_specs
    alter column emits set default '[]'::jsonb;

drop function agent_emits_text_array_to_jsonb(text[]);
