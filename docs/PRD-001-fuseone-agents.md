# PRD-001 · FuseOne Agents

**Status** Proposal · **Version** 0.2 · **Date** 2026-08-09
**Model** Single installation, on-premise · **Scope** one company → group (phase 2)
**References** internal RFC-001

An AI agent platform installed in the company's own environment. Anyone
describes a process in their own words; the platform builds, simulates and
operates the agent — with a complete trail and a known cost per run.

## Contents

1. [Executive summary](#1-executive-summary)
2. [Context and problem](#2-context-and-problem)
3. [Goals and non-goals](#3-goals-and-non-goals)
4. [Personas](#4-personas)
5. [Product vocabulary](#5-product-vocabulary)
6. [Pillar I — Ease of use](#6-pillar-i--ease-of-use)
7. [Pillar II — Auditability](#7-pillar-ii--auditability)
8. [Pillar III — FinOps](#8-pillar-iii--finops)
9. [Pillar IV — Deployment](#9-pillar-iv--deployment)
10. [Security model](#10-security-model)
11. [Architecture](#11-architecture)
12. [Non-functional requirements](#12-non-functional-requirements)
13. [Success metrics](#13-success-metrics)
14. [Phases](#14-phases)
15. [Risks](#15-risks)
16. [Open questions](#16-open-questions)

---

## 1. Executive summary

**FuseOne Agents** is an AI agent platform delivered as a single installation in
the customer's environment. It exists to remove the three blockers that keep
agents from leaving the pilot stage: **who may create them**, **how to trust
them**, and **what they cost**.

The product rests on four pillars, each with a verifiable target:

| Pillar | Target | What it means |
|---|---|---|
| **Ease of use** | < 1 h | A non-technical author puts an agent into shadow mode without help from the platform team |
| **Auditability** | < 30 s | Reconstruct any decision, under the policy in force at the time |
| **FinOps** | cost per run | Cost per run, per agent and per area, with a ceiling that actually cuts |
| **Deployment** | < 1 day | From a clean installation to the first agent in production |

> **Structuring decision**
>
> Authoring is **conversational**; execution is a **workflow**. The user
> describes the process in an interview; the system generates a deterministic
> graph whose individual steps are agentic. The model decides *how* to carry out
> each step; it never decides *which* steps exist or in what order.

One installation serves **one business group**. It begins by serving a single
company and grows to several companies in the same group, operated by the same
team under the same contract — never as SaaS for unrelated customers. There is
no external control plane and no data travels through FuseOne infrastructure.
The scope model that supports this evolution is in [§3.1](#31-scope-model-and-the-path-to-multi-company).

---

## 2. Context and problem

Companies have already proved that AI agents work in a pilot. Almost none have
managed to put them into continuous operation. Three causes repeat, and none of
them is model capability.

### The authoring bottleneck

Only people who program can turn a process into automation. The queue of
requests piled up in the platform team becomes the ceiling on the whole AI
programme — and whoever knows the process (CX, marketing, finance) is never the
one who builds it. The knowledge passes through a translation, and is lost in
it.

### The trust deficit

When an agent does something unexpected, nobody can reconstruct why in useful
time. The answer takes days, or never comes. After the first scare adoption does
not slow down — it stops, and does not come back. Trust is the most expensive
asset to recover in an internal platform.

### Cost with no owner

The LLM bill arrives as one large number, with no attribution by area or use
case. With no owner, the programme has no advocate in the budget cycle — and is
cut at the first squeeze, whatever value it was producing.

### Why the existing options do not solve it

| Category | Examples | Why it does not solve it |
|---|---|---|
| Agent frameworks | LangGraph, CrewAI, AutoGen | Require a developer. Governance, auditability and cost are left to whoever integrates them — which is to say, they do not exist |
| Flow builders | n8n, ActivePieces, Dify | Require thinking like a programmer (graph, branch, variable). No policy per action and no cost per run |
| Governed agent SaaS | Platforms in the segment | They get the governance-audit-cost triad right, but the data leaves the company and arrives bundled with multi-tenancy nobody asked for |

FuseOne already delivers governed integration inside the customer's cluster.
Agents is the next layer of the same product, with the same delivery model and
the same buyer.

---

## 3. Goals and non-goals

### Goals

| # | Goal | Verification |
|---|---|---|
| **O1** | Authoring by whoever knows the process, not by whoever programs | A CX author creates and simulates an agent without filing a ticket |
| **O2** | Every action reconstructible, under the rule in force at the time | An auditor answers "why did the agent do this" in under 30 s |
| **O3** | Cost known before, visible during, attributed after | The monthly close shows spend by area with no manual spreadsheet |
| **O4** | Installation and operation without a dedicated team | One Helm chart, one binary, one PostgreSQL. No additional infrastructure |
| **O5** | No irreversible effect without deterministic authorisation | Zero destructive actions executed without passing the Gate |

### Non-goals

| # | We will not | Reason |
|---|---|---|
| **N1** | **SaaS multi-tenancy.** Isolation between customers who do not trust each other | It is the largest source of complexity in SaaS platforms and adds nothing for somebody installing in their own cluster. **Not to be confused with multi-company** ([§3.1](#31-scope-model-and-the-path-to-multi-company)), which is on the roadmap and is a different problem |
| **N2** | **Operate as SaaS.** No customer data on our infrastructure | Removes data residency, DPAs and an external control plane from scope |
| **N3** | Compete as an agent framework | The value is in the governed runtime, not in the loop library |
| **N4** | Move data in volume | A tool is an MCP server, local or remote, and an agent is the orchestrator: mapping a field, choosing a route, transforming a record is what it does by reasoning. What it is bad at is being a pipe — half a million rows every night is not an agent's work, and one driven by a language model is expensive, slow and wrong |
| **N5** | Drag-and-drop builder as the **primary** authoring interface | Composing a graph is a technical skill dressed as a friendly UI — it fails precisely with the target audience, so the way in is prose. A builder offered **beside** it, for the author who would rather draw, is not excluded ([§3.2](#32-drawing-as-a-second-way-in)) |
| **N6** | Free-form conversation between agents as the default | Less predictable, more expensive and not auditable, authored by people who cannot evaluate it. Composition is by event, not by chat |
| **N7** | Replace the platform team | The role changes from executor to curator — it defines packs and classifies effects, it does not write agents |

### 3.1 Scope model and the path to multi-company

Many target customers are **groups with more than one company** — several legal
entities, separate budgets, separate audits, one IT team. The first delivery
serves one company; the second serves the group. They are different problems and
the distinction has to be clear from the design onwards.

| Dimension | SaaS multi-tenancy — **out of scope** | Group multi-company — **phase 2** |
|---|---|---|
| Who coexists | Customers who do not trust each other | Companies with the same owner, same IT team |
| Installations | One, shared by everyone | One, the group's |
| Operator | The vendor | The group itself |
| Nature of the separation | **Isolation** — cryptographic, database, network | **Scope** — visibility, allocation, policy, trail |
| Provisioning | Self-service, sign-up | Curator configuration |
| Upgrades | Per tenant, with windows | One, for the whole group |
| Noisy neighbour | Hard requirement | Not a problem — same owner |

> **The decision that has to be made now**
>
> It is not to *build* multi-company in phase 1 — it is to **not lock the scope
> model into a single level**. A hierarchy with a company node that today holds
> one value costs almost nothing; introducing that level later forces migrating
> the Ledger, budgets, policies and roles at the same time, with live runs in the
> middle.

#### Scope hierarchy

| Level | What it holds |
|---|---|
| **Installation** | The group. Global ceiling, MCP server catalogue, default model provider, root policy |
| **Company** | Legal entity. The boundary for cost allocation, audit and policy. **A single value in phase 1** |
| **Area** | CX, marketing, finance. The unit of budget and of authoring |
| **Agent** | Owner, capability pack, its own ceiling |

Policy and budget **inherit downwards and never widen**: an area's ceiling is
bounded by its company's, which is bounded by the installation's. A policy set at
the company can only be tightened by an area, never loosened.

#### What phase 1 must deliver so that phase 2 is cheap

| Req. | Preparation | Cost if done now |
|---|---|---|
| **ES-01** | Every Ledger, budget, specification, policy and role row carries a company key | One column with a single value |
| **ES-02** | Every query filters by scope, even with only one value | One predicate in the repository |
| **ES-03** | Cost allocation aggregates over the hierarchy, not over a flat list of areas | One more level in the projection |
| **ES-04** | Roles are granted on the pair (scope, role), not globally | One field on the grant |
| **ES-05** | Data labels include the company of origin | One more label in the set |

#### What genuinely belongs to phase 2

These are not a new column — they are real work, and that is why they are not in
phase 1:

- **Data barrier between companies.** Company A's data must not reach company B's
  agent, even with the same Curator configuring both. Cheap because it reuses
  labels ([§10.4](#104-data-labels)); non-existent without them.
- **Per-company tool catalogue.** Same kind of tool, different instances and
  credentials — A's CRM is not B's.
- **Segmented trail and export.** Company A's auditor receives A's trail, and
  only that.
- **Cross-entity close and allocation.** Shared installation cost distributed by
  an explicit, auditable rule.
- **Per-company model provider.** Different contracts or residency requirements
  between legal entities.
- **An agent that crosses companies.** Group consolidation. Requires explicit
  authorisation from both ends and a trail in both.

---

### 3.2 Drawing as a second way in

N5 refuses a drag-and-drop builder as the *primary* interface, and that refusal
stands: the way in is describing the process in words, because composing a
graph is a technical skill and the target audience does not have it. What N5
does not refuse is offering a builder to somebody who would rather draw. Both
are ways of saying the same thing, and neither is the record.

Three constraints hold whichever way an author works, and they are the reason
this is a clarification rather than a reversal.

**The versioned artefact is the specification, never the canvas.** Positions,
node identifiers and edge handles are a projection and are not persisted
(FU-18). The layout is re-derived on every read, so the same version draws the
same picture — an approver and an auditor looking at one version two years
apart must see one diagram, and a stored `position` is a second artefact that
can disagree with the text.

**Drawing authors the steps; the prose stays a person's.** The instructions are
what the model receives and what an auditor reads to understand a run (FU-08),
and generating them from fields would produce, by machine, the one part of a
definition that exists to be read by people. So the canvas edits the stages —
which is what the Gate is meant to obey — and what it proposes about the prose
is a draft somebody accepts.

**A proposal is never a grant.** Neither direction may widen an agent: a step
can only narrow the capability pack, and a tool named on the canvas that the
agent does not hold is dropped, exactly as one proposed by the assistant is.

## 4. Personas

| Persona | Who they are | What they need | We know it worked when |
|---|---|---|---|
| **Domain author** | CX, marketing, finance, operations. Does not program | To describe the process the way they would describe it to a new colleague | They create an agent and put it into shadow on their own |
| **Platform curator** | IT / platform team | To define capability packs, classify tool effects, approve a new tool | They stop being a queue and become a catalogue |
| **Approver** | Business area manager | To decide on risky actions, with enough context and without leaving Slack | They approve or deny in under 2 minutes |
| **Auditor** | Compliance, risk, internal audit | To reconstruct any past decision and re-evaluate it against a new policy | They extract the trail themselves, without asking for a database query |
| **Controller** | Finance / FinOps | To know how much each area spent and to forecast next month | They stop asking where the bill comes from |

---

## 5. Product vocabulary

These terms appear in the interface exactly as defined here. Names aimed at
whoever operates the product, not at whoever implements it.

| Term | Internal | Definition |
|---|---|---|
| **Agent** | `agent` | An automated process with an owner, a capability pack and an autonomy stage |
| **Version** | `agent_version` | Every published change creates a new version. Every run is pinned to the version that started it |
| **Run** | `run` | One complete pass of the agent, from trigger to close |
| **Step** | `step` | An atomic, immutable record in the Ledger. Every run is a sequence of steps |
| **Ledger** | `ledger` | Append-only log chained by hash. The single source of truth for state, audit, cost and replay |
| **Tool** | `tool` | An action the agent can invoke, exposed by an MCP server, with its effect classified centrally |
| **Capability pack** | `capability_pack` | A curated set of tools, ceilings and policies. The author picks a pack — never loose tools |
| **Gate** | `gate` | The deterministic sequence every action passes through before becoming an effect |
| **Verdict** | `verdict` | ALLOW · CONSTRAIN · APPROVE · BLOCK |
| **Reservation** | `budget_reservation` | An amount held from the budget before the call, reconciled afterwards against real cost |
| **Case** | `case` | A real historical occurrence used to simulate the agent before switching it on |
| **Correction** | `correction` | An adjustment the author makes on a simulated case. Becomes a permanent regression test |
| **Stage** | `autonomy_stage` | Draft → Shadow → Copilot → Autonomous |
| **Company** | `company` | A legal entity within the group. The boundary for allocation, audit and policy. A single value in phase 1 ([§3.1](#31-scope-model-and-the-path-to-multi-company)) |
| **Area** | `area` | An organisational unit inside a company — CX, marketing, finance. The scope of authoring and budget. Not isolation |

---

## 6. Pillar I — Ease of use

The premise: process knowledge is irreplaceable and only the domain author has
it. What the platform removes is not the knowledge — it is the **notation**. So
the system does not need to be a good code generator; it needs to be a **good
interviewer**.

| Stage | What happens |
|---|---|
| **1 · Interview** | Questions a CX analyst answers without effort |
| **2 · Read-back** | The system tells back, in plain language, what it understood |
| **3 · Simulation** | Runs against 50 real cases that already happened |
| **4 · Correction** | The author fixes the wrong cases, one by one |
| **5 · Shadow** | Runs without acting, compared against the human |

### 6.1 Structured interview

It is not a free-form prompt. It is a guided conversation that fills a fixed
schema. Every question is answerable by whoever performs the process, and every
answer populates part of the specification.

| Req. | Question to the author | What it fills |
|---|---|---|
| **FU-01** | When does this start? | Trigger |
| **FU-02** | What do you need to know in order to do it? | Read tools |
| **FU-03** | What are the steps? | Process graph |
| **FU-04** | What usually goes wrong? And then what do you do? | Exception handling |
| **FU-05** | What would you not decide on your own? | Human approval points |
| **FU-06** | How do you know you are done? | Closing criterion |
| **FU-07** | **What must never happen?** | Blocking policy |

> **Why FU-07 matters**
>
> A marketing person cannot state a security policy, but answers *"never send to
> the whole base without somebody reviewing it"* without hesitating. That is a
> guardrail expressed in business language — the only form in which this audience
> can produce one.

### 6.2 Narrative read-back

**FU-08.** Before any publication, the system presents what it understood as
running prose. The author approves the narrative — never YAML, JSON or a graph.

```
Every time an email arrives at support@, I will read the email,
look the customer up in the CRM and classify the subject.

If it is about billing, I open a ticket for finance.
I never reply to the customer without your approval.
If I cannot find the customer, I tell you and stop.

Estimated cost: R$ 0.31 per email · ~R$ 124/month at your current volume
```

### 6.3 Simulation over real cases

> **As built.** A simulation opens one run per case, marked, and a second
> worker pool drains them with a tool layer that answers with nothing. The runs
> are the queue: the lease, the backoff, the parking and the step ceiling are
> the ones every run gets, and a simulated run is a run in every respect except
> the call. The report is a fold of those runs rather than a record kept beside
> them.

**FU-09.** Before being switched on, the agent runs dry against the last N real
occurrences — emails, tickets, leads. The screen shows, per case: what it would
have done, where it would have asked for approval, where it was unsure, and what
it would have cost.

> **This is the central safety mechanism**
>
> A human description of a process is **always incomplete** — people describe the
> happy path and omit the exception. Simulation is what exposes that gap before
> production, and it is the only validation legible to somebody who cannot read a
> specification. Without it, non-technical authoring is reckless.

> **As built, with a gap named.** Leaving Draft requires a simulation to exist.
> Whether anybody *read* it is the half this platform cannot observe: it can put
> the report in front of a person and record that somebody asked for it, and it
> cannot know they thought about it. The check says what it checks.

**FU-10.** An agent cannot leave Draft without at least one simulation run and
reviewed.

### 6.4 Correction by example

**FU-11.** The author opens a case that came out wrong and says what should have
happened. They do not rewrite the specification — they **correct an example**,
which is how they would train a new colleague.

**FU-12.** Every correction is recorded as a **regression case**. Every future
change to the agent re-runs the full battery and shows what broke. The author
builds a test suite without knowing that tests exist.

**FU-13.** Corrections are anchored to a *step* of the graph, not to the whole
agent. That is what makes it possible to localise and fix without degrading the
rest — and it is the technical reason execution is a graph and not a free loop.

### 6.5 Progressive autonomy

No agent is born autonomous. Promotion is driven by measured evidence, not by a
calendar decision.

| Stage | The agent | The human | Promotion criterion |
|---|---|---|---|
| **Draft** | Only simulates | Reviews cases | 1 reviewed simulation |
| **Shadow** | Proposes, does not act | Does the work; the system compares | Agreement ≥ threshold over N cases |
| **Copilot** | Proposes each action | Approves with one click | Approval rate ≥ threshold |
| **Autonomous** | Acts inside the envelope | Handles exceptions only | — |

> **As built.** The stage is state beside the specification, not a field in it:
> a published version is immutable, every run is pinned to one, and promotion
> is not a new version. Draft may be simulated and may not act, refused at the
> opener because every route in goes through there. Copilot escalates every
> effect at the Gate, including one a written exception allows — otherwise the
> stage would mean nothing on exactly the agents somebody wrote an exception
> for.

**FU-14.** The platform measures the agreement rate and *suggests* promotion:
*"This agent agreed with you on 94% of the last 100 cases. Promote to copilot?"*
The decision is always human.

**FU-15.** Automatic demotion: if the rejection rate in Copilot crosses the
threshold, the agent returns to Shadow and the owner is notified. This covers
process drift — the process changes, the agent does not, and nobody notices.

### 6.6 Domain templates

**FU-16.** A catalogue of pre-configured agents for recurring cases (ticket
triage, lead qualification, reconciliation, alert response). The author starts
from a template and adjusts it through the interview, instead of starting from
nothing.

### 6.7 Generated diagram

**FU-17.** The platform renders the agent's graph as a **read-only** diagram with
deterministic automatic layout. The same specification always produces the same
drawing — an audit requirement, since this diagram appears on an approval screen
and in the historical record.

**FU-18.** The source of truth is the versioned specification. The rendering
library's data model is never persisted.

---

## 7. Pillar II — Auditability

The product treats auditability as an **adoption** feature, not a compliance one.
When an agent surprises somebody, the answer to "why" has to arrive in seconds —
otherwise trust evaporates and does not return.

### 7.1 The Ledger

**AU-01.** Every step is written to an append-only log chained by hash:
`hash(step) = H(previous_hash ‖ canonical_content)`. There is no update and no
delete of a step.

**AU-02.** The Ledger is the **single source of truth**. Run state, the audit
trail, the cost record, the basis for replay and the regression set are all
*projections* of it.

> **Design consequence**
>
> A second execution log — from an external orchestrator, for example — would
> create two sources of truth with different retentions and formats, which
> diverge in production. The Ledger is primary; run state is derived from it by
> sequential replay.

**AU-03.** Every step carries at minimum: **scope (company and area)**, agent and
version identifier, the delegating human identity, tool and arguments, verdict
and `policy_hash`, data labels, cost, idempotency key and timestamp.

**AU-04.** Bulky content (transcripts, attachments) goes to the customer's own
object storage; the step keeps a reference and a digest.

### 7.2 Identity and delegation

**AU-05.** The agent is a principal in its own right, distinct from the user who
triggered it. The trail always records the pair: *agent X acted on behalf of Y*.

**AU-06.** Effective scope is the **intersection** of the agent's capability pack
and the delegator's permissions. An agent never widens the reach of whoever
triggered it.

### 7.3 Replay

**AU-07.** *Faithful replay.* Reconstruct a run exactly as it happened, with the
specification and policy versions in force at that moment.

**AU-08.** *Counterfactual replay.* Re-evaluate past runs against a new policy,
answering "what would have changed?". A requirement for revising policy without
discovering the impact in production.

> **Derived requirement**
>
> AU-08 is only possible because the Ledger records `policy_version`, the inputs
> and the verdict of each decision separately. A log that records only the
> outcome of an evaluation allows replaying it, but not re-evaluating it.
>
> This was written as a description and was not true of the implementation for
> some time: a decision recorded its tool, its effect and its outcome, so a
> rule about untrusted data re-evaluated against no data and reported that it
> would change nothing. The taint and a digest of the arguments are now
> recorded beside the verdict.
>
> The arguments themselves are deliberately not kept. They carry whatever the
> case carried, and making a second copy of personal data to enable a reporting
> feature is the wrong trade beside AU-11. What that costs is exact: a rule
> reading argument content cannot be checked against a past decision, and the
> report says so rather than counting it as unchanged — "nothing would change"
> about a question nobody asked is the one way this feature can do harm.

### 7.4 A legible trail

**AU-09.** A run's timeline is presented in business language, with the technical
detail one click away. The approver and the auditor read the same screen, at
different depths.

**AU-10.** Every blocked or constrained action appears in the trail with the rule
that caused it, named — never "denied by policy".

### 7.5 Retention and export

**AU-11.** Retention configurable per installation, defaulting to 5 years. No
automatic purge below what is configured.

> **As built.** The window has a floor of one day: this is the one setting
> where a typo destroys data on the next sweep and cannot be undone. Erasure
> for a subject takes the runs the operator names, because nothing here indexes
> content by the person it concerns — an index of who appears in what would be
> the very record a subject is asking to be rid of. Finding the runs is done in
> the trail; performing it is recorded there as the single request it was.

**AU-12.** Signed export of Ledger ranges, independently verifiable through the
hash chain.

> **As built.** `agentd verify <file>` checks the chain and the signature and
> needs no database, no credential and no network — an export somebody has to
> ask us about is an export they are trusting us for. What it cannot tell them
> is whether the key is ours, so it prints the fingerprint and says so. The
> export format is written down separately from the internal types, because an
> auditor holds a copy for five years and a rename in the codebase must not
> change what they are reading.

**AU-13.** LLM observability (traces, latency) is a separate system with its own
retention, and does not replace the Ledger.

---

## 8. Pillar III — FinOps

Cost is not a report bolted on at the end — it is a first-class field on every
step. Every view below is an aggregation of the same data written to the Ledger.

### 8.1 Reserve before, reconcile after

**FO-01.** Before every model or tool call, the Gate **reserves** the estimated
maximum cost from the available budget. After the call it reconciles the real
cost against the reservation and returns the difference.

> **Why not accumulate**
>
> Adding up cost after the call opens a window between spending and accounting.
> With parallel steps, the ceiling is crossed before any check notices — and the
> automatic cut-off becomes decorative. A pessimistic reservation is the same
> pattern used in payment authorisation, and for the same reason.

### 8.2 Ceilings and cut-off

**FO-02.** Ceilings configurable along the scope hierarchy
([§3.1](#31-scope-model-and-the-path-to-multi-company)), each level bounded by
the one above: **installation → company → area → capability pack → agent**. The
company level exists from phase 1 with a single value, so that phase 2 does not
require migrating budgets.

**FO-03.** Beyond an amount, a budget covers a number of steps, a number of tool
calls and wall-clock time. All four limits are checked at the Gate.

**FO-04.** On reaching a ceiling, the run is **paused** and remains resumable —
never terminated. Raising the budget resumes from the exact point, without
repeating an effect that already happened.

**FO-05.** Progressive alerts per area at 50%, 80% and 100% of the monthly
budget.

**FO-06.** A global switch per agent, per pack and per installation, operable
without a deploy.

### 8.3 Views

| Moment | View | Source |
|---|---|---|
| **Before** | Estimate shown during the interview, in currency and plain language | Historical p50/p95 per step type × graph shape × declared volume |
| **Before** | Real cost of the simulation over the N cases | Dry-run execution, measured |
| **During** | Current consumption of the run and of the month | Sum over open steps |
| **After** | Allocation per run, agent, area and month | Aggregated projection of the Ledger |
| **Weekly** | Per-area summary delivered to the team's channel | The same projection, scheduled |

**FO-07.** The **run** is the primary accounting unit. Agent, area and period are
dimensions derived from it. Defined at product level so that totals always
reconcile.

**FO-08.** A step's cost breaks down input, output, cache reads and tool calls
separately — without that there is no way to diagnose an expensive agent.

### 8.4 Built-in efficiency

The product does not pass waste on to the customer. Three levers are the
platform's responsibility, not the author's:

| Req. | Lever | Implementation |
|---|---|---|
| **FO-09** | Prompt caching | A stable assembly order free of volatile content; tools serialised canonically. A cache read costs a fraction of the input price, and every step of a recurring agent re-sends the same prefix thousands of times a month |
| **FO-10** | Effort per step | Reasoning level configured per step type, before considering a change of model |
| **FO-11** | Model tiering | A strong model for the interview and for decision steps; an economical model for classification and high-volume reading |

---

## 9. Pillar IV — Deployment

### 9.1 Installation

**DE-01.** One Helm chart. One binary. One PostgreSQL. No additional mandatory
infrastructure component — no orchestration cluster, no external queue, no
time-series database.

**DE-02.** From a clean installation to the first agent in shadow mode: **under
one day**, including integration with corporate identity.

**DE-03.** Object storage is optional; without it the system degrades gracefully
by lowering the inline payload limit.

> **Design constraint**
>
> Every mandatory infrastructure dependency is a permanent operating cost for the
> customer and an obstacle at every installation. The "binary + PostgreSQL"
> constraint is a product decision and takes precedence over implementation
> convenience.

### 9.2 Corporate identity

**DE-04.** Authentication exclusively through the customer's identity provider
over OIDC. The platform keeps no password store.

**DE-05.** Provider groups map to the triple **(company, area, role)**. Four
roles: *Author*, *Approver*, *Curator*, *Auditor*. The same person may hold
different roles in different companies of the group.

**DE-06.** Tool credentials live in a vault, injected at the edge of the call.
The agent and the model never receive them in context.

### 9.3 Agent lifecycle

**DE-07.** Publishing an agent is an interface action, not a deploy. No pipeline,
no ticket, no change window.

**DE-08.** Every publication creates an immutable version, with author, date and a
legible difference against the previous one. Rolling back is selecting an earlier
version.

**DE-09.** Runs in flight continue on the version they started with. Publishing
never alters a live run.

**DE-10.** The version history is exportable as text for external review, without
the author needing to know version control.

### 9.4 Tool catalogue

**DE-11.** Tools arrive as MCP servers registered by the Curator — an open
protocol, no proprietary connector format.

**DE-12.** On registration, the Curator classifies each tool by effect: READ ·
WRITE · DESTRUCTIVE · FINANCIAL. Classification is central and singular — never
defined by the agent's author.

**DE-13.** A new tool arrives as READ by default and requires explicit
reclassification to allow writing.

**DE-14.** A tool from an untrusted third party can be marked as an untrusted
data source, propagating a label to everything derived from it.

### 9.5 Upgrades and continuity

**DE-15.** A platform upgrade does not interrupt runs in flight; they resume from
the last recorded step.

**DE-16.** Resumption after an abrupt crash never repeats an effect that already
happened — guaranteed by an idempotency key, not by attempted detection.

**DE-17.** Backup and restore are PostgreSQL operations plus object storage. No
proprietary procedure.

---

## 10. Security model

One rule, from which everything follows: **model output is a proposal, never an
effect.** Conversation is free and cheap; action passes through the Gate.

### 10.1 The Gate

**SE-01.** Every action passes the same checks, always in the same order. The
order is normative: the cheap and absolute come before the expensive and
contextual, and the earliest objection is the one reported — it is the failure
the operator actually has to fix.

```
capability → contract → data label → policy → reservation → idempotency
           → autonomy → approval
```

> Autonomy was added when the stages landed (FU-14), and it runs late on
> purpose. Placed early it reported "the agent is in Copilot" for calls a taint
> rule or a policy was already stopping for a specific reason, and the specific
> reason is the one somebody can act on. When nothing else objected, the stage
> is the explanation.

| # | Check | What it does |
|---|---|---|
| 1 | Capability | Is the tool in the agent's pack? The set is fixed at the start of the run and can only shrink |
| 2 | Contract | Arguments validated against a schema. A failure returns a structured error to the model, not an exception |
| 3 | Data label | Do the arguments derive from untrusted or sensitive data? See [10.4](#104-data-labels) |
| 4 | Policy | Deterministic, versioned evaluation. Produces one of the four verdicts |
| 5 | Reservation | Is there budget? Reserves the estimated maximum before spending |
| 6 | Idempotency | A key derived from the run, sequence, tool and arguments. A repeat returns the previous result |
| 7 | Autonomy | Is this agent trusted to act alone? A Copilot escalates every effect, including one a written exception allows |
| 8 | Approval | If required, suspends durably until a human decision or expiry |

### 10.2 Four verdicts

| Verdict | Effect | Example |
|---|---|---|
| **ALLOW** | Executes | Look a ticket up |
| **CONSTRAIN** | Executes with modified arguments | Send the campaign, but limited to 200 recipients |
| **APPROVE** | Suspends until a human decides | Reply to the customer |
| **BLOCK** | Denies and records | Issue a refund |

**SE-02.** *Constrain* is often more useful than deny — it preserves the value of
the automation while containing the risk. Concept adopted from RFC-001.

### 10.3 Capability packs

**SE-03.** The domain author **does not choose tools** — they choose a curated
pack. They never see the list of available tools.

**SE-04.** Deny by default: what is not in the pack is impossible to invoke,
whatever the specification asks for.

> **Division of responsibility**
>
> A non-technical author cannot be responsible for guardrails, because they have
> no way of knowing what is dangerous. The Curator designs the envelope; the
> author operates inside it and answers for the business outcome. That separation
> is what makes open authoring safe.

### 10.4 Data labels

**SE-05.** All incoming data receives labels (*untrusted*, *personal*,
*confidential*, origin). Labels propagate by the union of the steps that produced
each value.

**SE-06.** The Gate rejects a high-effect action whose arguments derive from
untrusted data, except with explicit human approval.

**SE-07.** Data-flow rules are checkable against the specification before
publication — "does personal data reach an external sending tool?" is answered
without running anything.

> **Why this is not optional**
>
> Without label propagation, malicious content read at step 2 becomes the premise
> of the action executed at step 6, and the other six checks are circumvented by
> text that came from outside. This is the vector specific to agentic systems and
> it has no mitigation by prompt.

### 10.5 Compensation and termination

**SE-08.** Every write tool declares its compensation. A failure in the middle of
a sequence triggers a compensation recorded step by step.

**SE-09.** Termination is structural, never instructed by prompt: a step ceiling,
a ceiling per pair of interlocutors, a maximum depth, quiescence and an explicit
closing criterion.

**SE-10.** Composition between agents is by **event**: one agent publishes a typed
event, another consumes it as a trigger. No free conversation, no direct call.
The graph of who triggers whom is static and inspectable.

---

## 11. Architecture

Seven components, one process, one database. The scope is deliberately smaller
than that of the FuseOne integration platform — the complexity here is in the
semantics, not in the topology.

| Component | Responsibility |
|---|---|
| **Studio** | Interview, read-back, simulation, correction, generated diagram |
| **Console** | Runs, approval inbox, cost per area, audit trail |
| **Registry** | Versioned specifications, capability packs, policies |
| **Executor** | Durable interpreter of the fixed loop: plan → gate → execute → record |
| **Ledger** | Append-only, hash-chained. The single source of state, audit, cost, replay and regression |
| **Gate** | The seven checks and the four verdicts |
| **Tools** | MCP clients, effect classification, credential injection from the vault |

### Technical choices

| Layer | Choice | Reason |
|---|---|---|
| Core | Go, single binary | Interface embedded via `embed`; trivial distribution; adequate concurrency |
| State | PostgreSQL | Ledger, specifications, cost. Vectors added only when there is long-term memory |
| Interface | React + Vite + shadcn/ui + Tailwind | FuseOne's existing design system, producing static assets embeddable in the binary |
| Diagram | React Flow + automatic layout (ELK) | The graph is generated, not drawn. Layout must be deterministic across renders |
| Tools | MCP | An open, adopted protocol. No proprietary connector format |
| Policy | Embedded engine, versioned rules | Deterministic evaluation, testable in CI, with a hash recorded per decision |
| Identity | The customer's OIDC | Agent-on-behalf-of-user delegation by token exchange |

### On an external orchestrator

Evaluated and not adopted at this stage. The agent loop is **fixed** and the
specification is **declarative** — the most expensive problem a durable
orchestrator solves, versioning code that is executing, here reduces to
versioning data. What remains (durable timers, concurrency, resumption) is
tractable over a known loop, while adopting an additional cluster would violate
DE-01 at every installation.

The boundary between deterministic logic and non-deterministic effect is kept
explicit in the code, so that the migration stays mechanical should any of the
triggers appear: runs lasting weeks, scale far beyond what is foreseen, or
long-running business process orchestration *above* the agents.

---

## 12. Non-functional requirements

| Req. | Dimension | Target |
|---|---|---|
| **NF-01** | Scale per installation | Hundreds of active agents; thousands of runs per day; dozens of concurrent runs |
| **NF-02** | Survivability | An abrupt crash at any point resumes with no duplicated effect and no manual intervention |
| **NF-03** | Interface latency | A run timeline loads in under 1 s for runs of up to 200 steps |
| **NF-04** | Human wait | A pending approval survives restarts and upgrades, for up to 30 days |
| **NF-05** | Trail integrity | The hash chain is verifiable end to end; a violation is detectable and alarmable |
| **NF-06** | Visibility scope | An author in one area does not read another area's runs, cost or trail. Every query is filtered by scope from phase 1, even with a single company (ES-02) |
| **NF-12** | Boundary between companies — **phase 2** | Data labelled as originating in company A does not reach company B's agent without explicit authorisation recorded in both trails |
| **NF-07** | Data residency | No business data leaves the customer's perimeter, except to the configured model provider |
| **NF-08** | Local model | Support for a compatible self-hosted provider, for installations that cannot use an external model |
| **NF-09** | Personal data | A personal-data label, with per-subject erasure reaching the referenced content while preserving the hash chain |
| **NF-10** | Languages | Interface in pt-BR and en-US, kept in parity |
| **NF-11** | Degradation | Model provider unavailability pauses runs resumably; it loses no state and duplicates no effect |

---

## 13. Success metrics

Adoption is the product's metric. Technical capability without use is failure,
and most internal AI platforms die from lack of adoption, not from lack of
features.

| Metric | Measures | Target (6 months after installation) |
|---|---|---|
| Time to first agent | Entry friction | < 1 h per new author |
| Autonomous authoring | Did the platform team become a bottleneck? | > 70% of agents created without Curator intervention |
| Active areas | Reach beyond the pilot team | ≥ 4 distinct areas |
| Promotion rate | Are agents earning trust? | > 50% of agents in Shadow reach Copilot |
| Time to answer "why" | Does auditability work in practice? | < 30 s, measured in a simulated audit |
| Duplicated effects | Correctness of the core | Zero. Any occurrence is a severe incident |
| Attributed cost | Does finance have visibility? | 100% of spend with an identified area |
| Ceiling overruns | Does the control work? | Zero spend above the configured ceiling |
| Shadow agents | Is the platform the easiest path? | No known case of AI automation outside the platform |

---

## 14. Phases

| # | Phase | Scope | Completion criterion |
|---|---|---|---|
| **F0** | **Core** | Ledger, Executor, Gate, MCP tools, cost per run, **scope hierarchy with a company level** (ES-01…ES-05). No authoring interface | One real agent in production; resumption after a crash mid-call, with no duplicated effect |
| **F1** | **Simulation** | Import of historical cases, dry-run, correction by example, regression battery | An agent is corrected purely by fixing examples, without editing the specification |
| **F2** | **Operation** | Console, approval in a team channel, OIDC, areas and roles, legible trail | A business approver decides without opening the platform |
| **F3** | **FinOps** | Reservation and reconciliation, ceilings at four levels, alerts, per-area allocation, weekly summary | The controller stops asking where the bill comes from |
| **F4** | **Authoring** | Interview, narrative read-back, domain templates, generated diagram | A CX person creates an agent with nobody sitting beside them |
| **F5** | **Trust** | Autonomy stages, promotion by evidence, automatic demotion, counterfactual replay | The first agent promoted to Autonomous on measured data |
| **F6** | **Scale** | Data labels, static flow checking, composition by event, long-term memory | A publication is blocked because personal data would reach an external tool |
| **F7** | **Group** | Multi-company ([§3.1](#31-scope-model-and-the-path-to-multi-company)): a second active company, per-company tool catalogue, data barrier between companies, segmented trail and allocation, per-company model provider | Two legal entities operate on the same installation; one's auditor cannot see the other, and the cost close comes out separated with no spreadsheet |

> **On the order**
>
> Simulation (F1) comes **before** interview-based authoring (F4), even though
> the interview demos better. Without a simulator, the interview produces agents
> nobody can validate, and the first production mistake costs the trust of the
> whole organisation — the most expensive input to recover. F0, meanwhile,
> delivers real value to the platform team itself before any interface exists.
>
> F7 depends on F6: the data barrier between companies is an application of
> labels, not a new mechanism. Attempting multi-company before labels means
> separating companies by query filter alone — which protects the *view*, but does
> not stop company B's agent from using company A's data that passed through the
> context.

---

## 15. Risks

| Risk | Impact | Mitigation |
|---|---|---|
| **Nobody uses it** — it remains easier to ask the IT team | Fatal | A paved path; domain templates; under 1 h to the first agent. The platform has to be the easiest way to obtain a credential safely |
| **A process with no history** — there are no cases to simulate | High | The existence of historical truth is a selection criterion for the first use case. Without it, non-technical authoring does not hold up |
| **An agent breaks production** | High and permanent | Read by default; mandatory simulation; shadow before copilot; the Gate before every effect |
| **Knowledge too tacit** — the interview produces something plausible and wrong | Medium | Simulation reveals it before production. This is why F1 precedes F4 |
| **Process drift** — the process changes, the agent does not | Medium | A mandatory owner per agent; automatic demotion by rejection rate; periodic review |
| **Model cost grows faster than value** | Medium | Caching, effort per step and model tiering as the platform's responsibility. The simulator exposes expensive agents before they are switched on |
| **Scope creep in the core** — building the perfect executor before the first user | Medium | F0 has a deadline and one real user: the platform team itself |

---

## 16. Open questions

| # | Question | Must be resolved by |
|---|---|---|
| **Q1** | What is the first use case, and does it have historical cases accessible in enough volume to simulate? | Before F0 |
| **Q2** | How are historical cases imported? A connector per system, a file, or capture in shadow mode? | F1 |
| ~~**Q3**~~ | ~~Per-subject erasure can invalidate the hash chain~~ — **answered: both, and by construction.** Personal data was never in the chain: bulky content is segregated into the claim check (AU-04) and the step keeps a reference and a digest, so erasing content never touches a step. Erasure leaves a tombstone rather than a deleted row, because erased and never-stored are different facts and a trail pointing at nothing has to say which | — |
| **Q4** | Policy language: do declarative rules in the pack solve the foreseen cases, or is a full rules engine needed from the start? | F2 |
| ~~**Q5**~~ | ~~The agreement threshold for promotion~~ — **answered, and the answer is asymmetric.** Twenty decisions at 95% suggests promotion; five at under 80% performs demotion. Global rather than per pack, because the number describes whether people agree with the agent and not with its tools. Promotion is only ever suggested and demotion is automatic: loosening on thin evidence risks harm, tightening on thin evidence costs somebody a few clicks | — |
| **Q6** | Licensing model per installation: per active agent, per run, or per seat? | Before the first external customer |
| **Q7** | Relationship with the FuseOne integration platform: a sibling product installable separately, or a module of the same chart? | F2 |
| **Q8** | A catalogue of approved MCP servers: do we curate our own list, or accept any server with manual classification? | F0 |
| **Q9** | In a group, who operates the installation — the holding company or each company? This decides whether the Curator is a single group-wide role or whether there is a Curator per company with a level above | F0 — affects the role model |
| **Q10** | Does any company in the group have a regulatory requirement forcing a separate database or cluster? If so, that case stops being scope and becomes a separate installation — a product decision, not an architectural one | Before F7 |
| **Q11** | Group licensing: per installation, per active company, or per consolidated volume? Interacts directly with Q6 | Before F7 |

---

**PRD-001 · FuseOne Agents · v0.2 · 2026-08-09.** A proposal document, open to
revision. Product scope: a single installation in the customer's environment,
serving one company in the first delivery and the business group in the second
([§3.1](#31-scope-model-and-the-path-to-multi-company)) — at no point as
multi-tenant SaaS. It incorporates concepts from the internal RFC-001 — market
expectations, the *constrain* verdict, an agent identity distinct from the user,
a ceiling with automatic cut-off, replay under the rules in force at the time,
and a reusable tool catalogue — repositioned for a single-tenant product
installed in the customer's environment.
