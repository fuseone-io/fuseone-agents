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

Decided in review of `39c96e0a`:

6. **Policies are created only on the policies screen.** For someone holding
   `policy:write`, the policy card opens that screen with the rule pre-filled.
   Review, simulation and saving stay behind its administrative boundary.

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
- it shows a **policy card**, marked as not part of the draft. The card states
  the rule the author described, in the policy editor's terms.

**The card never says a current policy covers the request.** The platform
cannot prove that. `POST /policies/simulate` replays a *new* rule against past
decisions; it does not test whether the installed rules imply the one asked
for. It also needs `policy:read`, which an author holding only `agent:publish`
may lack. A card reading "already covered" on that basis could lead someone to
leave a needed approval with no rule. What the author can observe is the
rehearsal. It runs under the installation's policies as they are, so on that
case the author sees whether the Gate held the action or let it through. The
card says exactly that much: "the rehearsal will show what the current policies
do on this case".

What the card offers depends on who is looking:

- **Someone holding `policy:write`** gets a link that opens the policies screen
  with the rule pre-filled. Review, simulation and saving stay on that screen,
  behind its own administrative boundary.
- **Anyone else** is told to ask a policy administrator. The card never names
  who holds `policy:write`. Finding that out means reading the directory and the
  grants, which is administrative and reveals identities, and no API grants it
  to an author.

An author's acceptance never becomes `policy:write`.

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

#### Where text ends up, and how erasure follows it

A sentence typed in a turn can be copied to four places:

- the transcript;
- the body of the operation proposed from it;
- the draft value that operation wrote;
- the content of every candidate sealed while that value stood.

Erasure has to reach all four, so provenance is kept on the **values**, not just
on the conversation.

- Each draft value carries its provenance: the ids of the operations that wrote
  its current text, and through them the turns and any run those turns
  referenced. A value the author typed by hand carries the author's id instead.
- Each seal records the provenance of every value it sealed.

Erasing a thread, or erasing a run a turn referenced, clears four things in one
transaction:

1. the transcript turns involved;
2. their operation bodies;
3. every draft value whose current provenance includes one of those
   operations. The value is replaced by an erasure marker, the draft's ETag
   moves, and the checklist shows the part as missing;
4. the content of every unpublished seal that sealed such a value. That
   candidate becomes impossible to activate (see [Retention](#retention)).

A value later overwritten by hand no longer carries the erased operation, so it
stays.

Subject erasure is promised only where the platform can know the subject:

- **Content brought in by reference.** When the author starts a rehearsal from
  a run, or imports a case, the turn records that run's id. Subject erasure of
  that run follows the reference along the chain above, as NT-015 makes it
  follow the corpus copy.
- **Anything typed or pasted.** A partial paste has no digest in common with any
  run, so subject erasure cannot find it. It is covered by retention, and by
  erasing the thread. The thread's author, or anyone with `data:erase` on the
  scope, can erase it, and that follows the same chain. The screen says this at
  the composer, so the author is not left assuming a promise the platform cannot
  keep.

**Only the published row is out of reach.** Activation writes the sealed bytes
into `agent_specs`, and that row is never erased, because runs are pinned to it
(`spec/retire.go`). Everything else stays erasable after activation, along the
same chain:

- the transcript;
- the operation bodies;
- the draft's values;
- the seal's own copy in the content store.

The activation screen shows the full text going live and says that this text is
kept. That is the last point where the author can remove what should not stay.

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
- The draft's values carry provenance, and erasure clears them along the chain
  in [where text ends up](#where-text-ends-up-and-how-erasure-follows-it).

## Candidate versions

A rehearsal and an experiment have to run a specification that cannot change
under them. A draft can change. Publishing makes a version current
(`publisher.go:102`), which would put an untested draft into production.

So the workbench **seals a candidate**: a version digest that is not current and
has never been published. The digest is computed exactly as publication
computes `version_id`, so a candidate that goes live keeps its id. Sealing and
activating are two acts, with different checks.

### How a candidate is stored

**`agent_specs` is not touched.** It stays insert-only (`0006_agent_specs.sql`),
and a row there still means "published". Candidates live in a table of their
own, `agent_seals`, which is also insert-only.

**A seal keeps the whole definition, as bytes.** `version_id` is the digest of
the specification file's bytes (`versionOf`, `spec.go:286`). The only copy that
cannot drift from the digest is therefore those exact bytes. They carry every
field the definition has: name, company and area, provider, model, effort,
fallback, cache TTL, reply locale, tools, budget, triggers, steps, emits, memory
learning, approvals and instructions. The bytes go to the content store under
owner kind `authoring` and owner `seal_id`. `p.judge`, the rehearsal, the
experiment and activation all parse those bytes, never a reconstruction.

Each row of `agent_seals` is one act of sealing, and holds only indexable
metadata and the reference:

- `(agent_id, version_id, seal_id)`;
- the **sealed scope**, company and area as they stood in the bytes;
- who sealed it, and when;
- the reference to the bytes;
- the provenance of the draft values it sealed.

A *candidate* is a `version_id` with at least one seal and no row in
`agent_specs`. Because one version can have many seals, the cases that collide
today resolve without any update:

| Case | Result |
|---|---|
| The draft's digest is already published | No seal is written. The workbench rehearses, experiments on and activates that published version. Activating it means `MakeCurrent`, under the same steps. |
| Two authors seal the same digest | Two seals, one candidate. The trail records both. |
| An ordinary publication of the same digest while a candidate exists | `agent_specs` gains the row, and from then on the version is published. Its seals stay as history, and activating it means `MakeCurrent`. |
| The candidate's text aged out or was erased | Re-sealing from the draft writes a **new** seal, with a new content reference. Nothing is restored over a tombstone. |

That last row matters. `Content.RestoreFor` clears any tombstone, including
one left by subject erasure. A candidate that restored its text by digest could
bring back what an erasure removed. A seal never restores. Text erased from the
draft also gives a different digest when re-sealed, so it cannot come back that
way either.

### Who can see and use a candidate

**Every door authorizes against the sealed scope, never the agent's current
scope.** This covers listing, reading, rehearsing, experimenting and
activating. Each of them needs `agent:publish` on the sealed scope. A candidate
is unpublished work, so `agent:read` is not enough to see it. Outside that
scope the answer is 404, as for a version that does not exist.

So when an agent moves from `finance` to `cx`, an author in `cx` gains nothing
sealed in `finance`. **Activation refuses a candidate whose sealed scope differs
from the agent's current scope**, with 409 `fuseone:candidate-scope-moved`.
Publishing those bytes would move the agent back without anyone deciding to.
The way forward is to re-seal from the draft in the new scope, as an author
there.

### Sealing

Sealing checks that the author holds `agent:publish` on the scope, and runs
`p.judge` so the rehearsal shows the same graph findings a publication would.
It takes **no lock**: its judgment is informational, and activation judges
again under the lock. It does **not** run `refuseStage`. Rehearsing an
improvement to an autonomous agent is safe; putting it live is what must be
refused.

### A candidate never opens a real run

A candidate has no row in `agent_specs`, so it is invisible by construction to
three places:

- the current-version fallback (`catalogue.go:41`, `event_graph.go:79`,
  `standing/create.go:39`);
- `Registry.Versions`;
- `trigger.Pin` (`trigger/version.go:15`).

A real run, a rerun included, can only pin a version `Pin` finds. A rehearsal or
experiment request resolves a candidate through a separate lookup on
`agent_seals`. The `Opener` (`trigger/open.go:187`) accepts that lookup only for
a simulated run marked as a rehearsal or an experiment. Any other request naming
a candidate gets the same error as a version the agent never published, so a
candidate's id reveals nothing. The tests below hold this fence even though the
table split already enforces it, so that a later change merging the lists breaks
a test, not production.

### Activating

**Publishing from the workbench activates the candidate that was experimented
on.** It does not seal a fresh one, so the digest that went through the
experiment is the digest that goes live. Activation repeats every guarantee of a
publication, in one transaction.

In semantic order, activation:

1. checks `If-Match` on the agent's current version. If the agent moved since
   the experiment started, the answer is 412 and the author re-runs the
   experiment;
2. checks `agent:publish` on the scope again, since it may have been revoked
   after the seal;
3. checks that every resource the candidate names still exists: tools,
   provider, connector instances, area. This is the fence NT-017 specifies for
   publishing (in review, #321);
4. runs `refuseStage` with `autonomous`;
5. runs `p.judge` again, against the graph as it is now;
6. checks that the `never` block is answered (see
   [the checklist](#3-the-seven-questions-become-a-checklist));
7. checks that the sealed scope is the agent's current scope;
8. reads a seal's bytes, checks `versionOf(bytes) == version_id`, inserts the
   `agent_specs` row from those bytes and makes it current.

**Lock order is the publication's, and it is the only one.** Every writer that
can change a current version acquires locks in this order:

0. **The agent's content fence, shared.** This is the fence an erasure and
   anything copying an agent's content already share (`regression/fence.go`,
   `domain.CorpusFence`). Activation and the opening of a candidate run hold it
   shared, around their whole transaction, because each of them reads a seal's
   bytes and writes them somewhere else. Every erasure that can reach a seal
   holds it exclusively: by thread, by referenced run, by subject, and by
   retention. So two orders are possible, and both are safe:
   - **activation first:** the erasure waits, and then erases the seal's copy.
     The published row is the one copy erasure never reaches, and the screen
     said so;
   - **erasure first:** activation reads a tombstone and fails closed with
     `fuseone:candidate-unreadable`.

   The fence is taken first, before any database lock, on its own connection,
   as it is today. An erasure takes only the fence and the content rows, so it
   never waits on the event graph or on an agent row.
1. **The event graph lock** (`lockEventGraph`), first, as `Publisher.Publish`
   takes it today (`publisher.go:71`).
2. **The agent's `agent_state` row**, `FOR UPDATE`. Step 1's `If-Match` and
   step 4's stage are both judged here, under this lock. This is the order
   `refuseStage` already follows inside `Publish`.
3. **The named resources**, `FOR SHARE`, in ascending order of their ids. Step 3
   is judged here.

Then, with nothing left to acquire, activation runs `p.judge`, the `never` and
scope checks, the read of the seal and the insert. Step 2 takes no lock and runs before the transaction begins.
Ordinary publication adopts the same third tier when NT-017's fence lands, so
activation and publication can never wait on each other in a cycle.

If any step fails, nothing changes, and the candidate stays a candidate.

### Retention

A seal's text lives in the content store under the installation's retention,
counted from the seal. When it is gone, through age or erasure, **the platform
fails closed**:

- **Activation** with no readable seal for the version answers 409
  `fuseone:candidate-unreadable`. It never publishes partial text, and never
  publishes text that does not match `version_id`.
- **Opening a rehearsal or experiment run** on such a candidate is refused with
  the same code, before any run exists.
- **A run already open** that cannot read its candidate's text when it next
  needs it ends with a failed step carrying that code. It never continues on a
  partial specification. An experiment counts that case as `not_comparable`.

The way back is to re-seal from the draft. If the text is unchanged, the new
seal has the same `version_id`, so the experiment evidence still applies to the
same digest. If erasure changed the text, the new seal has a different digest
and needs its own experiment.

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
| 2 | Candidates | Insert-only `agent_seals` holding metadata, the sealed scope and provenance, with the full specification bytes in the content store. Every door authorizes against the sealed scope. The `Opener` fences candidates. Activation runs the eight steps in the publication's lock order, behind the shared content fence. Unreadable candidates fail closed. |
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
  - the `never` block is empty;
  - the agent's scope differs from the sealed scope.
- The definition that is activated is byte-for-byte the definition that was
  sealed. A test seals an agent that uses every field: emits, memory learning,
  approvals, fallback, cache TTL, locale and effort. It activates the agent, and
  checks that the published row's `version_id` equals the sealed one and that
  `p.judge` at the seal saw its `emits`. Sabotage: rebuild the definition from
  metadata columns; the test must name the field that was lost.
- Each door, run in both directions, answers 404 outside the sealed scope:
  listing, reading, rehearsing, experimenting and activating. A `cx` author
  cannot see a candidate sealed in `finance`, and a `finance` author cannot see
  one sealed in `cx`. After the agent moves from `finance` to `cx`, the
  `finance` candidate is still invisible to `cx` and cannot be activated.
- Activation and erasure, run concurrently with `testdb.NewContention`, in both
  orders, with one stalled at the point where it holds what the other needs:
  - with activation first, the erasure waits, the published row holds the
    bytes, and the seal's own copy is erased;
  - with erasure first, activation answers `fuseone:candidate-unreadable` and
    no `agent_specs` row exists.

  Sabotage: drop the shared fence from activation. Erasure first must then
  publish erased bytes, and the test must name that.
- After activation, erasing the thread still clears the transcript, the
  operation bodies, the draft values and the seal's copy, and leaves only the
  published row.
- The policy card never names a person, for any author.
- Sealing a candidate for an autonomous agent is accepted.
- A rehearsal does not satisfy `HasSimulation` and does not move
  `LastBatteryAt`, in Postgres and in `ledger.Memory`.
- The world and rehearsal-decision endpoints refuse each of these: a real run,
  a battery run, an experiment run, and a rehearsal stopped at a different
  `atSeq`.
- Erasing a run that a turn referenced, and separately erasing a thread, must
  leave the text in none of these places:
  - the transcript;
  - the operation bodies;
  - the shared draft's values;
  - the content of every unpublished seal that sealed them.

  The test searches for the pasted string in all four. Sabotage: skip the draft
  values, then skip the seals; each sabotage must fail the test by naming where
  the string survived. A value overwritten by hand after the operation keeps
  its text.
- Re-sealing after a seal's text was erased by subject erasure writes a new
  seal and leaves the old tombstone erased. Sabotage: seal through
  `RestoreFor`, which must fail the test.
- Activation, and opening a rehearsal or an experiment, on a candidate with no
  readable seal answer `fuseone:candidate-unreadable`. No `agent_specs` row and
  no run appear.
- Activation reads text that does not match `version_id` and is refused, with
  no row written.
- Each of the three idempotent cases in
  [How a candidate is stored](#how-a-candidate-is-stored) holds. The ordinary
  publication case runs with a candidate of the same digest already sealed.
- An activation and an ordinary publication of the same agent, run
  concurrently with `testdb.NewContention`, both finish: one succeeds and the
  other is refused or waits. Neither deadlocks. Sabotage: take the agent row
  before the event graph lock in activation; the test must time out and name
  the order.
- The policy card's text never claims coverage. A test renders a card for an
  author without `policy:read`, and one for an author with it, and checks that
  neither reads as covered.

