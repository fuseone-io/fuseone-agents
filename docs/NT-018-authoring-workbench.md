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

| Operation | Validated against |
|---|---|
| `add_tool`, `remove_tool` | The catalogue, as `read.go` does today |
| `set_instructions` (one item: add, replace or remove) | Length limit |
| `add_step`, `edit_step`, `remove_step` | `reaches` must be in the catalogue and in the draft's tools |
| `set_approval_rule` | The tool's classified effect; the role must exist in the scope |
| `set_trigger`, `set_closing`, `set_never` | The same validation the form uses |
| `set_model`, `set_budget` | Provider registered; ceiling within the installation's maximum |

Scope, name and stage are **not** operations. The author sets them by hand.
Scope decides who may act and stage decides how far the agent is trusted, so
both stay human acts. A proposal that fails validation is shown crossed out with
the reason, as the mockup does for `pagamentos.estornar_cartao`, and cannot be
accepted. `read.go` grows from a filter on one reply into the validator of every
operation.

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

Whether publishing refuses a draft that leaves a part uncovered is
[open](#open).

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
store under owner kind `authoring`. They are covered by retention and by subject
erasure like a run's content. Without that, a ticket pasted into the chat would
be personal data with no erasure path.

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
that is not current. Three changes make this safe:

- `Publisher` gains a path that seals without `makeCurrentOn`.
- The current-version query falls back to the newest version when nobody chose
  one (`catalogue.go:41`, the same in `event_graph.go:79` and
  `standing/create.go:39`). That fallback must skip candidates. Otherwise an
  agent with no `agent_state` row would start running the first candidate
  anyone rehearsed. Each of the three queries gets a test with exactly that
  agent.
- **Publishing from the workbench is `MakeCurrent` of the candidate that was
  experimented on**, not a fresh seal. The digest that went through the
  experiment is the digest that goes live, and the screen can say so.

Every candidate is judged by the same `p.judge` and `refuseStage` checks as a
publication. A candidate an operator would not be allowed to publish cannot be
rehearsed either.

## Live rehearsal

A rehearsal is a simulated run of a candidate on **one** case the author typed
or pasted. It uses the same Gate, ledger and budget as today's simulations. Two
things are new.

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

There is no streaming today. **The first version polls**, every second while a
rehearsal is active and the run is not waiting on the author. A rehearsal spends
most of its time waiting for the author, so polling at that rate is cheap and
removes SSE from this note's critical path. SSE is its own decision, made after
we have seen the rehearsal in use.

## Correction becomes a case

`POST /agents/{id}/regressions` already accepts a simulated run. The workbench
calls it with the rehearsal's run when the author accepts a correction. One
accepted correction produces two things: an operation on the draft and a case.

The new part is that **a case keeps the world**. The answers the author typed
in the rehearsal are copied into the case, by reference and digest, beside the
input. Without them, replaying the case in an experiment would go back to empty
tool results, and the case would test a different situation from the one the
author corrected.

In an experiment, a case with recorded answers runs on a replay layer that
answers each call from the recording, matched by tool and arguments. A call the
recording does not cover gets an empty result and marks the case
`not_comparable`, with the call named. This uses the comparison's existing
state, so a case that behaves differently from the recording is never reported
as held.

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
| 0 | Close the door | `DecideApproval` refuses simulated runs |
| 1 | Server draft and operations | Draft table, operations with validation, trail, ETag. Workbench with the draft panel and hand editing |
| 2 | Candidates | Seal without making current; the three current-version queries skip candidates; publish by `MakeCurrent` of a candidate |
| 3 | The conversation | Turns, proposals, checklist, transcript in the content store, ceiling per turn |
| 4 | Live rehearsal | Parking tool layer, `world_*` and `rehearsal_decided` steps, one-second polling, Ensaio tab |
| 5 | Cases with the world, experiments in the thread | Recorded answers in the case, replay layer, `not_comparable` on an unscripted call, Experimento and Casos tabs |

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
- A replayed case whose agent makes a call the recording does not cover is
  `not_comparable`, never `held`.

## Open

| # | Question | Blocks |
|---|---|---|
| A | Does publishing from the workbench refuse a draft whose checklist leaves "what must never happen" uncovered? | Phase 3 |
| B | Do two authors on one draft see each other's conversation, or does each have their own thread over the shared draft? | Phase 3 |
| C | SSE after seeing the rehearsal in use, or never? | After phase 4 |
