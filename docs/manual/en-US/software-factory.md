---
title: Software factory
summary: Run agent roles that triage issues and review pull requests on a GitHub repository, each with its own GitHub App, every write approved by a person.
section: integrations
tags: factory, github, github app, roles, triage, review, qa, copilot
order: 22
---

# Software factory

A software factory is a set of agents, each in one **role**, that take a
repository's issues from triage to a pull request ready to merge, driven by a
GitHub Project board. The platform runs it with nothing but configuration:
the roles are ordinary agents, the official GitHub MCP server is their only
tool server, and the Gate governs every call they make.

This page is the operator's path. The reasoning behind it is design note
[NT-013](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/NT-013-software-factory-pilot.md),
and the role instructions live, reviewed like code, in
[`factory/`](https://github.com/fuseone-io/fuseone-agents/tree/main/factory).

## What the first version is

- **Three roles.** Triage turns a new issue into a prepared card; the
  Reviewer reviews a pull request; QA checks it against CI and the acceptance
  criteria. Each role is an agent created in the console or the Studio.
- **One GitHub App per role.** Each role reaches GitHub only through its own
  registration of the GitHub server, authenticated as its own App. The App's
  permissions are the first limit, and its name is what signs the role's
  comments and reviews.
- **Every role at copilot**, with standing approvals forbidden. Every write a
  role proposes — a comment, a label, a review, a card move — waits for a
  person. See [Draft, copilot and autonomous](autonomy-stages.md).
- **A person merges.** No role merges.
- **The board drives the work**, with five lanes: Triagem, Preparado, Doing,
  Travado and Done. Inside Doing, labels mark where a pull request is:
  `factory:needs-review`, `factory:changes-requested`, `factory:needs-qa` and
  `factory:ready-to-merge`. Only a person takes a card out of Travado.
- **One card per run.** A role wakes on its schedule, chooses a card, claims
  it with `$fuseone.work_item.claim` and works only that card. The claim is
  recorded in the run's trail, and no other run of the same role can take a
  card that is held.

The repository is public, so issue and pull request text is untrusted: a run
that read it can never take a destructive action, and its writes need a
person.

## 1. Create the GitHub Apps

Create one GitHub App per role, install it only on the repository the factory
works, and give it only what its tools need:

| Role | Repository permissions | Organisation permissions |
|---|---|---|
| Triage | Issues: write, Pull requests: read, Metadata: read | Projects: write |
| Reviewer | Pull requests: write, Issues: write, Contents: read, Checks: read, Commit statuses: read, Metadata: read | Projects: write |
| QA | Pull requests: write, Issues: write, Contents: read, Checks: read, Commit statuses: read, Metadata: read | Projects: write |

QA's verdict is a pull request review, so QA needs Pull requests: write just
like the Reviewer. Generate a private key for each App and note its App id
and installation id.

## 2. Register the GitHub server once per role

In the administration, register the hosted GitHub MCP server
(`https://api.githubcopilot.com/mcp/`) three times, one per role, with the id
`github-<role>`: `github-triage`, `github-reviewer`, `github-qa`. Choose the
**GitHub App** credential and fill in that role's App (see
[MCPs and credentials](mcp-and-credentials.md), section "GitHub App").

The hosted server's default toolsets do not include `projects`, so set the
**Toolsets** field (sent as `X-MCP-Toolsets`) on every registration:

| Role | Toolsets |
|---|---|
| Triage | `issues,projects` |
| Reviewer | `pull_requests,repos,issues,projects` |
| QA | `pull_requests,repos,issues,projects` |

The hosted server silently ignores a toolset name it does not know, so a typo
shows up later, as a publish refused for missing tools.

## 3. Classify the tools

Once each server is answering, classify every tool the roles use (see
[What the platform stops before it happens](what-the-gate-stops.md)). Reading issue, pull request
and comment text is a read whose result is **untrusted**. Comments, labels,
reviews and card moves are **writes**.

`projects_write` moves cards and can also remove an item. Classify it as a
**write**, not destructive: a removed card can be added back, and a person
approves every write. Classified destructive, it would be blocked outright,
and every role that reads issue text and moves cards would be refused at
publish.

## 4. Create the factory's area

Create an area for the factory, so its cost, budget and grants stay apart
from every other agent, and give it a budget (see
[Costs and limits](costs-and-limits.md) and
[Companies, areas and roles](scopes-and-roles.md)). The references use the area id
`factory`.

Each reference also carries a per-run ceiling, so a looping role parks
instead of spending:

| Role | Schedule | Ceiling per run | Tool calls | Steps |
|---|---|---|---|---|
| Triage | every hour at :00 | 500,000 micros | 30 | 60 |
| Reviewer | every hour at :15 | 2,000,000 micros | 80 | 160 |
| QA | every hour at :45 | 1,000,000 micros | 50 | 100 |

Micros are millionths of the installation's configured currency. Treat these
values as a first calibration.

## 5. Create the roles

For each role, create the agent in the Studio from its reference: the
frontmatter's fields, and the body as its instructions.

- [Triage](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/triage.agent.md)
- [Reviewer](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/reviewer.agent.md)
- [QA](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/qa.agent.md)

Then publish it — a new agent is a paused draft — set its stage to
**copilot** and start it. The stage is not in the file, so it is always set in
the console.

The console refuses to publish a tool the registered server does not offer, so
a wrong tool name, or a missing toolset, is found here rather than at run
time.

What the roles follow is in the same directory:
[RULEBOOK](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/RULEBOOK.md)
(the rules every role follows),
[REVIEW](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/REVIEW.md)
(the review format) and
[LANES](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/LANES.md)
(what done means for each lane and label). The
[role references README](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/README.md)
explains every line of a reference.

## Changing a role

A change to a role is made in the reference first, reviewed by pull request,
and then copied into the console. When the console and the reference
disagree, the console has drifted.

After changing a registration's toolsets, check the server's discovered tools
before trusting a publish. The tool catalogue only grows: a tool the server
stopped offering stays listed, and the publish check still accepts it, so the
role would fail at its first call instead (#219).
