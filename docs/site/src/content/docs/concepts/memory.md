---
title: Governed memory
description: Memory between runs without turning remembered text into hidden authority.
---

FuseOne memory stores structured assertions, not free-form prose that the model
is expected to obey later.

An assertion has a kind, subject, signature and claim. It also carries
evidence, labels, confirmation state, retention and erasure state. The model
can search memory, but it does not choose the company, area or agent namespace
being searched.

## Reading memory can taint the run

Memory does not make outside data trusted. If an assertion came from Slack,
GitHub, Loki or another external source, its labels travel with the memory
read. The fold applies those labels to the run, and the Gate evaluates the next
write with them.

That is enforcement, not a prompt instruction.

## Agent-proposed memory

Agents can suggest memory only when the published version opts in. In review
mode, a person accepts or dismisses the suggestion. In auto-confirm mode, the
same assertion must appear across the configured number of distinct runs before
it becomes active.

The model does not send confidence. Counts are derived by the platform.

## Search and budget

Memory search is indexed for broad substring matching, but responses are still
bounded. If matching assertions do not fit the response budget, the tool says
how many were omitted and why. Labels are unioned across all matching
assertions before that response cut, so an omitted risky assertion cannot make
the run look cleaner than it is.

## Related manual page

- [Governed memory](../../manual/en-us/memory/)
