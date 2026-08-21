---
title: Approving an action
summary: What you are deciding when a run stops, and when the right answer is to change the policy instead of approving.
section: operations
tags: approval, human queue, decide, refuse, escalate
order: 10
---

## What stopped, and why

When a run stops it appears in the **human queue**. The screen shows three things, and the first is the one that matters:

**The exact arguments** that will be sent. Not a description of what the agent intends to do — the real content of the call.

**The rule that stopped it.** It may be the effect ladder, the mark carried by content from outside, or a policy with its code and reason.

**What the run did up to that point.** The whole trail, so you understand the path and not only the last step.

## Approving releases that call

An approval covers **that call, with those arguments**. It does not switch the tool on, does not carry to next time, does not create an exception. The next run stops again.

Which is why reading the arguments is the work, not a formality. The question is not *"is this agent trustworthy?"* — it is *"should this specific call happen?"*.

## When the right answer is not to approve

**Approving the same thing ten times in a row is a signal**, not a routine. It means the area's policy is wrong — either because it stops something that should pass, or because whoever approves has stopped deciding.

Either way the fix is the policy, not the click.

The reverse holds too: if a call surprised you, refuse it and go read the agent's instruction. **Refusing is not punishing the agent** — it is the mechanism saying something was not anticipated.

## Refusing is information

A refusal is recorded with whoever decided. If the run can continue without that action it continues; if it cannot, it stops with the refusal in its trail.

Whoever wrote the agent sees that and learns what the real world brought that the rehearsal did not.

## Use cases

### The content came from a ticket

A run read a ticket's text and now wants to write. It stopped, because anything from outside carries a mark.

**What to look at:** whether what will be written reflects the ticket, or reflects something the ticket's text *asked* the agent to do. The second is an instruction attempting to arrive inside content, and the answer is to refuse.

### The agent wants to delete something

A destructive effect is blocked by default. If it reached you, somebody opened it by policy.

**What to look at:** the exact target in the arguments. Destructive is the one category where approving cannot be undone afterwards.

### You do not understand the call

Refuse. An approval you cannot explain is not an approval — it is a signature.

What the platform stops on its own is in [What the platform stops before it happens](what-the-gate-stops.md), and how to write the rule that changes it is in [Policies](policies.md).
