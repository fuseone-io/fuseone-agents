# NT-001 · Integration boundary and execution model

**Status** Opinion · **Date** 2026-08-09
**References** [PRD-001](PRD-001-fuseone-agents.md) · internal RFC-001
**Outcome** 4 new requirements + 1 ADR

A response to the two points raised in the review of PRD-001: the role of MCP
against integration patterns, and adopting an actor model with supervision in the
Go executor.

---

## 1. The boundary between MCP and integration patterns

The reading is correct — PRD-001 rules out building an integration engine (N4)
and adopts MCP as the way to expose tools (DE-11). But the phrasing "MCP instead
of EIP" is misleading, because the two do not compete: they solve different
problems, at different layers.

| | Integration patterns (EIP) | MCP |
|---|---|---|
| **Solves** | Moving and transforming data between systems | Exposing a typed action for a model to invoke |
| **Volume** | Batches and continuous flows, thousands of records | One call, one response |
| **Nature** | Deterministic plumbing | Semantic capability |
| **Typical patterns** | Content-based routing, split, aggregate, dead letter, delivery guarantees | A tool with an input and output schema |
| **Who consumes it** | Another system | The agent, inside a step |

### 1.1 The correct phrasing

It is not that we swapped EIP for MCP. It is that the agent platform does not do
integration — it makes decisions and invokes actions.

Synchronising two hundred thousand records between ERP and CRM, with retries,
deduplication and aggregation, is not the work of an agent calling tools. It is
the integration platform's work, which the agent at most triggers as a single
call and then follows.

This is a strength of the design, not a limitation: the two products end up
complementary rather than overlapping. It is also the natural answer to PRD
question Q7 — the agent platform decides, the integration platform transports.

### 1.2 Where the reliability patterns reappear

It is worth separating two families that tend to be treated as one. The
data-transport patterns do not exist in the agent platform. The reliability
patterns do exist, only at another layer — and they are already in the PRD:

| Pattern | Where it lives in PRD-001 |
|---|---|
| Retry and timeout | Executor, per step |
| Idempotency | Gate, check 6 |
| Compensation and rollback | SE-08 |
| Dead letter / parking | Supervision policy (NF-14, proposed below) |
| Degradation under unavailability | NF-11 |

### 1.3 The concrete risk

> **A predictable failure mode**
>
> The MCP server becomes a dumping ground for integration logic. Somebody writes
> a "tool" that joins three systems with pagination, retries and aggregation —
> and now there is hidden integration inside a tool, outside observability,
> outside per-step cost allocation and outside governance. It is the same failure
> the PRD avoids by refusing a second execution engine.

### 1.4 Proposed rule

| Req. | Statement |
|---|---|
| **DE-18** | An MCP tool is thin and idempotent: one operation, a bounded scope, a response within interaction time. Work involving fan-out, aggregation, extensive pagination or long transfers belongs to the integration platform and is invoked as a single tool call. The Curator rejects tools that violate this contract at registration |

---

## 2. Actor model and supervision

The point is well taken and the proposed shape is sound — with one caveat that
has to be explicit before it becomes code, because the corresponding mistake only
shows up in production.

**What holds.** The shape is naturally an actor model. Every run is an isolated
unit with its own state, processing events in sequence, whose failure must not
contaminate its neighbours. In Go this is idiomatic: a goroutine, a channel and a
context already are an actor. It is RFC-001's operating-system metaphor under
another name.

**The caveat.** Actor supervision guarantees in-memory state; the requirement is
durability. An OTP-style supervisor restarts the process from a sound state in
RAM — if the node dies, everything dies with it. NF-02 demands more: an abrupt
crash at any point resumes with no duplicated effect.

**What it gains us.** The model makes explicit an invariant that was missing. The
Ledger requires a single writer per run — see [2.2](#22-what-the-argument-reveals-that-the-prd-did-not-say).

### 2.1 The caveat, in one sentence

The restart strategy cannot be "restart with clean state". It has to be "reload
from the Ledger and continue from the last recorded step".

Put another way: the actor is the form of execution; the Ledger is the recovery
mechanism. Adopting supervision while assuming it delivers durability is exactly
the path to a duplicated effect — and a duplicated effect is, in the PRD, a
severe incident by definition (§13).

### 2.2 What the argument reveals that the PRD did not say

If two routines could write steps of the same run concurrently, the sequence
number would become a race and replay would stop being reliable — which
compromises audit (AU-07), counterfactual replay (AU-08) and the integrity chain
(NF-05) all at once.

One run, one owner, serialised writes. This is a product requirement derived from
the Ledger's design, not an implementation choice, and it is recorded as NF-15.

### 2.3 On adopting an actor framework

Recommendation: adopt the concepts, not the dependency. Actor frameworks in Go
pay for themselves through location transparency in a distributed cluster. The
PRD's target is a single binary, hundreds of agents and thousands of runs per day
(NF-01) — the benefit does not materialise, and the cost is a programming model
that competes with `context` and cancellation, making the code less legible to the
average Go developer.

Should a need to distribute runs across nodes ever appear, the decision is
revisited — and the Ledger is already the natural coordination point for it.

### 2.4 What is a requirement and what is design

The PRD sets out the *what* and the *why*; actors and supervision are the *how*.
Three guarantees, though, are observable and therefore belong in the PRD — the
remaining decisions go to an ADR.

---

## 3. Proposed changes to PRD-001

| Req. | Section | Statement |
|---|---|---|
| **DE-18** | §9.4 Catalogue | An MCP tool is thin and idempotent; fan-out, aggregation and long transfers belong to the integration platform, invoked as a single call |
| **NF-13** | §12 | Isolation between runs. A failure, loop or resource overrun in one run does not affect the others, nor the platform's availability |
| **NF-14** | §12 | Supervision policy. A transient failure resumes from the last recorded step, with progressive backoff. After N attempts the run is parked and the owner notified. Never a silent, indefinite restart |
| **NF-15** | §12 | A single writer per run. Serialised writes, a monotonic sequence, no write concurrency — a precondition for replay and the integrity chain |

### Out of the PRD, for ADR-001

- An actor per run in Go, with no framework — goroutine, channel and context
- The supervision tree and the classification of transient against permanent
  failure
- Restart by reloading the Ledger, including the exact point of resumption
- The parking criterion and the manual resumption path
- The single-writer contract and how it is enforced at the repository boundary

---

**NT-001 · FuseOne Agents · 2026-08-09.** A technical opinion in response to the
review of PRD-001. The four entries in section 3 are proposed changes and remain
pending acceptance before being folded into the main document.
