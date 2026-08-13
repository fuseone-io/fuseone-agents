-- A ledger the size a real installation reaches, for measuring plans.
--
-- The guard that runs in CI is internal/audit/plan_test.go: it seeds enough
-- rows for the planner to stop reading the table whole and asserts that a page
-- of the trail is answered from an index. This file is for the other question —
-- how long it actually takes — which needs a ledger too big to seed in a test.
--
-- Usage: make volume  (then psql -d agents_vol and EXPLAIN whatever you like)
\set steps :steps

insert into run_steps (
    run_id, seq, kind, company_id, area_id, agent_id, version_id,
    payload, labels, input_tokens, output_tokens, cost_micros,
    idem_key, policy_hash, at, prev_hash, hash)
select
    'run_' || (n / 20)::text,
    (n % 20) + 1,
    -- The real mix: every proposal produces a gate decision, most produce a
    -- call and a return, and a minority are put to a person.
    (array['planned','gate_decided','tool_called','tool_returned','gate_decided',
           'gate_decided','run_finished','approval_decided'])[1 + n % 8],
    'acme', (array['finance','support','ops'])[1 + n % 3],
    'agent_' || (n % 40)::text, 'v3',
    jsonb_build_object(
      'tool','erp.invoice.read',
      'ref','sha256:' || md5(n::text),
      'verdict', 1 + n % 4,
      'approved', n % 3 = 0,
      'by', 'usr_' || (n % 12)::text,
      'reason','the ladder allowed it and no authored policy covered the call',
      'args_digest','sha256:' || md5((n+1)::text),
      'latency_ms', 200 + n % 900),
    array['pii','customer'],
    1200, 340, 4100,
    case when n % 8 = 2 then md5(n::text) else '' end,
    'builtin/v1',
    now() - (n || ' seconds')::interval,
    case when n % 20 = 0 then null else sha256((n-1)::text::bytea) end,
    sha256(n::text::bytea)
from generate_series(0, :steps::bigint - 1) n;

insert into admin_events (at, principal_id, company_id, area_id, action, target)
select now() - (n * 97 || ' seconds')::interval,
       'usr_' || (n % 12)::text, 'acme',
       (array['finance','support','ops'])[1 + n % 3],
       (array['tool.classified','policy.published','budget.set'])[1 + n % 3],
       'erp.invoice.read'
from generate_series(0, (:steps / 200)::bigint) n;

analyze run_steps;
analyze admin_events;

select pg_size_pretty(pg_total_relation_size('run_steps')) as ledger_total,
       pg_size_pretty(pg_indexes_size('run_steps'))        as ledger_indexes,
       count(*)                                            as steps,
       pg_total_relation_size('run_steps') / count(*)      as bytes_per_step
from run_steps;
