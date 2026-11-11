# NT-018 — The authoring workbench

Status: proposed · 2026-11-11 · revises [NT-003](NT-003-conversational-authoring.md) §4 ·
builds on [NT-006](NT-006-evaluating-agents.md), [NT-015](NT-015-evaluation-datasets-and-experiments.md)
and [NT-016](NT-016-judge-evaluators.md)

## What this is

Today an agent is authored in four separate places that do not talk to each
other: an interview, a form editor, a batch simulation and an experiments page.
Each works, and none of them feels like working on the agent. The author fills
in a form, uploads a file of cases, waits for a report, opens a dialog to record
a correction, and goes to another screen to compare versions.

This note specifies a single screen, the **workbench**, where the author:

1. **builds** the agent by talking to an assistant, which proposes changes the
   author accepts one by one;
2. **rehearses** it live on one case, answering for the world when the agent
   reads something and deciding as the approver when the Gate holds an action;
3. **corrects** it in the same conversation, and the correction becomes both a
   draft change and a dataset case;
4. **experiments**, running the draft against the dataset and comparing it with
   the version in production, without leaving the thread.

The interactive mockup the direction was chosen from is
[Bancada de agentes v2](https://claude.ai/artifact/XZvoqboWikHSPTnsviX6t1). It
has a conversation on the left and a workspace on the right whose tab follows
the mode: Rascunho, Ensaio, Experimento, Casos.

**Who does what.** An author holding `agent:publish` on the agent's scope does
everything on this screen, in the console. Nothing on it reaches a customer
system, and nothing on it is done by an agent. Publishing remains a separate act
by the same person.

## Decided

Decided by Kleber on 2026-11-11:

1. **The conversation is free.** The assistant chooses what to ask next. This
   revokes NT-003 §4 ("the interview is a wizard, not a chat") and the comment
   at the top of `internal/authoring/interview.go` that rests on it. How the
   result stays reviewable is the subject of
   [A free conversation, and what is reviewed instead](#a-free-conversation-and-what-is-reviewed-instead).
2. **The author plays the world.** In a rehearsal, nothing reaches a real
   system. When the agent calls a tool, the author types what the tool would
   have answered. The rule in `internal/simulate/dry.go`, that a simulated run
   has no way to reach a real system, stands unchanged.

Decided in review of `e27dfcf7`:

3. **Activation refuses a draft with no answer to "what must never happen".**
   An explicit "no restriction beyond the installation's policies" counts. An
   empty block does not.
4. **One conversation per author** over a shared draft. The draft, diff and
   trail are common, and `ETag` settles concurrency.
5. **SSE is deferred.** Polling is the first version's contract.

## What exists today

Verified against `origin/main` at `b028e09a`.

| Piece | State | Where |
|---|---|---|
| Interview | Seven fixed questions; the model only translates answers into fields | `internal/authoring/interview.go`, `POST /agents/interview` |
| Free description → fields | One text in, seven fields out, no history | `POST /agents/interview/suggestions`, `translate.go` |
| Catalogue boundary | A tool the model names that is not connected does not exist | `internal/authoring/read.go` |
| Authoring provider and daily ceiling | Exists, enforced per call | `GET/PUT /admin/authoring`, `ErrOverCeiling` |
| Draft | **Browser only**: `sessionStorage` and React state | `web/src/features/agents/agent-draft.ts` |
| Editing | Whole spec sent on publish; no structured operations | `PUT /agents/{id}/versions` |
| Publishing | Always makes the new version current | `internal/spec/publisher.go:102` |
| Simulation | Batch, from JSONL or the corpus; every tool answers empty | `internal/simulate/dry.go`, `POST /agents/{id}/simulations` |
| Approvals in a simulation | The run stops at `awaiting_approval`; nobody decides it | `simulate/report.go` |
| Correction → case | Exists, from any run, simulated included | `POST /agents/{id}/regressions` |
| Case contents | Input, expectations, origin run, note; **no tool answers** | `domain.RegressionCase` |
| Experiments and comparison | Exist; per case `held`, `broke`, `absent`, `not_comparable` | `simulate/experiment*.go` |
| Judges | Model and worker exist; nothing opens a judge run yet | `internal/judge`, NT-016 release 2 |
| Streaming | **None.** No SSE, no `http.Flusher`, no `EventSource`; the console polls every 2 s | `simulation-api.ts`, `experiments-api.ts` |

`CLAUDE.md` describes `httpapi` as "HTTP + SSE". The second half is not true
today.

## A free conversation, and what is reviewed instead

NT-003 kept the questions fixed because a creation path whose questions change
every time cannot be reviewed, reproduced or audited. That argument is correct
about the conversation. The answer here is that **the conversation stops being
the thing anyone reviews**. Five rules replace the fixed questions.

### 1. The draft is the record, not the transcript

The draft is a specification. It changes only by **operations**, and an
operation lands only when a person accepts it. What is reviewed at publication
is what is reviewed today: the specification, its narrative read-back, and now
its diff against the version in production. Two conversations that reach the
same draft produce the same agent, so reproducing the conversation does not
matter.

### 2. The model proposes operations and never grants

Each assistant turn returns text plus zero or more proposals. A proposal is one
typed operation:

Every operation edits a field `AgentDefinition` already has, and claims no
more than that field does:

| Operation | Edits | Validated against |
|---|---|---|
| `add_tool`, `remove_tool` | `tools` | The catalogue, as `read.go` does today |
| `set_block` (add, replace or remove one block) | `instructions`: `objective`, `howToAct`, `whenToStop`, `never` | Length limit |
| `add_step`, `edit_step`, `remove_step` | `steps` | `reaches` must be in the catalogue and in the draft's tools |
| `set_approval_delivery` | `approvals` (`direct`, `notify`) | `notify` names people who may decide in the scope |
| `set_trigger` | `triggers` | The same validation the form uses |
| `set_model`, `set_budget` | provider, model, effort, budget | Provider registered; ceiling within the installation's maximum |

Scope, name and stage are **not** operations. The author sets them by hand.
Scope decides who may act and stage decides how far the agent is trusted, so
both stay human acts. A proposal that fails validation is shown crossed out with
the reason, as the mockup does for `pagamentos.estornar_cartao`, and cannot be
accepted. `read.go` grows from a filter on one reply into the validator of every
operation.

**No operation creates a Gate rule.** `approvals` decides only where an
approval is delivered (`domain.ApprovalPolicy`); whether a person must answer
is decided by the installation's policies, which need `policy:write`. The
`never` block is an instruction the model reads, not a policy the Gate
enforces. So when the author says "above R$ 500 the finance team approves", the
assistant does two things:

- it proposes the `never` or `howToAct` text that tells the agent;
- it shows a **policy card**, marked as not part of the draft. The card says
  which rule the Gate would need, whether a current policy already covers it
  (read with `POST /policies/simulate`), and links to the policies screen.

An author's acceptance never becomes `policy:write`. A rehearsal shows the
installation's policies as they are, so the author sees whether the hold they
asked for actually happens. Making a policy from the workbench, for someone who
holds `policy:write`, is [open](#open). The mockup shows the R$ 500 rule this
way: as the installation's policy, outside the draft.

### 3. The seven questions become a checklist

The PRD §6.1 schema is still the schema. The checklist shows which of its seven
parts the draft covers:

- trigger;
- read tools;
- steps;
- exceptions per step;
- approval points;
- closing criterion;
- what must never happen.

The checklist is computed from the draft, not from the conversation, so it is
deterministic. The assistant is told which parts are missing and asks about
them in whatever order the conversation allows. This keeps the PRD's best idea,
"what must never happen?", without making it question seven of a form. **PRD
§6.1 is amended** in the same change: from "not a free-form prompt" to "a
conversation that fills a fixed schema, which the platform checks".

**Activating a candidate refuses a draft without an answer to "what must never
happen".** An explicit answer of "no restriction beyond the installation's
policies" counts, recorded as such in the `never` block. An empty block does
not count: an empty block is not a decision. The other six parts are shown, and
do not block.

### 4. Acceptance leaves a trail; the transcript is content

Every accepted operation is a row in the authoring trail:

- draft;
- operation kind;
- digest of the operation;
- the turn it came from;
- who accepted it;
- when.

Following the rule that immutable rows hold no free text, the row carries ids
and digests only. The operation's body and the transcript live in the content
store under owner kind `authoring`, under the installation's retention.

**Each author has their own conversation** over the shared draft. The draft, its
diff and the trail of accepted operations are common; the threads are not. That
keeps one author's pasted tickets out of another's context.

Subject erasure is promised only where the platform can know the subject:

- **Content brought in by reference.** When the author starts a rehearsal from
  a run, or imports a case, the turn records that run's id. Subject erasure of
  that run follows the reference into the transcript and into the operations
  made in that turn, as NT-015 makes it follow the corpus copy.
- **Anything typed or pasted.** A partial paste, or text rewritten into an
  operation, has no digest in common with any run, so subject erasure cannot
  find it. It is covered by retention, and by erasing the conversation: its
  author, or anyone with `data:erase` on the scope, can erase a thread. The
  screen tells the author this at the composer, rather than leaving them to
  assume a promise the platform cannot keep.

### 5. What the author pastes is untrusted

A pasted ticket can carry instructions. The model reads it as data, but the real
protection is rule 2: nothing reaches the draft without a person accepting a
typed, validated operation. Proposals made in a turn whose context includes
pasted content are marked as such on their card, so the author knows where a
suggestion came from.

### Cost

Each turn is an authoring call and counts against the existing daily ceiling.
`ErrOverCeiling` ends the conversation for the day and leaves the draft intact.
The form editor still works. NT-003 §5's requirement that an installation with
no authoring provider can still publish agents is kept: without a provider the
workbench opens with the conversation disabled, and the draft is edited by hand.

## The draft lives on the server

A conversation that proposes operations needs something to apply them to that
survives a reload and that a second tab can see. Today the draft is in
`sessionStorage`.

- A draft is **mutable working state**, not the ledger and not a version. It
  lives in a table of its own, holding the specification, the version it
  started from, and an ETag.
- **One draft per agent**, shared by its authors. Applying an operation takes
  `If-Match` and answers 412 when the draft moved. The same pattern is used for
  the API in NT-017.
- The draft's content (instructions are free text) is subject to the same
  retention and erasure as the trail's content.

## Candidate versions

A rehearsal and an experiment have to run a specification that cannot change
under them. A draft can change. Publishing makes a version current
(`publisher.go:102`), which would put an untested draft into production.

So the workbench **seals a candidate**: an immutable version, with a digest,
that is not current and has never been published. Sealing and activating are
two acts, with different checks.

### Sealing

`Publisher` gains a path that seals without `makeCurrentOn`. Sealing checks
that the author holds `agent:publish` on the scope and runs `p.judge`, so the
rehearsal shows the same graph findings a publication would. It does **not**
run `refuseStage`: rehearsing an improvement to an autonomous agent is safe.
Putting it live is what must be refused.

### A candidate never opens a real run

A candidate is a row of `agent_specs` marked `candidate`. Three places must
never treat it as published.

- **The current-version fallback.** When nobody has chosen a version, the query
  takes the newest one: `catalogue.go:41`, `event_graph.go:79` and
  `standing/create.go:39`. All three must skip candidates. Otherwise an agent
  with no `agent_state` row would start running the first candidate anyone
  rehearsed.
- **The version list a run is pinned from.** `trigger.Pin` accepts any version
  `Registry.Versions` returns (`trigger/version.go:15`), and a rerun can name a
  version. A candidate in that list could be pinned by a real run.
- **The fence lives in the `Opener`**, after `Pin` (`trigger/open.go:187`). A
  candidate opens only a simulated run marked as a rehearsal or an experiment.
  Every other request naming one is refused, with the same error as a version
  the agent never published, so a candidate's id reveals nothing.

### Activating

**Publishing from the workbench activates the candidate that was experimented
on.** It does not seal a fresh one, so the digest that went through the
experiment is the digest that goes live. Activation repeats every guarantee of a
publication, in one transaction:

1. `If-Match` on the agent's current version. If the agent moved since the
   experiment started, the answer is 412 and the author re-runs it.
2. `agent:publish` on the scope, checked again. It may have been revoked since
   the seal.
3. Every resource the candidate names is held `FOR SHARE` and checked to exist:
   tools, provider, connector instances, area. This is the same fence that NT-017 (in
   review, #321) specifies for publishing.
4. `refuseStage` with `autonomous`.
5. `p.judge` again, against the graph as it is now.
6. The `never` block is answered (see [the checklist](#3-the-seven-questions-become-a-checklist)).
7. The candidate is marked published and made current, atomically.

If any step fails, nothing changes, and the candidate stays a candidate.

### Retention

A candidate that was never activated holds free text, in its instructions and
steps, that nobody published. That text is moved out of the version row into
the content store, under owner kind `authoring` and the installation's
retention. What remains in the row is ids, the tool list and the digest. A
rehearsal or experiment run pinned to it still names a version whose digest
verifies, and whose text may have aged out. Activation copies the text back
into the row, which from then on is a version like any other.

## Live rehearsal

A rehearsal is a simulated run of a candidate on **one** case the author typed
or pasted. It uses the same Gate, ledger and budget as today's simulations.

### A rehearsal is not a battery

Today a run that is `simulated`, names a `simulation` and a `case`, and names no
`experiment` counts as evidence about the agent. A rehearsal would match that
description:

- `HasSimulation` (`ledger/simulations_postgres.go:45`) would let one rehearsal
  take the agent out of Draft;
- `Batteries` and `LastBatteryAt` would let it reset the clock the drift sweep
  reads.

So `run_started` gains `rehearsal`, and the three readers exclude it, in Postgres
and in `ledger.Memory`. An older replica would still count a rehearsal, so this
ships the way experiments and judges did: **the readers in one release,
deployed on every replica, before the release that first writes a rehearsal.**

The endpoints that answer for the world and decide in a rehearsal accept only a
run marked `rehearsal`, stopped at exactly the request being answered (`atSeq`,
as `DecideApproval` requires today). A real run, a battery or an experiment is
refused.

Two things are new in the run itself.

### The author answers for the world

In rehearsal, `simulate.Deps` swaps in a tool layer whose `Invoke` **parks** the
run, instead of `DryTools`, which returns empty at once. The run waits for an
answer the way it waits for an approval:

- the call is recorded as today;
- a `world_awaited` step marks the wait;
- the author's answer is stored in the content store, and a `world_supplied`
  step records its reference and digest.

The answer carries `LabelUntrusted`, because it is external content as far as
the agent's next decision is concerned. Calling it trusted would let a rehearsal
take a path production would refuse.

`Deps` remains the only constructor. The parking layer, like `DryTools`, has no
argument or flag by which it can reach a real system. The test that guards
`dry.go` today is extended to cover it.

Writes are answered by the author too, for example a refund id. Nothing
executes, and the Gate has already decided whether the call was allowed before
the author is asked.

### The author decides as the approver

When the Gate holds an action, the rehearsal shows the decision in the trace. The
author's decision is recorded as a **`rehearsal_decided`** step. It is never
`approval_decided`, so no count, report or audit of real approvals can mistake
it for one. It needs `agent:publish` on the scope, not `approval:act`, because it
authorises nothing.

Before this ships, one existing door must close. `DecideApproval`
(`internal/httpapi/approvals.go:80`) does not check whether the run is
simulated. Simulated runs are hidden from the inbox, but a direct call by
someone holding `approval:act` would record a real `approval_decided` on a
simulated run. We have not verified whether the simulation pool resumes the run
afterwards. Either way the record is wrong, so `DecideApproval` refuses
simulated runs, tested with a simulated run held at `awaiting_approval`.

### Watching it happen

There is no streaming today. **Polling is the contract of the first version**:
every second while a rehearsal is active and the run is not waiting on the
author. A rehearsal spends most of its time waiting for the author, so polling at
that rate is cheap. SSE is deferred. It is reopened only after phase 4 has been
measured in use and under load.

## Correction becomes a case

`POST /agents/{id}/regressions` already accepts a simulated run. The workbench
calls it with the rehearsal's run when the author accepts a correction. One
accepted correction produces two things: an operation on the draft and a case.

The new part is that **a case keeps the world**. The answers the author typed
in the rehearsal are copied into the case, by reference and digest, beside the
input. Without them, replaying the case in an experiment would go back to empty
tool results, and the case would test a different situation from the one the
author corrected.

The recording is a **sequence**. Each entry holds:

- its position;
- the tool;
- a digest of the arguments;
- the reference to the answer.

The same call made twice with different answers is two entries, not one
ambiguous key.

In an experiment, a case with a recording runs on a replay layer that answers
calls strictly in order. Each entry is consumed once. The case is marked
`not_comparable`, with the first divergence named, when a call is:

- **missing:** the run settles before the recording is consumed;
- **extra:** a call made after the recording is exhausted;
- **repeated:** an entry asked for a second time;
- **out of order:** a call matching a later entry rather than the next one.

From that point the run gets empty results. This uses the comparison's existing
state, so a case that behaves differently from the recording is never reported
as held. Matching strictly in order is deliberate. It will mark as
`not_comparable` some runs that reorder two independent reads, which NT-003 §8
allows inside a step. That costs a re-recorded case. The alternative is a
replay that picks an answer for the agent, and that would be the platform
inventing evidence.

NT-015's erasure rules apply to the recorded answers as they do to the input,
since both came from the same rehearsal:

- erasure follows the input;
- erasure and copying exclude each other;
- deleting a case erases its copies.

Each new case creates a new dataset version, as NT-015 specifies.

## Experiments in the thread

No new mechanism. "Rodar contra os casos" starts an NT-015 experiment that pins
three things:

- the candidate;
- the latest dataset version;
- the candidate's configuration.

The thread then shows the comparison against the current version. Each case's
`was`/`now` maps onto the mockup's three colours:

- held to held: kept;
- broke to held: fixed in this version;
- held to broke: a regression.

The regressions are listed with a link that opens their trace in the Ensaio tab.
Judge opinions appear beside the verdict once NT-016 release 2 opens judge runs.
Until then, the column is absent rather than empty.

## Phases

One pull request per phase. The console screen grows with each phase, under a
new route `/agents/:agentId/workbench`. The form editor stays where it is.

| # | Phase | Ships |
|---|---|---|
| 0 | Close the doors | `DecideApproval` refuses simulated runs. `HasSimulation`, `Batteries` and `LastBatteryAt` exclude `rehearsal`, in Postgres and in memory. This must be released and deployed on every replica before phase 4 ships. |
| 1 | Server draft and operations | Draft table, operations with validation, policy card, trail, ETag. Workbench with the draft panel and hand editing. |
| 2 | Candidates | Seal without making current. Candidate text moves to the content store. The three fallback queries skip candidates. The `Opener` fences candidates. Activation runs the seven steps. |
| 3 | The conversation | One thread per author: turns, proposals, checklist, transcript in the content store, references to runs, thread erasure, ceiling per turn. |
| 4 | Live rehearsal | Parking tool layer, `rehearsal` marker, `world_*` and `rehearsal_decided` steps and their endpoints, one-second polling, Ensaio tab. |
| 5 | Cases with the world, experiments in the thread | Ordered recording in the case, replay layer that consumes it once, `not_comparable` on divergence, Experimento and Casos tabs. |

Phases 1 and 2 are useful without the rest. A server-side draft fixes losing an
interview to a closed tab, and candidates let an experiment test something
before it is in production.

## What this is not

- **Not a way to reach production data.** Reads in a rehearsal are answered by
  the author. NT-003 §10's rule that authoring does not touch customer data
  stands.
- **Not a model that publishes, grants or promotes.** It proposes typed
  operations. A person accepts each one, publishes, and promotes the stage.
- **Not a replacement for the form editor or the API.** Both stay, and NT-017's
  Terraform provider publishes versions without passing through here.

## Tests that must exist

Each is written red first. Each sabotage uses the value the bug would accept.

- An operation naming a tool outside the catalogue is refused. Sabotage: drop
  the catalogue check in the validator; the test must name the tool.
- No operation changes scope, name or stage. Sabotage: accept a `set_scope` op.
- Applying an operation with a stale ETag answers 412, and the draft is
  unchanged.
- A trail row holds no free text. A `CHECK` or a test reads every column
  against a pasted string.
- Subject erasure clears the transcript and the operation bodies of a draft
  whose conversation included the named run's input.
- For each of the three current-version queries, an agent with no
  `agent_state` row and only a candidate has no current version.
- Publishing from the workbench makes current the exact digest the experiment
  ran.
- A rehearsal run cannot reach a real tool. Extend `dry.go`'s guard to the
  parking layer.
- An author-supplied answer carries `LabelUntrusted`. After one, `checkTaint`
  sends a reversible write to a person and stops an irreversible one, exactly as
  in production.
- `rehearsal_decided` never counts as an approval. Use the approvals listing and
  the stats against a run that has one.
- `DecideApproval` on a simulated run held at `awaiting_approval` is refused.
- A replayed case is `not_comparable`, never `held`, for each of four
  divergences: a missing call, an extra call, a repeated entry, and an
  out-of-order call. Add a case where the same call is made twice with
  different answers, and check that each answer is consumed once and in order.
- No operation writes a policy. The sabotage is an operation that takes an
  author holding only `agent:publish` to a stored policy; the test must name
  the operation.
- A real rerun naming a candidate is refused.
- An agent whose only version is a candidate opens no real run.
- A rehearsal naming a candidate is accepted, and so is an experiment.
- Activation is refused, and changes nothing, in each of these cases (412 for a moved version, otherwise the answer publishing gives today for the same cause):
  - the current version moved since the experiment;
  - the agent became autonomous;
  - a tool, the provider or the area the candidate names no longer exists;
  - the author lost `agent:publish`;
  - the `never` block is empty.
- Sealing a candidate for an autonomous agent is accepted.
- A rehearsal does not satisfy `HasSimulation` and does not move
  `LastBatteryAt`, in Postgres and in `ledger.Memory`.
- The world and rehearsal-decision endpoints refuse each of these: a real run,
  a battery run, an experiment run, and a rehearsal stopped at a different
  `atSeq`.
- Erasing a run that a turn referenced clears that turn's transcript and its
  operations. Erasing a thread clears all of it.

## Open

| # | Question | Blocks |
|---|---|---|
| D | Should someone holding `policy:write` be able to turn a policy card into a policy from the workbench, or always go through the policies screen? | Nothing; phase 1 ships with the link |
