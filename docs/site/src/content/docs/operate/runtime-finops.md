---
title: Runtime and FinOps
description: Operational health, cost explanation and confidence boundaries.
---

FuseOne has two kinds of operational signal.

Prometheus metrics are volatile process signal. They answer "is this worker
seeing this right now?" and reset when the process restarts. Labels are kept to
fixed vocabularies so a third-party MCP server cannot grow metric cardinality
without bound.

Runtime projections are durable installation signal. They answer questions
such as "when did this failure first appear in the visible scope?" without
folding the run ledger on every page load.

## Cost explanation

Run spend is explained without mixing units:

- provider tokens and money come from the model cost record;
- prompt bytes are measured by the platform while building the transcript;
- compacted bytes are reported as bytes not sent to the model;
- cache hits are counted as external calls avoided, not tokens saved.

Aggregate spend is projected by planning step, not by run, so a step that uses
a different model is attributed to the model that actually spent the tokens.

## Trust Center

The Trust Center judgement is computed on the server. The page reads evidence,
not raw prompt or tool content, and the evidence has a declared window where
the claim is time-bound.

Unknown is not rendered as low risk. Partial coverage is not rendered as full.

## Related manual pages

- [Costs and limits](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/manual/en-US/costs-and-limits.md)
- [Simulation and regressions](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/manual/en-US/simulation-and-regressions.md)
- [Auditor guide](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/manual/en-US/auditor-guide.md)
