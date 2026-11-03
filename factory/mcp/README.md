# factory-mcp

The software factory's own MCP server (NT-013 § Guardrails, layers 1, 3, 5
and 5a). It hosts the GitHub writes the factory does not trust to a
general-purpose GitHub server:

| Tool | What it does |
|---|---|
| `move_card(item_id, lane)` | Changes only the Status of a card in the factory's project, and only along the calling role's row of the transition table. Never out of Travado or Done (#213). |
| `set_card_fields(item_id, work_type, size)` | Sets only a card's Work type and Size, and only while the card is in Triagem. Idempotent (#213). |
| `record_qa_verdict(pr, head_sha, verdict, summary[, comment_id])` | Sets the commit status `factory/qa` (`success` or `failure`) on exactly `head_sha`, under the QA GitHub App's identity. Refuses if `head_sha` is no longer the PR's head (#208). |
| `merge_if_ready(pr, head_sha)` | When it executes, re-reads GitHub and fails closed unless every signal is on `head_sha` and came from its authorised actor or app. Then it merges with `sha` set to `head_sha` (#193). |
| `list_ready_prs()` | Lists the open pull requests on the base labelled `factory:ready-to-merge` whose one card is in Doing, oldest first, each with its head SHA, its card's item id and `merge_if_ready`'s verdict on it now. Numbers, SHAs, ids and closed reasons only: it is the Merger's one read, and no text GitHub holds reaches the run (#199). |
| `send_back(pr, head_sha, reason)` | Writes a refused merge back to a pull request still labelled `factory:ready-to-merge`: a comment of closed text naming the reason, the head and the next action, then the label of the role whose signal is missing. A reason only a person can clear keeps the label, ends the comment with `Return to: Doing`, and answers `travado` (#199). |

This is the factory's tooling, not part of the platform. It lives in this
module so it is built and tested with the repository, but `agentd` does not
import it and it ships as its own binary. The logic is in
[`factory/guard`](../guard); this directory is the `main` package.

## Roles and their surfaces

Each role sees exactly its surface, and nothing more:

| Role | Tools | Lane changes `move_card` accepts |
|---|---|---|
| `triage` | `move_card`, `set_card_fields` | Triagem → Preparado, Triagem → Travado |
| `architect` | `move_card`, `set_card_fields` | Triagem → Preparado, Triagem → Travado |
| `developer` | `move_card` | Doing → Travado |
| `reviewer` | `move_card` | Doing → Travado |
| `qa` | `move_card`, `record_qa_verdict` | Doing → Travado |
| `merger` | `move_card`, `list_ready_prs`, `merge_if_ready`, `send_back` | Doing → Done, Doing → Travado |

For every role:

- **nothing leaves Travado** (`travado_is_human`): only a person moves a card
  out of it;
- **nothing leaves Done** (`done_is_final`);
- nobody acts in Preparado, so no agent moves a card out of it;
- moving a card to the lane it is already in is an idempotent success:
  `moved: false`, reason `already_in_lane`, not an error;
- every other change is `transition_refused`.

## What `move_card` checks

`item_id` must match `^PVTI_[A-Za-z0-9_-]{1,64}$` and `lane` must be one of
`Triagem`, `Preparado`, `Doing`, `Travado`, `Done`, exactly (`travado` is not
a lane). The arguments are decoded strictly: any property beyond `item_id`
and `lane` (a `field_id`, a `project_id`, a `value`), a missing one, a
non-string one or a key in another case is `invalid_input`, before any
request. The SDK's inferred schemas already say `additionalProperties:
false`, but the SDK rejects with prose, not a structured result, so the board
tools take their arguments raw and enforce it themselves.

Then one GraphQL `node(id)` query reads the item: whether it is archived, its
project's id and the option id of each single-select field. Titles, content
and text fields are never asked for. The refusals, in this order:

| Condition | Reason when it fails |
|---|---|
| The id is a project item GitHub knows | `item_not_found` |
| It belongs to `FACTORY_PROJECT_ID` | `wrong_project` |
| It is not archived | `item_archived` |
| Its Status is set, to an option `FACTORY_LANES` names | `lane_unknown` |
| The change is on the calling role's row (above) | `already_in_lane` (success), `travado_is_human`, `done_is_final`, `transition_refused` |

Then exactly one `updateProjectV2ItemFieldValue`, with `FACTORY_PROJECT_ID`,
`FACTORY_STATUS_FIELD_ID` and the lane's option id from `FACTORY_LANES`. It
counts as `moved` only when GitHub names the item back with no error. A
GraphQL `errors[]`, a non-2xx status (reported in `http_status`) or no item
in the answer is `move_refused`; an update that never left the process (no
token) is `move_not_sent`; one that was sent and whose answer was lost is
`outcome_unknown`. Setting a value is idempotent, so retrying is safe: a
retry that finds the card in the lane answers `already_in_lane`.

The result is `{moved, reason, item_id, from, to[, http_status]}`, where
`from` and `to` are lane names from the closed set and `item_id` is the
validated id.

### The residual race

GitHub's field updates have no compare-and-set. Between the read that
judges a move and the one mutation that makes it, a person can move the
card, and the mutation then overwrites their move. The window is one round
trip, the same shape as `merge_if_ready`'s re-read before its merge (where
GitHub's `sha` check at least closes the push case). A person who moved a
card to Travado in that window sees it leave and moves it back; the factory
does not try to detect it.

## What `set_card_fields` checks

`work_type` is one of `bug`, `feature`, `chore`, `docs`, and `size` one of
`S`, `M`, `L`, exactly; `item_id` as for `move_card`. Strict decoding as for
`move_card`. The same item read and the same first four refusals, then
**only a card in Triagem** is accepted. Work type and Size are the
classification triage decides on the way to Preparado (`factory/LANES.md`);
once a card has left Triagem they are locked, so no agent rewrites them
during construction or after it. A card in Preparado or Doing is
`fields_locked`, in Travado `travado_is_human`, in Done `done_is_final`, and
nothing is sent.

Each field is then compared with what the card holds: a field already
holding the value is reported in `already_set` and not written; each other
is one `updateProjectV2ItemFieldValue`, Work type first, then Size, with the
configured project and field. The first write that does not apply stops the
call (`update_refused`, `update_not_sent` or `outcome_unknown`), and
`changed` lists what was written before it. So if Work type was set and Size
failed, a retry writes only Size and answers `already_set: ["work_type"]`,
`changed: ["size"]`. When nothing needed writing the answer is `updated:
false`, reason `already_set`, not an error.

The result is `{updated, reason, item_id, work_type, size, already_set,
changed[, http_status]}`: closed names only.

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
runs read these results, and free text would taint them (NT-013 §
Guardrails, layer 4). The board tools follow the same rule: closed reasons,
lane and field names from the closed sets, and the validated item id.

A refusal is returned as a failed tool call (`isError: true`) that still
carries the structured result, so the run records a failure with a reason.

## Running it

```
go build -o bin/factory-mcp ./factory/mcp
```

There are two ways to give the server its GitHub credential, chosen with
`FACTORY_MCP_AUTH`.

### `FACTORY_MCP_AUTH=app` (the default): one role, its own App

The instance holds one GitHub App's key, mints its own installation tokens,
and serves the one role `FACTORY_ROLE` names: that role's whole surface from
the table above. (Before #213 a `qa` or `merger` instance exposed only its
one tool; `FACTORY_ROLE` now maps onto the role's surface, so those instances
also hold `move_card` and need the board's configuration.) Run one instance
per role.

- **stdio** is the default transport and the preferred one here:
  `FACTORY_ROLE=qa ... bin/factory-mcp`. No port, no bearer, nothing on the
  network to intercept.
- **Streamable HTTP**, when the instance must run apart from the worker: set
  `FACTORY_MCP_ADDR` and `FACTORY_MCP_TOKEN`. Every request must carry
  `Authorization: Bearer $FACTORY_MCP_TOKEN`.

### `FACTORY_MCP_AUTH=github-app-token`: every role, the caller's token

The server holds **no credential**. The platform's GitHub App mode (#216)
mints the calling role's installation token and sends it as the bearer to
the MCP URL; the server uses that bearer as the GitHub token of the request.
`FACTORY_ROLE`, `FACTORY_MCP_TOKEN` and `GITHUB_APP_*` are not read
(`FACTORY_ROLE` set in this mode is a startup error, so nobody believes it
narrows anything). HTTP only: over stdio there is no bearer to pass through.

One process then serves every role `FACTORY_ROLE_BOTS` lists, so a valid
token must not reach the union of their tools. The server authenticates the
**identity** behind each request's bearer and serves only that identity's
surface:

1. It asks GitHub whose token it is, with GraphQL `viewer { databaseId }`.
   An installation token's viewer is its App's bot account, and the bot's
   `databaseId` is its numeric user id: the same id `FACTORY_QA_BOT_ID`
   already pins, immutable, unlike a login.
2. It looks that id up in `FACTORY_ROLE_BOTS`. A bearer GitHub rejects is
   `401`; a valid token whose bot no role lists is `403`, with no tools.
3. It serves the request on that role's own MCP handler. Each role has its
   own handler, so a session opened by one identity is unknown to every
   other's: a token cannot ride another role's session into its tools.

The surface comes from the token alone. The server serves on any path and
neither the path nor any header (a role header, `X-MCP-Tools`) chooses or
widens it; per-path endpoints were dropped because a path could only ever
narrow what the identity allows, and one rule is simpler to audit than two.

Each tool call then acts on GitHub with the bearer of the HTTP request that
carried that call, not the one that opened the session: the platform
re-mints tokens hourly, and the token used is the one whose identity was
just checked. An identity is cached for ten minutes, well under a token's
hour, keyed by the token's SHA-256; the token itself is never stored as a
key and never logged.

Every role must have **its own GitHub App**: one bot, one role. A bot listed
for two roles is a startup error.

### Over HTTP, in either mode

The bearer must never cross a network in clear: whoever reads it can call
the tools directly, skipping the platform's approval, and in pass-through
mode it is a GitHub token. The server therefore refuses to start unless one
of these holds:

- **HTTPS**: `FACTORY_MCP_TLS_CERT` and `FACTORY_MCP_TLS_KEY` name the paths
  of the certificate and key PEM files; the address may then be any, e.g.
  `FACTORY_MCP_ADDR=:8443`. Add `FACTORY_MCP_CLIENT_CA` for **mTLS**: every
  client must then present a certificate signed by that CA. mTLS is
  optional, not mandatory, because FuseOne's MCP client authenticates with a
  bearer and does not present a client certificate; require it where a
  proxy in front of the server can.
- **Loopback**: the address is a literal loopback IP (`127.0.0.1:8080`,
  `[::1]:8080`), for a sidecar behind a TLS-terminating proxy in the same
  pod, or reached by the worker over the pod's loopback. A hostname, even
  `localhost`, does not count.

A non-loopback address without TLS, a certificate without its key, or (in
`app` mode) no token is a startup error naming the fix.

`GET /healthz` answers `ok` without a credential, for a readiness probe. It
says the process is listening, nothing about GitHub, and carries no secret.

`factory-mcp healthcheck` is that probe for a loopback listener: it GETs
`/healthz` on `FACTORY_MCP_ADDR` from inside the container and exits non-zero
unless it answers `ok`. The kubelet cannot reach a loopback listener itself:
it sends an `httpGet` probe to the pod IP, or with `host: 127.0.0.1` to the
node's own loopback, and the image has no shell or curl to exec. Off loopback
the command refuses, because the server then speaks TLS and an `httpGet`
probe with `scheme: HTTPS` reaches it.

## As a worker sidecar

The deployment the factory runs: one pass-through instance in every worker
pod, on the pod's loopback. The worker is the only process that connects to
MCP servers, so a server on `127.0.0.1` in its pod is reachable from exactly
the process that calls it, and the traffic never leaves the pod: no TLS, no
Service, no network policy rule. The server holds no credential, so the pod
gains no secret by carrying it.

The image is `ghcr.io/fuseone-io/fuseone-agents-factory-mcp`, published with
agentd from the same commits and with the same tags (`X.Y.Z`, `X.Y`, `X`,
`latest`, `main`, `sha-<short>`), the same provenance, SBOM and cosign
signature. Pin it to the same version as the agentd release the chart runs;
the first release that publishes it is the one after 0.89.0.

The chart knows nothing about it: `worker.extraContainers`
(`deploy/helm/fuseone-agents/values.yaml`) renders these containers verbatim.
In the installation's values:

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
      # exec, not httpGet: the kubelet's httpGet never reaches the pod's
      # loopback. A sidecar that is not ready holds the worker pod out of
      # Ready, and with maxSurge 0 that stops a rollout on a broken image
      # before it replaces the next worker.
      readinessProbe:
        exec: { command: ["/factory-mcp", "healthcheck"] }
        periodSeconds: 10
        timeoutSeconds: 5
      # /healthz touches nothing outside the process, so a failure is this
      # process wedged, never GitHub down: safe to restart for.
      livenessProbe:
        exec: { command: ["/factory-mcp", "healthcheck"] }
        initialDelaySeconds: 10
        periodSeconds: 20
        timeoutSeconds: 5
        failureThreshold: 3
      # The worker's own, stated again because the chart adds nothing to an
      # extra container. Nothing is written to disk, so no volume either.
      securityContext:
        runAsNonRoot: true
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        capabilities: { drop: ["ALL"] }
      resources:
        requests: { cpu: 10m, memory: 32Mi }
        limits: { memory: 128Mi }
```

The resources: idle and under a burst of 200 rejected requests the process
holds about 12 MiB, and each call is one or two small GitHub requests waiting
on the network, so `10m` and `32Mi` reserve what it uses with room, and a
`128Mi` limit is ten times that, enough for a slow GitHub to stack up
requests and low enough that a leak is killed before it starves the worker
beside it. No CPU limit, like the worker's own: throttling a process that
waits on GitHub only adds latency to a tool call the run is waiting for.

The environment the guard decides with comes from a ConfigMap. **None of it
is a secret**: project, field and option ids, bot user ids and a repository
name are identifiers GitHub shows to anyone who can see the board, and they
grant nothing. The sidecar's GitHub credential is each request's bearer,
minted by the platform per role, never stored here. The values below are
**this repository's board** (`fuseone-io` projects/2, see [the factory
board's ids](#the-factory-boards-ids)); another installation uses its own:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: factory-mcp
  # the namespace the chart is released into
data:
  FACTORY_ROLE_BOTS: triage=<id>,architect=<id>,developer=<id>,reviewer=<id>,qa=<id>,merger=<id>
  FACTORY_REPOSITORY: fuseone-io/fuseone-agents
  FACTORY_BASE_BRANCH: main
  FACTORY_PROJECT_ID: PVT_kwDOCda0tM4BpgXe
  FACTORY_STATUS_FIELD_ID: PVTSSF_lADOCda0tM4BpgXezhn7tpM
  FACTORY_LANES: Triagem=b6cc2db9,Preparado=81accba4,Doing=487fcd52,Travado=074ad6eb,Done=f46d8bb8
  FACTORY_WORKTYPE_FIELD_ID: PVTSSF_lADOCda0tM4BpgXezhn7uD4
  FACTORY_WORKTYPES: bug=c63ed0ce,feature=121ff620,chore=b5bce747,docs=bf073d82
  FACTORY_SIZE_FIELD_ID: PVTSSF_lADOCda0tM4BpgXezhn7uD8
  FACTORY_SIZES: S=4e5305ec,M=53fc9302,L=81761912
  FACTORY_REVIEWERS: <reviewer-app-slug>[bot]
  FACTORY_CHECKS: go,postgres,console,chart,factory-mcp-image,artifact
  FACTORY_CHECKS_APP_ID: "15368"
```

Each `<id>` is the role App's bot user id (`gh api
'/users/<app-slug>%5Bbot%5D' --jq .id`). `FACTORY_BASE_BRANCH` is there
because `qa` is listed and `record_qa_verdict` needs it; `merger` is listed,
so `FACTORY_REVIEWERS`, `FACTORY_CHECKS` and `FACTORY_CHECKS_APP_ID` are set
too (see [Environment](#environment)): the Reviewer App's bot login, the
check workflow's jobs, and GitHub Actions. Until the Merger App exists, leave
`merger` and those three out; the other roles do not need them. A missing or
malformed value stops the sidecar at start, naming the variable, and the
readiness probe keeps the pod out of Ready.

Each role's MCP registration then points at `http://127.0.0.1:8091/` with
that role's GitHub App. The platform accepts plain `http://` for a GitHub App
registration only to a loopback host (`internal/githubapp/tls.go`), for this
case. The worker and the sidecar start together; a first discovery that
reaches the sidecar before it listens fails and is retried by the worker's
reconciler, like any server that is briefly down.

## Environment

Nothing the guard decides with has a default. An unset or malformed variable
stops the server at start and names the variable. A setting is required only
when a served role uses it.

| Variable | Needed by | Required | Meaning |
|---|---|---|---|
| `FACTORY_MCP_AUTH` | all | no | `app` (default) or `github-app-token`. |
| `FACTORY_ROLE` | `app` mode | yes | `triage`, `architect`, `developer`, `reviewer`, `qa` or `merger`: the one role this instance serves. Refused in `github-app-token` mode. |
| `FACTORY_ROLE_BOTS` | `github-app-token` mode | yes | `role=bot-user-id` pairs, e.g. `triage=<id>,architect=<id>,developer=<id>,reviewer=<id>,qa=<id>,merger=<id>`. Only the listed roles are served. Role names from the closed set; each role and each id at most once. |
| `FACTORY_PROJECT_ID` | every role | yes | The factory project's node id (`PVT_...`). Cards of any other project are `wrong_project`. |
| `FACTORY_STATUS_FIELD_ID` | every role | yes | The Status field's id (`PVTSSF_...`). |
| `FACTORY_LANES` | every role | yes | `Triagem=<opt>,Preparado=<opt>,Doing=<opt>,Travado=<opt>,Done=<opt>`: all five, each name and each option id once. |
| `FACTORY_WORKTYPE_FIELD_ID` | triage, architect | yes | The Work type field's id. |
| `FACTORY_WORKTYPES` | triage, architect | yes | `bug=<opt>,feature=<opt>,chore=<opt>,docs=<opt>`: all four. |
| `FACTORY_SIZE_FIELD_ID` | triage, architect | yes | The Size field's id. |
| `FACTORY_SIZES` | triage, architect | yes | `S=<opt>,M=<opt>,L=<opt>`: all three. |
| `FACTORY_REPOSITORY` | qa, merger; every role in `app` mode | yes | The only repository it acts on: `fuseone-io/fuseone-agents`. In `app` mode, also the one repository the minted tokens are restricted to. |
| `FACTORY_BASE_BRANCH` | qa, merger | yes | The only base it accepts: `main`. |
| `FACTORY_QA_BOT_ID` | qa, merger | in `app` mode; see the cell | Numeric **user id of the QA App's bot account** (see below). The Merger accepts `factory/qa` only from it; the QA instance checks it created the status under it. In `github-app-token` mode there is **one QA identity**: the id is derived from the `qa` entry of `FACTORY_ROLE_BOTS`, and a `FACTORY_QA_BOT_ID` that is also set must equal it, or the server refuses to start. If the Merger is served and `qa` is not listed (QA served elsewhere), `FACTORY_QA_BOT_ID` must name the QA bot explicitly; without either, the server refuses to start rather than run a Merger with no QA identity to trust. |
| `FACTORY_REVIEWERS` | merger | yes | Comma-separated GitHub logins whose approval counts: the Reviewer App's bot, `<slug>[bot]`. No ruleset requires reviewers; `merge_if_ready` alone does. Compared case-insensitively. |
| `FACTORY_CHECKS` | merger | yes | Comma-separated check-run names required on the head, `go,postgres,console,chart,factory-mcp-image,artifact`: every job of `check.yml`, and **identical to the `main` ruleset's required checks** (see below). **Not `build`**. |
| `FACTORY_CHECKS_APP_ID` | merger | yes | The app that must have emitted them: GitHub Actions, `15368`. |
| `FACTORY_MERGE_METHOD` | merger | no | `squash` (default), `merge` or `rebase`. |
| `GITHUB_APP_ID` | `app` mode | yes | This instance's GitHub App id. |
| `GITHUB_APP_INSTALLATION_ID` | `app` mode | yes | That App's installation id. |
| `GITHUB_APP_PRIVATE_KEY_FILE` | `app` mode | one of | Path to the App's PEM private key (PKCS#1 or PKCS#8). Preferred: a mounted secret. |
| `GITHUB_APP_PRIVATE_KEY` | `app` mode | one of | The PEM itself, when a file is not possible. |
| `GITHUB_API_URL` | all | no | REST base URL. Default `https://api.github.com`. |
| `GITHUB_GRAPHQL_URL` | all | no | GraphQL URL. Default `https://api.github.com/graphql`. |
| `GITHUB_WEB_URL` | all | no | Web base URL for the status's link. Default `https://github.com`. |
| `FACTORY_MCP_ADDR` | all | in `github-app-token` mode | Serve streamable HTTP at this address instead of stdio. Without TLS, only a literal loopback IP is accepted. |
| `FACTORY_MCP_TOKEN` | `app` mode | with `FACTORY_MCP_ADDR` | Bearer token HTTP clients must present. Not read in `github-app-token` mode. |
| `FACTORY_MCP_TLS_CERT` | all | for a non-loopback address | **Path to a PEM file** holding the certificate (chain) the server presents. Not the PEM content. |
| `FACTORY_MCP_TLS_KEY` | all | with `FACTORY_MCP_TLS_CERT` | **Path to a PEM file** holding its private key. Not the PEM content. |
| `FACTORY_MCP_CLIENT_CA` | all | no | **Path to a PEM file** holding the CA bundle; when set, clients must present a certificate it signed (mTLS). Not the PEM content. |

### The factory board's ids

The ids of the live board (`fuseone-io` projects/2, "FuseOne Factory") as of
2026-11-02. They belong here and in the deployment, never in code. Re-read
them with `gh project field-list 2 --owner fuseone-io --format json` and
`gh project view 2 --owner fuseone-io --format json --jq .id`.

```
FACTORY_PROJECT_ID=PVT_kwDOCda0tM4BpgXe
FACTORY_STATUS_FIELD_ID=PVTSSF_lADOCda0tM4BpgXezhn7tpM
FACTORY_LANES=Triagem=b6cc2db9,Preparado=81accba4,Doing=487fcd52,Travado=074ad6eb,Done=f46d8bb8
FACTORY_WORKTYPE_FIELD_ID=PVTSSF_lADOCda0tM4BpgXezhn7uD4
FACTORY_WORKTYPES=bug=c63ed0ce,feature=121ff620,chore=b5bce747,docs=bf073d82
FACTORY_SIZE_FIELD_ID=PVTSSF_lADOCda0tM4BpgXezhn7uD8
FACTORY_SIZES=S=4e5305ec,M=53fc9302,L=81761912
```

Each role's bot user id, for `FACTORY_ROLE_BOTS`:

```
gh api '/users/<app-slug>%5Bbot%5D' --jq .id
```

### The expected checks, and why `build` is not one

The expected set is every job of `.github/workflows/check.yml`, which runs on
every pull request: `go`, `postgres`, `console`, `chart`, `factory-mcp-image`,
`artifact`, by the check-run names GitHub reports. `factory-mcp-image` builds
and starts the sidecar's image (#213); a Merger that did not require it would
merge a PR whose sidecar image no longer builds. When a job is added to
`check.yml`, it joins this list: `TestReadme_factoryChecksExample_namesEveryJobOfEveryPullRequest`
(`factory/mcp/checks_doc_test.go`) fails until the example above names every
job of `check.yml` and nothing else.

`build` is the job in `.github/workflows/docs.yml`, which runs only on pull
requests that touch `docs/**` or that workflow. On every other PR it never
runs, so requiring it would refuse every non-docs PR with `check_missing`, and
the ruleset would block them the same way. #207 was corrected the same way.

**`FACTORY_CHECKS` and the `main` ruleset's required checks must be the same
list, and applying both is a prerequisite before the Merger is activated.**
A check the ruleset requires and `FACTORY_CHECKS` omits makes GitHub refuse a
merge the Merger judged ready (`merge_refused`); a check `FACTORY_CHECKS`
requires and the ruleset omits leaves a person's own merge less guarded than
the Merger's. The ruleset is configured on GitHub, not from this repository:
it is the checks ruleset, applied with the command in
[factory/agents/README.md](../agents/README.md) and named in
[#207](https://github.com/fuseone-io/fuseone-agents/issues/207).

### In `app` mode: why App credentials, not a token

In `app` mode the server takes the App id and private key and mints its own installation
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

One App per role, each installed on the organisation and **restricted to this
repository**. Every role also moves cards, which needs the separate
organisation permission for Projects: a repository-restricted installation
token keeps its `organization_projects` permission (GitHub's documentation;
verified once more by the setup canary below). The table is what this
server's tools need; each App also holds what its role's GitHub-server tools
need, in `factory/agents/README.md` (QA's App keeps Pull requests: read and
adds Issues: write, through which it comments and labels the pull request).

| App | Repository permissions | Organisation permissions | Why |
|---|---|---|---|
| **Triage**, **Architect** | What their issue work needs. Metadata: read (mandatory). | Projects: read and write. | Move cards out of Triagem and set their fields. |
| **Developer**, **Reviewer** | What their own work needs. Metadata: read (mandatory). | Projects: read and write. | Move a card to Travado. |
| **QA** | Commit statuses: read and write. Pull requests: read. Metadata: read (mandatory). | Projects: read and write. | Set `factory/qa`; read the PR's head and base; move a card to Travado. |
| **Merger** | Contents: read and write. Pull requests: read and write. Checks: read. Commit statuses: read. Metadata: read (mandatory). | Projects: read and write. | Merge (contents and PRs write); read reviews, check runs, statuses and the base comparison; move a card to Done. |

The Merger App must **never** be on the ruleset's bypass list (#207). The QA
App has no repository write beyond statuses, so it can vouch for a commit but
never merge one.

## Setup canary: one real query and one real mutation

No test here can prove what real GitHub does with a repository-restricted
installation token. Before the roles are activated, an operator runs these
once, as each role's App, with a token minted exactly as the platform mints
it (restricted to `fuseone-agents`), against a throwaway card in the factory
project. `GH_TOKEN` is that installation token; never paste it anywhere
else.

1. The identity the server will read. Expect the App's bot, and a
   `databaseId` equal to the id listed for that role in `FACTORY_ROLE_BOTS`:

   ```
   GH_TOKEN=<installation token> gh api graphql -f query='query { viewer { login databaseId } }'
   ```

2. The item read `move_card` makes. Expect `isArchived`, the project's id
   (`FACTORY_PROJECT_ID`) and the Status option id:

   ```
   GH_TOKEN=<installation token> gh api graphql -F id=PVTI_<throwaway card> -f query='
     query($id: ID!) { node(id: $id) { ... on ProjectV2Item { id isArchived project { id }
       fieldValues(first: 50) { pageInfo { hasNextPage } nodes {
         ... on ProjectV2ItemFieldSingleSelectValue { optionId field { ... on ProjectV2SingleSelectField { id } } } } } } } }'
   ```

3. The one mutation `move_card` makes, here to Travado. Expect the item's id
   back and no `errors`; the card then shows in Travado, and a person moves
   it back:

   ```
   GH_TOKEN=<installation token> gh api graphql \
     -f project=PVT_kwDOCda0tM4BpgXe -f item=PVTI_<throwaway card> \
     -f field=PVTSSF_lADOCda0tM4BpgXezhn7tpM -f option=074ad6eb -f query='
     mutation($project: ID!, $item: ID!, $field: ID!, $option: String!) {
       updateProjectV2ItemFieldValue(input: {projectId: $project, itemId: $item, fieldId: $field,
         value: {singleSelectOptionId: $option}}) { projectV2Item { id } } }'
   ```

4. **QA only:** a comment and a label on a pull request through the Issues
   API, which is all QA writes on a pull request: its App holds Pull
   requests: **read** and Issues: write, and this proves Issues: write
   covers both on a pull request. Use a throwaway pull request. Expect `201`
   for the comment and `200` with the label for the label, then remove both:

   ```
   GH_TOKEN=<QA installation token> gh api repos/fuseone-io/fuseone-agents/issues/<throwaway PR>/comments \
     -f body='QA canary: a comment through Issues: write.' --jq .id
   GH_TOKEN=<QA installation token> gh api repos/fuseone-io/fuseone-agents/issues/<throwaway PR>/labels \
     -f 'labels[]=factory:needs-qa' --jq '.[].name'
   GH_TOKEN=<QA installation token> gh api --method DELETE repos/fuseone-io/fuseone-agents/issues/<throwaway PR>/labels/factory:needs-qa
   GH_TOKEN=<QA installation token> gh api --method DELETE repos/fuseone-io/fuseone-agents/issues/comments/<comment id>
   ```

If any of these fails for a role, that role is not activated: the board
tools would answer `move_refused` or fail the read on every call, and a QA
that cannot comment or label would record a verdict it cannot route.

## Registering in FuseOne

### Pass-through (`github-app-token`)

Register the instance **once per role**, `factory-<role>`, every
registration with the same URL (an `https://` address, or the sidecar's
loopback `http://127.0.0.1:8091/`) and the platform's GitHub App mode (#216)
with **that role's App**, the same one as its `github-<role>`. A registration
holds one App, so it is the registration that decides which token a run
sends, and one registration per role is what gives each role its own
identity. A tool is addressed as `<server name>.<tool>`:
`factory-triage.move_card`, `factory-architect.set_card_fields`,
`factory-qa.record_qa_verdict`. Which of them a run can actually call is
decided by the token it sends, whatever its pack says.
`factory/agents/README.md` § Registering it once per role has the steps.

### Own App (`app`)

Register one MCP server per role, each an instance with its own
`FACTORY_ROLE` and App, e.g. `factory-qa` and `factory-merger`, giving
`factory-qa.record_qa_verdict`, `factory-merger.merge_if_ready`, and each
one's `move_card`.

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

### Classification

The Curator classifies:

| Tool | Effect | Untrusted result | Compensator |
|---|---|---|---|
| `merge_if_ready` | **destructive** | no (structured state only) | none |
| `record_qa_verdict` | **write** | no (structured state only) | none |
| `move_card` | **write** | no (structured state only) | none |
| `set_card_fields` | **write** | no (structured state only) | none |
| `send_back` | **write** | no (structured state only) | none |
| `list_ready_prs` | **read** | **no** (structured state only) | none |

`list_ready_prs` must be ruled **not** untrusted. Every imported tool starts
untrusted, and an untrusted read anywhere before `merge_if_ready` blocks the
merge outright: the Gate never lets an irreversible effect derive from
untrusted data, and no approval releases that. Its answer holds numbers,
SHAs, item ids and closed reasons and nothing anyone wrote, which is what
makes that ruling true.

`send_back` maps each refusal to its route:

| Reason | Label change | Card |
|---|---|---|
| `head_moved`, `review_missing`, `review_stale` | `ready-to-merge` → `needs-review` | stays in Doing |
| `changes_requested`, `qa_not_success`, `check_not_success` | `ready-to-merge` → `changes-requested` | stays in Doing |
| `qa_missing`, `qa_untrusted_creator` | `ready-to-merge` → `needs-qa` | stays in Doing |
| `base_behind`, `not_mergeable`, `merge_refused`, `outcome_unknown` | none | Travado, `Return to: Doing` |

It refuses any other reason: `check_missing` and `mergeability_unknown` clear
by themselves, so the Merger waits for its next run.

The caller's reason and SHA are never proof. Before writing anything,
`send_back` observes the cause itself, and answers `cause_not_observed`
otherwise:

- a cause a re-read can rebuild is judged again now, exactly as
  `merge_if_ready` would, and must be that same cause on that same head: a
  signal repaired or a head pushed since the listing writes nothing;
- `merge_refused` and `outcome_unknown` cannot be rebuilt by reading, so they
  must be what this process's own `merge_if_ready` returned for that head,
  within the hour, and that refusal proves **one** `send_back`: it is spent
  when used, so a second call, or one after a person cleared the cause and
  returned the card, writes nothing. A comment known not to have been written
  gives it back for a retry. The record is the process's memory only: a run
  resumed on another instance finds none, `send_back` refuses, and the next
  run proposes the merge again. The comment goes first
and the labels after (the new one added before `ready-to-merge` comes off),
so a failure between the writes leaves the pull request still ready to merge
and the next `send_back` repairs it; its one cost is a repeated comment.

Then, per NT-013 § Guardrails:

- only the QA role's pack holds `record_qa_verdict`, only the Merger's pack
  holds `merge_if_ready`, and only Triage's and the Architect's hold
  `set_card_fields`; the raw GitHub `merge_pull_request` is in no pack. In
  pass-through mode the server enforces the same table from the token's
  identity, so a pack that held more would still be refused;
- an authored policy escalates `merge_if_ready` to approval (a destructive call
  is blocked outright by default), and no standing approval may cover
  `merge_if_ready` or `record_qa_verdict` during the pilot (layer 6).
