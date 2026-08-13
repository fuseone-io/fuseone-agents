-- The audit trail, at the size the ledger actually reaches.
--
-- The trail reads gate decisions and approval decisions ordered by time, and
-- the only index over kind (0001) covers the inbox sweeps — approval_requested,
-- parked, failed — so neither of the trail's kinds was indexed at all. Measured
-- on a million steps, one page of a hundred entries took 875 ms and read 66,667
-- pages; from this index it takes 0.09 ms and reads 20. The gap grows with the
-- ledger, which is the wrong direction for the record an auditor depends on.
--
-- The order is (at desc, seq desc) because that is the order the trail is read
-- in, and an index that supplies the ordering turns the page into a top-N walk
-- instead of a sort over everything that matched.
create index run_steps_trail_idx
    on run_steps (at desc, seq desc)
    where kind in ('gate_decided', 'approval_decided');

-- Agreement (FO-05) tallies approvals per agent over a window. Same reasoning,
-- different access path: it groups by agent, so the agent leads the key.
create index run_steps_agreement_idx
    on run_steps (agent_id, at desc)
    where kind = 'approval_decided';

-- Decisions (FO-06) reads the Gate's own record, newest first, and is the
-- densest kind in the ledger — every proposal produces one.
create index run_steps_decided_idx
    on run_steps (at desc, seq desc)
    where kind = 'gate_decided';
