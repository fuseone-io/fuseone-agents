# NT-011 — Durable agent execution and workflow engines

FuseOne has a ledger, leases, retries, resumable waits and idempotency keys.
Those words also appear in durable workflow engines, especially Temporal. The
overlap is real, but the products do not expose the same programming model or
make the same operational promise.

This note draws the boundary. Without it, one reader can understate FuseOne as
a stateless agent loop while another assumes that an append-only ledger makes
it a replacement for a general-purpose workflow engine. Both readings are
wrong.

## Decision

FuseOne owns the durable execution of one governed agent run. A fixed engine
interprets a versioned agent definition, records domain steps and folds those
steps to recover the run's state. Gate, labels, approval, budgets, memory,
simulation and cost evidence are part of that interpreter.

FuseOne will not grow a second, general workflow programming model inside that
engine. When a business process needs arbitrary workflow code, general durable
timers, child workflows, broad fan-out, workflow-level version migration or an
operational envelope beyond the run queue, a workflow engine belongs beside
FuseOne rather than hidden inside it.

Temporal is the concrete comparison in this note because it makes the boundary
easy to see. It is not a compatibility target and this note does not claim a
built-in Temporal integration.

## The promise FuseOne makes

The durable unit is a run step, not a stack frame and not model state.

1. `engine.Fold` in `internal/engine/fold.go` derives the current run state from
   the ordered ledger steps. No worker-owned state has to survive between
   turns.
2. `worker.turn` in `internal/worker/turn.go` leases one claimable run, advances
   it once and releases it. The queues in `internal/ledger/queue_memory.go` and
   `internal/ledger/queue_postgres.go` make an expired lease claimable again.
3. Approval, parking, budget reservation, tool-call identity and completion are
   recorded as steps. A restarted process reads those facts rather than asking
   the model to reconstruct them from prose.
4. Gate decisions are appended before an external tool is invoked. An allowed
   effect always has a recorded decision behind it.
5. Arguments, results and closing answers are stored by reference and digest.
   Recovery can retain the audit chain while retention erases the bytes it
   names.

These properties make a run resumable across process and worker failure. They
do not prove that every operation outside the ledger happened exactly once.

## The promise FuseOne does not make

FuseOne does not promise:

- deterministic replay of application-authored workflow code;
- a general API equivalent to Temporal Workflows, Activities, task queues,
  Signals, Queries, Updates, child workflows or Continue-As-New;
- exactly-once commits in a remote API, database or message broker;
- exactly-once billing by a model provider;
- a particular multi-region, throughput or years-long execution envelope
  merely because the run history is durable;
- that projections alone are sufficient for recovery — the ledger and the
  referenced content required by the run remain installation state.

Tests prove the engine's semantic ordering, fold, leases and recovery cases.
They do not turn those local properties into an unstated deployment SLO. An
operator still has to prove backup, restore, capacity and failure behavior for
the installation it runs.

## A different replay model

| Concern | FuseOne | Temporal-style workflow engine |
|---|---|---|
| Authored program | Agent definition interpreted by one platform engine | Deterministic application workflow code |
| History | Governed domain steps such as planned, gate decided, tool called and approval decided | Workflow events consumed by replaying workflow code |
| Recovery | Fold the steps into `engine.State`, then advance one turn | Replay workflow code to reconstruct workflow state, then schedule tasks |
| External work | Tool adapters invoked only after Gate | Activities or equivalent workers scheduled by workflow code |
| Human input | Product states for approval, rejection, parking and resume | Generic messages and application-defined handlers |
| Domain controls | Built-in labels, policies, autonomy, budgets, memory, trust and audit | Built by the application on top of orchestration primitives |
| Versioning boundary | A run is pinned to an agent version; the interpreter is platform code | Workflow code and worker-version compatibility are part of the application model |

Temporal's replay can rebuild arbitrary workflow state because the workflow
code obeys its deterministic model. FuseOne does not replay the model's code or
the model's hidden state. It replays a small vocabulary of platform-owned facts
through a platform-owned fold. That restriction is what lets Gate and taint
labels stay mechanical rather than conventions in each workflow.

The restriction also means FuseOne cannot express every process a workflow
language can. That is a boundary, not missing syntax.

## Failure boundaries

The place a failure occurs decides what can be known on recovery.

| Failure point | What the ledger proves | Recovery behavior | Remaining responsibility |
|---|---|---|---|
| Before a model response is recorded | The prior run state | Plan again | The provider may bill both requests |
| After a Gate decision, before a tool call is recorded | The proposal was allowed; no call is recorded | Re-plan from the recorded decision boundary | No remote effect was requested by the recorded call |
| After `tool_called`, before invocation | The call and idempotency key exist; no result exists | Close it as an unknown outcome and fail closed on the same call | A person may need to decide whether to try a changed action |
| After the remote commit, before `tool_returned` | The same unknown outcome | Do not infer success or failure | The integration must deduplicate, reconcile or compensate |
| After `tool_returned` | The recorded result and effect classification | Fold it as completed; do not repeat a write with the same key | Retain or erase the referenced bytes according to policy |
| While awaiting approval or parked | The pending decision or parking reason | No worker advances it until a person acts | The human queue and operating process must be monitored |

The two middle rows can look identical from the ledger even though the remote
effect differs. No local transaction can atomically commit an arbitrary remote
system and the FuseOne ledger. Calling this exactly-once would hide the most
important operational fact in the design.

## What an integration must guarantee

FuseOne passes a stable idempotency key with a tool call. A governed write
integration should use that fact rather than treating it as decoration.

In descending order of confidence, an integration should:

1. Store the key with the remote operation and return the first result when it
   sees the same key again.
2. Use a natural business key and an idempotent remote operation, while still
   correlating the FuseOne call for audit.
3. Expose a read that can reconcile whether the requested change landed.
4. Provide a compensation for a confirmed effect that must be undone.
5. Require a human decision when none of those can distinguish success from
   failure.

Read calls may poll changing sources only after the ledger proves that the
earlier call completed as a read. Writes reclassified as reads, legacy calls
without an effect and orphaned calls remain blocked. The rule lives in
`executionIdempotencyKey` and `duplicateWithinRun` in
`internal/engine/idempotency.go`.

## Choosing the owner of the process

Use FuseOne as the process owner when:

- the unit of work is one agent investigation or action;
- the important controls are scope, Gate, taint, approval, budget and memory;
- parking and resuming the agent run are the human workflow required; and
- the process is complete when that governed run reaches its final state.

Use a general-purpose workflow engine as the process owner when:

- the process spans many independent services or agent runs;
- timers, branching, fan-out or child processes are application concepts;
- workflow code needs its own compatibility and deployment strategy;
- the process remains alive after an agent run finishes; or
- the required availability and scale envelope is the workflow platform's
  central product promise.

The two can be composed without pretending they are the same layer. A workflow
may start and observe a FuseOne run as one governed step. FuseOne may call a
workflow API through a classified tool when starting or updating the process
is itself the approved effect. In either direction, correlation, timeouts,
retries and idempotency at the boundary must be explicit.

## Consequences

- New durability work should strengthen the fixed run interpreter and its
  proofs, not introduce user-authored workflow code into `engine`.
- Product copy should say "durable agent execution", not simply "workflow
  engine" or "exactly-once execution".
- A new external write shape is incomplete until its ambiguous-outcome
  strategy is documented and tested.
- Recovery tests should include lost model responses, orphaned tool calls,
  expired worker leases, approval across restart and content erasure.
- Requirements that cross the boundary in this note should trigger an explicit
  compose-or-adopt decision rather than a quiet imitation of workflow-engine
  features.

## What proves the current boundary

- `TestFold_*` in `internal/engine/state_test.go` proves state derived from
  ledger steps, including approval and parked/resumed runs.
- `TestAdvance_*` in `internal/engine/runner_test.go` proves Gate ordering,
  idempotency, orphan recovery and the allowed/refused run loop.
- Queue contract tests in `internal/ledger` exercise both the in-memory and
  PostgreSQL lease implementations.
- `TestAdvance_erasableContentStaysOutsideTheLedger` proves that erasable
  argument, result and answer bytes stay behind references.
- `internal/arch/layering_test.go` keeps the fixed interpreter's package
  boundary from reversing toward infrastructure.

Those tests accuse changes to the implementation properties named here. The
comparison with another product's scale and maturity is not a testable property
of this repository, so this note states the limitation instead of implying a
proof.

## Related

- [NT-010](NT-010-the-shape-of-the-platform.md) — topology, the run loop and data placement
- [NT-001](NT-001-integration-boundary-and-execution-model.md) — the integration boundary
- [DP-001](DP-001-data-protection.md) — what is stored and what can be erased
- [Temporal Workflow Executions](https://docs.temporal.io/workflow-execution)
- [Temporal Activities](https://docs.temporal.io/activities)
