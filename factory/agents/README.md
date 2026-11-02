# Role references

One file per role: `<role>.agent.md`, in the platform's own agent format
(frontmatter for the contract, Markdown body for the instructions, the format
`spec.Parse` reads in `internal/spec/spec.go`). Each role's reference is added
by that role's issue (#195, #196, #197, #198, #200), in a pull request reviewed
like any other change:

| Role | Reference | Its card |
|---|---|---|
| Triage | [`triage.agent.md`](triage.agent.md) | the issue, in Triagem |
| Reviewer | [`reviewer.agent.md`](reviewer.agent.md) | the pull request labelled `factory:needs-review` |
| QA | [`qa.agent.md`](qa.agent.md) | the pull request labelled `factory:needs-qa` |
| Developer | [`developer.agent.md`](developer.agent.md) | the pull request labelled `factory:changes-requested`, first; otherwise the issue of the oldest *Doing* card with no linked pull request |

Triage works the board's cards. The Reviewer and QA work pull requests, so
their claim reference is the pull request's own number
(`github:fuseone-io/fuseone-agents#<PR number>`): it is in the listing they
choose from, while the issue a pull request closes is only known after reading
its text, which the claim has to come before. Their search asks for the
`number` and `created_at` fields only (`search_pull_requests` takes a `fields`
list), so the listing carries no title or body. That is an instruction to the
model, not something the platform enforces: the tool's output is still
classified as untrusted text.

The Developer works both. A pull request sent back with
`factory:changes-requested` comes first, claimed by its own number and listed
by `number` and `created_at` like the Reviewer's. Otherwise it lists the
*Doing* lane with `projects_list`, method `list_project_items`, asking for the
Labels and Linked pull requests fields, and claims the oldest card's issue,
skipping a card labelled `factory:needs-runtime` and a card that already has a
linked pull request. It sees titles before the claim, like Triage, under the
same rule; its pre-claim lines are sealed in
`internal/spec/factory_references_test.go`.

### The Developer's Level 1 boundary

The Developer changes files through the GitHub API and runs nothing (#154
Level 1): a docs or config fix, a mechanical rename, a single-file patch, a
version pin whose lock or checksum file can be written by hand. Where code is
involved the test is its own first commit, and the pull request is opened on
it, so CI's run on that commit is the red the Developer cannot produce
itself. A card that needs a build or a test run before its pull request is
not attempted: the Developer labels its issue `factory:needs-runtime`
(LANES.md), says why in a comment, and stops. Both sentences are sealed as
exact lines in `internal/spec/factory_references_test.go`.

Level 2, build and test before a pull request, will come from a coding
runtime behind one MCP tool. The intended runtime is FuseOne Forja
([fuseone-io/fuseone-forja](https://github.com/fuseone-io/fuseone-forja), a
separate repository, phase 0 proposed), with `forja.implement` as a write tool
under the Gate. Until it exists, a person takes a `factory:needs-runtime`
card.

## How a role is created in v1

In v1 the roles are **created in the console or the Studio**, not published
from this repository (NT-013 § v1). These files are the reference a person
copies from:

1. Create the factory's labels, once per repository (below), and register
   the role's GitHub MCP server in the administration (below).
2. Create the agent in the Studio from the reference: the fields of the
   frontmatter, and the body as its instructions. A Studio that imports the
   file format can take the file as it is.
3. Publish it. A new agent is a paused draft.
4. Set its stage to **copilot** and start it. The stage is state beside the
   specification, not a field in it, so the file cannot carry it.

A change to a role is made here first, reviewed, and then copied into the
console. The console is where the role runs; this directory is where its
instructions are reviewed. When the two disagree, the console has drifted.

### The labels

Every `factory:*` label a role sets or reads must already exist in the
repository: GitHub applies only labels the repository defines, so a role that
sets a missing one fails at that write, and the card it meant to move stays
where it was, to be picked again on the next run. Create them once, from a
clone of the repository (or with `--repo <owner>/<name>`); `--force` makes
each line safe to run again:

```sh
gh label create factory:needs-design --force --color fbca04 --description "Triage asks the Architect for a plan"
gh label create factory:needs-review --force --color fbca04 --description "PR waits for the Reviewer"
gh label create factory:changes-requested --force --color fbca04 --description "Reviewer or QA asked for changes"
gh label create factory:needs-qa --force --color fbca04 --description "PR waits for QA"
gh label create factory:ready-to-merge --force --color fbca04 --description "Review and QA approved the head; a person merges"
gh label create factory:needs-runtime --force --color fbca04 --description "Needs a build or a test run before its PR; waits for Level 2"
```

`internal/spec/factory_setup_test.go` reads every `factory:*` label from the
references and LANES.md and fails when this list, or the manual's, misses
one, so a new label arrives with its line here.

## What every role's reference carries

| Line | Why |
|---|---|
| `area: factory` | cost, budget and grants of the factory, apart from every other agent (NT-013 § Guardrails, layer 7) |
| no `company:` | the company is the installation's, chosen in the console |
| `triggers: [{ type: cron, schedule: "…" }]`, and no other trigger | a role is woken by its schedule; nothing else starts it (NT-013 § How a role is woken) |
| `$fuseone.work_item.claim` in `tools`, and the claim line in the body | once it has chosen a card from the board, a run's first act is claiming it (#206); see below |
| `budget.micros`, `budget.steps` and `budget.tool_calls` | a per-run ceiling, so a looping role parks instead of spending |
| every MCP tool on `github-<role>` | each role reaches GitHub only through its own registration, with its own GitHub App (NT-013 § Guardrails, layer 1) |
| `standing_approvals: forbidden` | a grant given ahead of time would release a factory write without a person, and count as an approval nobody gave (#192) |
| `reply_locale: en-US` | everything a role writes in this repository is in English (RULEBOOK § 7) |
| no `merge_pull_request` | a person merges in v1 |

### Waking on GitHub instead of a schedule

The references stay on cron in v1. The platform can also start a role when
GitHub sends an event (#202): a webhook trigger with `from: github` verifies
GitHub's signature, takes `X-GitHub-Delivery` as the delivery id and starts a
run only for the events it lists. A role switched to it would declare, for
example:

```yaml
triggers:
  - type: webhook
    path: factory/reviewer
    from: github
    events: [pull_request.opened, pull_request.synchronize, pull_request.labeled]
```

with the GitHub webhook pointing at `/hooks/factory/reviewer` and the key the
console generates in its Secret field (manual: "GitHub webhooks"). The run's
input is the delivery, untrusted like every issue and pull request text.
Switching is a separate decision: until one role can wake another without a
cycle (#203), a webhook and a role's own writes can feed each other.

### The claim line

Every role's instructions contain this line, exactly, on a line of its own:

```
First act after choosing a card: call $fuseone.work_item.claim with the card's reference before reading the issue or writing anything.
```

A fixed sentence rather than "mentions the tool somewhere": "Never call
$fuseone.work_item.claim" also mentions it. The role's own wording around the
line (which reference, what a refusal means) follows on the next lines.

"Before reading the issue" means its body and comments. Triage chooses its
card from a board listing, and `projects_list` always returns item titles,
so Triage sees titles before it claims (#220). The reference tells it to use
a title only to tell cards apart, to choose by age and label, and to act on
nothing a title says. That is an instruction, not something the platform
enforces: a title is untrusted text the model reads before the claim, and it
can sway which card the model picks.

The title is not the only text the Projects grant can reach before the claim.
`projects_list` and `projects_get` are multiplexed by a `method` argument, and
the same tools serve the board's status updates (`list_project_status_updates`,
`get_project_status_update`), whose bodies are free text. Triage is told to
list its lane only with `projects_list`, method `list_project_items`, and
never to call a status-update method before the claim, a line sealed in
`internal/spec/factory_references_test.go`. The Gate classifies a tool, not
its method, so nothing stops a steered model from calling one; the pilot
accepts that within the claim's bounded blast radius below, and preventing it
mechanically needs the deferred narrow tool.

The claim itself is not held for approval. The Gate exempts
`$fuseone.work_item.claim` as a coordination operation, even after untrusted
content (`internal/gate/coordination.go`), so a crafted title can steer which
card gets claimed with no person in the loop. What such a claim can do is
bounded:

- it holds that reference for this agent while the run is open, parked or
  waiting on a person included, so other runs of the same role skip it;
- it releases when the run ends, however it ends;
- the run's cost is attributed to that reference;
- the reference only has to be canonical (`github:<owner>/<repo>#<n>` or
  `ghproject:<id>`): nothing checks that the card is in the lane or exists;
- it writes nothing to GitHub.

Every GitHub write after it — comment, label, card move — waits for a
person's approval, and the run page names the card the run claimed. A tool
that picks and claims the next card atomically, exposing its content only
afterwards, would remove the title from the pre-claim view; it is deferred.

The order cannot be enforced with steps. The claim is a platform tool: the step
machinery ignores it, so a first step reaching only the claim would never
advance, and `spec.Parse` refuses a step that names a platform tool at all. At
run time the claim enforces itself: one card per run, refused while another
open run of the role holds it.

### Steps

Steps narrow the pack as the run advances (`internal/engine/envelope.go`):
once a run calls a tool of a later step, the earlier steps' tools are out of
reach. The claim is offered at every step to an agent that declares it, so no
step lists it.

## Setting up a role's GitHub server

The official GitHub MCP server is the only tool server in v1. Register it once
per role, in the installation's administration:

- server id `github-<role>`;
- its own **GitHub App**, installed only on `fuseone-io/fuseone-agents`, with
  only the permissions the role's tools need. Its identity is what signs the
  role's comments and reviews: the Reviewer's and QA's approvals are told apart
  by it;
- its own toolsets, chosen by an `X-MCP-Toolsets` header on the registration
  (below);
- every tool classified by the Curator before the role can call it (NT-013 §
  Guardrails, layer 3). Reading issue, PR and comment text is a read marked
  untrusted; comments, labels, reviews and card moves are writes, and so are
  the Developer's branches, file commits and pull requests.

**Choosing the toolsets.** The roles use the hosted server
(`https://api.githubcopilot.com/mcp/`), not the local one, so
`GITHUB_TOOLSETS` does not apply: that is the local server's variable. The
hosted server's `default` set does not include `projects`, and a
`/x/{toolset}` URL serves exactly one set. Sets are combined with the
`X-MCP-Toolsets` header ([remote server
reference](https://github.com/github/github-mcp-server/blob/main/docs/remote-server.md#optional-headers)),
set as a configuration header on the registration next to the GitHub App
credential:

| Role | `X-MCP-Toolsets` |
|---|---|
| Triage | `issues,projects` |
| Reviewer | `pull_requests,repos,issues,projects` |
| QA | `pull_requests,repos,issues,projects` |
| Developer | `pull_requests,repos,issues,projects` |

The Reviewer and QA need more than `pull_requests,repos`: a label on a pull
request is set with `issue_write` and a comment with `add_issue_comment`, both
in `issues`; and any role may move a card to Travado (LANES.md), which takes
the `projects` tools. The Developer needs the same four for different tools:
`repos` for `create_branch`, `create_or_update_file`, `push_files` and the
reads around them, `pull_requests` for `create_pull_request`, `issues` for
comments and labels, and `projects` to list *Doing* and move a card to
Travado.

**The GitHub App's permissions.** The App is what makes a tool call succeed;
the toolset only offers it. Each role's App needs:

| Role | Repository permissions | Organization permissions |
|---|---|---|
| Triage | Issues: write, Pull requests: read, Metadata: read | Projects: write |
| Reviewer | Pull requests: write, Issues: write, Contents: read, Checks: read, Commit statuses: read, Metadata: read | Projects: write |
| QA | Pull requests: write, Issues: write, Contents: read, Checks: read, Commit statuses: read, Metadata: read | Projects: write |
| Developer | Contents: write, Pull requests: write, Issues: write, Metadata: read | Projects: write |

The Developer's App has **no Workflows permission**, so GitHub refuses any
write it makes under `.github/workflows`; a change there is not Level 1.
Contents: write reaches every branch, `main` included. The reference keeps the
Developer to branches it creates under `factory/` and off `factory/` and the
`CLAUDE.md` files, two lines sealed in
`internal/spec/factory_references_test.go`, and the person approving each
write sees the branch it names. Neither is a mechanism, so a ruleset is a
prerequisite, not a recommendation:

Before the Developer is started, main must carry the minimal ruleset: a pull request required before merging, force pushes and deletions blocked, and no bypass for any factory GitHub App.

Until `main` carries it, the Developer stays paused. In the repository's
Settings → Rules → Rulesets, that is a branch ruleset, enforcement Active,
targeting `refs/heads/main`, with **Require a pull request before merging**
(zero approvals is enough: a person still merges), **Block force pushes** and
**Restrict deletions** ticked, and an empty bypass list, so no factory App can
skip it. The same, through the API:

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

Everything else a ruleset on `main` should require is deferred to #207:
required status checks pinned to their source, `factory/qa` (which does not
exist in v1), the authorised reviewers, and the rules the Merger is held to.

Submitting a review needs Pull requests: **write**, for QA as much as for the
Reviewer: in v1 QA's verdict is a review, not a commit status, so QA needs no
Commit statuses: write.

Without the header from the start, Triage's server never discovers its
`projects_*` tools, and the console refuses to publish the Triage reference,
because it names tools the installation's catalogue does not have. The hosted
server silently ignores an unknown set name, so a typo on a new registration
shows up the same way: as a publish refusal that names the missing tools.

The same holds after a change. Each successful discovery is the whole answer
for its server (`Curator.Publish` in `internal/admin/tool_catalogue.go`), so
a registration that later loses `projects` — the header removed, or a typo
edited in — loses the `projects_*` tools from the catalogue at its next
discovery, and the publish is refused again. Their rulings are kept: the
toolset restored brings them back classified. A server that cannot be
reached keeps the tools of its last successful discovery, so after changing a
role's `X-MCP-Toolsets` check that the server shows as reachable before
trusting a publish.

Beside a GitHub App the registration accepts only the hosted server's
documented configuration headers (`X-MCP-Toolsets`, `X-MCP-Tools`,
`X-MCP-Readonly`, `X-MCP-Lockdown`, `X-MCP-Insiders`); any other header,
`Authorization` included, is refused, because the App credential is the only
identity the server may see.

**Moving a card.** Cards move with the server's projects tool
(`projects_write`). It is classified as a **write**, so a copilot run proposes
the move and a person approves it, like any other GitHub write. NT-013 classifies a
bundled tool by its most dangerous action, and `projects_write` can also remove
an item. The v1 decision (NT-013 § v1) is to classify it as a write anyway: a
removed card can be added back, and a person approving every GitHub write is what
keeps it in check. Classified destructive, it would be blocked outright, and
the publish check would refuse every role that reads issue text and moves
cards. A narrower
card-moving tool is deferred with the rest of the factory's own server.

**The Developer's tools.** Every read it declares is untrusted:
`search_pull_requests`, `projects_list`, `projects_get`, `issue_read`,
`pull_request_read`, `get_file_contents`, `search_code`, `get_commit`. Its
writes are `create_branch`, `create_or_update_file` and `push_files` (a
branch and commits on it), `create_pull_request`, `add_issue_comment`,
`issue_write` (labels) and `projects_write` (a card to Travado). None is
destructive: a commit is undone by another commit, and a branch can be
deleted. It does not declare `delete_file`, `update_pull_request`,
`update_pull_request_branch`, `merge_pull_request` or
`pull_request_review_write`.

The tool names in the references are the GitHub MCP server's own. The console
refuses to publish a tool the registered server does not offer, so a wrong name
is found when the role is created, not at run time.
