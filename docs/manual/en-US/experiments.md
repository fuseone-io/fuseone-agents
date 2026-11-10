---
title: Experiments
summary: How to run one agent version against a dataset version on another provider, model or effort, read the result case by case, and compare two experiments without mistaking one for a battery.
section: operations
tags: experiment, model, provider, effort, repetitions, dataset, comparison, simulation
order: 27
---

## An experiment asks "what if", and changes nothing

An experiment runs one version of an agent against the cases of a dataset
version, under a configuration: another provider, another model, another
effort, or none of these. It answers one question: would this version do
better or worse on another model?

It changes nothing about the agent. Moving to the model that won is publishing
a new version, which gets its own battery. Until then the agent runs exactly as
it was published.

Every run of an experiment is a [simulation](simulation-and-regressions.md):
the model is called, the Gate decides, policies match, ceilings apply, and the
tool layer is dry. Nothing reaches Slack, GitHub or any other system.

## An experiment is never a battery

A battery is the corpus run against a version as it was published, and it is
the evidence the start gate and the drift check read. An experiment may run the
version on a model it would never use, so it never counts as a battery,
whatever its configuration, including none at all:

- it does not start or stop a version;
- it does not count as the version's last battery;
- it does not let an agent leave Draft.

To start an agent, run its corpus, not an experiment.

## Start one

Open the agent, choose **Experiments** in its menu, then **New experiment**.

| Field | What it does |
|---|---|
| Agent version | The version to run. The current one by default; an older one works too. |
| Provider | Leave it **As published** to keep the version's provider. |
| Model | Leave it empty to keep the version's model. |
| Effort | Leave it **As published** to keep the version's effort. |
| Repetitions | How many times each case runs, 1 to 5. |

Every field is optional, and a field you leave alone is the version's own. A
provider alone runs the published model on it. A model alone runs under the
published provider. An effort alone changes only the effort. The pair that
results has to be one this installation can actually call; otherwise the
experiment is refused before any run opens.

The console runs the latest dataset version. To pin an older one, use the API
below.

### Any change runs without the fallback

A version may declare a fallback model for when its own keeps failing. An
experiment with **any** change, even effort alone, runs without it: when the
chosen model's retries run out, the run parks and the report says so. The
result never reports one model while measuring another.

An experiment that changes nothing is the version as published, and keeps its
fallback.

## Repetitions and cost

Repetitions are for certainty, not for an average. A model can answer the same
case differently each time, and a case broken in **any** repetition is broken.

Each repetition is its own run, reserved and billed. Before you start, the form
shows cases × repetitions = runs, and what that may cost at most at the
version's ceiling per run. Five repetitions of forty cases is two hundred runs.

## Read the result

A result shows, in this order, what was run and then what was found:

1. dataset version;
2. agent version;
3. configuration, or **As published**;
4. cases run, out of the cases in the version;
5. erased cases, which could not run;
6. repetitions;
7. held, broken, and cost.

Each case reads **Held**, **Broke**, **Erased** or **Did not run**, and in how
many repetitions it broke out of how many ran: `1 of 5`. One of five is a model
that sometimes gets it wrong; five of five is a model that gets it wrong. When
fewer repetitions ran than were asked, the row says so.

The report is readable while the experiment is still running; the cases that
settled are complete rows.

## Compare two experiments

In the list, tick two experiments and choose **Compare the two**. They are
always compared oldest to newest, whichever you ticked first.

On the **same dataset version**, both asked the same questions and every case
is compared. On **different dataset versions** they did not: the comparison
lists only the cases both versions have, says so above everything else, and
counts how many each side had that the other did not.

Cost moves **per run**, so an experiment with five repetitions is not shown as
five times dearer than one with a single repetition at the same price. A case
one side never ran is broken there, and moves no cost.

## Through the API

| Request | What it does |
|---|---|
| `POST /agents/{agentId}/experiments` | Starts an experiment. Takes `version`, `datasetVersion`, `config` and `repetitions`, all optional. |
| `GET /agents/{agentId}/experiments` | The 20 most recently opened experiments. |
| `GET /agents/{agentId}/experiments/{experimentId}` | One experiment's report. |
| `GET /agents/{agentId}/experiment-comparison?from=&to=` | Two experiments, case by case. |

The `Idempotency-Key` names the experiment. Asking again with the same key
reaches the runs it already opened, even when the dataset or the agent has a
newer version by then, and opens the runs an interrupted first attempt left
unopened. A field the retry leaves out is the first attempt's. A retry that
states something else is refused with a 409 and opens nothing. Use a new key
for a new experiment.

An experiment lives where its runs were recorded. If an agent is republished
under another area, its old experiments stay there: somebody who may read the
new area sees a 404 for them, as for an experiment that does not exist.

## Common mistakes

### "The experiment won, so the agent now uses that model"

It does not. Publish a new version with the model you chose, and run its
battery.

### "I only changed the effort, but the run did not fall back"

That is the rule: any change runs without the declared fallback, so the result
measures the model you chose. Leave every field as published to keep it.

### "The comparison left cases out"

The two experiments ran different dataset versions. Only the cases both have
are compared, and the screen says how many each side had alone.
