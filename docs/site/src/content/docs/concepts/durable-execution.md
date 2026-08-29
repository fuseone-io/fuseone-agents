---
title: Durable agent execution
description: What survives a failure, what an integration must guarantee, and when a workflow engine belongs beside FuseOne.
---

FuseOne makes an agent run durable without turning the agent into workflow
code. The platform records domain steps, folds them into the current run state
and lets any worker continue from that state.

This is a deliberately bounded promise. It covers the governed run loop. It
does not make every remote effect exactly-once, and it does not turn FuseOne
into a general-purpose workflow engine.

## What survives a failure

- A run's current phase, accumulated labels, budgets, approvals and recorded
  calls are derived from its append-only ledger.
- A worker leases one run for one turn. If the worker disappears, the lease
  expires and another worker can fold the same history and continue.
- Approval and parked states do not depend on a browser, request or worker
  staying alive. A person can decide or resume them later.
- Tool arguments, results and final answers remain outside the ledger behind
  references and digests, so retention can erase their bytes without editing
  the audit chain.

A recovered run continues from the last fact that reached the ledger. FuseOne
does not claim that work which happened outside the ledger can always be
observed after a failure.

## The external-effect boundary

The Gate records its ruling before an allowed tool is invoked. The tool call
also receives a stable idempotency key. These two facts make the decision
auditable and give an integration a way to suppress a repeated effect.

There is still an unavoidable ambiguous interval: a remote system may commit
the effect just before the worker loses the response. FuseOne then knows which
call was attempted, but it cannot infer whether the remote commit happened. It
records the outcome as unknown instead of pretending success or failure.

For write, destructive and financial tools, the integration should provide at
least one of these properties:

1. Honor FuseOne's idempotency key and return the earlier result for a retry.
2. Make the operation naturally idempotent for the same business identity.
3. Provide a compensation that can safely undo a confirmed effect.
4. Require a person to reconcile an outcome that cannot be proven.

The same honesty applies to model cost. If a provider returned a plan but the
worker failed before recording it, a later worker may need to ask again. The
provider can bill both requests even though only one answer reaches the trail.

## FuseOne and a workflow engine answer different questions

| Question | FuseOne | General-purpose durable workflow engine |
|---|---|---|
| What is authored? | A versioned agent definition interpreted by FuseOne's fixed run loop | Application workflow code or a general state machine |
| How is state recovered? | Fold governed domain steps from the run ledger | Replay workflow history through deterministic workflow code |
| How do effects start? | A model proposes a tool call; the Gate decides whether it may happen | Workflow code schedules an activity or equivalent task |
| How do people intervene? | Approval, rejection, parking and resume are product states | Generic signals, updates, forms or application-defined messages |
| What is built in? | Scope, labels, policy, budgets, memory, simulation, trust and audit | Durable orchestration primitives; domain governance is application work |
| Are external effects exactly-once? | No. Tools must be idempotent, compensatable or reconcilable | Not automatically. External activities still need an idempotency strategy |
| Best fit | Governed agent investigations and actions | Long-lived business processes and arbitrary orchestration |

Temporal is a useful concrete comparison, but not a compatibility target.
FuseOne does not expose Temporal Workflows, Activities, task queues, Signals,
Queries, Updates, child workflows or Continue-As-New as its programming model.
It also does not claim Temporal's maturity, scale envelope or operational
history.

## When to use them together

Use FuseOne by itself when the unit of work is an agent run and the important
questions are what the agent may read, what it may change, who must approve,
what context contaminated the decision and what the run cost.

Use a workflow engine beside FuseOne when the surrounding process needs
application-defined orchestration: long-lived timers, many independent
branches, child processes, workflow-level version migration or service-level
guarantees beyond FuseOne's run queue.

Two compositions are reasonable:

- A workflow owns the business process and starts or observes a FuseOne run
  when it needs a governed agent decision.
- FuseOne reaches a workflow API through a governed tool when the approved
  agent action is to start or update that process.

These are architecture patterns, not a built-in Temporal adapter. The owner of
the outer process must still define correlation, retries and idempotency at the
boundary between the two systems.

## What to verify before production

- Restart a worker during planning and during a tool call; inspect the resumed
  trail and any duplicate provider charge.
- Confirm every write integration's idempotency or compensation behavior with
  an intentionally lost response.
- Exercise approval, park and resume across a full deployment restart.
- Test database backup and restore, worker lease expiry and the installation's
  actual concurrency and retention volume.
- Decide whether the business process ends with the agent run. If it does not,
  name the system that owns what comes next.

## Related design note

- [NT-011 — Durable agent execution and workflow engines](../../design/nt-011-durable-agent-execution-and-workflow-engines/)
