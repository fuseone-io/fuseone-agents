-- An ask remembers the scope of the connection it arrived on (NT-019, phase 4).
--
-- The consumer judges an ask after the door or the Socket Mode session wrote it,
-- and by then the connection's row may have been removed, or recreated under the
-- same name at another scope. What the ask is judged by is the scope the
-- receiving side held the connection at, so it is written down with the ask.
--
-- The installation by default, which is exact for every row the previous binary
-- writes: it verified each event with a connection of the installation's, the only
-- place it looked. This binary always writes the scope it held.
alter table channel_inbox
    add column if not exists connection_company text not null default '*',
    add column if not exists connection_area    text not null default '';
