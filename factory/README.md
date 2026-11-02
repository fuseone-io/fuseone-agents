# The factory

Agents that take this repository's issues from triage to a merge-ready pull
request, each in a role with the narrowest authority that role needs, driven by
a GitHub Project board, under this platform's Gate. The design is
[NT-013](../docs/NT-013-software-factory-pilot.md); the board is
[FuseOne Factory](https://github.com/orgs/fuseone-io/projects/2).

## v1: no factory code

The pilot's first version runs entirely inside the platform (NT-013 § v1):

- every role is an agent **created in the console or the Studio**, from the
  reference text in this directory;
- the official **GitHub MCP server** is the only tool server, registered once per
  role with that role's own GitHub App;
- every role runs at **copilot** with `standing_approvals: forbidden`, so every
  GitHub write it proposes waits for a person, and that is the measurement.
  The one write that does not is the work-item claim, a coordination
  operation that only records which card a run holds (LANES.md § claim);
- **a person merges.** No role merges, and nothing here automates the merge.

This directory is the harness as **reference text** (NT-013 § The harness):
what makes a role do its job well rather than merely within bounds. It changes
by pull request and is reviewed like any other change. Whoever configures a
role in the console copies from here, and changes the reference here first.

The factory encodes the process this repository already follows by hand. It
does not invent one. Where a rule here and `CLAUDE.md` seem to disagree,
`CLAUDE.md` wins and this directory has a bug.

## Layout

```
factory/
  README.md              this file
  RULEBOOK.md            the rules every role follows
  REVIEW.md              the review format the Reviewer and QA submit
  LANES.md               definition of done per lane and sub-state label
  agents/
    README.md            how a role is created in the console from a reference
    <role>.agent.md      the reference specification of one role
  guard/, mcp/           the factory MCP server (merge_if_ready,
                         record_qa_verdict, #211): deferred, not used in v1
```

## Roles

| Role | Issue | Reference |
|---|---|---|
| Triage | #195 | [agents/triage.agent.md](agents/triage.agent.md) |
| Architect | #196 | added by #196 |
| Reviewer | #197 | [agents/reviewer.agent.md](agents/reviewer.agent.md) |
| QA | #198 | [agents/qa.agent.md](agents/qa.agent.md) |
| Developer | #200 | [agents/developer.agent.md](agents/developer.agent.md), Level 1 |

The Developer works at **Level 1** (#154): small, self-contained changes
through the GitHub API, with no build and no test run before the pull request.
A card that needs either is labelled `factory:needs-runtime` on its issue and
left for **Level 2**, a coding runtime behind one MCP tool. The intended
runtime is FuseOne Forja
([fuseone-io/fuseone-forja](https://github.com/fuseone-io/fuseone-forja), a
separate repository, phase 0 proposed), with `forja.implement` as a write tool
under the Gate.

There is no Merger in v1 (#199): a person merges a pull request once it is
labelled `factory:ready-to-merge` (LANES.md).
