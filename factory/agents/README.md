# Role references

One file per role: `<role>.agent.md`, in the platform's own agent format
(frontmatter for the contract, Markdown body for the instructions, the format
`spec.Parse` reads in `internal/spec/spec.go`). Each role's reference is added
by that role's issue (#195, #196, #197, #198, #199, #200), in a pull request reviewed
like any other change:

| Role | Reference | Its card |
|---|---|---|
| Triage | [`triage.agent.md`](triage.agent.md) | the issue, in Triagem |
| Architect | [`architect.agent.md`](architect.agent.md) | the issue, in Triagem, labelled `factory:needs-design` |
| Reviewer | [`reviewer.agent.md`](reviewer.agent.md) | the pull request labelled `factory:needs-review` |
| QA | [`qa.agent.md`](qa.agent.md) | the pull request labelled `factory:needs-qa` |
| Developer | [`developer.agent.md`](developer.agent.md) | the pull request labelled `factory:changes-requested`, first; otherwise the issue of the oldest *Doing* card with no linked pull request |
| Merger | [`merger.agent.md`](merger.agent.md) | the pull request labelled `factory:ready-to-merge` whose card is in Doing |

Triage works the board's cards, and leaves the ones labelled
`factory:needs-design` to the Architect, which takes only those; both choose
from the Triagem listing asked for the Labels field, and both lines are sealed
in `internal/spec/factory_references_test.go`. The Reviewer and QA work pull requests, so
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

The Merger reads nothing anyone wrote. It proposes `merge_if_ready`, which is
destructive, and the Gate blocks an irreversible effect outright in a run that
read untrusted data: no approval releases it. So it holds no GitHub server at
all. Its one read is the factory server's `list_ready_prs`, which answers the
ready pull requests' numbers, head SHAs, card item ids and `merge_if_ready`'s
verdict on each, and nothing else; it refuses or blocks through `send_back`,
which writes comments of fixed text. Its lines are sealed in
`internal/spec/factory_merger_test.go`.

### The Developer's link line

The Developer declares `$fuseone.work_item.link` beside the claim, and its
instructions carry this line, exactly, on a line of its own, in *Open the pull
request* (sealed in `internal/spec/factory_link_test.go`):

```
Right after create_pull_request succeeds, call $fuseone.work_item.link with the pull request's reference, github:fuseone-io/fuseone-agents#<pull request number>, and with nothing else.
```

The link records the pull request as another name of the issue the run
claimed (#204), so the issue's trail shows the Reviewer's and QA's runs on the
pull request. The run gives only the pull request; the platform fills in the
card from the run's own claim. It is not held for approval when the run's own
`create_pull_request` answered naming that pull request: the platform checks
that against the run's record, and the Gate then treats the link like the
claim (`internal/gate/coordination.go`). That answer counts only from the
hosted GitHub MCP server (`https://api.githubcopilot.com/mcp/`) registered with
the role's GitHub App. Keep `create_pull_request` a write whose result is
**untrusted**, the default: the answer's content taints the run like any
other, and every later write still waits for a person. What proves the link is
not that content but the platform's own reading of it, one pull request
reference under that registration's provenance. Any other link waits for a
person, like every write. A pull request has one card for good: linking it again is a
success with no effect, and linking one another card owns is refused with
`work_item_linked_elsewhere`. The link writes nothing to GitHub. No other role
declares it.

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

### The Architect's boundary

The Architect plans and never builds: no branch, no commit, no file change,
no pull request, and no plan that hands factory/, `CLAUDE.md` or
`web/CLAUDE.md` to a role. It verifies every claim it writes down in the code,
with read tools only (file contents, code search, commits), and treats code
and commit messages as untrusted as issue text: the repository is public, and
a comment in a file can carry an instruction as easily as an issue can.

It writes the plan and the acceptance criteria as **a comment on the issue,
not by editing its body**. The body is what the requester asked and stays
theirs; a comment is signed by the role's App and dated, so the plan can be
reviewed, answered and replaced by a later comment without losing the
earlier one, and the latest plan the Architect wrote is the one in force.
Triage writes comments the same way, and the Developer reads the issue and
its comments. Issues #16 and #27 show the shape a plan takes in this
repository; the reference points to them as examples, not as text to copy.

A large item is split into sub-issues, each created under the parent in one
write: `issue_write`, method `create`, with `parent_issue_number`. The hosted
server's `issues` set also has `sub_issue_write`, which the Architect does
not declare: it takes an issue's ID rather than its number, so attaching an
issue after creating it is a second write that can fail on its own, and its
`remove` and `replace_parent` methods detach issues from other parents.
Sub-issues are not added to the board; a person pulls each one in with the
work.

The card leaves in one of three ways, each sealed as an exact line:

- to **Preparado**, once the plan is written, removing
  `factory:needs-design` first (LANES.md § Triagem → `factory:needs-design`);
- to **Travado**, with the plan so far and each open decision as a question
  addressed to whoever decides, with an @mention so GitHub notifies them,
  keeping the label so the card comes back to the Architect when the board
  operator returns it to Triagem, the lane its comment names (§ Who takes a
  card out of Travado);
- to **Travado**, when a sub-issue, the label or the move cannot be
  written, or when the issue already carries a plan the Architect wrote with
  no person's comment after it (an earlier run stopped half way). A card left
  in Triagem with its label would be the Architect's oldest card on every run.

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

### Who takes a card out of Travado

No role does: a role that lists its lane skips a card in Travado, and
clearing what blocked it does not move the card. In v1 that is a
person's standing job, given to whoever operates the board:

The board operator reviews the Travado lane every working day and, once the blocker named in the comment that blocked a card is cleared, moves the card back to the lane that comment names, Triagem or Doing, leaving its labels as they are; that comment is the most recent one by the blocking role's GitHub App that ends with a Return to: line, on the issue or pull request the role worked on, where a pull request is either the card itself or the one in the card's Linked pull requests field.

A card goes back to no lane once the item it represents is finished: the board operator moves it from Travado to Done when the card is an issue that is closed, or a pull request that is merged or closed.

A pull request closed without a merge while the card's issue stays open does not finish the card: it goes back to the lane its Return to: line names, and a person starts the next attempt, since the Developer skips a card that has a linked pull request.

That comment is on the issue for Triage and the Architect, and on the pull
request for the Reviewer, QA and the Developer answering a review; from the
card, the pull request is the card itself or the one its Linked pull requests
field lists. Each role ends that comment with a fixed line naming the lane the
card came from, a line sealed per role in `internal/spec/factory_references_test.go`:
`Return to: Triagem` from Triage and the Architect (a card still labelled
`factory:needs-design` resumes with the Architect), `Return to: Doing` from
the Developer, the Reviewer and QA (the pull request keeps its sub-state
label, so the role it names resumes it).

Every role's Travado comment names what blocks the card and the action a
person must take to clear it, as a question or a request, never as a fact
alone: a line sealed per role, beside the return line. The operator returns
the card once that action is taken, and taking it is what keeps the role from
blocking the card again: for the Architect's half-way rule, a person's comment
after its plan. LANES.md § Any lane → Travado lists every way in and its
action. Without an operator, a blocked card waits forever.

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
| every GitHub tool on `github-<role>`, every board tool on `factory-<role>` | each role reaches GitHub only through its own registrations, both with its own GitHub App (NT-013 § Guardrails, layer 1) |
| no `projects_write` | cards move with `move_card`, fields are set with `set_card_fields`, both on the factory's own server (#213); sealed in `internal/spec/factory_board_test.go` |
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
person's approval, and the run page names the card the run claimed. The
Developer's link is the other platform write that does not wait, and only when
the run's own `create_pull_request` proves it (see the Developer's link line). A tool
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

Each role reaches two tool servers: the official GitHub MCP server for
issues, pull requests, code and the board's reads, and the factory's own
server for the board's writes and QA's verdict (below). Register the GitHub
server once per role, in the installation's administration:

- server id `github-<role>`: `github-triage`, `github-architect`,
  `github-developer`, `github-reviewer` and `github-qa`, the prefix every
  GitHub tool in that role's reference carries;
- its own **GitHub App**, installed only on `fuseone-io/fuseone-agents`, with
  only the permissions the role's tools need. Its identity is what signs the
  role's comments and reviews: the Reviewer's and QA's approvals are told apart
  by it;
- its own toolsets, chosen by an `X-MCP-Toolsets` header on the registration
  (below);
- its surface: exactly the tools the role's reference declares on it, and
  nothing else the toolsets offer (below);
- every tool classified by the Curator before the role can call it (NT-013 §
  Guardrails, layer 3). Reading issue, PR and comment text is a read marked
  untrusted; comments, labels, reviews and new sub-issues are writes, and so
  are the Developer's branches, file commits and pull requests.
  `projects_write` is left **unclassified**.

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
| Architect | `issues,projects,repos` |
| Reviewer | `pull_requests,repos,issues,projects` |
| QA | `pull_requests,repos,issues,projects` |
| Developer | `pull_requests,repos,issues,projects` |

The Reviewer and QA need more than `pull_requests,repos`: a label on a pull
request is set with `issue_write` and a comment with `add_issue_comment`, both
in `issues`; and any role may move a card to Travado (LANES.md), which needs
the card's item id from the `projects` reads. The Architect needs `issues` for
comments, the label and sub-issues, `projects` for the board's reads, and
`repos` for its reads (`get_file_contents`, `search_code`, `list_commits`,
`get_commit`). The `repos` set also offers writes — `create_branch`,
`create_or_update_file`, `push_files`, `delete_file`, `fork_repository`,
`create_repository` among them — and the `projects` set offers
`projects_write`. The Developer needs the same four for different tools:
`repos` for `create_branch`, `create_or_update_file`, `push_files` and the
reads around them, `pull_requests` for `create_pull_request`, `issues` for
comments and labels, and `projects` to list *Doing*.

**Choosing the surface.** A toolset offers more than a role declares, and
`projects_write` is the one that matters: it updates any field of any item
and deletes items, and no role holds it any more (#213). So on each
`github-<role>` server's **Tools and surface** page, bring in exactly the
tools below and nothing else. A tool outside the surface does not exist for
the installation's agents: the publish check refuses a reference that names
it, and a call to it is refused as an unknown tool (`internal/tools/invoke.go`),
whatever the hosted server offers. A tool the server adds later starts
outside the surface. Each row is also the value `X-MCP-Tools` takes (below):

| Server | Its tools |
|---|---|
| `github-triage` | `projects_list,projects_get,issue_read,list_issues,search_issues,add_issue_comment,issue_write` |
| `github-architect` | `projects_list,projects_get,issue_read,search_issues,get_file_contents,search_code,list_commits,get_commit,add_issue_comment,issue_write` |
| `github-reviewer` | `search_pull_requests,pull_request_read,issue_read,get_file_contents,search_code,get_commit,pull_request_review_write,issue_write,add_issue_comment,projects_list,projects_get` |
| `github-qa` | `search_pull_requests,pull_request_read,issue_read,get_file_contents,issue_write,add_issue_comment,projects_list,projects_get` |
| `github-developer` | `search_pull_requests,projects_list,projects_get,issue_read,pull_request_read,get_file_contents,search_code,get_commit,create_branch,create_or_update_file,push_files,create_pull_request,add_issue_comment,issue_write` |

`internal/spec/factory_surface_test.go` fails when a row stops being exactly
what its reference declares.

**Narrowing the hosted server itself.** The same list can narrow what the
hosted server offers, before the platform sees it: `X-MCP-Tools` set to a row
above, with **no** `X-MCP-Toolsets`, makes the server list those tools and
nothing else. Upstream, `InventoryFiltersForRequest`
(`pkg/http/handler.go` in github/github-mcp-server) disables every toolset
when the request names tools and no toolset, and registers only the tools
named; an unknown name fails the request instead of being ignored. With
`X-MCP-Toolsets` also set, the tools are added to the sets instead, which is
why the two must not be combined. The registration accepts `X-MCP-Tools`
beside a GitHub App (`internal/githubapp/headers.go`), but the console's App
panel writes only the Toolsets field, so today it is set through the
administration API's server route (`PUT /admin/integrations/mcp-servers/{name}`,
`headers`). The surface above is required either way: it is the platform's
own fence, and it does not depend on the hosted server honouring a header.
`X-MCP-Readonly` cannot do this job (it also drops `issue_write`), and
`X-MCP-Exclude-Tools` is not accepted beside a GitHub App.

**The GitHub App's permissions.** The App is what makes a tool call succeed;
the toolset only offers it. Each role's App needs:

| Role | Repository permissions | Organization permissions |
|---|---|---|
| Triage | Issues: write, Pull requests: read, Metadata: read | Projects: write |
| Architect | Contents: read, Issues: write, Metadata: read | Projects: write |
| Reviewer | Pull requests: write, Issues: write, Contents: read, Checks: read, Commit statuses: read, Metadata: read | Projects: write |
| QA | Pull requests: read, Issues: write, Contents: read, Checks: read, Commit statuses: write, Metadata: read | Projects: write |
| Developer | Contents: write, Pull requests: write, Issues: write, Metadata: read | Projects: write |
| Merger | Contents: write, Pull requests: write, Checks: read, Commit statuses: read, Metadata: read | Projects: write |

The Merger's App merges (Contents: write), reads the reviews, and writes its
fixed-text comments and labels on the pull request (Pull requests: write); it
reads checks and statuses and holds no Issues permission. The setup canary
proves with its own token that Pull requests: write covers those writes.

The Architect's App has Contents: **read** and no Pull requests permission,
so the minimal ruleset on `main` below is not its prerequisite: that ruleset
exists because the Developer's Contents: write reaches `main`, and the
Architect's App cannot write a file or a branch anywhere, so GitHub refuses
every contents write it could be steered into.

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
skip it. No bypass includes people: a maintainer's change reaches `main`
through a pull request too, the release changelog cut included (a release
branch, the changelog commit, its pull request and CI, the merge, then the
tag on the merged commit; a ruleset on `refs/heads/main` does not touch
tags). The same, through the API:

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

The Merger merges, so before it starts `main` also requires the checks
`merge_if_ready` waits for (#207):

Before the Merger is started, main must also carry the checks ruleset: the six checks of the check workflow required from GitHub Actions on an up-to-date head, and no bypass for anyone.

It is a second ruleset beside the minimal one, so each can change on its own:
the six jobs of `check.yml`, exactly `FACTORY_CHECKS`, each pinned to GitHub
Actions (app id 15368), since a check's name proves nothing (anyone who can
write checks can publish one by any name); strict, so the head must contain
the current base, which is what `merge_if_ready`'s `base_behind` already
requires; and no bypass actors:

```sh
gh api --method POST repos/<owner>/<name>/rulesets --input - <<'CHECKS'
{
  "name": "main: checks required",
  "target": "branch",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["refs/heads/main"], "exclude": [] } },
  "rules": [
    { "type": "required_status_checks", "parameters": {
        "strict_required_status_checks_policy": true,
        "do_not_enforce_on_create": false,
        "required_status_checks": [
          { "context": "go", "integration_id": 15368 },
          { "context": "postgres", "integration_id": 15368 },
          { "context": "console", "integration_id": 15368 },
          { "context": "chart", "integration_id": 15368 },
          { "context": "factory-mcp-image", "integration_id": 15368 },
          { "context": "artifact", "integration_id": 15368 }
        ] } }
  ],
  "bypass_actors": []
}
CHECKS
```

Two of the Merger's conditions stay outside any ruleset, and `merge_if_ready`
alone requires them on the head. `factory/qa`: required with no bypass, it
would block every pull request that never goes through the factory, a
person's own and every release's changelog cut. And the Reviewer's approval:
a ruleset requires reviews by count, code owners or teams, and a GitHub App
can be none of those, so the Reviewer's bot cannot be named.

QA's verdict is the `factory/qa` commit status, set by `record_qa_verdict`
under QA's App, so QA's App needs Commit statuses: **write**. It submits no
review, so it holds Pull requests: **read** only: it reads the pull request,
and its verdict comment and labels go through the Issues API
(`add_issue_comment`, `issue_write`), which Issues: write covers on a pull
request too. The setup canary proves that with QA's own token before QA
starts. The Reviewer's verdict is a review and needs Pull requests: write; it
reads statuses, never sets one.

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

**Moving a card.** Cards move with the factory server's `move_card`, and
Triage and the Architect set Work type and Size with `set_card_fields` (next
section). The hosted server's `projects_write` is in no reference, and it
stays **unclassified**: an unclassified tool never executes
(`internal/domain/effect.go`, `EffectUnknown`), so even a registration that
still offers it lends a steered run nothing. It is the second fence behind
the surface above, not a substitute for it.

**The Architect's tools.** Every read it declares is untrusted:
`projects_list`, `projects_get`, `issue_read`, `search_issues`,
`get_file_contents`, `search_code`, `list_commits`, `get_commit`. Its writes
are `add_issue_comment` (the plan, the questions), `issue_write` (the label
removed, and sub-issues created under the parent), and on `factory-architect`
`move_card` (the card to Preparado or Travado) and `set_card_fields` (the
Size field). None is destructive. It
does not declare `sub_issue_write`, `update_issue_comment`, or any `repos` or
`pull_requests` write.

**The Developer's tools.** Every read it declares is untrusted:
`search_pull_requests`, `projects_list`, `projects_get`, `issue_read`,
`pull_request_read`, `get_file_contents`, `search_code`, `get_commit`. Its
writes are `create_branch`, `create_or_update_file` and `push_files` (a
branch and commits on it), `create_pull_request`, `add_issue_comment`,
`issue_write` (labels) and, on `factory-developer`, `move_card` (a card to
Travado). None is
destructive: a commit is undone by another commit, and a branch can be
deleted. It does not declare `delete_file`, `update_pull_request`,
`update_pull_request_branch`, `merge_pull_request` or
`pull_request_review_write`.

The tool names in the references are the GitHub MCP server's own. The console
refuses to publish a tool the registered server does not offer, so a wrong name
is found when the role is created, not at run time.

## Running the factory's own server beside the worker

The board tools (`move_card`, `set_card_fields`) and QA's
`record_qa_verdict` come from the factory's own MCP server,
[`factory/mcp`](../mcp/README.md), not from GitHub's. It runs as a sidecar in
every worker pod, in pass-through mode on the pod's loopback: it holds no
credential, each role's registration sends that role's GitHub App token as
the bearer, and the server serves only the surface of the bot behind it.
The platform accepts plain `http://` beside a GitHub App only to a loopback
host.

The chart carries it through its generic `worker.extraContainers` hook, so
the platform knows nothing about the factory. The values, with the image
pinned to the same release as agentd (full reasoning for the probes and
sizes in [`factory/mcp/README.md`](../mcp/README.md#as-a-worker-sidecar)):

```yaml
worker:
  extraContainers:
    - name: factory-mcp
      image: ghcr.io/fuseone-io/fuseone-agents-factory-mcp:0.90.0
      imagePullPolicy: IfNotPresent
      env:
        - { name: FACTORY_MCP_ADDR, value: "127.0.0.1:8091" }
        - { name: FACTORY_MCP_AUTH, value: github-app-token }
      envFrom:
        - configMapRef: { name: factory-mcp }
      readinessProbe:
        exec: { command: ["/factory-mcp", "healthcheck"] }
        periodSeconds: 10
        timeoutSeconds: 5
      livenessProbe:
        exec: { command: ["/factory-mcp", "healthcheck"] }
        initialDelaySeconds: 10
        periodSeconds: 20
        timeoutSeconds: 5
        failureThreshold: 3
      securityContext:
        runAsNonRoot: true
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        capabilities: { drop: ["ALL"] }
      resources:
        requests: { cpu: 10m, memory: 32Mi }
        limits: { memory: 128Mi }
```

The probes are `exec`, not `httpGet`: the kubelet's httpGet goes to the pod
IP, never to the pod's loopback. The `factory-mcp` ConfigMap holds the board
ids, lanes, `FACTORY_ROLE_BOTS` and `FACTORY_REPOSITORY`; **none of it is a
secret**, only identifiers that grant nothing. Its contents, with this
repository's board as the example, are in
[`factory/mcp/README.md`](../mcp/README.md#as-a-worker-sidecar).

Its image is checked on every pull request by the `factory-mcp-image` job,
which is one of the checks the Merger requires: `FACTORY_CHECKS` lists every
job of `check.yml` (`go,postgres,console,chart,factory-mcp-image,artifact`),
and the `main` ruleset's required checks must be that same list. Applying
both is a prerequisite before the Merger is activated; the ruleset is
configured on GitHub, and its required checks are part of
[#207](https://github.com/fuseone-io/fuseone-agents/issues/207). See
[the expected checks](../mcp/README.md#the-expected-checks-and-why-build-is-not-one).

Before the first worker runs it, check that the image can be pulled without a
credential: GitHub creates a container package private, and a private one
leaves the sidecar in `ImagePullBackOff` and the worker pod out of Ready.

```sh
gh api /orgs/fuseone-io/packages/container/fuseone-agents-factory-mcp --jq .visibility
```

It must answer `public`; otherwise give the worker's service account an
`imagePullSecret` for GHCR.

### Registering it once per role

Register the factory server once per role, beside that role's GitHub server:

- server id `factory-<role>`: `factory-triage`, `factory-architect`,
  `factory-developer`, `factory-reviewer`, `factory-qa` and `factory-merger`,
  the prefix the board tools in that role's reference carry;
- URL `http://127.0.0.1:8091/`, transport HTTP;
- the **GitHub App** credential, with **the same App** as `github-<role>`:
  the token the platform mints for it is what the server reads the role
  from, so a registration with another role's App serves that role's tools;
- no toolsets and no other header: the server serves the bot's surface
  whatever the request says.

Each registration then shows exactly its role's row of
[the server's table](../mcp/README.md#roles-and-their-surfaces):

| Server | Tools it offers |
|---|---|
| `factory-triage` | `move_card`, `set_card_fields` |
| `factory-architect` | `move_card`, `set_card_fields` |
| `factory-developer` | `move_card` |
| `factory-reviewer` | `move_card` |
| `factory-qa` | `move_card`, `record_qa_verdict` |
| `factory-merger` | `list_ready_prs`, `merge_if_ready`, `send_back`, `move_card` |

A registration that shows no tools, or another row, has the wrong App, or an
App whose bot id is missing from `FACTORY_ROLE_BOTS`.

The Curator classifies `move_card`, `set_card_fields` and `record_qa_verdict`
as **writes** whose result is **not untrusted**: each answers only closed
reasons, lane and field names from closed sets, numbers and hex SHAs, never
text anyone wrote on GitHub, and `factory/guard`'s free-text canary tests
hold it to that. A copilot run proposes each call and a person approves it,
like every GitHub write; no standing approval covers them (layer 6), and
`record_qa_verdict` has no internal exemption. `projects_write` stays
unclassified on every `github-<role>`.

The Merger's tools are ruled the same way, with two of their own:
`merge_if_ready` is **destructive**, and `list_ready_prs` is a **read** whose
result is **not untrusted**. Left at the untrusted default, that read would
taint every Merger run before its merge, and the Gate would block every
merge. `send_back` and `move_card` are writes.

A destructive call is blocked outright by default, and an approval never
releases a block, so `merge_if_ready` needs one authored policy, stored in
the console's Policies or through the API, exactly this narrow (a tool of
`*`, or the whole installation as reach, would make every destructive tool
approvable):

```http
PUT /api/v1/policies/FACTORY-MERGE
Content-Type: application/json

{
  "name": "A person approves every factory merge",
  "reason": "The Merger only proposes a merge: a person approves it, and merge_if_ready re-checks the review, QA and the checks on the head before it merges.",
  "resource": "factory-merger.merge_if_ready",
  "effects": ["destructive"],
  "reach": "agents",
  "agents": ["factory-merger"],
  "effect": "escalate",
  "mode": "enforce",
  "enabled": true
}
```

Start the Merger only once the FACTORY-MERGE policy is stored and enforced; without it, the Gate blocks every merge and no approval releases it.

### The operator canary, before activating the roles

No test here can prove what real GitHub does with a repository-restricted
installation token. Before any role is started on these tools, run the three
calls of [the setup canary](../mcp/README.md#setup-canary-one-real-query-and-one-real-mutation)
once per role's App, with a token minted restricted to this repository,
against a throwaway card: `viewer { databaseId }` must answer the id listed
for that role in `FACTORY_ROLE_BOTS`, the item read must return the project
and the Status option, and the `updateProjectV2ItemFieldValue` mutation must
answer the item with no `errors`. With QA's token, also comment on and label
a throwaway pull request through the Issues API (the canary's fourth step):
QA's App holds Pull requests: read, so this is what proves Issues: write
covers what QA writes on a pull request. With the Merger's token, label and
comment on a throwaway pull request too: its App holds no Issues permission,
so this proves Pull requests: write covers what `send_back` writes. A role
whose canary fails is not activated.
