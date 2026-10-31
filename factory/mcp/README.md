# factory-mcp

The software factory's own MCP server (NT-013 § Guardrails, layers 5 and 5a).
It hosts the two GitHub writes the factory does not trust to a general-purpose
GitHub server:

| Tool | Role | What it does |
|---|---|---|
| `record_qa_verdict(pr, head_sha, verdict, summary[, comment_id])` | QA | Sets the commit status `factory/qa` (`success` or `failure`) on exactly `head_sha`, under the QA GitHub App's identity. Refuses if `head_sha` is no longer the PR's head (#208). |
| `merge_if_ready(pr, head_sha)` | Merger | When it executes, re-reads GitHub and fails closed unless every signal is on `head_sha` and came from its authorised actor or app. Then it merges with `sha` set to `head_sha` (#193). |

This is the factory's tooling, not part of the platform. It lives in this
module so it is built and tested with the repository, but `agentd` does not
import it and it ships as its own binary. The logic is in
[`factory/guard`](../guard); this directory is the `main` package.

## What `merge_if_ready` checks

All of these, re-read when the tool executes (after a person approved), in
this order. The first one that fails is the answer, and nothing is merged.

| Condition | Reason when it fails |
|---|---|
| `pr` is positive and `head_sha` is a full 40-character lowercase SHA | `invalid_input` |
| The PR's base repository is `FACTORY_REPOSITORY` (a redirect is never followed) | `wrong_repository` |
| The PR targets `FACTORY_BASE_BRANCH` | `wrong_base` |
| The PR is open | `pr_not_open` |
| `head_sha` is still the PR's head | `head_moved` |
| No authorised reviewer's latest review requests changes | `changes_requested` |
| An authorised reviewer's latest review approves `head_sha` itself | `review_missing`, or `review_stale` if the approval is of another commit |
| The latest `factory/qa` status on `head_sha` was created by the QA App's bot | `qa_missing`, `qa_untrusted_creator` |
| ...and it is `success` | `qa_not_success` |
| Every check in `FACTORY_CHECKS`, as emitted by `FACTORY_CHECKS_APP_ID` on `head_sha`, completed with `success` (`skipped` and `neutral` do not count) | `check_missing`, `check_not_success`, with the check's name |
| `head_sha` contains the current base (strict status checks) | `base_behind` |
| GitHub has computed the PR as mergeable | `not_mergeable`, `mergeability_unknown` |

Then one merge call, `PUT /pulls/{pr}/merge` with `sha=head_sha`. If GitHub
refuses it, because a push landed (409, reported as `head_moved`) or the
ruleset (#207) saw a signal change on the same commit after the read (a review
dismissed, a check gone back to pending), the tool reports `merge_refused` with
the HTTP status. It does not retry: only a fresh proposal may judge the PR
again.

If the merge request never left the process (the token could not be
obtained, or the request could not be built), the answer is `merge_not_sent`:
known not merged, safe to retry, no reconciliation. If it was sent but its
response was lost (a cut body, a reset
connection, a timeout), GitHub may have merged. The tool never reports that as
a bare error and never merges again. It re-reads the PR: merged with exactly
`head_sha` is `merged` with the merge commit; anything else, including a
re-read that fails, is `outcome_unknown`, which means a person must look. A
call for a PR already merged with exactly `head_sha` (a retry after a result
that never reached the run) answers `merged`, not `pr_not_open`; a PR merged
with any other head is not claimed.

Labels are never read. A QA verdict recorded as a label counts for nothing.

## Structured state only

Results are JSON with a closed `reason`, numbers, hex SHAs and, for a failing
check, the name from this server's own configuration. Nothing GitHub wrote
comes back: no review bodies, comments, titles, descriptions, check output or
error messages. The client does not even decode those fields, and error
messages carry the method, the path this server built and the status code. The
Merger's run reads these results, and free text would taint it (NT-013 §
Guardrails, layer 4).

A refusal is returned as a failed tool call (`isError: true`) that still
carries the structured result, so the run records a failure with a reason.

## Running it

```
go build -o bin/factory-mcp ./factory/mcp
```

Each role runs **its own instance**, with its own GitHub App and its own
`FACTORY_ROLE`. The QA instance exposes only `record_qa_verdict`; the Merger
instance exposes only `merge_if_ready`.

- **stdio** is the default and the preferred transport:
  `FACTORY_ROLE=qa ... bin/factory-mcp`. No port, no bearer, nothing on the
  network to intercept.
- **Streamable HTTP**, when the instance must run apart from the worker: set
  `FACTORY_MCP_ADDR` and `FACTORY_MCP_TOKEN`. Every request must carry
  `Authorization: Bearer $FACTORY_MCP_TOKEN`, so the bearer must never cross a
  network in clear: whoever reads it can call `record_qa_verdict` (the QA App
  signs the status) or `merge_if_ready` directly, skipping the platform's
  approval. The server therefore refuses to start unless one of these holds:
  - **HTTPS**: `FACTORY_MCP_TLS_CERT` and `FACTORY_MCP_TLS_KEY` name the paths
    of the certificate and key PEM files; the
    address may then be any, e.g. `FACTORY_MCP_ADDR=:8443`. Add
    `FACTORY_MCP_CLIENT_CA` for **mTLS**: every client must then present a
    certificate signed by that CA. mTLS is optional, not mandatory, because
    FuseOne's MCP client authenticates with a bearer and does not present a
    client certificate; require it where a proxy in front of the server can.
  - **Loopback**: the address is a literal loopback IP (`127.0.0.1:8080`,
    `[::1]:8080`), for a sidecar behind a TLS-terminating proxy in the same pod.
    A hostname, even `localhost`, does not count.

  A non-loopback address without TLS, a certificate without its key, or no
  token is a startup error naming the fix.

## Environment

Nothing the guard decides with has a default. An unset or malformed variable
stops the server at start and names the variable.

| Variable | Roles | Required | Meaning |
|---|---|---|---|
| `FACTORY_ROLE` | both | yes | `qa` or `merger`: which single tool this instance exposes. |
| `FACTORY_REPOSITORY` | both | yes | The only repository it acts on: `fuseone-io/fuseone-agents`. |
| `FACTORY_BASE_BRANCH` | both | yes | The only base it accepts: `main`. |
| `FACTORY_QA_BOT_ID` | both | yes | Numeric **user id of the QA App's bot account** (see below). The Merger accepts `factory/qa` only from it; the QA instance checks it created the status under it. |
| `FACTORY_REVIEWERS` | merger | yes | Comma-separated GitHub logins whose approval counts. **The same list the ruleset requires (#207).** Compared case-insensitively. |
| `FACTORY_CHECKS` | merger | yes | Comma-separated check-run names required on the head, `go,postgres,console,chart,artifact`. **Not `build`**: see below. |
| `FACTORY_CHECKS_APP_ID` | merger | yes | The app that must have emitted them: GitHub Actions, `15368`. |
| `FACTORY_MERGE_METHOD` | merger | no | `squash` (default), `merge` or `rebase`. |
| `GITHUB_APP_ID` | both | yes | This instance's GitHub App id. |
| `GITHUB_APP_INSTALLATION_ID` | both | yes | That App's installation id on the repository. |
| `GITHUB_APP_PRIVATE_KEY_FILE` | both | one of | Path to the App's PEM private key (PKCS#1 or PKCS#8). Preferred: a mounted secret. |
| `GITHUB_APP_PRIVATE_KEY` | both | one of | The PEM itself, when a file is not possible. |
| `GITHUB_API_URL` | both | no | REST base URL. Default `https://api.github.com`. |
| `GITHUB_WEB_URL` | both | no | Web base URL for the status's link. Default `https://github.com`. |
| `FACTORY_MCP_ADDR` | both | no | Serve streamable HTTP at this address instead of stdio. Without TLS, only a literal loopback IP is accepted. |
| `FACTORY_MCP_TOKEN` | both | with `FACTORY_MCP_ADDR` | Bearer token HTTP clients must present. |
| `FACTORY_MCP_TLS_CERT` | both | for a non-loopback address | **Path to a PEM file** holding the certificate (chain) the server presents. Not the PEM content. |
| `FACTORY_MCP_TLS_KEY` | both | with `FACTORY_MCP_TLS_CERT` | **Path to a PEM file** holding its private key. Not the PEM content. |
| `FACTORY_MCP_CLIENT_CA` | both | no | **Path to a PEM file** holding the CA bundle; when set, clients must present a certificate it signed (mTLS). Not the PEM content. |

### Why `build` is not an expected check

`build` is the job in `.github/workflows/docs.yml`, which runs only on pull
requests that touch `docs/**` or that workflow. On every other PR it never
runs, so requiring it would refuse every non-docs PR with `check_missing`, and
the ruleset would block them the same way. The expected set is the jobs that
run on every PR, from `check.yml`: `go`, `postgres`, `console`, `chart`,
`artifact`. #207 was corrected the same way. Keep `FACTORY_CHECKS` and the
ruleset's required checks the same list.

### Why App credentials, not a token

The server takes the App id and private key and mints its own installation
tokens, each restricted to the one repository and replaced five minutes before
it expires. It does not take a pre-minted token for two reasons:

- an installation token lives one hour, and a server that runs for days
  cannot hold one;
- QA must be an App: the ruleset pins `factory/qa` to the QA App's
  `integration_id` (#207), and a status is attributable to the App only when
  that App's installation token created it.

### `FACTORY_QA_BOT_ID`: the bot's user id, not the App id

A commit status does not carry the id of the App that created it, only its
`creator`, which for an App is the App's bot user (`<slug>[bot]`). The guard
therefore matches the creator's user id and requires its type to be `Bot`. A
user id is immutable, unlike a login, which changes if the App is renamed.
Find it with:

```
gh api '/users/<qa-app-slug>%5Bbot%5D' --jq .id
```

The ruleset pins the same App by its App id (`integration_id`); both identify
the one QA App.

## GitHub App permissions

Two Apps, each installed **only on this repository**.

| App | Repository permissions | Why |
|---|---|---|
| **QA** | Commit statuses: read and write. Pull requests: read. Metadata: read (mandatory). | Set `factory/qa`; read the PR's head and base. Nothing else. |
| **Merger** | Contents: read and write. Pull requests: read and write. Checks: read. Commit statuses: read. Metadata: read (mandatory). | Merge (contents and PRs write); read reviews, check runs, statuses and the base comparison. |

The Merger App must **never** be on the ruleset's bypass list (#207). The QA
App has no write beyond statuses, so it can vouch for a commit but never merge
one.

## Registering in FuseOne

Register two MCP servers in **Integrations**, one per role. A tool is addressed
as `<server name>.<tool>`, so with these names the tool ids are
`factory-qa.record_qa_verdict` and `factory-merger.merge_if_ready`.

| Server name | Instance | Surface |
|---|---|---|
| `factory-qa` | `FACTORY_ROLE=qa`, QA App | `record_qa_verdict` |
| `factory-merger` | `FACTORY_ROLE=merger`, Merger App | `merge_if_ready` |

**stdio (preferred).** The command is `factory-mcp`, with the variables above
set through the **Env** shape and the private key through the **Config file**
shape, its variable set to `GITHUB_APP_PRIVATE_KEY_FILE`. A local server needs
an administrator to accept local execution, and runs as the worker; nothing
listens on a port.

**Remote HTTP**, when the instance must keep its App key outside the worker:
the URL is the instance's `https://` address (TLS on the server, or a
TLS-terminating proxy in front of a loopback-bound sidecar), and the credential
is the **Bearer** shape with that instance's `FACTORY_MCP_TOKEN`. Never an
`http://` URL across a network: the server refuses to listen that way, and a
proxy must not undo it.

The Curator classifies:

| Tool | Effect | Untrusted result | Compensator |
|---|---|---|---|
| `factory-merger.merge_if_ready` | **destructive** | no (structured state only) | none |
| `factory-qa.record_qa_verdict` | **write** | no (structured state only) | none |

Then, per NT-013 § Guardrails:

- only the QA role's pack holds `factory-qa.record_qa_verdict`, and only the
  Merger's pack holds `factory-merger.merge_if_ready`; the raw GitHub
  `merge_pull_request` is in no pack;
- an authored policy escalates `merge_if_ready` to approval (a destructive call
  is blocked outright by default), and no standing approval may cover either
  tool during the pilot (layer 6).
