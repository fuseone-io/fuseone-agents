---
title: Draft, copilot and autonomous
summary: What changes between the three stages, and how to decide an agent is ready for the next one.
section: governance
tags: autonomy, draft, copilot, autonomous, promote, trust
order: 8
---

## Three rungs, not a switch

Every published agent starts in **draft** and paused. Leaving each rung is somebody's decision, and it is recorded.

| Stage | What it may do |
|---|---|
| **Draft** | Opens no real run. Rehearsal only |
| **Copilot** | Runs for real, and every write waits for a person |
| **Autonomous** | Runs and writes on its own, within what policy and classification already allow |

Notice what does **not** change between them: the Gate evaluates the same in all three. Autonomous is not "no brakes" — it is "no human wait on what was already cleared". A destructive tool is still blocked, taint still stops a write, a ceiling still holds.

## Draft

The agent exists, has a published version, and opens no real run.

This is where you write, rehearse and correct. A rehearsal runs against situations you choose with the tools dry — nothing is sent to or changed in an external system.

**Leave when:** the rehearsal passes the situations that matter, and the ones it marked as *needs a look* you have understood and resolved.

## Copilot

Runs for real. Every write stops and waits for a person to approve that specific call.

This is the rung where you learn what the agent actually does with real data, which is never exactly what the rehearsal showed.

**Leave when:** approvals have become routine without surprises. There is no magic count, but there are three questions:

- **Are the approvals all the same?** If you approve the same thing every time without thinking, it has stopped being a decision.
- **Has any refusal surprised you?** If so, the agent is still learning the work — or the policy is wrong.
- **Would you read one of these runs' trails and be comfortable?** If that depends on who was looking, not yet.

## Autonomous

Runs and writes without waiting for approval, **within what was already permitted**.

What still stops it: destructive and financial effects, content marked as coming from outside, a policy that escalates or denies, and any ceiling.

**Which means promoting to autonomous is not the moment to relax policy — it is the moment policy becomes the only thing holding.** Before, a human reviewed every write and would have caught what policy let through. Now nobody does.

## Promoting safely

The order that works:

1. **Rehearse** until the situations that matter pass.
2. **Publish in draft** and unpause.
3. **Copilot**, and approve properly — reading the arguments rather than clicking from habit.
4. **Before going autonomous**, write the policies that would hold whatever worries you, and turn them on in **monitor mode**. See whether they would match.
5. **Switch those policies to enforcing**, and only then promote.

Step 4 is the one usually skipped, and it is what separates promoting from gambling.

## Going back is normal

Dropping a rung is not a failure — it is the mechanism working. An agent that started getting things wrong after a change on the other side goes back to copilot until somebody understands what changed.

What a policy decides is in [Policies](policies.md), and what the Gate stops regardless of stage is in [What the platform stops before it happens](what-the-gate-stops.md).
