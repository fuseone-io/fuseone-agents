---
title: What is FuseOne Agents?
description: A governed runtime and control plane for AI agents that act inside business operations.
---

FuseOne Agents runs AI agents where mistakes have real operational cost:
support queues, observability systems, internal APIs, secret stores, identity
systems and legacy tools.

The platform does not treat an agent as a chatbot with plugins. Every outside
effect crosses the Gate, every run leaves an append-only record, and the
console shows who approved what, which data labels were carried, what was
spent, and why an action did or did not happen.

## What the platform controls

- Which agents can run, in which company and area.
- Which MCP tools or governed connectors an agent can see.
- What each tool or connector operation is allowed to do.
- Whether a call is read, write, destructive or financial.
- Whether untrusted, scoped or personal data reached a later action.
- Whether a duplicate effect should be skipped instead of repeated.
- Whether memory is active, suggested, expired or source-erased.
- What was spent, which model spent it, and which prompt source drove it.

## Why this matters

Agents are useful because they can act across systems. That is also the risk.
FuseOne puts governance on the path of the action, not only in a policy
document next to it.

The model can propose. The platform decides whether the proposed effect is
allowed, needs approval, should be skipped as a duplicate, or must stop.

## Start here

- [Install FuseOne Agents](start/install/)
- [Understand durable execution](concepts/durable-execution/)
- [Understand the Gate](concepts/gate/)
- [Choose an integration path](concepts/integrations/)
- [Use governed memory](concepts/memory/)
