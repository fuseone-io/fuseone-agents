---
title: Monitoring event dispatch
summary: How far event dispatch is behind, when an open database transaction is holding it back, and the metrics and health signal that say so.
section: operations
tags: monitoring, prometheus, metrics, events, dispatch, xmin, transaction, health, runtime
order: 24
---

## What can delay an event

When a run finishes and its agent publishes an event, the worker's event
dispatcher opens the runs of the agents that listen for it. The dispatcher
sweeps every fifteen seconds and reads finished runs forward from one cursor
for the whole installation.

It reads only runs that finished before the **oldest transaction still open
in the database** began. That is what guarantees no run is ever skipped: a
transaction that is still open could commit a finished run with an earlier
position, and the cursor must not have passed it. The price is that a long
transaction anywhere in the database — a report, a manual `psql` session left
open, a stuck migration — delays every event dispatch while it is open.
Dispatch is delayed, never skipped: when the transaction ends, the waiting
runs are dispatched on the next sweep.

## On the Runtime health page

Readers with access to the whole installation see an **Event dispatch** panel:

| Field | Meaning |
|---|---|
| Waiting for the next sweep | Finished runs past the cursor the dispatcher can read now. Normally zero or a handful, cleared within fifteen seconds. |
| Held by an open transaction | Finished runs that committed after the oldest open transaction began. Sweeping more often does not reach them; ending that transaction does. |
| The oldest waiting run finished … ago | The lag: how long the oldest finished run past the cursor has waited. |
| Open transaction age | How long the oldest open transaction has been open, shown when runs are held. |
| Healthy / Degraded | Degraded once the lag passes five minutes. |

A degraded dispatcher also appears under **Needs attention**. Degraded is not
a failure: nothing is lost, and the platform keeps working.

Readers scoped to one company or area do not see this panel. The cursor
belongs to the installation and its count covers every scope's runs.

## Prometheus metrics

The worker's metrics endpoint (`--metrics-addr`) exposes these, read on the
worker's first sweep and then once a minute:

| Metric | Type | Meaning |
|---|---|---|
| `fuseone_event_dispatch_backlog_runs{state="ready"}` | gauge | Runs waiting for the next sweep. |
| `fuseone_event_dispatch_backlog_runs{state="held_by_xmin"}` | gauge | Runs held by the oldest open transaction. |
| `fuseone_event_dispatch_backlog_capped` | gauge | 1 when a count stopped at 1,000: the backlog is at least that. |
| `fuseone_event_dispatch_lag_seconds` | gauge | How long the oldest waiting run has waited. |
| `fuseone_event_dispatch_held_by_xmin` | gauge | 1 while committed runs wait on an open transaction. |
| `fuseone_event_dispatch_oldest_xact_age_seconds` | gauge | Age of the oldest open transaction holding an id. Omitted when unknown. |
| `fuseone_event_dispatch_oldest_xact_age_known` | gauge | 0 when a transaction is open and its start cannot be seen. |
| `fuseone_event_dispatch_degraded` | gauge | 1 once the lag passes five minutes. |
| `fuseone_event_dispatch_observed_timestamp_seconds` | gauge | When the worker last read the backlog. |
| `fuseone_event_dispatch_observe_failures_total` | counter | Backlog reads that failed. |

The only label is `state`, with two values. No run, agent or transaction id
ever becomes a label. The gauges appear only after the first successful read,
so a worker that never looked cannot report that dispatch is caught up.

The counts stop at 1,000 (shown as `1000+` on the page). The reading happens
during the very incidents it describes, on every worker, so it is bounded: two
index ranges of at most 1,001 rows and one row for the lag, however large the
backlog grows.

A starting point for alerts:

```
fuseone_event_dispatch_degraded == 1                     for 5m
time() - fuseone_event_dispatch_observed_timestamp_seconds > 180
```

## Seeing the holder's age

The age of the open transaction comes from `pg_stat_activity`. PostgreSQL
shows every role that another session holds a transaction, but only shows
**when it began** to a role that is a superuser, a member of the session's
role, or a member of `pg_read_all_stats`. A plain application role sees the
transaction is open but not its age: the panel says so and the metric
`fuseone_event_dispatch_oldest_xact_age_known` reads 0.

To see the age, grant the monitoring role to the role the platform connects
as:

```sql
grant pg_read_all_stats to <platform role>;
```

Nothing else depends on this grant. Without it — and even if
`pg_stat_activity` has been revoked — the backlog counts, the lag and the
health signal still work.

## When dispatch is held

1. Check `fuseone_event_dispatch_held_by_xmin`. At 0 with a growing lag, the
   dispatcher itself is not sweeping: look at the worker logs for
   `event dispatch failed`.
2. At 1, find the oldest open transaction:

   ```sql
   select pid, usename, application_name, state, xact_start, left(query, 80)
     from pg_stat_activity
    where backend_xid is not null
    order by age(backend_xid) desc
    limit 5;
   ```

   It can be in any database of the same PostgreSQL cluster.
3. Ending that transaction releases dispatch. The held runs are dispatched on
   the next sweep, each once.
