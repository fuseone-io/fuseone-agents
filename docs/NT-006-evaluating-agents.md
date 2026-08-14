# NT-006 · Evaluating agents

**Status** Implemented · **Date** 2026-08-13 · **Delivered** 2026-08-14
**References** [PRD-001](PRD-001-fuseone-agents.md) — FU-10, FU-11, FU-12, FU-13, FO-05, AU-01, AU-02, DE-01, N4 · [NT-005](NT-005-interaction-channels.md)
**Outcome** Finish a mechanism that is already most of the way built, rather than adopt a second one

The Gate proves an agent had **permission** to act. Nothing proves it is still
**competent**, and the only thing that comes close is FU-10 — one simulation,
reviewed by one person, on one afternoon before publishing.

That is the gap evals close. This note argues that closing it here is small
work, because most of an eval harness already exists in this platform under
other names, and that every off-the-shelf alternative would force a second
record of the same runs.

---

## 1. What is already an eval, under another name

| Here | In eval vocabulary |
|---|---|
| `domain.RegressionCase` + `domain.Expectation` | A dataset with assertions |
| `simulate.Battery` | The runner: it counts held and broken, and marks a case that never ran as broken |
| FU-10 | A release gate |
| `replay` | Reproducibility — re-execution that catches a forged verdict |
| The ledger | The trace, and a corpus labelled by human decisions |
| Agreement / autonomy demotion (FO-05) | An online eval, on the strongest label there is |

The important decision was already taken, in `domain.Expectation`: *"it should
have asked me" is a sentence a person understands and nothing can check, so
what is recorded is the checkable half of it.* That is the line between an eval
and an opinion, and it is already drawn.

## 2. This platform can assert on structure, not only on prose

Most agent evals are hard because they judge **text**, and judging text needs a
model — expensive, noisy, and not reproducible.

What matters here is not the prose. It is **which tool was proposed and what
the Gate said**. An agent that writes a beautiful summary and proposed a
transfer is a failure even when the transfer was blocked. Those assertions are
objective, cheap and deterministic, and the ledger already records every one of
them.

That advantage compounds: the `Clock` is injectable, `time.Now()` is banned
from business logic and `math/rand` from auditable paths. Evals here are
reproducible in a way most are not.

### 2.1 Two classes of assertion, with different semantics

The received wisdom is that evals should report distributions rather than
pass/fail, because the system is non-deterministic. That is right about text
and wrong about everything this platform cares most about.

An agent that reached `erp.transfer` once in twenty runs is not 95% good. It is
a breach. Non-determinism is an argument for running the case more times, not
for softening the criterion.

| Class | Examples | Semantics |
|---|---|---|
| **Structural** | `never_calls`, `asks`, `settles`, a cost ceiling, a step ceiling | Binary. One breach is a breach. |
| **Quality** | tone, completeness, whether the answer is any good | A distribution, with a threshold and sampling |

Everything that exists today is the first column. It is the cheap, objective,
deterministic one, and in a governed platform it is also the one that matters.

## 3. What was missing, and what shipped

**A gate, not a score.** `Battery` counted held and broken and a person read
it. What shipped is a gate — and it is on **starting** an agent, not on
publishing it, which is where this note originally put it and where it cannot
work: a version's identifier is the digest of its own bytes, so it does not
exist until it is published and nothing can have been simulated against it
before. Publishing writes a definition down; starting is what makes it act.

It is binary rather than a threshold, per §2.1. These are structural
assertions, and an agent that reached a financial tool in one case out of
twenty is not ninety-five per cent good. It is also narrow: only a battery run
against that exact version counts, because FU-10 already stands before a first
publication and demanding a fresh battery for every corrected sentence would
make the corpus something people route around. Stopping an agent is never
refused.

**Comparison between versions.** Shipped, on `GET /agents/{id}/comparison`.
Nothing is stored for it: each side is the newest battery run against that
version, so the comparison is a fold of two folds. Cost is in it as well as
held and broken — an agent that reaches the same answer for three times the
money is worse, and a diff of corrections alone reports that as no change.

**Drift.** Shipped, as two sweeps in the worker. The reading is two batteries
of the *same* version: a version that did not change is an agent that did not
change, so a correction that held last night and does not hold this morning is
the world moving underneath it. Which means the comparison above was already
the mechanism — drift is `Compare` with both sides on one version.

The battery runs daily and the reading pass hourly, and they are deliberately
not chained: a battery is a set of runs the pool advances over minutes, so a
loop that opened one and read it back would read a half-fold every time. Each
battery is named after the window it belongs to rather than randomly, so three
workers sweeping the same night ask for the same runs and the ledger accepts
one of them. The notice goes to the channel from NT-005, and drift is in that
channel's default events — it fires rarely, and it is the one notice nobody
would think to opt into.

**Two assertion kinds.** Shipped: `costs_at_most` and `within_steps`, beside
`calls_before`. An agent that reaches the right answer for three times the
money is a regression, and it used to pass green.

**The model, pinned to the case.** Shipped. A case records the model and
effort it last held against, so a rise in broken is no longer ambiguous: every
case still naming the model it always did is a change somebody made, and one
where the model moved underneath is drift. That is the difference that decides
who gets woken up.

**Adversarial cases.** Still a corpus somebody writes rather than code to
write. NT-005 §5 establishes that text arriving from an
external channel is entirely adversary-controlled. The assertion for that is
already expressible: given this poisoned input, did the agent still refuse to
reach a financial tool? That is `never_calls` on a hostile case — no judge, no
score, yes or no. This corpus is, in practice, the Gate's own test against
prompt injection, and it is worth having even if the rest waits.

## 4. Adopt or write

The advice in the field is to adopt: DeepEval, Promptfoo, RAGAS, or a hosted
platform. Neither half transfers here.

**The hosted ones are out on the product's own terms.** LangSmith, Braintrust,
Arize and Langfuse Cloud work by receiving the traces. This platform is
installed inside a customer's network, holds tool results behind a claim check
and erases them per data subject (AU-04, NF-09). Sending run content to a third
party is the one thing it is built not to do.

**The libraries are Python, and the artefact is one static binary.** DE-01 is
"one binary, one PostgreSQL, nothing else required". Adding a Python runtime,
a package manager and a pip supply chain into an air-gapped installation is not
a dependency, it is a second product to install, secure and patch.

**Self-hosting one of them is worse than either.** It would put a second
platform beside this one — its own store, its own interface, its own identity —
holding a second record of the same runs, in traces and spans rather than in a
hash-chained ledger with a Gate. Then "what did this agent do" has two answers
and an auditor has to be told which one counts. That is exactly what
`internal/audit` refuses to do for the two trails it already merges, and for
the same reason.

**So: write it here — and the reason is not that ours would be better.** It is
that the thing to write is small. The dataset, the runner, the traces, the cost
accounting, the human review and the online signal all exist. What is missing
is a score, a threshold, a comparison, a schedule and two assertion kinds. That
is finishing a mechanism, not building a framework.

### 4.1 What that gives up, honestly

Breadth of metrics: BLEU and ROUGE, embedding similarity, RAGAS faithfulness
and groundedness. Little of it applies — there is no retrieval here — and none
of it is the structural half. What is worth taking from the field is the
**technique** of a judge rubric, not a dependency: a rubric is a prompt, and
this platform already has a model registry to run one through.

## 5. Where a judge belongs, and where it does not

Only where free text has to be judged, and never for anything the ledger can
answer.

When it is used, a judge is a model call: it costs money, it belongs in the
ledger with its cost, and **its verdict is recorded as an opinion with a
provenance, never as a fact**. A platform that gave a judged score the same
standing as a recorded verdict would be giving two grades to one claim.

An ensemble of judges is a real technique and is not proposed here. It
multiplies cost for a column that is already the less important one.

## 6. Cold start

A fresh installation has no history, so it has no corpus, so FU-10 is satisfied
by whatever the author happened to try. Generating cases with a strong model
solves that, with one condition: **a synthetic case is marked synthetic**.
Otherwise an installation certifies an agent against examples nobody has ever
seen happen, and reports it in the same number as the ones that did.

The field's rule of thumb — fifty to a hundred examples already changes things
— is a useful calibration for the threshold. FU-10 today asks for at least one.

## 7. Why this is worth doing beyond engineering hygiene

The EU AI Act, ISO 42001 and SOC 2 for AI ask for **evidence** of performance
and safety, not for assurances.

This platform already proves permission with a hash chain. An eval whose result
is sealed into the same ledger proves competence with the same grade of proof:
*this version was checked against these eighty-seven cases, on this date,
against this model, and here is the hash*. That sentence is one an auditor
accepts, and one very few products can say at all.

That, and not the engineering, is the argument that decides whether this gets
built.

## 8. What this is deliberately not

- **A general evaluation platform.** N4 rules out building an integration
  engine and the reasoning carries: this is one agent's regression suite, tied
  to publishing and to drift. The moment it becomes a place to benchmark
  foundation models it is a different product.
- **A model leaderboard.** Which provider is better on public benchmarks is
  answered elsewhere and changes nothing about whether *this* agent still does
  *its* job.
- **A second trace store.** The ledger is the trace. Anything that needed its
  own copy would be a second record of the same runs.

---

## 9. What remains

**Adversarial cases** (§3). The assertion is already expressible — `never_calls`
on a poisoned input, no judge and no score — so what is missing is a corpus
somebody writes, not code. It is worth writing: in practice it is the Gate's
own test against prompt injection.

**Asserting on a tool's arguments.** `never_calls` and `calls` say whether a
tool was reached and say nothing about what it was asked to do, which leaves
"refunded the right customer" outside what a case can check. The arguments are
behind the claim check by design (AU-04), and the fold has no content store —
so this is a real piece of work rather than a missing switch.

**Synthetic cases for a cold start** (§6). Unbuilt, and still gated on the same
condition: a synthetic case is marked synthetic, or an installation certifies
an agent against examples nobody has ever seen happen.
