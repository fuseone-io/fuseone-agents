---
title: Data labels and barriers
summary: How company, area and untrusted-source labels travel with a run, and what the platform refuses.
section: security
tags: data labels, company barrier, area barrier, context, untrusted, provenance
order: 12
---

## Labels are carried with the data

When a run starts, the platform seals the agent's company and area on the
opening step. When the run came from outside — Slack, webhook, event input or a
tool result — the source marks travel with it too.

The important part is that labels live beside the content reference, not in the
prose the model wrote. A model cannot remove them by summarising, and a person
cannot grant them away by approving one tool call.

## The barrier is the scope rule

A run may carry data only from scopes it reaches.

| Data label | Run scope | Result |
|---|---|---|
| `area:acme/platform` | `acme/platform` | allowed |
| `area:acme/platform` | `acme` | allowed |
| `area:acme/platform` | `acme/finance` | blocked |
| `area:acme/platform` | `cora/platform` | blocked |

This is the same containment rule used for grants: installation reaches every
company, a company reaches its areas, and an area never reaches a sibling.

## Query filters are not barriers

A search query like `company = acme` narrows what one request returns. It does
not prove that data will stay there after the model reads it.

The barrier is the carried label. Once `area:acme/platform` is in the run, the
Gate refuses actions in a run outside that scope. If an event would open a
listener in another area, the run is not opened.

## What an operator sees

In a trail, a blocked call names the rule `data_barrier` and explains that the
run carries data from outside this company or area. That is not a policy to
edit and not a missing approval; it means the data flow crossed a boundary the
platform does not have authorization to cross.

If this happens:

1. Check whether the agent was published in the right company and area.
2. Check whether the channel conversation points at the same area as the
   agent.
3. Check event wiring before assuming the listener is wrong.
4. Do not fix it by broadening a query. The label already travelled.

## Why context sharing depends on this

Future shared context between agents must pass artifacts by reference, with
labels, digest and origin. Free-form prompt passing is not enough: it copies
words and loses the authority attached to the content.

The rule that must remain true is simple: a listener can receive context only
when its scope is allowed to carry the labels on that context, or when an
explicit cross-boundary authorization is recorded.
