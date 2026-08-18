---
title: Agents, versions and runs
summary: What gets published, what gets pinned, and what stays on the record.
order: 1
---

## An agent is a text, not a program

An agent is written in plain language, in blocks: the objective, what it may
use, what it never does. There is no code inside it and no drag-and-drop
canvas. What is written is what the platform reads.

That is a choice, and it has a consequence you feel daily: the person who
writes the agent is the person who understands the process, not the person who
understands programming.

## A version is a freeze

Every time an instruction is published, the platform stores the whole text and
derives a version from it. Publishing the same text twice does not create two
versions — the version *is* the content.

This answers the question that arrives months later: **which instructions did
that run under?** Not today's. That day's, which are still stored.

## A run is pinned to the version it opened on

A run carries the version that was current when it opened, and keeps it until
it ends. Publishing a correction mid-afternoon does not change what is already
running.

That rules out the case where somebody approves an action, the instruction
changes, and a different action executes. What was approved and what ran are
the same thing, always.

## Everything is read from one record

The platform stores no "agent state" anywhere. It stores steps, in order, each
sealed against the one before it. State is what you get by reading the steps
from the beginning.

In practice: no screen shows a summary somebody could have updated on the side.
What the screen shows and what an auditor reads come from the same reading.

A recorded step is never altered or deleted — the database refuses. A
correction is a new step, and the correction is on the record too.

What stops a run from doing something is covered in
[What the platform stops before it happens](what-the-gate-stops.md).
