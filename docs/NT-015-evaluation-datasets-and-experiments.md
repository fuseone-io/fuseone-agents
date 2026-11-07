# NT-015 — Evaluation datasets and experiments

Status: proposed · 2026-11-06 · extends [NT-006](NT-006-evaluating-agents.md) · issue [#261](https://github.com/fuseone-io/fuseone-agents/issues/261)

## What this is

[NT-006](NT-006-evaluating-agents.md) built the structural half of evaluation:
a corpus of corrections, a battery that runs it, a gate on starting an agent, a
comparison between versions and a sweep for drift. It answers *did this version
break what we already corrected?*

It cannot answer two further questions:

- **Against exactly which cases was this result obtained?** The corpus has no
  versions, so a result cannot be tied to the cases as they stood.
- **Would this agent do better on another model, before anyone publishes that?**
  A simulation runs a published version as it is.

This note specifies versioned datasets and experiments, which answer those two.
It also records three defects found while checking how the corpus behaves today.
Two of them concern personal data and are fixed first.

**Who does what.** An author with `agent:publish` on the agent's scope builds a
dataset and starts an experiment, in the console or through the API. Nothing here
is done by an agent. Reading a result needs `agent:read`, as a comparison does
today.

## Scope

| Phase in #261 | Here |
|---|---|
| 1. Versioned datasets | Specified |
| 2. Experiments | Specified |
| 3. Judge evaluators | Constraints only, in [What this leaves room for](#what-this-leaves-room-for) |
| 4. Online evaluation, 5. Review queue | Not decided. Each needs its own decision after phase 3 |

[#106](https://github.com/fuseone-io/fuseone-agents/issues/106) remains the
release decision: thresholds, adversarial coverage, argument assertions. This
note provides the records that decision reads.

NT-006 §8 says this is not a general evaluation platform, and that still holds.
**A dataset belongs to one agent and exists to check that agent's job.** There are
no cross-agent datasets and no model leaderboard.

## What we take from Datadog, and what we do not

Datadog's LLM product has the vocabulary worth borrowing. A
[dataset](https://docs.datadoghq.com/llm_observability/experiments/datasets.md)
holds records with an input, an optional expected output and metadata, and any
change to the records creates a new version. An
[experiment](https://docs.datadoghq.com/llm_observability/experiments/setup.md)
runs a task over a dataset with a recorded `config`, then evaluates each record.

We take the immutable version and the recorded configuration. We leave two things:

- **The product itself.** It works by receiving the traces, and NT-006 §4 rules
  that out for a platform installed inside a customer's network. So does its
  Python SDK, under [DE-01](PRD-001-fuseone-agents.md).
- **A single evaluator model.** There, a pass/fail check and a score are the same
  kind of thing. Here a structural breach stays binary and authoritative (NT-006
  §2.1), and no score can offset it.

## What the corpus does today

On 2026-11-06 we reproduced the erasure behaviour against a throwaway Postgres and
the comparison output with `simulate.Battery` and `simulate.Compare`. Anything else
below comes from reading the code, and says so.

### Subject erasure does not reach the corpus

`RecordRegression` copies the occurrence with
`PutFor(ctx, "regression", agentID, 1, input)`, and `Erasures.ForSubject` erases
only `EraseTx(ctx, tx, "run", runID, "subject")`. After erasing the run a person
asked about, the run's copy reads as erased and **the corpus copy still returns
the bytes**.

[DP-001](DP-001-data-protection.md) §4 says erasure clears the content of the runs
an operator identified, so this copy falls outside the promise an operator would
assume. The operator has no signal that it exists, and every battery opens new
simulated runs holding the same input again.

`regression.Store.Delete` removes the row and leaves the content, so a deleted case's
copy stays in `run_content` with nothing pointing at it. Following `from_run` cannot
find that copy.

### Retention erases the corpus, and one erased case stops every battery

`ErasePastTx` filters by age alone, so a corpus copy older than the window is
erased. Migration `0018` and the comments in `regressions.go` say the copy exists
so the corpus survives retention. `occurrences.go` says the opposite: that a case
belongs under retention. The code follows the second.

Then `Corpus.Occurrences` refuses the whole corpus when one case cannot be read,
and so does `corpusOccurrences`. From reading the code, after one case is erased:

- a battery someone asks for answers 400;
- the drift sweep logs an error on every pass and no notice is sent;
- the start gate keeps reading the last battery that exists.

Refusing is right, because a short battery that reports green is worse. But nothing
tells the author, and the corpus stays stopped until someone deletes that case. The
default window is five years. The floor is 24 hours, so the short case is reachable.

### A past result changes when the corpus changes

`CompareVersions` and `brokenCases` read a past battery against the corpus as it is
now. Adding a correction after a battery changes what that battery says:

```
v1 read then:   held=1 broken=0
v1 read today:  held=1 broken=1
comparison v1→v2:  regressed=0 fixed=1
```

The added case never ran against v1, yet the comparison reports it as **fixed**.
From reading the code (not reproduced), `brokenCases` reports that same case as
broken, so a correction recorded after the last battery can refuse a start and
name a case that has never run. `Record` also replaces a case in place, so edited
expectations re-judge old runs in the same way.

## Fix first: erasure

These change no evaluation behaviour, so they ship ahead of phase 1 in their own
pull request, with DP-001 updated in the same change.

- **Erasure follows the input.** `ForSubject` also tombstones, within the same agent,
  the corpus copies and the simulated runs holding the digest of a named run's input.
  The run recorded that input and a tombstone keeps its digest, so the link survives the
  correction being deleted and reaches corrections sharing the copy. It reads an index of
  what is still readable rather than the ledger. No new index by person is added. The
  admin event counts them.
- **Erasure and copying exclude each other.** Recording a correction and opening a
  battery read an occurrence and write it elsewhere, under a per-agent lock held shared.
  An erasure holds it exclusively, so neither can undo the other.
- **Erasure clears the case's note.** `regression_cases.note` is free text about the
  run, so it goes with the copy.
- **Deleting a case erases its copy and its replays.** Copies already orphaned are erased once, by a
  sweep for `regression://` content that no case references. Phase 1 changes this
  rule, because a version keeps naming a deleted case. From then on, the version rows
  hold `from_run` and erasure follows them too.
- **Erased is a state of a case, not a failure of the corpus.** A battery runs what
  can be read and reports the rest as `erased`: neither held nor broken, and stated
  as a gap. A corpus with every case erased is refused, as an empty one is now.
- **The author is told.** A notice goes out on the `drifted` subscription, which is
  on by default and reaches whoever already cares that a corpus stopped covering what
  it used to. Like drift, it is said once per process, so a restart can repeat it.
- **A case can be recorded again.** The reference is the digest, so a new copy of the
  same bytes lands on the old tombstone. Recording overwrites that tombstone, and a
  retried step still cannot.
- **Retention wins.** The corpus stays under the installation's window. That window
  is a promise an operator configured, and a safety net is not a reason to break it.
  The comments that say otherwise are corrected, and the manual says that a case
  ages out and must be re-recorded.

Uploaded simulation sets (`OwnerKind = "case"`) hold their owner as the simulation
id and have no run to follow. An operator has to name those simulations. DP-001
will state this limit instead of hiding it.

## Datasets

Today's `regression_cases` becomes the **working set** of the agent's default
dataset, so corrections, FU-12, the start gate and the drift sweep keep working.

**Named datasets are deferred; phase 1 delivers the default dataset only.** Further
datasets need a dataset id in the key of `regression_cases`, and nothing consumes
them until experiments exist. They need their own decision.

Two things from #261 are not added:

- **An expected output.** Nothing checks one until a judge exists, and a field
  nothing reads is how a dataset starts to look more verified than it is.
- **Tags.** Nothing in phases 1 and 2 reads them, and a tag is free text. In an
  immutable row, `customer:ana@example.com` would outlive every erasure. If tags
  are ever needed, they go in erasable content and into the digest.

A **version** is immutable:

```
datasets               id, agent, name, scope                      ordinary row; a name can be changed
dataset_versions       dataset id, version, digest, created_by, created_at
dataset_version_cases  version → case id, input ref, input digest,
                                 expectations ref, expectations digest, from_run
```

- Any added, changed or deleted case writes a new version in the same transaction.
  Renaming does not. The migration writes version 1 from the corpus as it stands.
- **Immutable rows hold no free text.** They hold generated ids, references and
  digests. A case id is already derived (`regressionID`), and a dataset's name stays
  on the ordinary row. Expectations are stored as content, because
  `Expectation.Value` is an unvalidated string, and anything in it would otherwise
  outlive an erasure the way tags would.
- The digest covers the ordered case ids, input digests and expectation digests.
  Changing only the expectations changes the digest. It covers digests and not
  bytes, so erasing content does not change a version's identity.
- Versions hold references. They never copy bytes, because a copy is what caused the
  first defect above. Content erased stays erased in every version that names it, and
  a case whose input or expectations are gone reads as `erased`.
- Deleting a case removes it from the next version. Earlier versions still name it,
  since they record what was checked. **So once versions are active, deleting a case
  no longer erases its data:** the copy and the replays stay until retention or a
  subject erasure takes them. The API and the manual have to say so.
- Expectations are stored as content, one object per case. The bytes name the case,
  so that identical expectations on two cases are two objects, and erasing one does
  not leave the other as a gap. Erasure takes them directly through the version rows
  and clears the working row. Their digest never joins the search for a run's input,
  because it is not an input and must not lead to replays.

No `UPDATE` or `DELETE` reaches these two tables. A database trigger enforces it, as
on `run_steps`. Retention and erasure act on the content they reference and never on
these rows.

### Pinning a battery

`run_started` already records the `case` a run replays. It also records
`dataset_version`. The report remains a fold of the ledger, and a battery is read
against the version it names rather than the corpus as it is now. That removes the
third defect.

A battery recorded before this has no pin. It is read as today and labelled
`unpinned`, because inventing which version it ran would be worse than admitting
that we do not know.

`CompareVersions` compares two batteries only when they name the same version. If
they differ, the cases present on both sides are compared and the rest are listed
as `not_comparable`, never as fixed.

**The gate changes in one way.** It refuses on a case that ran against this version
and broke. A case added after the last battery is reported as `not_checked`, and
the author is told to run the battery. It no longer refuses a start by itself.

## Rollout

A version is only trustworthy if every write produced one, and during a rolling
update an older pod still writes `regression_cases` without knowing versions exist.
Keeping the old column protects old readers and does nothing about old writers. So
activation is separate from the release that can honour it:

1. **Release A adds the tables and a switch, off.** Writers read the switch inside
   their transaction. While it is off they behave as they do today, and the working
   row stays authoritative.
2. **A later migration activates, in one transaction.** It holds the switch's row
   and locks the table, so a writer in flight finishes first and the next one sees it
   on. It converts every case's expectations to content, writes version 1 and turns
   the switch on.
3. **That migration also adds a trigger refusing writes that do not declare
   themselves.** Inserting, deleting or changing anything a version covers raises an
   error unless the transaction set the marker only version-aware writers set. A note
   is outside a version, so an older erasure still works. An older pod, or an
   installation that skipped release A, fails without changing the working set rather
   than writing a state no version records.

Release A does not protect erasure the same way, because erasure cannot wait for a
switch. From `0102`, a database trigger clears a correction's expectations whenever
its copy is tombstoned, whichever binary did it. The activation trigger has to let
that write through.

A case whose input is already erased is converted to a tombstone. Its expectations
never become readable content. A converted object takes the age of the case's input,
so conversion does not extend how long it lives.

A case whose input is nowhere is treated the same way: it has nothing to run, and with no
copy nothing would ever age its words out. It takes the age of the case, and it clears
its note and expectations, because a binary being rolled back to would read them.

Whatever empties a case's expectations also takes its object, by trigger, because a pod
of the previous release erases too and reports success without knowing objects exist.

That migration is `0104`, and release A is 0.96.0. One limit remains for an
installation that skips release A: during the rollout, its older pod
reads a run's input from the ledger and the working row only, so it can miss a case a
newer pod just deleted when that run's input was too large to keep whole. Upgrading
through release A closes this.

Reads prefer the content and fall back to the column only where no reference exists.
A tombstone is an answer, never a reason to fall back. **The column is dropped only
once the version that reads it has left the rollback window**, not simply in the next
release.

## Experiments

An experiment is a simulation with three things pinned: a dataset version, an agent
version and a configuration.

```
config: { provider, model, effort }    every field optional; absent means the version's own
repetitions: 1..5                      default 1
```

It reuses everything simulation has: `simulate.Open`, `simulate.Deps` with no tool
layer, the Gate, the ledger, budget reservation and the worker pool. There is no
second queue and no second record. The `run_started` step records an `experiment` id
and the configuration, so the fold can state both.

Three rules protect what already exists:

1. **An experiment is never a battery, whatever its configuration.** The identity is
   structural: every experiment run records an `experiment` id generated at the edge,
   and an empty configuration is still an experiment. It does not depend on an override
   being present, because a run with none would otherwise read as a battery. A battery
   is a simulated run with a `case` and no `experiment`. Every place that decides this
   changes together: `Batteries` and `LastBatteryAt`, on both Postgres and the in-memory
   fake.
   Otherwise an experiment becomes the evidence that the start gate and the drift sweep
   read about this version, and it resets the clock that decides when a real battery
   runs.
   `HasSimulation`, which is FU-10's gate on leaving Draft, excludes experiments too: an
   agent has to be simulated as it is, not on a model it would not use.
2. **A configuration reaches experiment runs only.** The override is honoured only for
   a simulated run with an `experiment`, and it is refused for any other, as `simulate.Deps`
   leaves no path from a simulated run to a real tool. The test uses the adversary's
   moves: a non-simulated request carrying a configuration, and a battery carrying one.
3. **The model must be available.** A configuration resolves through the same model
   registry as a published agent (`spec.Resolver`), so an experiment cannot reach a model
   the installation has not allowed.

**Repetitions are for structural certainty, not an average.** NT-006 §2.1 says
non-determinism is an argument for running a case more times. A case breached in any
repetition is broken, and the report shows `1 of 5`.

**Cost is stated before starting.** The console's existing preview also counts the
repetitions. An experiment spends and reserves budget the way a simulation does today,
and the trail names who asked.

### Reading an experiment

A result shows, in this order: dataset version, agent version, configuration, cases
run out of cases in the version, erased cases, repetitions, then held, broken and cost.

Experiments can be compared when they name the same dataset version. Comparing across
versions lists only the shared cases, and says so.

An agent version is what gets published. **An experiment changes nothing about the
agent.** Moving to the model that won is an ordinary new version, with its own battery.

## What this leaves room for

Phase 3 adds a judge. Its constraints follow from NT-006 §5:

- A judged result is an opinion with provenance: rubric digest, model and cost,
  recorded apart from the Gate's verdicts.
- It never enters the `held` and `broken` counts.
- The output a judge reads is untrusted, and a run must not be able to talk its way
  to a high score. That needs a hostile test.

Nothing in the tables above has to change for this. Results attach to a run and a case.

## Tests that must exist

- **Erasure:** a subject erasure of a run reaches its corpus copy, the case's note and the
  simulated runs that replayed it, and leaves other cases intact. After phase 1 it also
  reaches the input and expectations of a case that was deleted and that an old version
  still names.
- **Erased case:** the battery runs the others, reports it as `erased`, and sends the
  notice once rather than on every pass.
- **Pinning:** adding a case after a battery leaves that battery's numbers unchanged,
  and the comparison does not report the case as fixed.
- **Immutability:** an `UPDATE` and a `DELETE` on a version are refused by the database.
- **Digest:** changing only one case's expectations changes the version's digest.
- **No free text:** a test reads the columns of the immutable tables and fails on a new
  text column that is not on an explicit list of ids, references and digests.
- **Gate:** an experiment with an **empty configuration**, and one on another model,
  neither lets a version start nor stops it, does not count as its last battery, and does
  not let an agent leave Draft. Sabotage: remove the exclusion from each door in turn,
  on Postgres and on the fake, and a test must fail each time.
- **Fence:** a configuration on a non-simulated request or on a battery is refused.
- **Journey:** from the API through a real run to the fold, on real Postgres, with the
  configuration visible in the report.

## Not in this note

- Judges, online evaluation and the review queue (phases 3 to 5).
- Assertions on tool arguments and adversarial coverage, which stay in #106.
- Synthetic cases. NT-006 §6 still applies: when they come, they are marked synthetic.
- Copying a case between scopes or agents.
- Import and export of a dataset. Uploading a set for one simulation already exists,
  and a file holding real occurrences is a data-protection decision of its own.
