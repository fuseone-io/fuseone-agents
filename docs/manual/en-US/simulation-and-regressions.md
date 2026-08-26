---
title: Simulation and regressions
summary: How to rehearse an agent before production, turn real cases into a corpus, and read the result without mistaking dry tools for free runs.
section: operations
tags: simulation, regression, rehearsal, corpus, dry run, cost, policy
order: 14
---

## Rehearsal is a paid model run with dry tools

A simulation opens runs that look like production runs in the trail. The model
is called, the Gate decides, policies match, ceilings apply, and the result is
recorded.

The tool layer is dry: nothing is sent to Slack, GitHub, a CRM, a database or
any other external system. That makes rehearsal safe for effects, not free for
model usage. The cost preview says the maximum money exposure before the run
starts.

Use simulation before publishing, before promotion, and before changing a
policy from monitor to enforce.

## Choose situations, not prompts

A good situation is the thing the agent will actually receive:

```json
{
  "channel": "slack",
  "message": "@FuseOneAgent investigate alert #175979",
  "thread": [
    "alertNodeDownProdUS severity=critical",
    "namespace=payments job=engineering-ai-agents instance=172.16.109.29:8080"
  ]
}
```

Write situations that cover the shape of the work:

- a normal case that should finish;
- a case that should ask for a person;
- a case that should be refused by policy;
- a case with missing data;
- a case with noisy or misleading input.

Do not write situations as ideal instructions. If production sends Slack text,
simulate Slack text. If production sends webhook JSON, simulate the webhook
payload.

## Read the result

The report separates three outcomes:

| Result | What it means |
|---|---|
| Passed | The run reached an acceptable ending |
| Needs a look | A person should inspect the trail before trusting this case |
| Failed | The observed ending contradicts the expectation |

A policy block in a simulation is not automatically a failure. If the case is
supposed to prove that a risky call is stopped, the block is the evidence you
wanted.

Open the run trail for any case marked needs a look. The important question is
not whether the final text sounds good; it is which facts the agent read,
which tools it tried, and which rule stopped or allowed each action.

## Save a regression corpus

When a simulation covers real situations, save it as a corpus. A corpus is a
set of cases the platform can run again after a change.

Use one corpus for one promise:

- "the Slack troubleshooting agent handles node-down alerts";
- "the GitHub reply agent never writes from untrusted input without approval";
- "the billing agent asks for a person before a destructive CRM action".

Small corpora are easier to trust. Ten cases that each teach something are
better than a hundred cases nobody reads.

## When to run regressions

Run the corpus when:

- instructions changed;
- tools were added or removed;
- a tool classification changed;
- a policy changed;
- the autonomy stage changed;
- the provider or model changed.

If a regression starts failing after a policy changed, read the Gate step
first. The policy may be doing exactly what it was written to do.

## Common mistakes

### "The simulation did not post to Slack"

That is expected. Simulation keeps tools dry. To test Slack delivery, run a
real agent in draft or copilot with a narrow conversation.

### "The simulation cost money"

That is expected. The model was called. The cost preview is there so a person
decides before spending.

### "The report is green but production stopped"

Check whether production carried labels the corpus did not. A Slack mention,
webhook body or context artifact can mark the run as untrusted, and the Gate
will stop a write that a clean rehearsal did not try.

The rules are in [What the platform stops before it happens](what-the-gate-stops.md)
and the trail is explained in [Reading a run](reading-a-run.md).
