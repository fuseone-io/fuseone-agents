---
title: Duplicate effect recognition
summary: How the platform skips the same effect across runs without reusing a tool response or bypassing the Gate.
section: operations
tags: dedupe, duplicate, effect, curator, gate, trail
order: 17
---

## Duplicate recognition is not cache

Duplicate effect recognition prevents an agent from doing the same external
effect twice. It is not result cache.

Cache says: the platform already has this tool response, so it can reuse the
response and avoid calling the remote system. Duplicate recognition says: the
effect already happened, so the platform skips the effect instead of doing it
again.

That distinction is visible in the trail. A duplicate step is recorded as a
Gate decision with a duplicate verdict. It does not claim a new tool call, and
it does not reuse a previous response as if the tool had answered again.

## The Gate still sees the proposal

No effect bypasses the Gate. The engine resolves duplicate state before the
Gate, passes that state into the Gate request, and the Gate records the verdict.

That means a run still explains what would have happened. If a contaminated run
proposes the same write another run already completed, the trail can show the
duplicate decision without pretending the write was allowed by policy.

Duplicate is its own verdict. It is not a policy block, and it does not count
as a normal blocked action. If the model keeps proposing the same duplicate,
the run can stop after repeated skips rather than loop forever.

## Configure the semantic key in Curator

Duplicate recognition is enabled on a tool classification. The curator chooses
which argument paths define the effect and how long the recognition window
lasts.

For a GitHub issue creation tool, a semantic key might be:

```text
owner
repo
title
```

That makes `trace_id`, timestamps and other per-run fields irrelevant. The
platform does not fall back to hashing the whole argument body, because that
would make harmless noise defeat recognition.

If a declared path is absent, the call does not get a duplicate fingerprint.
Missing data is an error, not an empty value.

## Scope is not configurable

Company, area, agent and tool id are always part of the duplicate key. The
model and the curator cannot remove them.

This prevents duplicate recognition from becoming a side channel. A call in
one company must not skip a call in another company just because the visible
arguments look the same.

The agent version is intentionally not part of the key. Two versions of the
same agent proposing the same effect are still proposing the same effect in the
external world.

## Pending and confirmed are different

When a run is about to execute a governed duplicate-aware effect, it reserves
the key. Only one run owns the reservation. Other runs that see the reservation
wait briefly and retry.

The key becomes confirmed only after the tool returns successfully. If the tool
fails, the reservation is released. If confirmation itself fails after the
effect happened, the reservation expires later; the platform prefers the risk
of trying again to pretending an unrecorded effect is permanently done.

When a confirmed duplicate is found, the trail points to the source run and
step when that source is known. Older per-run idempotency may have no source;
the duplicate is still real, but the platform does not invent a pointer.

## Choose windows by operation

The window is part of the governance decision:

| Operation | Typical window |
|---|---|
| Create an incident issue | hours or days |
| Send a notification | minutes or hours |
| Rotate a credential | per rotation campaign |
| Disable a principal | long or permanent |
| Restart a workload | short, or disabled |

Do not enable duplicate recognition where repeating is the intended behavior.
If the correct operation is "send a reminder every day", the key must include
the day or the feature should stay disabled.

## Use it with memory, not instead of memory

Duplicate recognition solves repetition of effects. Governed memory solves
reuse of knowledge.

Use duplicate recognition when the question is "did this action already happen
for this subject?" Use [governed memory](memory.md) when the question is "what
did we learn from earlier evidence?"
