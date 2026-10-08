---
title: Standing approvals
summary: Pre-approve one exact tool for one agent, with a name, a ceiling and an expiry, so the night shift does not wait for a click.
section: operations
tags: approval, standing, grant, autonomy, taint
order: 20
---

# Standing approvals

A standing approval is a human approval given ahead of time: one person
pre-approves **one exact tool** for **one agent** in **one scope**, with a
**daily ceiling** and an **expiry**. When a run parks waiting for approval
and a mandate covers it, the platform decides the park immediately — with
the grant owner's name and the mandate's id on the decision, exactly as if
they had clicked. The run's trail reads "Approved by" that person, because
that person did approve it; they just did it in daylight.

## When to grant one

Grant a standing approval when all three are true: the action's arguments
come from content the platform rightly distrusts (logs, messages), so every
use would otherwise wait for a click; the moment of use is when nobody is
awake; and **the tool itself carries structural guards** that make the
worst malicious input survivable. The Cloudflare block list is the shaped
example: literal addresses only, protected ranges refused in code, its own
ceiling, blocks that expire.

A standing approval releases the park — it does not make a tool safe. That
judgement is yours, and the required reason field is where you write it.
The block list's `unblock_ip` is the worked counter-example: do **not**
grant it a mandate. Removing protection is exactly the action a misled
agent should never take unattended, and the human click is its ceiling.

## The mandate's bounds

- **One exact tool id**, instance included. Patterns are refused: a mandate
  must know what it covers.
- **A daily ceiling** (at most 100). Covering and spending a use are one
  atomic step, so the ceiling holds under concurrency; once spent, parks
  wait for a click again until midnight.
- **An expiry**, at most 90 days. Renewal is a deliberate re-decision.
- **Revocation is immediate** and keeps the row: the trail shows the
  mandate and its end.

## Who may grant

Granting needs its own permission (`approval:grant`, Admin by default) —
approving a class of future actions is strictly more than clicking one
card, which any Approver can do. Creation, revocation and every single use
land in the administrative trail.
