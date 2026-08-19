-- A watched channel message chooses no agent in its text and is not sent by a
-- bound person. The automation rule that accepted it must survive the inbox
-- just like the message itself, or a crash between receive and open would lose
-- the authority that made the run legitimate.

alter table channel_inbox
    add column if not exists agent text not null default '',
    add column if not exists run_as text not null default '',
    add column if not exists source jsonb not null default '{}'::jsonb;
