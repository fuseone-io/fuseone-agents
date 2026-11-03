---
title: Software factory
summary: Run agent roles that triage issues, plan them, make small changes and review pull requests on a GitHub repository, each with its own GitHub App, every GitHub write approved by a person.
section: integrations
tags: factory, github, github app, roles, triage, architect, plan, developer, review, qa, copilot
order: 22
---

# Software factory

A software factory is a set of agents, each in one **role**, that take a
repository's issues from triage to a pull request ready to merge, driven by a
GitHub Project board. The platform runs it with nothing but configuration:
the roles are ordinary agents, their tools come from the official GitHub MCP
server and from the factory's own MCP server, which moves the board's cards
and records QA's verdict, and the Gate governs every call they make.

This page is the operator's path. The reasoning behind it is design note
[NT-013](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/NT-013-software-factory-pilot.md),
and the role instructions live, reviewed like code, in
[`factory/`](https://github.com/fuseone-io/fuseone-agents/tree/main/factory).

## What the first version is

- **Five roles.** Triage turns a new issue into a prepared card, or hands it
  to the Architect with `factory:needs-design` when it needs a plan; the
  Architect verifies the change in the code and writes its plan and
  acceptance criteria on the issue; the Developer makes a small change and opens its pull request; the Reviewer
  reviews a pull request; QA checks it against CI and the acceptance
  criteria. Each role is an agent created in the console or the Studio.
- **One GitHub App per role.** Each role reaches GitHub only through its own
  registrations, of the GitHub server and of the factory's server, both
  authenticated as its own App. The App's permissions are the first limit,
  and its name is what signs the role's comments, reviews and QA's
  `factory/qa` status.
- **Narrow board tools.** Cards move with the factory server's `move_card`,
  only along the role's own transitions (Triage and the Architect from
  Triagem to Preparado or Travado; the Developer, the Reviewer and QA from
  Doing to Travado), and nothing leaves Travado. Triage and the Architect set
  Work type and Size with `set_card_fields`, only in Triagem. No role holds
  the GitHub server's `projects_write`.
- **Every role at copilot**, with standing approvals forbidden. Every GitHub
  write a role proposes — a comment, a label, a review, a card move — waits
  for a person. The card claim below, and the Developer's link of the pull
  request it just opened, are the writes that do not, and neither writes to
  GitHub. See [Draft, copilot and autonomous](autonomy-stages.md).
- **A person merges.** No role merges.
- **The board drives the work**, with five lanes: Triagem, Preparado, Doing,
  Travado and Done. Inside Doing, labels mark where a pull request is:
  `factory:needs-review`, `factory:changes-requested`, `factory:needs-qa` and
  `factory:ready-to-merge`. A card the Developer cannot do without a build
  carries `factory:needs-runtime` on its issue. Only a person takes a card out
  of Travado.
- **One card per run.** A role wakes on its schedule, chooses a card, claims
  it with `$fuseone.work_item.claim` and works only that card. The claim is
  recorded in the run's trail, and no other run of the same role can take a
  card that is held. Before the claim, Triage is told to list its lane only
  with `projects_list`, method `list_project_items`, so it sees the cards'
  titles, which the board listing always returns. The same Projects tools can
  also return the board's status updates, free text, and nothing in the
  platform stops a model from asking for them, because the Gate classifies a
  tool, not its method. The pilot accepts this within the claim's limits
  below; preventing it needs the narrow tool that is deferred. A title is
  untrusted text:
  the role is told to use it only to tell cards apart and to choose by age
  and label, and it reads the issue's body and comments after the claim. A
  title can still sway which card the model picks, and the claim needs no
  approval, even after untrusted text: it is a coordination operation, so two
  runs never wait on a person to reserve a card. A claim on the wrong card
  only holds that card for this role while the run is open, releases it when
  the run ends, and puts the run's cost on that card. It writes nothing to
  GitHub, and every GitHub write after it waits for a person. A tool that
  picks and claims a card in one step, hiding even the title, is deferred.
  The Architect lists the Triagem lane, and the Developer the Doing lane,
  the same way, and both are told the same.
- **A card's trail.** The claim's **Item** line in a run's trail links to
  every run that claimed the card, with each role's approvals and cost (see
  [Reading a run](reading-a-run.md)). The issue and the pull requests linked
  to it are joined: right after it opens a pull request, the Developer links
  it to the issue it claimed with `$fuseone.work_item.link`, and from then on
  the Reviewer's and QA's runs on that pull request are on the issue's trail,
  marked **Pull request**. Opening the trail of a linked pull request shows
  the issue's trail and says so. The link waits for no one when the run's own
  `create_pull_request` named that pull request — answered by the hosted GitHub
  server registered with the role's GitHub App — and for a person otherwise.
  Keep `create_pull_request`'s result **untrusted**, the default: the link is
  proved by the pull request the platform reads out of the answer, not by the
  answer, which goes on tainting the run so every later write still waits for
  a person;
  a pull request belongs to one card for good, and a link you cannot read is
  shown to you as no link at all.

The repository is public, so issue and pull request text is untrusted: a run
that read it can never take a destructive action, and its GitHub writes need
a person.

## 1. Create the labels

The board routes pull requests and issues by labels, and GitHub applies only
labels the repository already defines. A role that sets a label that was never
created fails at that write, and the card stays where it was, to be picked
again on the next run. Create every factory label once, from a clone of the
repository the factory works (or add `--repo <owner>/<name>`). `--force`
makes each line safe to run again:

```sh
gh label create factory:needs-design --force --color fbca04 --description "Triage asks the Architect for a plan"
gh label create factory:needs-review --force --color fbca04 --description "PR waits for the Reviewer"
gh label create factory:changes-requested --force --color fbca04 --description "Reviewer or QA asked for changes"
gh label create factory:needs-qa --force --color fbca04 --description "PR waits for QA"
gh label create factory:ready-to-merge --force --color fbca04 --description "Review and QA approved the head; a person merges"
gh label create factory:needs-runtime --force --color fbca04 --description "Needs a build or a test run before its PR; waits for Level 2"
```


## 2. Create the GitHub Apps

Create one GitHub App per role, install it only on the repository the factory
works, and give it only what its tools need:

| Role | Repository permissions | Organisation permissions |
|---|---|---|
| Triage | Issues: write, Pull requests: read, Metadata: read | Projects: write |
| Architect | Contents: read, Issues: write, Metadata: read | Projects: write |
| Reviewer | Pull requests: write, Issues: write, Contents: read, Checks: read, Commit statuses: read, Metadata: read | Projects: write |
| QA | Pull requests: read, Issues: write, Contents: read, Checks: read, Commit statuses: write, Metadata: read | Projects: write |
| Developer | Contents: write, Pull requests: write, Issues: write, Metadata: read | Projects: write |

QA's verdict is the `factory/qa` commit status on the head it checked, so its
App needs Commit statuses: write. It submits no review, so it needs only Pull
requests: read: it comments its verdict and relabels the pull request through
the Issues API, which Issues: write covers (the canary in step 3 proves it).
The Architect reads the code and writes only issues and the
board: give its App Contents: **read**, never write, so GitHub refuses any
file or branch it could be talked into writing. Do not give the Developer's App the Workflows permission:
without it, GitHub refuses any change the Developer makes under
`.github/workflows`. Generate a private key for each App and note its App id
and installation id.

Contents: write reaches every branch, `main` included. The Developer is told to
write only to branches it creates under `factory/`, and the person approving
each write sees the branch it names, but neither stops a write. So a ruleset
is a prerequisite, not a recommendation:

Before the Developer is started, main must carry the minimal ruleset: a pull request required before merging, force pushes and deletions blocked, and no bypass for any factory GitHub App.

Until it exists, leave the Developer paused. In the repository's Settings →
Rules → Rulesets, create a branch ruleset with enforcement **Active**, target
`refs/heads/main`, **Require a pull request before merging** (zero approvals
is enough: a person still merges), **Block force pushes** and **Restrict
deletions** ticked, and no bypass actors, so no factory App can skip it. No
bypass includes people: a person's change, a release's changelog commit
included, reaches `main` through a pull request too, and the release tag goes
on the merged commit (the ruleset targets `refs/heads/main`, not tags). Or
through the API, replacing `<owner>/<name>`:

```sh
gh api --method POST repos/<owner>/<name>/rulesets --input - <<'EOF'
{
  "name": "main: pull request required",
  "target": "branch",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["refs/heads/main"], "exclude": [] } },
  "rules": [
    { "type": "pull_request", "parameters": {
        "required_approving_review_count": 0,
        "dismiss_stale_reviews_on_push": false,
        "require_code_owner_review": false,
        "require_last_push_approval": false,
        "required_review_thread_resolution": false } },
    { "type": "non_fast_forward" },
    { "type": "deletion" }
  ],
  "bypass_actors": []
}
EOF
```

The rest of what `main` should eventually require — status checks pinned to
their source, `factory/qa`, the authorised reviewers and the Merger's rules —
is deferred
([#207](https://github.com/fuseone-io/fuseone-agents/issues/207)); none of it
is needed in v1.

Before the Merger is activated, two lists must be applied and kept identical:
the ruleset's required checks and the Merger's `FACTORY_CHECKS`. Both are every
job of the `check` workflow, by the names GitHub reports:
`go`, `postgres`, `console`, `chart`, `factory-mcp-image`, `artifact`. The
ruleset part is #207; `FACTORY_CHECKS` is described in `factory/mcp/README.md`.

## 3. Register the tool servers once per role

In the administration, register the hosted GitHub MCP server
(`https://api.githubcopilot.com/mcp/`) five times, one per role, with the id
`github-<role>`: `github-triage`, `github-architect`, `github-reviewer`,
`github-qa`, `github-developer`. Choose the
**GitHub App** credential and fill in that role's App (see
[MCPs and credentials](mcp-and-credentials.md), section "GitHub App").

The hosted server's default toolsets do not include `projects`, so set the
**Toolsets** field (sent as `X-MCP-Toolsets`) on every registration:

| Role | Toolsets |
|---|---|
| Triage | `issues,projects` |
| Architect | `issues,projects,repos` |
| Reviewer | `pull_requests,repos,issues,projects` |
| QA | `pull_requests,repos,issues,projects` |
| Developer | `pull_requests,repos,issues,projects` |

The hosted server silently ignores a toolset name it does not know, so a typo
shows up later, as a publish refused for missing tools.

The toolsets offer more than a role declares — the `projects` set also
offers `projects_write`, which edits any field of any card and deletes cards,
and which no role holds. Once a server answers, open its **Tools and
surface** page and bring in exactly these tools, nothing else. A tool left
outside the surface does not exist for agents: a role naming it is refused at
publish, and a call to it is refused.

| Server | Its tools |
|---|---|
| `github-triage` | `projects_list,projects_get,issue_read,list_issues,search_issues,add_issue_comment,issue_write` |
| `github-architect` | `projects_list,projects_get,issue_read,search_issues,get_file_contents,search_code,list_commits,get_commit,add_issue_comment,issue_write` |
| `github-reviewer` | `search_pull_requests,pull_request_read,issue_read,get_file_contents,search_code,get_commit,pull_request_review_write,issue_write,add_issue_comment,projects_list,projects_get` |
| `github-qa` | `search_pull_requests,pull_request_read,issue_read,get_file_contents,issue_write,add_issue_comment,projects_list,projects_get` |
| `github-developer` | `search_pull_requests,projects_list,projects_get,issue_read,pull_request_read,get_file_contents,search_code,get_commit,create_branch,create_or_update_file,push_files,create_pull_request,add_issue_comment,issue_write` |

The hosted server can also be narrowed at its end: the `X-MCP-Tools` header
set to a row above, with **no** `X-MCP-Toolsets`, makes it list only those
tools (with both set, the tools are added to the sets instead). The console's
App panel only sets Toolsets, so that header is written through the
administration API; the surface is required either way.

### The factory's own server

The factory's server runs as a sidecar in every worker pod, on
`http://127.0.0.1:8091/`, holding no credential of its own: each request's
App token tells it which role is calling, and it serves only that role's
tools. How to deploy it, and the ConfigMap it reads, are in
[`factory/mcp/README.md`](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/mcp/README.md#as-a-worker-sidecar).
Before the first worker runs it, check that its image can be pulled without a
credential:

```sh
gh api /orgs/fuseone-io/packages/container/fuseone-agents-factory-mcp --jq .visibility
```

It must answer `public`; otherwise give the worker a GHCR pull secret.

Then register it five more times, with the id `factory-<role>`:
`factory-triage`, `factory-architect`, `factory-reviewer`, `factory-qa`,
`factory-developer`. The URL is `http://127.0.0.1:8091/` (plain `http://` is
accepted beside a GitHub App only to loopback), the credential is **GitHub
App**, and the App is **the same one** as that role's `github-<role>`. Set no
toolsets. Each registration then offers its role's tools:

| Server | Tools |
|---|---|
| `factory-triage`, `factory-architect` | `move_card`, `set_card_fields` |
| `factory-developer`, `factory-reviewer` | `move_card` |
| `factory-qa` | `move_card`, `record_qa_verdict` |

One that offers nothing, or another row, was given the wrong App.

**Run the operator canary before starting any role.** No test can prove what
GitHub does with a token restricted to one repository, so once per role's
App, with such a token, run the three calls of the
[setup canary](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/mcp/README.md#setup-canary-one-real-query-and-one-real-mutation)
against a throwaway card: the `viewer { databaseId }` query must answer that
role's bot id, the item query must return the project and the card's Status,
and the move mutation must answer the item with no errors. With QA's token,
also comment on and label a throwaway pull request through the Issues API
(the canary's fourth step): QA's App has Pull requests: read only, and this
proves Issues: write is enough for what QA writes there. A role whose canary
fails stays stopped.

## 4. Classify the tools

Once each server is answering, classify every tool the roles declare (see
[What the platform stops before it happens](what-the-gate-stops.md)).

**Every read tool is untrusted** — not only the ones that read an issue's
body. A listing returns text people wrote: `projects_list` returns the titles
of the board's issues and pull requests even when asked for no fields (#220).

| Classify as | Tools |
|---|---|
| read, **untrusted** | `projects_list`, `projects_get`, `search_pull_requests`, `search_issues`, `list_issues`, `issue_read`, `pull_request_read`, `get_file_contents`, `search_code`, `list_commits`, `get_commit` |
| write | `add_issue_comment`, `issue_write`, `pull_request_review_write`, `create_branch`, `create_or_update_file`, `push_files`, `create_pull_request` |
| write, result **not** untrusted | `move_card`, `set_card_fields`, `record_qa_verdict`, on the `factory-<role>` servers |

Classify each tool on every role's server: `github-triage.projects_list` and
`github-reviewer.projects_list` are separate entries. `issue_write` is also how
the Architect creates a sub-issue under the card's issue, so it stays a write
on `github-architect` too.

The Architect's `repos` toolset also offers file and branch writes, and no
header removes them on the same server: `X-MCP-Readonly` would remove its
issue writes as well. Its reference declares none of them, and its App's
Contents: read refuses them.

The factory's tools answer only structured state — closed reasons, lane and
field names, numbers, commit SHAs — never text anyone wrote on GitHub, so
their results are not untrusted. They are still writes: a person approves
every move, every field and every QA verdict.

Leave `projects_write` **unclassified**. No role declares it, it is outside
every surface, and an unclassified tool never executes.

## 5. Create the factory's area

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
| Architect | every hour at :20 | 2,000,000 micros | 80 | 160 |
| Reviewer | every hour at :15 | 2,000,000 micros | 80 | 160 |
| QA | every hour at :45 | 1,000,000 micros | 50 | 100 |
| Developer | every hour at :30 | 3,000,000 micros | 100 | 200 |

Micros are millionths of the installation's configured currency. Treat these
values as a first calibration.

## 6. Create the roles

For each role, create the agent in the Studio from its reference: the
frontmatter's fields, and the body as its instructions.

- [Triage](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/triage.agent.md)
- [Architect](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/architect.agent.md)
- [Reviewer](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/reviewer.agent.md)
- [QA](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/qa.agent.md)
- [Developer](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/developer.agent.md)

Then publish it — a new agent is a paused draft — set its stage to
**copilot** and start it. The stage is not in the file, so it is always set in
the console. Start the Developer only once `main` carries the minimal ruleset
(step 2).

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

## What the Architect does

The Architect takes the oldest Triagem card labelled `factory:needs-design`,
which Triage leaves to it. It checks every claim about the code in the code,
with read tools only, and writes the plan and its acceptance criteria as a
comment on the issue: the issue's body stays the requester's, and a later plan
is a new comment rather than an edit. A large item is split into sub-issues,
created under the card's issue. Then:

- with no decision open, it removes `factory:needs-design` and moves the card
  to Preparado, where a person pulls it into Doing;
- when a decision belongs to the owner, it writes the plan so far and each
  open decision as a question, @mentioning whoever decides so GitHub
  notifies them, and moves the card to Travado, keeping the label, so the
  card comes back to it once the question is answered and the board operator
  returns it to Triagem, as its comment's last line says (below);
- if a sub-issue, the label or the move cannot be written, it says which on
  the issue and moves the card to Travado, so the card is not picked again
  every hour.

It never writes code, opens a branch or a pull request, or plans a change to
`factory/` or the `CLAUDE.md` files. Its App needs no ruleset on `main`
before it starts: with Contents: read it cannot write there.

## What the Developer does, and what waits

The Developer works at **Level 1**: small, self-contained changes made
through the GitHub API, with no build and no test run before the pull
request — a docs or config fix, a mechanical rename, a single-file patch, a
version pin whose lock file needs no tool to regenerate. Where code is
involved it commits the test first and opens the pull request on that
commit, so CI shows the test failing before the change. It labels its pull
request `factory:needs-review`, and when the review sends it back with
`factory:changes-requested` it answers each finding with a commit and a
comment.

A card that needs a build or a test run before its pull request is not
attempted. The Developer labels its issue `factory:needs-runtime`, says why
in a comment, and leaves the card in Doing for a person. If the label cannot
be set, because it was never created (step 1), it says so and moves the card
to Travado instead, so the card is not picked again every hour. **Level 2**, build
and test before a pull request, will come from a coding runtime behind one
MCP tool: the intended runtime is
[FuseOne Forja](https://github.com/fuseone-io/fuseone-forja), a separate
project still at its proposed phase 0, with `forja.implement` as a write tool
under the Gate.

## Who takes a card out of Travado

No role moves a card out of Travado, and clearing what blocked it does not
move it either. In the first version this is a person's standing
job, given to whoever operates the board:

The board operator reviews the Travado lane every working day and, once the blocker named in the comment that blocked a card is cleared, moves the card back to the lane that comment names, Triagem or Doing, leaving its labels as they are; that comment is the most recent one by the blocking role's GitHub App that ends with a Return to: line, on the issue or pull request the role worked on, where a pull request is either the card itself or the one in the card's Linked pull requests field.

A card goes back to no lane once the item it represents is finished: the board operator moves it from Travado to Done when the card is an issue that is closed, or a pull request that is merged or closed.

A pull request closed without a merge while the card's issue stays open does not finish the card: it goes back to the lane its Return to: line names, and a person starts the next attempt, since the Developer skips a card that has a linked pull request.

Triage and the Architect write that comment on the card's issue; the
Reviewer, QA and the Developer answering a review write it on the pull
request, which is the card itself or the one in the card's **Linked pull
requests** field. The last line of that comment is fixed: `Return to: Triagem` when Triage or
the Architect blocked the card, `Return to: Doing` when the Developer, the
Reviewer or QA did. A card back in Triagem that still carries
`factory:needs-design` resumes with the Architect; a card back in Doing
keeps its pull request's label, so the role that label names resumes it.

The role that blocked the card says in that comment what blocks it and what
a person must do to clear it — answer a question, create a missing label,
say whether to resume or discard a half-finished plan — never only what
happened. Taking that action is what keeps the role from blocking the card
again on its next run; the operator is who notices it was taken.
[LANES](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/LANES.md)
lists every way into Travado and its action. Without someone in this job, a
blocked card waits forever.

## Per role, per week

The **Work item** screen (`/work-items`) opened with no card shows **Per role,
per week**: what the run ledger knows about the factory's work, one table per
ISO week (Monday 00:00 UTC), newest first. The current week is marked
**partial**. Pick 4, 8, 12 or 26 weeks, and narrow to one area or one role. A
card's own trail still opens when you follow a claim's **Item** link.

Each row is one role in one week, counting the runs that opened that week:

- **Runs**: the runs that opened, with how many are still open beside them.
- **Cards**: the distinct cards the runs claimed, a linked pull request
  counted as its issue. A run on a card only because an event or a GitHub
  delivery opened it adds no card.
- **Approvals asked**, **Approved**, **Refused**, **Pending**, **Abandoned**:
  what the runs asked people and how people answered. Beside **Approved** and
  **Refused** is their share of the decided ones (approved plus refused).
- **Revised after refusal** and **Repeated after refusal**: refusals after
  which the same run proposed the same tool again with other arguments, or
  with the same ones, beside their share of the refusals.
- **Standing**: approvals a standing approval gave. Every factory role declares
  `standing_approvals: forbidden`, so for them this should be 0. A row with
  any, for an agent whose published definition forbids them, is marked **The
  agent's policy forbids standing approvals**. For an agent that allows them
  it is only a count.
- **On cards**, **Inherited**, **Off card**: where the cost went — runs that
  claimed a card, runs on a card only by how they were opened, and runs on no
  card. **Per card** is the cost on cards over the cards.

A share with nothing under it shows a dash, never 0 %, and so does **Per card**
in a week with no card. Without `cost:read` over the row every cost shows a
dash, never R$ 0.

Under the tables, **Not in these counts** says in words what this panel cannot
measure. Proposal edits are not recorded by the platform: a decision approves
or refuses a proposal as it was asked, so the revised and repeated counts are
what the ledger has instead. Lane time and findings caught later come from the
project board and are read by the factory CLI, not by this screen.

That CLI is
[`factory/measure`](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/measure/README.md).
It reads the board and the timelines of its issues and pull requests, writes
nothing to GitHub, and prints the same weeks as markdown: each lane's median
and p90 hours, the time each role and each person owned, and the signals
caught later. Run its canary with the token you will use before trusting a
report. Until it passes, nobody has confirmed that GitHub shows a role's moves
as that role's App.

## Changing a role

A change to a role is made in the reference first, reviewed by pull request,
and then copied into the console. When the console and the reference
disagree, the console has drifted.

After changing a registration's toolsets, check that the server shows as
reachable before trusting a publish. Its next discovery replaces its tools: a
tool the new toolsets no longer offer leaves the catalogue, and the publish
check refuses a role that still names it. A server that cannot be reached
keeps the tools of its last successful discovery, so until it is reached
again the catalogue still shows the old set.
