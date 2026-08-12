# NT-003 · Conversational authoring

How an agent gets created. What the PRD asks for, what exists, what has to be
decided before any of it is built.

Companion to [NT-002](NT-002-remaining-work.md), which closed the screens.
This one opens the product's central claim: that somebody who performs a
process can author the agent that does it.

---

## 1. What the PRD asks for

Section 6 states the premise: process knowledge is irreplaceable and only the
domain author has it. What the platform removes is not the knowledge — it is
the **notation**. So the system does not need to be a good code generator; it
needs to be a good interviewer.

| Stage | Requirements | What it produces |
|---|---|---|
| 1 · Interview | FU-01…07 | The specification's fields |
| 2 · Narrative read-back | FU-08 | Prose the author approves |
| 3 · Simulation | FU-09/10 | What it would have done to real cases |
| 4 · Correction by example | FU-11/12/13 | A regression battery |
| 5 · Progressive autonomy | FU-14/15 | Draft → Shadow → Copilot → Autonomous |

Seven questions, each filling one part of the specification:

| Question to the author | Fills |
|---|---|
| When does this start? | Trigger |
| What do you need to know in order to do it? | Read tools |
| What are the steps? | Process graph |
| What usually goes wrong? And then what do you do? | Exception handling |
| What would you not decide on your own? | Human approval points |
| How do you know you are done? | Closing criterion |
| **What must never happen?** | Blocking policy |

The last one is the document's best idea. Somebody in marketing cannot draft a
security policy, but answers *"never send to the whole base without somebody
reviewing it"* without hesitating. That is a guardrail in business language —
the only form in which this audience can produce one.

---

## 2. What exists today

Nothing of section 6.

What exists is the agent editor: name, area, provider, model, instructions,
tools, ceilings, triggers. That form **is the notation section 6 set out to
remove**. It is not wrong — it is the path for somebody technical, and it was
the fastest way to prove that publishing works end to end. But it is stage
zero, and it is worth being explicit that the product's central claim is
currently unimplemented.

`Spec` has no autonomy stage. `PlannedPayload.Node` exists, is written by the
runner, and nothing sets it.

---

## 3. The correction this note has to make

NT-002 concluded that the specification has no graph, and that FU-17's "the
agent's graph" is really the run's. That describes the code. It does not
describe the PRD, and FU-13 says so with its reason attached:

> Corrections are anchored to a *step* of the graph, not to the whole agent.
> That is what makes it possible to localise and fix without degrading the rest
> — and it is the technical reason execution is a graph and not a free loop.

So the "graph as a constraint" that NT-002 offered as an option is a
**prerequisite of stage 4**. Without named steps there is nowhere to anchor a
correction, and correcting an example collapses into rewriting the
specification — which is precisely what FU-11 forbids.

This does not resurrect the visual builder. N5 still rules out drag-and-drop
authoring, and the interview still produces the steps. What changes is that
the steps have to exist as data, and `PlannedPayload.Node` — reserved and
unused — is where a proposal records which one it came from.

---

## 4. The interview is a wizard, not a chat

The PRD is explicit: *"It is not a free-form prompt. It is a guided
conversation that fills a fixed schema."*

That settles a design question before it is asked. The seven questions are
fixed and deterministic; no model chooses what to ask next. The model's job is
**translation**, at three points:

1. turning a business-language answer into specification fields;
2. choosing, among the tools already connected, which serve "what do you need
   to know";
3. writing the narrative read-back back out as prose.

A model that conducted the conversation would decide what to ask, and an
authoring path whose questions vary per run cannot be reviewed, reproduced or
audited. Keeping it a wizard is what makes the output governable.

---

## 5. Where the authoring model is configured

An agent's provider is a capability the installation grants: it decides what
agents may use, it has policy and per-run cost implications, and it is Curator
work. The authoring model is a tool of the platform — it never touches a
customer system, and it only produces text a person approves.

Different decisions, taken by different people. But **one connection**.

- **Integrações** stays the owner of connections: address, sealed credential,
  health. A provider is registered once.
- **Administração** holds the *choice*: which connected provider, which model,
  which effort the authoring assistant uses.

A pointer, not a second registry. Two credential stores would mean two places
to leak from, two to rotate, two to audit, and an installation with one
Anthropic account configuring it twice. The `settings` table already carries
`kind`/`name`/`value` per scope with a vault beside it, so the choice is a row.

Three consequences that come with it:

**It spends money outside any run.** No Gate, no ledger, no ceiling — today
there is no place for that expense to appear. It needs a ceiling of its own and
it belongs in the administrative trail, or it becomes the only invisible spend
in a product whose argument is that nothing is invisible.

**It has to be switchable off.** An installation with no authoring provider
must keep publishing agents through the current form. The interview is the good
path, not the only one; the alternative is an air-gapped install with no strong
model being unable to create an agent at all.

**It reads the tool catalogue, and nothing else.** Question two asks what the
author needs to know, and the answer is matched against tools already
connected. The authoring model never sees customer data — only tool names and
descriptions somebody already published.

---

## 6. Decisions to make before building

| # | Decision | Blocks | Note |
|---|---|---|---|
| D1 | ~~Where the simulation's cases come from~~ | — | **Answered in §10** |
| D2 | ~~What a "step" is~~ | — | **Answered in §8**, against a real agent |
| D3 | ~~Whether regression cases live in the ledger or beside the specification~~ | — | **Answered in §11**, by building it |
| D4 | Whether the autonomy stage is a field on the spec or state beside it | Stage 5 | A published version is immutable; promotion is not a new version |

D2 was the one that unblocked the most and was the least reversible, so it was
answered first, by writing one real agent's steps by hand. §8 has the result.

---

## 7. Recommended order

**Narrative read-back first, then the interview.** It reads backwards and it is
not: the read-back is derivable from the specification that exists today, needs
no domain change, and it is what makes the interview verifiable. Built the
other way round, the interview produces an artefact nobody can review — which
is the problem it was brought in to solve.

| Order | What | Size against work already delivered | Depends on |
|---|---|---|---|
| 1 | Authoring provider: the choice in Administração, its ceiling, its trail entries | Small — the settings row and a form | — |
| 2 | Narrative read-back of an existing specification (FU-08) | Small | 1 |
| 3 | The seven-question interview producing a draft (FU-01…07) | Medium | 1, 2 |
| 4 | Steps in the specification (FU-13's prerequisite) | Medium, and it touches the engine | D2 |
| 5 | Simulation over real cases (FU-09/10) | Large | D1, 4 |
| 6 | Correction by example and the regression battery (FU-11/12) | Large | D3, 4, 5 |
| 7 | Autonomy stages and promotion (FU-14/15) | Medium | D4, 5 |

Steps 1 and 2 are worth doing regardless of how the rest is decided: an
installation that can read back, in plain language, what a published agent
actually says is better off than one that cannot, whether or not anybody is
ever interviewed.


---

## 8. D2 answered: what a step is

Written out by hand for `dev/agents/suporte.agent.md`, which exists and runs,
rather than for an agent invented to fit a shape.

Its instructions say: identify the customer in the CRM by the ticket's email,
search the knowledge base for the reported subject, summarise what was found,
then reply. And: if the customer is not found, say so and stop.

| # | Step | Reaches | Ends when |
|---|---|---|---|
| 1 | Identify the customer | `crm.lookup` | A customer is found — or is not, and the run stops |
| 2 | Research the subject | `kb.search` | There is enough to summarise |
| 3 | Summarise | *nothing* | The summary exists |
| 4 | Reply | `crm.reply` | The reply is sent |

Three things fell out of the exercise that no amount of discussion would have
settled.

**A step is not a tool call.** Step 3 reaches nothing at all — it is the agent
thinking. A model where steps are tools cannot represent this agent, and this
agent is the simplest real one the repository has.

**The exception belongs to a step, not to the agent.** "If I cannot find the
customer, stop" is a property of step 1 and of nothing else. That is FU-04
answered per step, and it is what lets a correction be localised: an agent that
replied badly is corrected at step 4 without touching how it looks customers
up.

**Strict order breaks it.** Searching the knowledge base before looking the
customer up is a legitimate way to run this process, and a specification that
forbade it would be describing the author's first draft rather than their
process. But a plain unordered set loses the guardrail that matters most —
"reply only after the summary exists".

So a step is an **envelope with a gate at its exit**:

- Steps are ordered, and the run occupies exactly one at a time.
- Inside a step the loop stays free: the interpreter decides what to call and
  in what order, bounded by the tools that step reaches.
- Advancing is monotonic. A run moves forward when the step's closing condition
  holds, and never back.

That preserves "the reply comes after the lookup stage" without dictating the
order of calls within a stage, which is the distinction the author actually
cares about and the one a rigid sequence destroys.

### What it changes

**The capability check narrows.** Today the Gate asks whether a tool is in the
agent's pack. It would ask whether it is in the current step's envelope, which
is strictly tighter and costs nothing: an agent that may reply cannot reply
before it has looked anything up.

**`PlannedPayload.Node` finally carries something.** It records the step a
proposal came from — reserved since the beginning, written by the runner,
never set. That is the anchor FU-13 asks for, and it means the run diagram
already built can group its nodes by step without any new record.

**A specification with no steps keeps working.** One step whose envelope is the
whole pack is exactly today's behaviour, which is what lets this land without
republishing anything.

### What it does not settle

Who decides a step is over. The closing condition is written in business
language by the author, and something has to judge it against the run so far.
The candidates are the interpreter asking the model, or a declared condition
over the ledger. That is a separate decision and it blocks stage 4, not this
one.


---

## 9. What the first real interview showed

Run against a connected Anthropic provider, from four prose answers about a
support process. Twelve seconds, and it returned four steps:

| # | Step | Reaches |
|---|---|---|
| 1 | Identificar cliente pelo e-mail do chamado no CRM | `crm.lookup` |
| 2 | Procurar o assunto na base de conhecimento | `kb.search` |
| 3 | Resumir o que foi encontrado | *nothing* |
| 4 | Responder ao cliente após revisão | `crm.reply` |

That is the table §8 arrived at by hand, reached independently from a
description in Portuguese — including the step that reaches nothing. The shape
survived contact twice, by two routes.

Two defects the call exposed, both real.

**The ceiling does not bind.** The call came back with `micros: 0`, because
`PricePerMTok` is never set by anything: the platform counts tokens and refuses
to guess at money, and no screen supplies a price list. So the daily ceiling
somebody configures is decoration — the spend leaves the installation, the
trail records nothing, and nothing decrements. This is a bigger gap than a
per-model dropdown: it is what makes a run's cost a number, what makes an
agent's ceiling mean money, and what lets the read-back carry the estimate
FU-08 shows in its own example.

**The exception does not land.** `stops_when` came back empty on every step
across two prompt attempts, so an author's "if I cannot find the customer, I
say so and stop" is lost. FU-04 is answered per step precisely so a correction
can be anchored where it happened, and losing it there costs stage 4 its
anchor.

Prompt tuning was tried twice against the live endpoint and moved nothing. That
is also the wrong loop — twelve seconds and real money per guess. Two
structural answers are more likely to hold than a better sentence: asking the
exception as its own question per step, or extracting it in a second pass whose
only job is that field.

### Order for the next session

1. **Price list per model.** Turns the ceiling into a ceiling and cost into a
   figure. Needs a settings kind, a store, an endpoint, a screen, and wiring
   into both `Config` paths — the planner's and the completer's.
2. **The exception per step**, structurally rather than by rewording.
3. Stage 3 of the PRD, simulation over real cases, which still waits on D1.


---

## 10. D1 answered: where the cases come from

A file first, the ledger second, shadow capture third, a connector never.

**Shadow capture cannot be the only source, because it is circular.** FU-10
says an agent cannot leave Draft without a reviewed simulation; shadow comes
after Draft. If cases only accumulate in shadow, the first agent can never be
simulated — and the first agent is exactly when simulation matters most, since
it is the first time somebody non-technical publishes something.

**A connector per system is N4's non-goal**, and it breaks something else on
the way. The authoring path deliberately does not touch customer data — it is
the one part of the platform that does not pass through the Gate, and not
touching production is why that is defensible. A connector reading the last
fifty tickets would quietly end that.

**A file works on day one**: for the first agent, in an air-gapped install,
with no integration work, for any system that can export. And it leaves the
author deciding what the platform sees, which is the right default when what it
sees is real customer records.

### The order

| # | Source | Serves |
|---|---|---|
| 1 | An uploaded file (JSONL) | The first agent, which is the hard case |
| 2 | The run ledger | Rewriting an agent that already runs — its own past inputs |
| 3 | Shadow capture | FU-12's regression battery, accumulating on its own |
| 4 | A connector | Somebody's own MCP tool, not a platform feature |

Source 2 is nearly free and was hiding in plain sight: `run_started` already
stores what each run was about, outside the ledger, as a reference and a digest
(`opener.go`). The platform has had real cases since the first webhook.

### Where the cases live

In the content store, not in a table of their own. A case is a real customer
record — an email, a ticket — and the claim check exists for exactly this
shape: bulky or sensitive payloads held outside the ledger as a reference plus
a digest, under retention (AU-04). A new table would be a second place for
personal data to accumulate, with its own retention nobody remembers to set.

---

## 11. Where a case set lives — D3, answered by building it

The question was whether regression cases belong in the ledger or beside the
specification, and the note said they are neither runs nor spec text: fixtures
with expected outcomes.

Building the simulation answered it, and the answer is the ledger's claim
check, filed under the simulation that ran them.

**Why not beside the specification.** A case is a real customer record — a
ticket, a message, an invoice. Putting it next to the definition would put
personal data into the thing an author edits, a reviewer reads and a
publication renders into a version digest. It would also make the case set part
of what a version *is*, so uploading twenty new cases would publish a new
agent.

**Why not a table of its own.** Retention and per-subject erasure already work
per owner on `run_content` (AU-11, NF-09). A second store for the same class of
data is a second retention policy nobody remembers to set, and it would be
holding exactly the data the first one exists to govern.

**Why under the simulation rather than the agent.** Correcting an agent by
example means running the next version against the same occurrences (FU-12).
A set filed under the agent is overwritten by the next upload, which makes the
comparison meaningless — the two runs would be against different sets and
nothing would say so.

The expected outcome half of "fixtures with expected outcomes" is still open,
and it is the smaller half: a case today has an input and a report, not an
assertion. What an author corrects against — "this one should have asked me"
— is FU-13's shape, and it anchors to the step the proposal came from, which
`PlannedPayload.Node` now records.
