---
title: Governed memory
summary: How agents reuse knowledge between runs without turning remembered text into hidden authority.
section: security
tags: memory, assertions, provenance, data labels, retention, erasure
order: 16
---

## Memory is not remembered prose

FuseOne memory stores structured assertions. It does not store a paragraph for
the model to obey later.

A memory assertion names a kind, a subject, a signature and a claim. It also
keeps evidence, observations, confirmation count and source labels. The model
can search those assertions, but it does not choose which company, area or
agent namespace is read. That scope comes from the platform.

That difference matters. A remembered paragraph can become a hidden
instruction. A structured assertion is data with provenance, labels and a
lifetime.

## What agents can ask

An agent uses the platform memory read tool. It can ask for fields such as:

```json
{
  "kind": "incident_signature",
  "subject": "checkout-api",
  "signature": "loki:error:datasource-timeout",
  "limit": 3
}
```

The response is structured. It includes matching observations, confirmation
counts and source state. It does not hand the model an arbitrary content ref,
and it does not let the model reach across a scope boundary.

Shared memory uses the same scope rules as [context sharing between agents](context-sharing.md).
An assertion shared at an area stays inside that area unless the platform
explicitly created a broader-scoped namespace.

## Labels travel with memory

Memory does not make outside data trusted. If an assertion came from Slack,
GitHub, Loki or another external source, its labels travel with the memory read.

That means reading memory can mark the current run as `untrusted`, `pii` or
with an area label. The next write is then evaluated by the Gate as usual. A
memory read followed by a destructive action can be stopped before the action
leaves the worker.

This is mechanical, not a prompt instruction. The model is not asked to treat
memory carefully; the run carries the labels and the Gate reads them.

## Who should write memory

Use memory for stable facts that were reviewed or confirmed by repeated
evidence:

```json
{
  "kind": "incident_signature",
  "subject": "checkout-api",
  "signature": "loki:error:datasource-timeout",
  "claim": "Usually caused by an expired Grafana datasource token",
  "confirmed": 7
}
```

Good memory is narrow and falsifiable. It helps the next run choose a first
query, a likely owner or a known remediation path.

Avoid memory for:

- instructions the model should obey;
- secrets or secret values;
- one-off facts that must be fetched fresh;
- broad opinions such as "this system is unreliable";
- approvals, permissions or decisions that belong in the Gate.

## Evidence can expire or be erased

Memory follows retention and erasure. If the evidence behind an assertion is
erased, the assertion is marked as source-erased and is no longer recalled as
active memory.

That is deliberate. The platform does not keep a useful claim alive after the
record that justified it is gone. The event history remains append-only until
retention removes it, so an auditor can still see why the assertion changed
state.

## What to expect

Memory reduces repeated investigation. It does not guarantee that today's case
matches yesterday's case.

Expect agents to use memory as a hint: start with the known signature, check
the evidence, and then act through the normal Gate path. If a remembered
assertion carries external labels, writes may still require approval or stop.

If your goal is only to avoid opening the same ticket twice, use
[duplicate effect recognition](duplicate-effects.md). Memory is for reusable
knowledge; duplicate recognition is for not repeating the same effect.
