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
| D1 | Where the simulation's real cases come from: a connector per system, a file, or capture in shadow mode | Stage 3 | The PRD leaves this open at Q2. Stage 3 is what it calls the central safety mechanism |
| D2 | What a "step" is in the specification: an ordered stage naming reachable tools, or something finer | Stage 4, FU-13 | Also decides what `PlannedPayload.Node` records |
| D3 | Whether regression cases live in the ledger or beside the specification | Stage 4, FU-12 | They are neither runs nor spec text; they are fixtures with expected outcomes |
| D4 | Whether the autonomy stage is a field on the spec or state beside it | Stage 5 | A published version is immutable; promotion is not a new version |

D2 is the one that unblocks the most and is the least reversible. It should be
answered first, and it should be answered by writing one real agent's steps by
hand and seeing whether the shape survives contact.

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
