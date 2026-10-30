# NT-013 — A software factory, piloted on this repository

Status: proposed · 2026-10-30 · extends #154

## What this is

Agents that take this repository's issues from triage to merge, each in a role
with the narrowest authority that role needs, driven by a GitHub Project board,
under this platform's Gate. #154 maps the capability in levels; this note
fixes the shape of the pilot: the roles, the board, the guardrails and the
harness, and what the platform must gain before the pilot can run unattended.

The pilot is this repository because the process already exists here, done by
hand: an issue is triaged, a change is written test-first, an independent
review returns findings with reproductions, the author answers them, and a
merge happens only on a pasted approval. The factory encodes that process; it
does not invent one.

## Roles

Each role is one agent specification, versioned in this repository and
published like any other agent. A role is defined by four things: the lanes it
reads, the tools it holds, the stage it runs at, and the instructions that make
its work good — the harness, below.

| Role | Reads | Does | Writes | Stage in the pilot |
|---|---|---|---|---|
| **Triage** | lane *Triagem* | classifies type and size, finds duplicates, asks what is missing | issue comment, labels, card status | copilot |
| **Architect** | *Triagem* items marked `factory:needs-design` | writes the plan and acceptance criteria, splits large items | issue comment, sub-issues, card status | copilot |
| **Developer** | lane *Doing* | implements, opens the PR, answers review findings | branch, commits, PR | copilot (Level 1 only) |
| **Reviewer** | PRs labelled `factory:needs-review` | reviews the diff; findings ranked P1–P3, each with a reproduction | PR review comments, labels | copilot |
| **QA** | PRs labelled `factory:needs-qa` | checks every acceptance criterion against the PR and its CI | PR comment, labels | copilot |
| **Merger** | PRs labelled `factory:ready-to-merge` | proposes `merge_if_ready` for the PR's current head; the wrapper re-checks review, QA and checks on that SHA when it runs | merge (through the wrapper), card status | copilot, merge always human-approved |
| **Release** (later) | merged work since the last tag | cuts the changelog, tags, bumps the platform repo | changelog commit, tag | not in the pilot |

Reviewer and QA are separate on purpose. In this repository's history the
independent review is what found the defects that tests written by the author
did not — a race in an image cleanup, an idempotent retry that stopped finding
its run, a fallback that promotion could unlock. A second reader with a
different question is the guardrail; collapsing it into the author's role
removes it.

## The board

A GitHub Project with a single-select **Status** field:

| Lane | Meaning | Who moves a card out |
|---|---|---|
| **Triagem** | new, not yet understood | Triage (→ Preparado or Travado), Architect when design is needed |
| **Preparado** | understood: type, size, plan and acceptance criteria written | a person, by pulling it into Doing — the pilot's WIP control |
| **Doing** | being built and reviewed | Merger (→ Done), any role (→ Travado) |
| **Travado** | blocked, with the reason as the last comment | a person only |
| **Done** | merged | — |

Sub-states inside *Doing* are labels, not lanes, so the board stays the five
lanes people read: `factory:needs-review`, `factory:changes-requested`,
`factory:needs-qa`, `factory:ready-to-merge`.

The board holds the card's state. The platform has no case spanning several
runs and agents today — tickets exist only for Slack support, and event
dispatch does not carry them — so in the pilot every run reads the card's
state from the board and the issue, acts once, and writes the outcome back.
What the platform must hold is narrower: which card each run is working on,
recorded in the ledger, as below.

## How a role is woken

Pilot: **cron, one card per run, claimed before any work.** Each role is a
scheduled agent that lists the cards in its lane and picks the oldest one it
has not already acted on. Cron triggers exist, and a scheduled run carries the
moment it was fired for.

Picking is not owning. Each firing gets a fresh idempotency key, and the
engine's idempotency is scoped to a run, so nothing stops two runs of one role
choosing the same card: while the first waits for a person to approve its
comment, the board has not changed and the next firing picks the same card,
duplicating cost and, if both are approved, comments, branches or PRs.
Writing to the board afterwards does not close that race. So a run's **first
act is a platform claim** — `work_item.claim(<card id>)` — which:

- records the card on the run, in the ledger, as a queryable work-item key;
- refuses when another **open** run of the same agent already holds that card
  (parked for approval counts as open), so the role picks the next card;
- is a platform tool with no effect outside the platform, classified as such.

The same key is what joins cost, approvals and outcomes to a card, including
runs that fail before their first write.

Not webhooks, yet. The platform's webhook route authenticates with an
`X-FuseOne-Secret` header and requires an `Idempotency-Key`; GitHub sends
neither, and the route does not verify `X-Hub-Signature-256` or read
`X-GitHub-Event`. A GitHub adapter is listed below as platform work.

Not agent-to-agent events, yet. `emits`/`event` exist, but only a direct
self-loop is refused: an A → B → A chain has no hop limit, and the factory's
developer ⇄ reviewer exchange is exactly that cycle. Until a chain has a depth
bound, the board is the handoff.

## Guardrails

Defence in depth: each layer assumes the one above it failed.

**1. GitHub's own permissions, per role.** Each role registers its own GitHub
MCP server with its own credential — a fine-grained token or GitHub App scoped
to this repository — and its own toolset (`GITHUB_TOOLSETS`, `--read-only`
where the role only reads). Triage cannot push; only the Merger's credential
can merge. A prompt injection that talks a role into an action meets a token
that cannot perform it.

**2. The capability pack and steps.** A role's `tools` lists only what it
needs; steps narrow it further, and a step can no longer reach past the pack.

**3. Classification.** The GitHub MCP server's tools enter unclassified and are
refused until the Curator rules on them. `merge_pull_request` is destructive;
branch, commit, PR, comment, label and card changes are writes; reading issue,
PR and comment text is a read marked **untrusted**. Some GitHub tools bundle
several actions behind one name (`projects_write` adds, updates and deletes
items); the classification takes the most dangerous action the tool can
perform.

**4. Taint.** This repository is public: anyone can write an issue or a
comment, so their text is untrusted, and a run that read it is tainted for the
rest of the run. A tainted run needs a person for every write and can never
perform a destructive action — approval does not release that block. Two
consequences shape the design:

- Every write in the pilot is approved by a person. That is the measurement:
  how many proposals were right, how many were edited, how many refused.
- **The Merger must not read free text.** If it read a comment, it could not
  merge.

**5. A merge bound to the head that was reviewed.** Structured is not the same
as trustworthy, and a decision is not the same as its execution. A label, a
review and a check can each belong to a different commit — a label survives a
new push — and an approval can wait while the PR changes underneath it: on
resume, the engine executes the arguments frozen at proposal time without
reading GitHub again. So the Merger does not call `merge_pull_request`
directly. It proposes **`merge_if_ready(pr, head_sha)`**, a deterministic
wrapper that, **when it executes** (after the person approved), re-reads
GitHub and fails closed unless all of these hold for that exact `head_sha`,
which must still be the PR's head:

- an approving review by an authorised reviewer identity on that SHA;
- the QA verdict on that SHA, recorded by the QA role's identity;
- the expected set of checks, completed successfully, emitted by the
  authorised apps, on that SHA;

and then merges with `expectedHeadSha`, so a push between the check and the
merge refuses rather than merging unreviewed code. Every signal is validated by
its actor or app, not only by the absence of free text.

**6. Policy, and no standing approvals in the pilot.** An authored policy
escalates the merge to approval (a destructive call is blocked outright by
default). Every approval request first tries to consume a standing grant, so
a grant for any factory tool would release that write without a person and
corrupt the measurement. **During the pilot no standing approval may cover
any factory write** — comments, labels, commits, PRs, board changes and the
merge — enforced, not merely avoided.

**7. Budget.** A per-run ceiling per role, and a scope budget for the factory's
area, so a looping role parks instead of spending.

**8. The ledger.** Every card's history is reconstructible through the
work-item key the claim records: which role read what, proposed what, who
approved, what it cost.

## The harness

What makes a role do its job *well* rather than merely within bounds. It is
code: role instructions live in this repository under `factory/`, reviewed by
pull request like any change, and published from there.

- **A shared rulebook.** The engineering rules every role follows are the ones
  `CLAUDE.md` and `web/CLAUDE.md` already state: test first and show it fail,
  break the mechanism to prove a test bites, one commit per logical change,
  size limits, i18n parity, English in the repository.
- **Per-role instructions** that encode how this repository's people do the
  job. The Reviewer ranks findings P1/P2/P3, gives a reproduction for each,
  verifies a claim in the code before stating it, and states a verdict per PR.
  The Developer answers each finding with a commit and a PR comment carrying
  the red and the sabotage evidence.
- **Definitions of done per lane**, checked by the role that moves the card:
  Preparado needs type, size, plan and acceptance criteria; ready-to-merge
  needs an approving review, QA passed and every expected check green — all on
  the PR's current head, which the merge wrapper re-checks rather than trusts.
- **Evaluation before promotion.** Each role gets a regression corpus built
  from this repository's real history — issues that were triaged, reviews that
  found real defects — and stays at copilot until its agreement with people is
  measured, as for any agent.

## Platform work the pilot depends on

| Need | Why | Blocks |
|---|---|---|
| GitHub Project board and labels | the lanes and sub-states above | everything |
| GitHub MCP per role, classified | layers 1 and 3 | every role |
| A durable work-item claim, recorded in the ledger | no two runs of a role on one card; cost per card | every role |
| Merge policy, and standing approvals refused for every factory write | layer 6 | every role |
| `merge_if_ready`: a merge bound to the reviewed head, signals validated by actor | layer 5 | Merger |
| Role specs and the rulebook under `factory/`, published from the repository | the harness | every role |
| A Level 1 developer: GitHub-API commits for small changes | #154 Level 1 | Developer |
| A coding runtime behind one MCP tool | #154 Level 2: build and test before a PR | Developer and QA beyond chores |
| A GitHub webhook adapter (signature, delivery id, event filter) | event-driven roles instead of polling | after the pilot |
| A hop limit on event chains | agent-to-agent handoff | after the pilot |
| A case spanning runs and agents | the card carried across event-opened runs, beyond the claim's key | after the pilot |

## Measured during the pilot

Per role: proposals made, approved as-is, edited, refused; cost per card,
joined through the work-item key the claim records; time in each lane; review findings that a later stage or a person caught and the
role missed. The pilot's question is whether each role, at copilot, is right
often enough to be worth its approvals — answered per role, not for the
factory as a whole.

## Open questions

- Does the Developer start at Level 1 (small changes through the GitHub API) or
  wait for the coding runtime?
- One factory area with its own scope budget, or the roles inside an existing
  area?
- Should a card that bounced between Developer and Reviewer more than N times
  move to Travado automatically?
