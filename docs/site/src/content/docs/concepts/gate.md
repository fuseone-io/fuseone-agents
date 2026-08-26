---
title: Gate and labels
description: How FuseOne decides whether an agent can perform an external effect.
---

The Gate is the boundary every external effect crosses. It runs before the
effect reaches an MCP server, a governed connector, a channel or another
outside system.

The Gate is deterministic. It receives the proposed tool call, the run state,
the classified effect, the agent's autonomy stage, policy, budget and labels.
It does not query the database while deciding.

## Labels travel with data

Inputs, channel messages, tool results, shared artifacts and memory can carry
labels such as `untrusted`, `pii` or a scope label. The run fold unions those
labels as the run advances.

That makes taint mechanical. If an agent reads untrusted data and later tries
to write, the Gate sees the label on the run. The model is not asked to
remember that the data was risky; the state carries it.

## Approval is not a bypass

Some calls need a person. A grant can release an action that only required
approval.

A grant cannot override a hard block such as a scope boundary, malformed
contract, unavailable credential, data barrier or other rule that says the
effect must not leave the worker.

## Related manual pages

- [What the Gate stops](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/manual/en-US/what-the-gate-stops.md)
- [Approving](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/manual/en-US/approving.md)
- [Data barriers](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/manual/en-US/data-barriers.md)
