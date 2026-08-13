# NT-004 · Ledger volume and paging

**Status** Implemented · **Date** 2026-08-13
**References** [PRD-001](PRD-001-fuseone-agents.md) — AU-01, AU-02, AU-05, NF-15
**Outcome** 2 migrations, cursor paging on three endpoints, one plan guard

Two questions came out of preparing the first installation: whether the audit
record is ready for the volume a customer will produce, and whether the console
can reach past the first page of anything. They turned out to be the same
question asked from two ends, and the answer to both started somewhere neither
of them pointed at.

---

## 1. What was measured

A million steps — fifty thousand runs of twenty, about a month of traffic at
1,600 runs a day — seeded into Postgres 18 and measured rather than estimated.

| | Before | After |
|---|---|---|
| Bytes per step, indexes included | 755 B | 863 B |
| One page of the audit trail | **875 ms**, 66,667 pages read | **0.4 ms**, 30 pages read |
| Partitions | — | 26 months |

At 200k steps a day — a busy installation — that is roughly 55 GB of ledger a
year. Large, and entirely manageable. The 875 ms was not.

## 2. The audit trail was not indexed at all

`internal/audit` reads `kind in ('gate_decided', 'approval_decided')`. The only
index over `kind` (migration 0001) is partial on
`('approval_requested', 'parked', 'failed')` — the inbox sweeps. Neither of the
trail's two kinds was covered, so every page was a parallel sequential scan of
the whole ledger. A comment in `Decisions` asserted that "the index on
(kind, at) is what keeps this from walking the whole ledger", which was true of
an index that did not exist.

The cost of this grows with the installation. At ten million steps the page
takes about nine seconds; at a year of a busy install, over a minute. The
screen fails exactly when it becomes the one somebody needs.

### 2.1 An index alone would not have fixed it

The query unioned both records and then ordered and limited the result, which
makes the database read every matching row before it can answer a page — an
index cannot supply an ordering that is applied above a `UNION ALL`.

Top-N of a union is the union of each side's top-N, so the ordering and the
limit moved inside each branch. The plan becomes a `Merge Append` over two
index scans, and the sort that remains runs over the hundred rows that survive.

## 3. Paging

The contract already declared a `cursor` parameter on `/runs` and `/approvals`
and a `nextCursor` in both response schemas. Nothing implemented either. A
client that trusted the contract paged forever over the first page — worse than
no cursor, because it looks like it works.

**Keyset, not offset.** These lists are newest first and rows arrive while
somebody reads them, so page two at offset fifty is not the fifty-first row; it
is whatever the first fifty have been pushed down to. Rows repeat and rows
vanish. The position is a tuple — `(started_at, run_id)` for runs — because
several runs share an instant.

**One position per record for the trail.** The audit trail merges two tables,
so a page boundary falls in a different place in each. A single position would
either repeat what the other record already returned or skip past it.

**A cursor carries a position and no authority.** The caller's scopes are
applied to the resumed page exactly as to the one before it, and there is a
test that hands a cursor obtained under a wide grant to a narrow one.

| Screen | Was | Is |
|---|---|---|
| Runs | 50, with a total over the whole set beside it | cursor, "N of M", load more |
| Audit trail | 100 | cursor, load more |
| Approvals | none | cursor |
| A run's trail | 200, silently truncated | every page fetched |

A run's trail is the exception that does not get a button. The cost, the
diagram, the side rail and the step count are all folds of it, so a trail
stopped at two hundred steps is not a shorter page — it is a screenful of wrong
figures.

Two lists were deliberately left alone. `/decisions` feeds a twelve-row
overview panel and `/admin/events` a scoped administrative panel; both are
bounded samples, and the trail that pages is where either question is properly
answered.

## 4. Partitioning, and the key it must not use

`run_steps` only grows: nothing updates it and nothing deletes from it, by
design and by trigger. The costs that eventually bite are maintenance ones —
vacuum, reindex, moving a year nobody reads onto slower storage — and those are
partition operations.

**The obvious key is wrong.** Postgres requires the partition key inside every
unique constraint. Partitioning on the step's own timestamp makes:

- the primary key enforcing one writer per run (NF-15), and
- the unique index enforcing idempotency (Gate check 6)

unique only *within a partition*. Neither matters until a run is still open
when the month turns — parked over a weekend, a long compensation — and then
its steps sit in two partitions where neither constraint can see the other
half. Two writers could each claim sequence 12; the same effect could be billed
twice. Both silent.

So the key is `opened_at`: the run's start, carried on every step and never
changing. A run's steps always share a partition however long it runs.

**What this still gives up.** Two runs sharing a `run_id` and opened in
different months would no longer collide. The identifier embeds a millisecond
timestamp, so it cannot happen — but it is now prevented by how identifiers are
made rather than by the database. That is a weaker guarantee than the one it
replaced, and it is written into the migration where somebody changing it will
read it.

**The default partition.** A month nobody created still has to be recordable: a
ledger that refused an append because of its own housekeeping is a worse
failure than a large table. Rows outside the declared months land in the
default partition and are as correct as anywhere else — every unique constraint
contains the partition key, so rows that must not collide cannot be in
different partitions. What is lost is archival, and it cannot be repaired
afterwards: moving rows out of a default partition means deleting them from it,
which the append-only trigger refuses, and it refuses for housekeeping too.
`agentd` therefore keeps twelve months ahead of the clock and warns when the
default partition is not empty.

**Timing.** Done before the first installation on purpose. Converting a
customer's populated `run_steps` to partitioned means rewriting the table with
a stop window in their environment; doing it now costs a migration.

### 4.1 What partitioning broke, and how it was caught

`insertStep` distinguished an idempotency conflict from a sequence conflict by
matching the constraint name exactly. On a partitioned table the violation is
raised against the *partition's* copy of the index —
`run_steps_2026_08_opened_at_idem_key_idx` — so the match stopped firing and
every duplicate effect was reported as a sequence conflict, which the caller
retries. The existing contract suite caught it on the first run.

## 5. The guard

`internal/audit/plan_test.go` seeds enough rows for the planner to stop reading
the table whole, proves the sequential-scan counter it reads can move, and then
asserts that a page of the trail costs no scan. It runs in `make check-pg`.

Asserting scans rather than milliseconds is deliberate: a timing assertion on a
laptop measures the laptop. `make volume` seeds a ledger too large for a test —
`STEPS=1000000 make volume` — for the question the guard cannot answer, which
is how long it actually takes.
