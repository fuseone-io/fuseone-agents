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

Triage works the board's cards. The Reviewer and QA work pull requests, so
their claim reference is the pull request's own number
(`github:fuseone-io/fuseone-agents#<PR number>`): it is in the listing they
choose from, while the issue a pull request closes is only known after reading
its text, which the claim has to come before. Their search asks for the
`number` and `created_at` fields only (`search_pull_requests` takes a `fields`
list), so the listing carries no title or body. That is an instruction to the
model, not something the platform enforces: the tool's output is still
classified as untrusted text.

## How a role is created in v1

In v1 the roles are **created in the console or the Studio**, not published
from this repository (NT-013 § v1). These files are the reference a person
copies from:

1. Register the role's GitHub MCP server in the administration (below).
2. Create the agent in the Studio from the reference: the fields of the
   frontmatter, and the body as its instructions. A Studio that imports the
   file format can take the file as it is.
3. Publish it. A new agent is a paused draft.
4. Set its stage to **copilot** and start it. The stage is state beside the
   specification, not a field in it, so the file cannot carry it.

A change to a role is made here first, reviewed, and then copied into the
console. The console is where the role runs; this directory is where its
instructions are reviewed. When the two disagree, the console has drifted.

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
  untrusted; comments, labels, reviews and card moves are writes.

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

The Reviewer and QA need more than `pull_requests,repos`: a label on a pull
request is set with `issue_write` and a comment with `add_issue_comment`, both
in `issues`; and any role may move a card to Travado (LANES.md), which takes
the `projects` tools.

**The GitHub App's permissions.** The App is what makes a tool call succeed;
the toolset only offers it. Each role's App needs:

| Role | Repository permissions | Organization permissions |
|---|---|---|
| Triage | Issues: write, Pull requests: read, Metadata: read | Projects: write |
| Reviewer | Pull requests: write, Issues: write, Contents: read, Checks: read, Commit statuses: read, Metadata: read | Projects: write |
| QA | Pull requests: write, Issues: write, Contents: read, Checks: read, Commit statuses: read, Metadata: read | Projects: write |

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
the move and a person approves it, like any other write. NT-013 classifies a
bundled tool by its most dangerous action, and `projects_write` can also remove
an item. The v1 decision (NT-013 § v1) is to classify it as a write anyway: a
removed card can be added back, and a person approving every write is what
keeps it in check. Classified destructive, it would be blocked outright, and
the publish check would refuse every role that reads issue text and moves
cards. A narrower
card-moving tool is deferred with the rest of the factory's own server.

The tool names in the references are the GitHub MCP server's own. The console
refuses to publish a tool the registered server does not offer, so a wrong name
is found when the role is created, not at run time.
