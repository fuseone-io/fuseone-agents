---
title: Writing good blocks
summary: Instructions that make the agent act now, stop in the right place, and avoid promising future work.
section: authoring
tags: blocks, instructions, agent, example, stopsWhen, tools
order: 3
---

## The block is the agent's contract

An agent receives no hidden intent. It receives text blocks. A good block says
what to do, what evidence to collect, when to stop, and what must never happen.

The text should be operational: the person doing the work today should
recognise the process in it.

## A final answer ends the run

When the model returns text without calling a tool, the run ends. Avoid
sentences like "I will check", "I will continue" or "I will investigate". If
the next action needs a tool, the agent must call the tool in that turn.

Write that explicitly:

```text
Do not announce future work. If you need logs, metrics or another system, call
the tool now. Write the final answer only when the analysis is complete or when
you can explain why you cannot continue.
```

## Purpose

Purpose is one sentence about the outcome. Do not put the whole sequence here.

Good:

```text
Diagnose a Slack alert and reply in the thread with probable cause, evidence
consulted, and safe next steps.
```

Bad:

```text
Use Grafana, search logs, look at metrics, maybe check the wiki and reply.
```

The bad version mixes outcome, tools and maybe. Maybe is a gap, not an
instruction.

## How to act

How to act is the runbook. Use stable names: datasource ids, labels, fields and
filters. Display names change; ids and labels are what tools understand.

SRE example:

```text
1. Read the message and thread context. Identify alertname, application,
   namespace, pod, instance, cluster, severity and approximate time.
2. Query metrics in Grafana with the Mimir Query datasource
   (id 7-JKlf87k). Prefer filters by namespace_name, pod_name,
   container_name, job or instance.
3. Query logs in Grafana with the Loki datasource (id bDvEKnCnk) using the
   same filters. Never start with a broad query.
4. Compare metric and log evidence. If a tool fails because the datasource is
   invalid or empty, list datasources and retry with the correct id.
5. Reply in the thread with diagnosis, evidence and next steps.
```

## When to stop

When to stop is not "when finished". Name the conditions that end the run.

```text
Stop when there is enough evidence to state a probable cause and next steps,
or when there is no minimum identifier to query safely. If application,
namespace, pod, instance or time is missing, say which data is missing and
stop.
```

This avoids two failures: spending tokens on generic search and finishing as
if the analysis were complete when it was not.

## Never

Never is a guardrail. Write forbidden actions, not vague preferences.

```text
Never use broad Loki or Mimir queries. Never consult runbooks or Outline for
this agent. Never say you will continue the investigation: continue with tools
now or stop with a clear reason.
```

Never does not remove a tool from reach. If the agent must not be able to call
a tool, remove the tool from the pack or from the server surface. Text is
instruction; the Gate is mechanism.

## Complete example

```text
Purpose
Diagnose Slack alerts and reply in the thread with probable cause, evidence
and next steps.

How to act
Read the message and thread context. Identify alertname, application,
namespace, pod, instance, cluster, severity and approximate time.

Use Grafana to query Mimir Query (datasource id 7-JKlf87k) and Loki
(datasource id bDvEKnCnk). Use specific filters: namespace_name, pod_name,
container_name, job, instance or equivalent labels. If a datasource id fails
or returns empty, list datasources and retry with the correct id.

Cross-check metrics and logs before concluding. If you find a probable cause,
cite the evidence. If you do not, say exactly which queries were run and what
information was missing.

When to stop
Stop when you have a probable cause and next steps, or when minimum data is
missing and any further query would be broad.

Never
Never use broad Loki or Mimir queries. Never consult runbooks or Outline.
Never announce future work: if you need to check something, call the tool now;
if you cannot, answer why you stopped.
```

## Checklist before publishing

- The block says the expected outcome.
- The runbook names tools, ids and filters where they matter.
- There is a clear stop condition.
- Never forbids concrete behaviours.
- The agent does not promise to continue after writing an answer.
- The tool pack matches the text. If the text forbids a tool, remove that tool
  from reach too.
