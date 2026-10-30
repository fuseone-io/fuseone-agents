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
| **Merger** | PRs labelled `factory:ready-to-merge` | verifies approvals and green checks, merges | merge, card status | copilot, merge always human-approved |
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

The board is the work item. The platform has no case spanning several runs and
agents today — tickets exist only for Slack support, and event dispatch does
not carry them — so in the pilot every run reads the card's state from the
board and the issue, acts once, and writes the outcome back. A run holds no
memory of the card that the board does not.

## How a role is woken

Pilot: **cron, one card per run.** Each role is a scheduled agent that lists
the cards in its lane, takes the oldest one it has not already acted on, and
works only that card. Cron triggers exist, and a scheduled run already carries
the moment it was fired for.

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
- **The Merger must not read free text.** It decides from structured signals
  only — check conclusions, review states, labels, the base branch — through
  tools classified as trusted reads. If it read a comment, it could not merge.

**5. Policy.** An authored policy escalates `merge_pull_request` to approval
(by default a destructive call is blocked outright). A standing approval must
never cover it.

**6. Budget.** A per-run ceiling per role, and a scope budget for the factory's
area, so a looping role parks instead of spending.

**7. The ledger.** Every card's history is reconstructible: which role read
what, proposed what, who approved, what it cost.

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
  needs an approving review, QA passed and every check green.
- **Evaluation before promotion.** Each role gets a regression corpus built
  from this repository's real history — issues that were triaged, reviews that
  found real defects — and stays at copilot until its agreement with people is
  measured, as for any agent.

## Platform work the pilot depends on

| Need | Why | Blocks |
|---|---|---|
| GitHub Project board and labels | the lanes and sub-states above | everything |
| GitHub MCP per role, classified | layers 1 and 3 | every role |
| Merge policy, and a way to forbid standing approval for a tool | layer 5 | Merger |
| Trusted structured reads for the Merger | layer 4 | Merger |
| Role specs and the rulebook under `factory/`, published from the repository | the harness | every role |
| A Level 1 developer: GitHub-API commits for small changes | #154 Level 1 | Developer |
| A coding runtime behind one MCP tool | #154 Level 2: build and test before a PR | Developer and QA beyond chores |
| A GitHub webhook adapter (signature, delivery id, event filter) | event-driven roles instead of polling | after the pilot |
| A hop limit on event chains | agent-to-agent handoff | after the pilot |
| A case spanning runs | the card as a first-class work item | after the pilot |

## Measured during the pilot

Per role: proposals made, approved as-is, edited, refused; cost per card; time
in each lane; review findings that a later stage or a person caught and the
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
