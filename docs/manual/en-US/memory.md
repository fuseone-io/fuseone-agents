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

## Agent-proposed memory

An agent does not write active memory by default. Memory learning is an
opt-in field on the published agent version, so changing it creates a new
version and is visible in the publication review.

```yaml
memory_learning:
  mode: review
  ttl_days: 30
```

V1 is review mode. The agent can call `$fuseone.memory.suggest` with a
structured assertion: kind, subject, signature and claim. The platform records
that suggestion with the run labels and evidence, but `memory.find` does not
return it yet. A person with publish permission in the scope must accept or
dismiss it from the Memory page.

When memory learning is enabled, the platform also offers
`$fuseone.memory.find`. The agent should use it before suggesting memory for a
case that may already be remembered. The suggestion path also checks active
memory for the same kind, subject and signature, so a remembered fact does not
keep creating review items just because the model proposes different wording.

Review-mode suggestions do not ask for a second approval before entering that
queue. The review queue is the approval point. The suggestion still carries
the run's labels, and an authored policy, missing capability or data-barrier
violation can still stop it.

V2 is auto-confirm mode. The same structured suggestion must be observed in
the configured number of distinct runs before it becomes active memory. The
count is derived by the platform. Repeating the same suggestion inside one run
does not make the memory stronger. The model does not send confidence and does
not choose the company, area or agent namespace.

Auto-confirm suggestions are write effects in the ordinary Gate path. If the
observation came from untrusted data, it is downgraded to review mode for that
suggestion: the run may enqueue it without a second approval, but a person must
accept it before it becomes active memory.

The downgrade follows the accumulated suggestion, not only the latest run. If
one observation came from untrusted data, later clean observations do not wash
that label away; the suggestion stays in human review.

```yaml
memory_learning:
  mode: auto_confirm
  min_observations: 3
  ttl_days: 30
```

Both modes keep suggestions separate from active memory until the promotion
rule is satisfied. A pending suggestion is review material, not remembered
fact. If the source evidence is erased before review, the suggestion is marked
source-erased and cannot be promoted.

Use review mode first for agents that write, approve, disable accounts or
touch production systems. Auto-confirm fits low-risk diagnostic memories where
repetition is itself useful evidence and a wrong hint still has to pass the
normal Gate path before any effect.

## Correcting active memory

Active memory is corrected, not silently edited. The Memory page lets a person
rewrite the claim while keeping the same scope, agent namespace, kind, subject,
signature, evidence, labels and expiry. The correction must include a reason
and is recorded as a new memory event.

If the remembered condition itself was keyed incorrectly, disable that
assertion and record a new one with the right signature. Changing the signature
means changing what future runs search for, so it should be a new assertion.

## Evidence can expire or be erased

Memory follows retention and erasure. If the evidence behind an assertion is
erased, the assertion is marked as source-erased and is no longer recalled as
active memory.

That is deliberate. The platform does not keep a useful claim alive after the
record that justified it is gone. The event history remains append-only until
retention removes it, so an auditor can still see why the assertion changed
state.

## Search and response size

Runtime memory search is indexed for substring matches across subject,
signature and claim. Broad searches still return only a bounded result set.

The memory tool also has a response byte budget. When matching assertions do
not fit, the response says how many were omitted by the budget. The assertions
remain stored; the agent can call a narrower query by kind, subject or
signature if the first response is not specific enough.

Worker metrics report memory read count, latency, returned assertions and
omitted assertions. Those metrics deliberately do not include agent names,
scopes, search text, assertion ids or claims as labels.

## What to expect

Memory reduces repeated investigation. It does not guarantee that today's case
matches yesterday's case.

Expect agents to use memory as a hint: start with the known signature, check
the evidence, and then act through the normal Gate path. If a remembered
assertion carries external labels, writes may still require approval or stop.

If your goal is only to avoid opening the same ticket twice, use
[duplicate effect recognition](duplicate-effects.md). Memory is for reusable
knowledge; duplicate recognition is for not repeating the same effect.
