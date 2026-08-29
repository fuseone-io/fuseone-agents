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

## Teaching from a run

Every memory comes from a run. There are two routes and both are governed.

**"Remember this", in the run's trail.** The button appears on the closing step
of a run that produced something citable, and only for whoever can publish. It
opens a sheet where the platform has already answered everything it can: the
scope and the run come from the record being read, the citation from the step,
the labels from the trail, and the agent is read from the ledger when the
request arrives.

**The Memory page.** The same form, with one difference: there a person chooses
a finished run from the scopes they can access, because no run is on screen.

Either way, the authored content is only a short subject and claim. The subject
is also the stable human key: the platform records new teaching with kind
`fact` and derives its signature from the trimmed subject. This is deterministic
on purpose. Spending a model turn to invent internal vocabulary would add cost
and could give the same fact a different identity on a later run.

Governance choices remain explicit. Shared memory is never reached by leaving
an agent blank, and every write still needs a reason:

| Asked for | Derived by the platform |
|---|---|
| subject and claim | kind `fact` and signature from the subject |
| who reads it (agent or shared) | the agent, from the cited run |
| the reason | labels, folded from the trail up to the cited step |
| nothing else | the citation's digest and step, opening counters and 30-day expiry |

The derived identity is shown before saving and cannot be edited. Existing
memory with an older kind or signature keeps that identity when its claim is
corrected; silently deriving a new one there would create a second fact instead
of correcting the first. The subject therefore names the memory: two different
facts about one service need two specific subjects, not two claims under one
broad subject.

### The evidence is read, not typed

The sheet shows the run, the step, the artifact and the digest, and lets none of
them be changed. This is not a convenience restriction: every part is the
ledger's answer, and a field somebody can change is one they can change to
something the run never produced — the server refuses that, so an editable box
there would only ever lead to a refusal.

When the run published more than one citable output, the person picks among the
**names the ledger recorded**. Never types one.

### The vocabulary of artifacts

A citation names a run and one of its outputs:

- **`final_answer`** — the closing answer. The default and the common case. No
  run may publish an artifact under this name; it is reserved.
- **a published artifact** — named outputs the run shared by reference with
  whoever listens for the event.
- **`memory_suggestion`** — the arguments of a proposal the agent itself made.
  The platform resolves this form so older proposals have provenance, but the
  console does **not** offer "Remember this" on it: that proposal is already in
  the review queue, and accepting it is how it becomes memory.

### Labels are on screen before the decision

The sheet shows the labels the run had accumulated **up to the cited step**, not
those of the step alone. A clean answer inside a poisoned run is a fact the
poison reached, and remembering it as trustworthy is the inference the Gate
refuses to make.

While the trail is still loading, the sheet says it is reading — never that
there are no labels. Absent and none are different answers.

## What already answers this

Before saving, the sheet says what the platform already holds about that
identity. It is not a block: teaching a fact that already exists **corrects**
it, and that is usually what somebody means. What they cannot tell from the form
is which of the two they are doing.

| State | What is offered |
|---|---|
| active | saving corrects the wording, keeping counters, authorship and evidence |
| disabled | **reactivate** — the server will not merge into a disabled row |
| expired | saving with this evidence **renews** it for 30 days |
| source erased | nothing. That is the honest answer |
| covered by shared | **improve the shared one**, explicitly |
| pending proposal | nothing here; it is decided in the review queue |

**Covering is not correcting.** An equivalent shared memory covers a creation in
an agent's namespace, and stays byte for byte the same. Improving it is a button
that switches the form's namespace — never something that happens underneath an
agent write.

## Expiry

Memory lasts **30 days** from the decision that wrote it. Once expired it is no
longer recalled by runs, but it stays visible and stays the same memory.

| Transition | Expiry |
|---|---|
| correcting an active memory | preserved |
| reasserting with new evidence | renews for 30 days |
| auto-confirming after new observations | renews for 30 days |
| reactivating a disabled one | renews for 30 days |
| accepting a proposal over an active memory | preserved, never shortened |

## Content shaped like a secret

The platform refuses memory carrying a private key or a complete token in a
recognised format. Nothing clears that refusal, and the refusal **never repeats
the value it refused** — not in the message, not in the log, not in the event.

Text long and random enough to be a credential raises a warning, which a person
with publish permission may override. The override is not a receipt: it marks
the assertion with the `secret` label, which shows on the row, in the list and
in the event detail. An override nobody can see afterwards is a guard that
quietly stopped applying.

Auto-confirmation has no override, because it has nobody on it. A proposal the
platform cannot tell apart from a credential stays pending for review rather
than becoming readable to every run for having been made twice.

## Agent-proposed memory

An agent does not write active memory by default. Memory learning is an
opt-in field on the published agent version, so changing it creates a new
version and is visible in the publication review.

```yaml
memory_learning:
  mode: review
  ttl_days: 30
```

V1 is review mode. The agent can call `$fuseone.memory.suggest` with a subject
and claim. The platform derives the same `fact`/subject identity used when a
person teaches, then records the suggestion with the run labels and evidence.
`memory.find` does not return it yet. A person with publish permission in the
scope must accept or dismiss it from the Memory page.

When memory learning is enabled, the platform also offers
`$fuseone.memory.find`. At the start of a run with human input, FuseOne performs
one recorded memory lookup before the first model call, using short terms from
that input. The lookup is still a tool call: it crosses the Gate, appears in
the trail, and any labels on returned memory travel into the run.

The agent may call `memory.find` again later with a narrower kind, subject or
signature. Because human and agent teaching use the same platform-owned
identity, the suggestion path finds active memory and pending proposals for the
same subject instead of creating a second review vocabulary.

The automatic lookup runs once. If the model later proposes an equivalent
`memory.find` call with only JSON ordering or whitespace changed, the platform
skips it as the same call. A materially different or narrower lookup is still
allowed; the agent can refine from broad search text to a kind, subject or
signature without being mistaken for a retry.

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

Runtime memory search splits free text into a small set of terms and ranks
matches across subject, signature and claim. Strong identifiers such as
`not_in_channel` or `superset.alert.delivery` carry more weight; ordinary words
must still agree with enough of the assertion for it to be returned. A search
like `Slack not_in_channel` can match an assertion whose subject names Slack and
whose claim names the error code. Broad searches still return only a bounded
result set.

The free-text search also has a term budget. Strong identifiers are kept first,
then ordinary non-filler terms are added up to six distinct normalized terms.
Common Portuguese and English filler words are ignored, but short identifiers
such as `s3`, `db` or `qa` are still searchable when they appear as their own
term.
When a runtime tool call includes more, the response names the terms used, how
many terms were omitted and the reason
`search_term_budget`. That makes a bounded search different from "no memory
exists"; the agent can retry with stronger identifiers such as an error code,
system name or signature.

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
