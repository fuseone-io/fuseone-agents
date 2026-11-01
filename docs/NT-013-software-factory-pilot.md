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

## v1: the pilot without factory code

Amended 2026-10-31. The first version of the pilot runs entirely inside the
platform, with no code written for the factory:

- **Roles are configured in the platform.** Each role is an agent created in
  the console or the Studio, from the reference text under `factory/` (the
  rulebook, the review format, the lane definitions of done, and one reference
  specification per role). The repository holds what is reviewed; the console
  holds what runs.
- **The official GitHub MCP server is the only tool server**, registered once
  per role, each registration with its own GitHub App installed only on this
  repository. The App's identity is what signs a role's comments and reviews.
- **Copilot everywhere, and a person merges.** Every role runs at copilot with
  `standing_approvals: forbidden`, and once it has chosen a card from the board
  its first act is the work-item claim, before it reads the issue or writes.
  Choosing from the board shows Triage the cards' titles — the board listing
  always returns them — so a title is the one untrusted text a role reads
  before its claim (#220). Triage is told to use it only to tell cards
  apart, but a title can still sway the choice, and the claim is the one
  write that follows unapproved (it is a Gate coordination exemption). A
  wrong claim only holds that card for this role while the run is open,
  releases when the run ends and puts the run's cost on that card; it writes
  nothing to GitHub, and every GitHub write after it waits for a person. An
  atomic pick-and-claim tool that would hide the title is deferred.
  The Reviewer's and QA's verdicts are both pull-request reviews on the head
  SHA they read (APPROVE or REQUEST_CHANGES), under their own Apps. A person
  merges once both approve the current head and its checks are green. Cards
  move with the GitHub MCP server's projects tool, classified as a write, so
  every move is a proposal a person approves.

**Deferred** until the pilot's data justifies automating the merge: the merge
wrapper (`merge_if_ready`) and the Merger role, the `factory/qa` commit status
(`record_qa_verdict`), the ruleset pinning each signal to its producer, a
narrow `move_card` tool, and publishing role specifications from the
repository. The sections below that describe them are kept, marked
*deferred in v1*, as the design to return to. The factory's own MCP server
(`factory/mcp`, #211) exists in the repository and is not used in v1.

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
| **Reviewer** | PRs labelled `factory:needs-review` | reviews the diff; findings ranked P1–P3, each with a reproduction | a pull-request review — APPROVE or REQUEST_CHANGES — on the head SHA it read; labels | copilot |
| **QA** | PRs labelled `factory:needs-qa` | checks every acceptance criterion against the PR and its CI | its verdict as a commit status `factory/qa` on the head SHA, under its own identity; labels. *In v1: a pull-request review on the head SHA, like the Reviewer's* | copilot |
| **Merger** (*deferred in v1: a person merges*) | PRs labelled `factory:ready-to-merge` | proposes `merge_if_ready` for the PR's current head; the wrapper re-checks review, QA and checks on that SHA when it runs | merge (through the wrapper), card status | copilot, merge always human-approved |
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
act is a platform claim** — `$fuseone.work_item.claim(<card id>)` — which
holds two uniquenesses, atomically:

- **one card per run**: the same run claiming the same card again is an
  idempotent retry; the same run claiming a *different* card is refused, so a
  run talked into grabbing the queue cannot hold more than one card;
- **one run per card per agent**: another **open** run of the same agent
  holding that card refuses the claim (parked for approval counts as open), so
  the role picks the next card.

It records the card on the run in the ledger as a queryable work-item key. A
claim is held only while its run is non-terminal — derived from the run's
phase, not released by a separate write — so a crash cannot leave an orphan.
It is a platform-owned tool: listed explicitly as one, never advancing a step,
and outside the MCP availability checks. Platform-owned settles envelope and
availability, not effect, so its passage through the Gate is stated rather
than inferred: the claim is an **internal coordination operation**. It changes
only the platform's own record of who works on what, so it passes without
approval even in a tainted run — as a write it would park for a person before
reserving and reopen the race — and it is never served from a read cache,
because its answer depends on every other open run at that moment.

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
to this repository — and its own toolsets. On the hosted server those are
chosen with the `X-MCP-Toolsets` header (`issues,projects` for Triage),
because the default set leaves `projects` out and a `/x/{toolset}` URL serves
a single set; `factory/agents/README.md` § Setting up a role's GitHub server
has the per-role table. Triage cannot push; only the Merger's credential
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
rest of the run. A tainted run needs a person for every write — except the
work-item claim, a coordination operation the Gate lets through (§ v1) — and
can never
perform a destructive action — approval does not release that block. Two
consequences shape the design:

- Every GitHub write in the pilot is approved by a person; the claim, which
  writes only the platform's record of who holds a card, is not. That is the
  measurement:
  how many proposals were right, how many were edited, how many refused.
- **The Merger must not read free text.** If it read a comment, it could not
  merge.

*Layers 5 and 5a, and the merge policy in layer 6, are deferred in v1: a person
merges, reading the two approving reviews and the checks on the current head.*

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

- an approving review by an authorised reviewer identity on that SHA — the
  Reviewer submits it as a pull-request review with `commitID` set to the head
  it read;
- the QA verdict on that SHA: a `factory/qa` commit status set by the QA
  role's identity, which only that identity can write;
- the expected set of checks, completed successfully, emitted by the
  authorised apps, on that SHA;

and then merges with `expectedHeadSha`, so a push between the check and the
merge refuses rather than merging unreviewed code. Every signal is validated by
its actor or app, not only by the absence of free text.

`expectedHeadSha` closes a new push; it does not close a signal that changes on
the same commit — a review dismissed, a check returning to pending — between
the wrapper's read and the merge. Only GitHub can check that atomically, at
the merge itself, so the repository enforces the same conditions:

**5a. A ruleset on `main`, pinned to who produces each signal.** Today `main`
has neither branch protection nor a ruleset. The pilot requires one, and a
status name proves nothing — anyone with statuses write can publish
`factory/qa` — so every required signal is pinned to its source:

- `factory/qa` required **from the QA GitHub App** (its `integration_id`), which
  is why QA's identity must be a GitHub App rather than a token;
- every CI check required from GitHub Actions' app, and `build` included;
- the required review restricted to the same reviewer set `merge_if_ready`
  authorises (code owners or a named team), so a dismissed agent review cannot
  be replaced by any other approval;
- `strict_required_status_checks_policy`, so a head tested against an older
  `main` must be brought up to date and tested again;
- bypass only for a named maintainers team, in pull-request mode — manual
  squash merges keep working and direct pushes do not — and never for the
  Merger's credential.

The wrapper's own checks then fail fast and explain; the ruleset is what makes
a stale read harmless. The wrapper also refuses mechanically any repository
other than this one and any base other than `main`, since the ruleset protects
nothing else.

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
pull request like any change, and published from there. *In v1 they are
reference text, copied into the console; publishing from the repository is
deferred.*

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
| Standing approvals refused for every factory write | layer 6 | every role |
| Merge policy (*deferred in v1*) | layer 6 | Merger |
| `merge_if_ready`: a merge bound to the reviewed head, signals validated by actor (*deferred in v1*) | layer 5 | Merger |
| A ruleset on `main` requiring review, `factory/qa` and CI, no bypass for the Merger (*deferred in v1*) | layer 5a | Merger |
| A trusted way for QA to set `factory/qa` under its own identity (*deferred in v1*) | the evidence `merge_if_ready` requires | QA, Merger |
| Role references and the rulebook under `factory/` | the harness | every role |
| Role specs published from the repository (*deferred in v1*) | the registry never drifts from what was reviewed | — |
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
