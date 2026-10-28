-- The language of everything user-facing the agent writes, finish summary
-- included, chosen in its definition. The platform's prompt scaffolding is
-- English and outweighs a request written inside the instructions, so the
-- engine injects this next to the generation instead. Empty asks for nothing.
alter table agent_specs
    add column reply_locale text not null default '';
