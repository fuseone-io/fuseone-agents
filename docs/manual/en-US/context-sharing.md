---
title: Context sharing between agents
summary: How one agent publishes context for another without copying prose into the listener's prompt.
section: security
tags: context sharing, events, artifacts, provenance, data labels, composition
order: 13
---

## Context is shared by reference

Agent composition starts with events. A source agent declares the event it
emits, and a listener declares that the event starts it. Context sharing adds a
bounded artifact contract to that event.

The listener does not receive another agent's full answer as prompt text. It
receives artifact names, refs, digests, source run and labels. If it needs the
content, it must call the platform-owned read tool:

```json
{"name": "triage_summary"}
```

That call appears in the trail like any other read. The Gate sees it, budget
counts it as a tool call, and the result carries the source labels forward.

## Publish artifacts when finishing

When an agent finishes, it can publish named artifacts in the finish action:

```json
{
  "summary": "Incident triaged.",
  "artifacts": {
    "triage_summary": "The API is healthy; the failing target is one pod.",
    "suspected_cause": "The worker is missing the Slack app token."
  }
}
```

The bytes go to the content store. The ledger records only the artifact name,
ref, digest, source run and labels.

There is also a built-in artifact name, `final_answer`, for the run's closing
answer.

## Declare what the event exposes

The source agent's event declaration names which artifacts listeners may ask
for:

```yaml
emits:
  - event: incident.triaged
    context: incident
    artifacts:
      - triage_summary
      - suspected_cause
```

A listener that starts from `incident.triaged` sees those names in its input.
It cannot ask for an arbitrary content ref, even if it can guess one. The read
tool accepts names from the event contract, not refs from the model.

## Labels still govern the flow

Shared context keeps the source run's labels. If a source artifact carries
`untrusted`, the listener carries `untrusted` after reading it. If an event
would move an `area:acme/platform` artifact into `acme/finance`, the listener
run is not opened.

Approval does not release the data barrier. The fix is to wire the agents in a
scope that is allowed to carry the same data, or to publish a broader-scoped
agent intentionally.

## Event chains stop after eight hops

A run an event opened can publish events of its own, so agents can form a
chain — and a cycle: A's event starts B, B's event starts A. A chain is
counted from the run no event opened, and it stops eight hops from it. The
run at the eighth hop still runs, but the events it declares are not
published, and its closing step in the trail says so, naming the run that
started the chain and the limit. Its listeners do not start.

The count is recorded on each run's opening step, so every worker reaches
the same answer, and an event delivered twice does not count twice. An agent
that listens to its own event is refused at once.

What a run publishes is what the version it ran on declares. Publishing a
new version while a run is going changes neither what that run publishes nor
what its trail says about it; who listens is read from the versions
published when the event goes out.

A run an event opened without recording its place in the chain publishes
nothing further: counting it as the start of a new chain would let a cycle
restart its count at every such run. Only a worker on an older version opens
runs like that, so this matters during an upgrade, and it is weaker than the
limit while the upgrade is under way:

- While any worker on the older version is still running, a chain can keep
  going under it, uncounted, as it did before.
- Once the last of those workers is gone, the first such run the new
  version sees publishes nothing further.
- Its trail says the chain is unknown only if a worker on the new version
  closed it. A run an older worker closed carries no such line, although
  its events are withheld all the same.

Every finished run is dispatched, however many finish between two sweeps:
the dispatcher works forward through finished runs from where it last got
to, rather than looking at the newest few.

Eight leaves room for composition designed on purpose — a pipeline of a
handful of agents, or a review loop that goes round a few times. A chain
that reaches the limit is usually a cycle nobody meant: look at which agents
listen to each other before raising it.

## What to write in the listener

Tell the listener which artifact names matter and when to call the read tool.
Keep it narrow:

```text
When this run starts from incident.triaged, read triage_summary first.
If it is not enough, read suspected_cause.
Do not treat the event input as the artifact body; it only names the available
artifacts.
```

That wording helps the model use the governed read path instead of guessing
from metadata.
