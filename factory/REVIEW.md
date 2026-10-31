# The review format

How the Reviewer role (#197) reviews a pull request, and how the author answers.
It is the format this repository's review rounds already use: the rounds on
#185, #186, #189, #205 and #209 are the model. This file takes their
structure, not their content.

QA (#198) submits its verdict the same way, as a pull-request review on the
head SHA it checked (§ Submitting the verdict). Its body is the per-criterion
check of LANES.md § `factory:needs-qa`, not a code review.

## What the Reviewer reads, and what it does not

- The issue the PR closes, with its acceptance criteria.
- The diff, **at one head SHA**. Write the SHA down first; everything below is
  about that commit.
- The code around the diff, as deep as a claim needs (RULEBOOK § 1).

Not the Developer's reasoning, its run, or its notes. The review is
independent because it asks a different question with a different credential;
a reviewer handed the author's argument tends to confirm it (NT-013 § Roles).

## The review

One review per round, in this order.

```markdown
Review of <head SHA, full 40 characters>.

## Findings

### P1: <the defect, as one sentence stating the consequence>

**Where.** `internal/engine/runner.go`, `Runner.plannerFor`, and the caller
that bypasses it.

**Reproduction.** The concrete sequence that shows it, checked in the code:
1. A copilot run exhausts the declared provider and records `model_fallback`.
2. The agent is promoted to autonomous.
3. The next turn plans with the fallback: nothing on that path reads the stage.

**Why it matters.** Which invariant, guardrail or promise in NT/PRD/issue it
breaks, and who is hurt.

**What would close it.** Optional. A direction, not a patch: the author owns
the fix.

### P2: <...>

### P3: <...>

## Confirmed correct

- <a mechanism the change relies on, read and found right, with where>
- <a case the reviewer tried to break and could not, and how>

## Validation

- `go build ./... && go vet ./...`: clean / what failed
- `go test -count=1 ./internal/ledger/ ./internal/engine/` against Postgres: pass / fail
- <anything not run, and why>

## Verdict

REQUEST_CHANGES: P1 and the first P2 block; the P3 can land with them.
```

### Ranking

| Rank | Meaning | Blocks the verdict |
|---|---|---|
| **P1** | Breaks correctness, an invariant (`CLAUDE.md` § Non-negotiable rules), a guardrail of NT-013, or a security or privacy property, in a path that is reachable today. A race, a lost write, a bypass of the Gate, a promise the PR makes and does not keep. | yes |
| **P2** | Wrong in a narrower but real case: a scope that leaks, an input the immutable ledger should never have accepted, a missing index on a probe every request runs, a failure that loses the cause it should carry. | yes |
| **P3** | Hygiene that a reader pays for: a misleading name, an invisible character in a source file, a comment that states the wrong rule. | no, on its own |

Rank by consequence, not by size of the fix. A one-line fix can be a P1.

### Each finding has a reproduction, verified in the code

A finding without a reproduction is an opinion. The reproduction is concrete:
the inputs, the order of events, and the line where it goes wrong, or a test
the author can paste and watch fail. The replies on the rounds that mattered
show what that buys: on #185 the cleanup race became a test that holds a
competing transaction open under READ COMMITTED; on #186 the retry that
stopped finding its run became a test of a retry after the input was erased;
on #189 the author wrote "the worker test follows your reproduction". A
reproduction is the author's first red.

**Never assert from the diff alone.** Before writing a finding, follow the path
in the code (RULEBOOK § 1) and confirm it happens. If you could not confirm it,
say so in the finding, and do not rank it above P3.

### What is confirmed correct

Say what was checked and found right. It tells the author what not to touch,
tells the next reviewer what was covered, and turns "no findings" from an
absence into a statement.

### The validation run

What the reviewer ran, on which SHA, and what it said. What was not run is
named, with the reason (no Postgres in the environment, no console changes).

### One verdict

- **APPROVE** when there is no P1 and no P2. Remaining P3s are listed; the
  author may fix them in the same PR.
- **REQUEST_CHANGES** when there is at least one P1 or P2.

There is no "approve with comments" for a P1 or a P2, and no verdict that
depends on a promise to fix it later.

## Submitting the verdict

The verdict is a **pull-request review**, submitted with `commitID` set to the
head SHA the review was written against (`pull_request_review_write` with
method `create`), event
`APPROVE` or `REQUEST_CHANGES`, with the whole review above as its body.

- **Not a comment, not a label.** The person who merges reads two approving
  reviews on the PR's current head, the Reviewer's and QA's, each from that
  role's own GitHub App. A comment that says "approved" proves nothing, and a
  label survives a new push. A review carries the SHA it was written against,
  so a review of an older commit is visibly stale.
- **If the head moved while reviewing**, the review is about a commit that is no
  longer the PR. Do not submit it against the new head: start again on the new
  SHA.
- Then label the PR: the Reviewer `factory:changes-requested` or
  `factory:needs-qa`, QA `factory:changes-requested` or
  `factory:ready-to-merge` (LANES.md). The label routes the card; the review is
  the evidence.

The rounds on the PRs above were delivered outside GitHub: those PRs carry the
author's replies but no review objects. That is exactly what the factory
cannot do, because nothing could verify such a verdict, not a person reading
the PR and not a later automated merge.

## A re-review

A new round on the new head SHA, in the same format, with one section first:

```markdown
## Previous findings

- P1 <title>: fixed in <commit>. Verified by <how: the reproduction now
  fails at …, the mutant the author reported was re-applied and the test went red>.
- P2 <title>: not fixed. <what is still reachable>.
```

New findings follow, ranked as usual. A re-review that finds a new P1 in the
fix is normal: on #189 the second round found that the fix needed an index and
had lost the provider's failure from the parking.

## The author's reply

The Developer answers on the PR, one section per finding, in rank order:

```markdown
### P1: <the finding's title> — `<commit sha>`

**Root cause.** <one paragraph, verified in the code>.

**Fix.** <what changed and where, and why there rather than elsewhere>.

**Red on the previous code:**
    --- FAIL: TestX_condition_expectation
        file_test.go:NN: <the assertion's message>

**Sabotage** (each compiles, anchor matched once, restored by `cp` and
checked with `sha256sum -c`):

| Mutant | Fails |
|---|---|
| <the smallest edit that removes the fix> | <test and failure line> |

### Checks
- `go build ./... && go vet ./...` clean; tests run and against what; not run and why.
```

One commit per finding (RULEBOOK § 4). A finding the author disagrees with is
answered with the evidence, not ignored: the reviewer then confirms or
withdraws it in the next round.
