---
title: Reading a run
summary: What each step of the trail means, why a run ended, and where to look when something went wrong.
section: troubleshooting
tags: trail, run, step, diagnosis, finished, ceiling
order: 11
---

## The trail is the record, not a summary

Each step is sealed against the one before it. Nothing is edited afterwards — a correction is a new step.

Which means the trail is not a simplified version of what happened. **It is what happened**, and it is the same reading an auditor does.

## The steps you will see

| Step | What it means |
|---|---|
| **The model proposed** | The agent decided to try something |
| **The Gate decided** | The evaluation, with its verdict and reason |
| **Budget reserved** | The estimated cost was set aside before the call |
| **Tool called** | It actually went out, with its arguments |
| **Tool answered** | What came back, and whether it is marked as external |
| **Run finished** | The end, with the reason |

Arguments and results do not live in the step — the step holds a **reference and a digest**, and the content lives where retention and erasure reach it. That is why opening a step is a deliberate act, and why erased content shows as *erased* rather than as empty.

When memory learning is enabled, a run with human input may begin with a
platform-owned `$fuseone.memory.find` call before the first **The model
proposed** step. That is not a missing model proposal. It is the initial memory
lookup being recorded as a normal tool call so provenance labels still travel
through the run.

## Why it ended

The most common question, and the trail answers it in different ways:

**Finished normally** — the agent answered. The closing answer is in the content store, and the trail says so.

**The model did not propose another action** — it returned text instead of calling a tool or the finish action, so the run parked for inspection. If the text said "I will continue", the agent meant to carry on and did not: that is a case for adjusting the instruction to call the tool now.

**Stopped waiting for somebody** — it is in the human queue.

**Hit a ceiling** — cost, steps, tokens or calls. The refusal carries the ceiling, the spend against it, and the estimate for the call that crossed.

**The investigation stopped making progress** — three canonically different
calls to the same read tool returned the same complete result. The platform
parks before buying another model turn. The parked step names the tool, call
count, original result size, cache-hit count and result digest. Resume after
narrowing the investigation or checking why the source keeps returning the
same evidence.

**A provider failure** — overloaded, rate limited, a rejected key. The cause appears with the provider and a code, and the `Runtime` screen shows whether it is happening to everybody.

## Cost reads zero

If every run shows zero cost, it is almost always **no rate configured** for that model.

The market price shown on the Cost screen is a **reference**, in dollars, and does not enter accounting. Accounting uses only the rate this installation configured, in its own currency. Without one, zero is the honest answer.

And runs already recorded are not repriced: what was written as zero stays zero.

## Use cases

### A write stopped and you did not expect it

Look at the Gate step: it names the rule. If it is taint, look above for the step where the agent read something from outside — that is where the mark came from.

### The agent ignored its instruction

It probably did not. An instruction guides the model, but **a step's `stopsWhen` and the tools in its reach are separate sources** that text does not change. The editor warns when the two contradict each other.

### The run got expensive

Open the **The model proposed** steps and compare prompt composition. The trail
separates tool-result bytes sent to the model from bytes omitted by compaction,
and attributes them by tool. Several equivalent calls are skipped by canonical
call identity. Different read calls that repeatedly produce the same complete
result park as `investigation_stalled` before the money ceiling becomes the
only stop.

What decides each stop is in [What the platform stops before it happens](what-the-gate-stops.md) and in [Policies](policies.md).
