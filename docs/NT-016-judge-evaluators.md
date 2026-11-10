# NT-016 — Judge evaluators

Status: proposed · 2026-11-10 · extends [NT-006](NT-006-evaluating-agents.md) and
[NT-015](NT-015-evaluation-datasets-and-experiments.md) · issue
[#298](https://github.com/fuseone-io/fuseone-agents/issues/298), phase 3 of
[#261](https://github.com/fuseone-io/fuseone-agents/issues/261)

## What this is

An experiment answers *did this configuration break what we already corrected?*
Every expectation it checks is structural and binary: which tool was reached, whether
the run asked, what it cost. It cannot answer *is the answer any good?*, because that
question is about text, and only a model reads text.

This note specifies a judge: a rubric, run by a model, over the outputs of an
experiment that has already settled. Its result is an opinion with a provenance. It
sits beside the structural verdict and never changes it.

**Who does what.** An author with `agent:publish` on the scope where the experiment was
recorded asks for a judgment of that experiment, in the console or through the API, and
sees what it may cost before it starts. Reading one needs `agent:read` on that scope,
as reading the experiment does. Nothing here is done by an agent, and nothing runs
unattended: online evaluation is phase 4, and it needs its own decision.

## What is already decided

From NT-006 §5, NT-015 and #298:

- **Provenance.** A judged result is an opinion: rubric digest, model and cost, recorded
  apart from the Gate's verdicts.
- **No effect on structure.** It never enters the `held` and `broken` counts, so a
  structural breach fails the case whatever its score.
- **Untrusted output.** A run must not be able to talk its way to a high score, and a
  hostile test has to show it.
- **Same execution path.** Judges run through the model registry, and their cost is in
  the ledger.
- **No schema change.** Results attach to a run and a case.
- **No ensemble.** NT-006 §5 leaves it out: it multiplies cost for the less important
  column.

## A judgment is a set of runs

Three ways to run a judge were considered.

| Option | Why not |
|---|---|
| A call outside any run, the way the authoring assistant uses `model.Completer` | `Completer` exists because the assistant spends outside a run, and its cost comes back with the answer and goes nowhere. A judge would be the same: no ledger, no ceiling, no trail. |
| A step appended to the judged run, after `run_finished` | It gives the run a second writer once its owner is done (NF-15), and the experiment's cost per run would start including the judge's. |
| **A run of its own** | It gets the worker pool, the lease, the per-run ceiling, the ledger and erasure without anything new. |

So a **judgment** is one rubric, one judge configuration and a number of repetitions,
applied to one settled experiment. Each judged output gets one **judge run** per
repetition. A judge run:

- is **simulated**, under the judged run's agent and version and the **scope the
  experiment was recorded in**, not the scope the agent is published in today;
- records on `run_started` the **same `experiment` id** as the runs it judges, the
  judged run's `case`, a `simulation` id of its own generated at the edge (the
  judgment's), and a `judge` field: the judged run, the rubric digest and reference,
  the judge configuration and the repetition;
- is opened **with the case's input as its input**, held as its own content, as a
  battery run's is, plus a copy of the judged run's output, whose digest equals that
  run's `OutcomeDigest`;
- is offered **no tools**: its pack is empty, whatever the judged agent declares. It
  runs in the simulation pool anyway, where `simulate.Deps` puts `DryTools` in place of
  the tool layer, and it gets no memory and no context artifacts.

Carrying the experiment id is what keeps the gate safe on binaries that know nothing of
judges. `HasSimulation`, `Batteries` and `LastBatteryAt` exclude every run that names an
experiment, and every replica has done so since 0.100.0. A judge run therefore cannot
let an agent leave Draft, start or stop a version, or reset the battery clock, even on a
binary that was never told about it.

Its own simulation id is what keeps the experiment's own record intact.
`simulate.Recorded`, `Gather` and the experiment report read the runs under the
experiment's simulation id, so a judge run is in none of them. A retry of the experiment
does not count judge runs as repetitions, and the experiment's cost stays the agent's
cost.

## The rubric

A rubric is the judge's instructions plus a declared output type. Its version is its
digest: the same text and declaration give the same digest, and any change gives a new
one. It is carried by the judgment request and is not part of the agent definition.
Putting it there would make every rubric edit a new agent version, with its own battery,
for something that does not change the agent.

The text is content, like an agent's answer. Each judge run holds a copy under its own
retention and erasure, and its opening step holds only the reference and the digest
(see [What the ledger holds](#what-the-ledger-holds)). After retention, the
judgment still states which rubric it used, and a rubric kept in the author's own
repository can be proved to be the same one.

**Output types**, each with its pass criterion declared beside it:

| Type | The judge answers | Passes when |
|---|---|---|
| `pass_fail` | `pass` or `fail` | `pass` |
| `score` | an integer between a declared minimum and maximum, at most 11 values | at or above a declared threshold |
| `category` | one of 2 to 10 declared identifiers | in a declared passing set |

A category is an identifier (`^[a-z][a-z0-9_]{0,31}$`) rather than prose, so it can be
recorded in the ledger. There is no free-form JSON type and no float.

## What the ledger holds

The judge answers with a value and a reason. **The value is sealed and the reason is
content.**

- The engine parses the judge's answer against the rubric's declared type. A conforming
  answer finishes the run with a `judgment` on `run_finished`: the value, which is
  `pass` or `fail`, an integer or a declared identifier, and whether it passes.
- The reason, and the answer as the model wrote it, go where every closing answer goes:
  `OutcomeRef` and `OutcomeDigest`, in the content store, under retention and erasure.
- Anything else is a **failed judgment**, never a score: two values, a value outside the
  declared range or set, an answer that is not the declared shape, a ceiling reached, or
  a model error. The run finishes without a `judgment`, and the report counts it as not
  judged with its cause. There is no default value.

Only the engine writes `judgment`, and only on a run whose opening names a `judge`. A
worker that does not know judges never writes it, so a run it planned can never read as
a judged result.

## Untrusted output

The platform defends untrusted content today at the Gate: a label taints what derives
from it, and a high-effect act on tainted arguments needs a person. A judge has no tools,
so there is nothing for the Gate to stop. This needs a defense of its own, and each part
is a mechanism a test can assert:

1. **Position.** The platform's judge preamble and the rubric are the system
   instructions. The case input and the judged output appear only in the user turn,
   each inside a block marked as data to be judged and never as instructions.
2. **A boundary the output cannot forge.** Each block's delimiter carries a value
   derived from the judge run's id. That id is generated after the judged output was
   written, so the agent could not have known it. An output that contains a closing
   delimiter, with any other value, stays inside its block.
3. **A closed answer.** Only a value of the declared type counts, and everything else is
   a failed judgment (above). An output that persuades the judge to write prose has
   produced no score.
4. **Nothing to anchor on.** The judge does not see the structural verdict, the other
   repetitions, other runs or the agent's own instructions.
5. **Provenance on every number.** A judged result is always shown with its rubric
   digest and judge model, labelled as an opinion, and after held and broken.

No prompt makes a model immune to persuasion, and this note does not claim one does.
What the mechanisms above guarantee is narrower and testable: whatever an output says
reaches the judge only as data, it cannot change the rubric or the answer's shape, and
it cannot touch the structural verdict. Whether a given model resists a given trick is
measured, not asserted. The hostile set below is run against a real model by the
installation's owner, outside CI, as a judgment like any other.

## Which outputs are judged

- **Broken runs are not judged.** A case broken in a repetition is broken, and a score
  for that repetition would be the second grade NT-006 §5 rules out. Not judging it also
  saves its cost. The report counts these as *not judged: broken*.
- **Runs without an answer are not judged:** parked, failed, or erased. Each is counted
  with its cause.
- **The experiment must have settled.** A judgment requested while any of its runs is
  still open is a 409, so a judgment always covers a fixed set.

## Reading a judgment

A judgment shows, in this order:

1. what it judged: the experiment, with its dataset version, agent version and
   configuration;
2. the rubric digest, output type and pass criterion, and the judge's provider, model and
   effort;
3. outputs judged out of the experiment's outputs, with each reason for one not being
   judged, and the repetitions;
4. the results: passing judgments out of judgments (`k of n`); for a `score` or
   `category`, the count for each value; and how many outputs were **unstable**, meaning
   their repetitions disagreed on passing;
5. the cost.

Each case shows its own `k of n`, beside its structural result and never in its place.
When the judge model is the model being judged, the screen says so. It is allowed,
because it is often the only model available.

The judgment's cost is its own. It is not added to the experiment's cost, so comparing
two experiments still compares what the agent cost.

## Comparing

Two experiments can be compared on quality only when **both have a judgment with the
same rubric digest and the same judge provider, model and effort**. A different rubric
or judge is a different question, and its numbers are not comparable. Otherwise the
comparison shows the structural side and says why there is no quality side.

When they are comparable, quality is compared over the cases that **held on both sides**,
on top of NT-015's rule for dataset versions. Without this, the side that broke its hard
cases would be judged only on its easy ones and look better for it.

## Cost and whose budget pays

#261 asked whether judges spend the agent's budget or a separate evaluation budget. What
the code does today decides most of it:

- A simulated run, whether a battery or an experiment run, is limited by its per-run
  ceiling, narrowed by its scope's remaining headroom (`Worker.scopedBudget`).
- What it spends never counts against that scope's period ceiling. `SpentSince` sums
  real runs only.
- What it spends never reaches the FinOps projection. `finops.writeSpend` skips every
  simulated planning call.

**For phase 3, a judge pays the way an experiment does,** and the person who asked for
the judgment approved its stated cost:

- the request declares a **per-run ceiling for the judge**, required and greater than
  zero, because a judge's call is not shaped like the agent's;
- the console states, before it starts, the number of judge runs (outputs judged ×
  repetitions) and their ceiling, as it does for an experiment;
- a judgment never consumes the agent's period ceiling, so measuring an agent cannot
  stop it working.

Two things stay open and are not part of phase 3:

- **Measurement is invisible in FinOps today**, for batteries and experiments as well as
  judges. Showing it as its own line, apart from operating spend, is a FinOps change of
  its own. Judges make the gap bigger, so it should get an issue before they ship.
- **A separate evaluation ceiling over a period** is needed when something spends with
  nobody asking each time. That is phase 4, and it is decided there.

## Erasure

A judge run holds the case input, a copy of the judged output, the rubric and the
judge's reason. All of these are reached by the erasure that already exists, with no new
search:

- A subject erasure follows a named run's input digest to its corpus copies and to the
  simulated runs of the same agent that hold the same bytes (`replaySearchSQL`). It then
  erases each such run's content whole. A judge run holds the case input as its own
  content, so it is found the way a battery or experiment run is.
- Retention does the same through `expiredReplaysSQL`.
- A simulated run's tools answer with nothing (`DryTools`), so a judged output holds
  nothing a tool fetched. What it says about the case's subject came from the case
  input, and following the input digest therefore reaches every judgment of it.
  Anything else an output repeats came from the agent's instructions or its memory,
  which have erasure of their own.

What survives erasure is what NT-015 already keeps: the run, its steps, digests and
references, and the sealed `judgment` value. The value is `pass`, an integer or an
identifier, and says nothing about the person.

## Rollout

A worker on 0.101.0 that claims a judge run resolves it as an ordinary experiment run
with no configuration (`worker.resolve`). It would plan **the judged agent itself** on
the case input and spend its money on an answer nobody asked for. It would never write a
`judgment`, so no false score could result. It is still a wrong run.

The experiment listing on 0.101.0 also groups every simulated run that names the
experiment (`experimentGroups`), so it would count judge runs among the experiment's
runs.

So this ships in two releases, as experiments did:

1. **Readers and workers.**
   - The simulation pool resolves a judge run as a judge, and refuses one whose opening
     is not simulated or names no experiment.
   - Every reader that groups an experiment's runs leaves judge runs out.
   - Reading a judgment's simulation id as a simulation is a 404.
   - Nothing opens a judge run yet.
2. **Writers.** The API, the console and the manual, enabled only once production
   `readyz` shows release 1 on every replica.

After the first judgment, rolling back below release 1 is not safe. Rolling back to
release 1 itself is: it knows judge runs and cannot start them. As with experiments, any
judgment still running should settle first.

## What this is not

- **A judge in the start gate, the battery or the drift sweep.** Those stay structural.
  An opinion does not decide whether an agent may run.
- **A comparison with an expected answer.** NT-015 leaves expected output out of a
  dataset, so a rubric judges an output on its own merits, with the case input as
  context.
- **Online sampling, the review queue, or an ensemble.**
- **A model leaderboard** (NT-006 §8). A judgment belongs to one agent's experiment.

## Tests that must exist

- **Gate:** a judge run, from the writer's own opening, neither lets a version start nor
  stops it, does not count as a battery, and does not let an agent leave Draft. The
  readers are the ones already deployed, on Postgres and on the fake.
- **Experiment intact:** after a judgment, the experiment's report, its listing counts,
  its cost and the answer to a retry with its key are the same as before. Sabotage:
  remove the exclusion from each experiment reader in turn, and a test must fail each
  time.
- **Old worker:** a judge run claimed by a pool without judge resolution never writes a
  `judgment`, and the report counts it as failed.
- **Fence:** a `judge` on a non-simulated run, or on one with no experiment, is refused
  by the opener and again by the pool.
- **No tools:** a judge run's pack is empty, whatever the judged agent declares.
- **Hostile:** with a fake model that records what it was sent, outputs that impersonate
  the rubric, forge a closing delimiter, or announce their own score appear only inside
  their data block, never in the system instructions. Every answer that is not a single
  value of the declared type is a failed judgment, and never a pass. Use the values a
  permissive parser would accept: a value with trailing prose, two values, an
  out-of-range integer, an undeclared category.
- **Structure first:** a run broken in a repetition is not sent to the judge, and no
  judged result changes `held` or `broken`.
- **Comparison:** different rubric digests, or different judges, give no quality side.
  Comparable judgments compare only cases held on both sides.
- **Erasure:** a subject erasure of the run a case came from erases every judge run of
  that case's replays, including the copy of the output and the reason. The sealed
  values stay.
- **Ceiling:** a judgment request without a positive per-run ceiling is refused, and a
  judge run that reaches its ceiling is a failed judgment.
- **Scope:** a judgment is read, listed and requested only in the scope the experiment
  was recorded in, and every door answers 404 before any other answer.
- **Journey:** from the API, through a settled experiment and real judge runs, to the
  report, on real Postgres.

## Decisions needed before implementation

1. **Budget.** This note proposes: the judge pays as an experiment does, a per-run
   ceiling is required, and the agent's period ceiling is never touched. A FinOps line
   for measurement spend gets its own issue first, and a separate period ceiling waits
   for phase 4.
2. **Broken runs are not judged.** This saves cost and keeps a score from sitting on a
   breach. The alternative is to judge them, which costs more and adds a column that
   must never be read as quality.
3. **The rubric lives in the request, addressed by its digest,** not in the agent
   definition.
4. **The judge model may be the judged model,** with the screen saying so.
