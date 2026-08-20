---
title: Costs and limits
summary: How price, currency, per-run ceilings and area budgets relate without mixing market references with accounting.
section: cost
tags: cost, price, currency, ceiling, budget, tokens, provider
order: 6
---

## Cost is calculated from configured rates

The platform counts tokens in each run. To turn tokens into money, the
installation needs a configured rate for the provider/model pair the run used.

Market prices shown on the screen are references. They help an operator fill
the form, but they do not count toward ceilings until saved as installation
rates. This avoids adding market USD inside an installation configured in BRL,
EUR or another currency.

## Why a run can show zero cost

A run with tokens and zero cost usually means one of these:

| Symptom | Reading |
|---|---|
| Tokens appear, cost $0.00 | No configured rate for that provider/model |
| A ceiling blocked, but cost is zero | The ceiling may be steps, calls or duration, not money |
| A rate was created and an old run stays zero | History is not repriced |
| A rate was created and a new run stays zero | The worker has not loaded the revision, or provider/model do not match exactly |

The model name must match what the run uses. `claude-opus-5` and
`claude-opus-5-20260801` are different prices if the provider records
different names.

## Currency

The installation currency says how stored values and configured rates are
rendered. Changing the currency does not convert history. It changes how
numbers are read.

Do not use currency changes as financial conversion. If the installation moved
from BRL to USD, review ceilings and rates explicitly.

## Ceilings

A run can stop on four families of limit:

- money;
- tokens or estimated cost;
- number of tool calls;
- steps or duration.

When the Gate blocks, read the event sentence. It says which ceiling was hit:
money, calls, steps or another limit. Configuring prices fixes only the
financial ceiling; it does not raise the step limit.

## Reservation and reconciliation

Before spending, the platform reserves the estimated cost. After the call, it
reconciles with the real cost and releases the rest.

This is about time, not neat accounting: if the platform counted only after
the call, twenty workers could open calls at the same time before any of them
recorded cost.

## Cache reads and cache writes

Some providers charge cached input at a lower rate. The screen separates
input, output, cache read and cache write because they do not cost the same.

Even when cache read is cheap, it is still consumption. A good optimisation is
one that appears in accounting, not one that disappears from it.

## Configuration checklist

1. Configure the installation currency.
2. Check the provider/model shown on the run.
3. Open Cost and limits and create a rate for that exact pair.
4. Use the market price as a reference, not as an automatic value.
5. Run a new run.
6. If cost is still zero, check whether the worker loaded the revision and
   whether the model name matches exactly.
