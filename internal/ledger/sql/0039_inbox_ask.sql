-- The ask as the platform understands it, beside the payload it arrived in.
--
-- The door is the only layer that knows a vendor's shape, and it has already
-- read the message by the time it writes the row. Storing the raw payload
-- alone would make the consumer parse it again — which means the consumer
-- knows Slack's shape too, and the second reader of a format is where the two
-- readings start to disagree.
--
-- The payload stays. It is what the digest is of and what an auditor reads
-- when they want the thing that actually arrived rather than what we made of
-- it.
alter table channel_inbox
    add column if not exists asked_by text not null default '',
    add column if not exists text     text not null default '',
    add column if not exists thread   text not null default '';
