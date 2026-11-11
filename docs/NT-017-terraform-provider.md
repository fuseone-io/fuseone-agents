# NT-017 — A Terraform provider for FuseOne Agents

Status: proposed · 2026-11-11 · builds on #212 (service accounts) and #268 (read-back
and `If-Match`) · issue [#270](https://github.com/fuseone-io/fuseone-agents/issues/270)

## What this is

Installations that manage their platform with Terraform want to manage FuseOne Agents
the same way: an area, who works in it, the tool servers it reaches, how each tool is
classified and the agents that run there, all reviewed in a pull request beside the
rest of their infrastructure.

This note specifies a Terraform provider for that, and the changes the API needs first.
It widens #270, whose first cut was five integration resources. The request in practice
is "everything an administrator configures", so the note covers that, in three phases,
and says for each resource what the API lacks today.

**Who does what.** An administrator writes the configuration and runs `terraform plan`
and `apply` with a service-account token. A person creates that service account, gives
it roles and issues its token; the provider never does. Nothing the provider does puts
an agent to work, changes its autonomy stage or releases an approval. The provider is a
client of the public API and nothing else: if it needs something the API lacks, the API
gets it, for every client.

## What is already decided

From #270, #212, #268 and the review of this note's scope:

- **Public API only.** Types come from `api/openapi.yaml` at a released tag, so the
  provider and the server cannot drift apart unnoticed.
- **Bootstrap stays with a person.** No token can hold `identity:write`, so the provider
  creates no service accounts, tokens or identity providers.
- **Starting things stays out.** No `apply` changes an agent's stage, resumes it, or
  releases anything the Gate held. The one exception is explicit: destroying an agent
  retires it, which pauses it, and the plan shows that as a retirement (see
  [Agents](#agents)).
- **Console edits are drift.** A person may still change these resources in the console.
  The next plan shows the change; it is not silently overwritten.
- **Secrets never reach the state file.**
- **Wider scope, three phases**, decided 2026-11-11: what the API already supports,
  then companies, areas and classification, then access to an area.
- **Access may be delegated under four rules** (see [Access](#phase-3-access-to-an-area)),
  decided 2026-11-11, the same rules for people and tokens. The only role delegation
  grants is `author`. `identity:write` stays barred for tokens.
- **Repository:** `fuseone-io/terraform-provider-fuseoneagents`, so resources are named
  `fuseoneagents_mcp_server`, `fuseoneagents_agent` and so on.

## What the API offers today

Checked against `main` at `b028e09a`.

| Resource | Write | Read one | `ETag` / `If-Match` | Identical write is a no-op | Token may write |
|---|---|---|---|---|---|
| MCP server | `PUT /admin/integrations/mcp-servers/{name}` | yes | yes | yes | `provider:write` |
| Connector instance | `PUT /admin/integrations/connectors/instances/{name}` | yes | yes | yes | `provider:write` |
| Model provider | `PUT /admin/integrations/providers/{name}` | yes | yes | yes | `provider:write` |
| Channel | `PUT /admin/channels/{name}` | yes | yes | yes | `provider:write` |
| Policy | `PUT /policies/{code}` | yes | yes | yes | `policy:write` |
| Agent | `PUT /agents/{id}/versions` | yes | **no**: neither publish nor retire takes `If-Match` | yes | `agent:publish` |
| Company | `POST /admin/companies`, `PATCH /{company}` | no | no | `POST` answers 409 | `company:write` |
| Area | `POST /admin/scopes`, `DELETE /{company}/{area}` | no | no | `POST` upserts | `scope:write` |
| Tool classification | `PUT /admin/tools/{id}/classification` | list only | no | **no**: each write records an event | `tool:classify` |
| Grants | `PUT /admin/people/{id}/grants` | list only | no | no | **never** (`identity:write`) |

Every row except grants is writable by a service account with the right role today
(`internal/domain/access.go`, `Principal.barredFrom` in `internal/domain/principal.go`).

Four things hold for every resource and shape the provider:

1. **Omitting an optional field keeps the stored value.** Removing an attribute from the
   configuration clears nothing on the server, so the provider sends each resource's
   explicit clear (`clearToken`, an empty object, an empty list) when an attribute leaves
   the configuration.
2. **Some values come back normalised.** An omitted MCP `transport` reads back `stdio`,
   a cache `maxEntries` of 0 reads back 256, a policy `resource` of `""` reads back `*`,
   lists are trimmed and deduplicated. Each one is a default or a plan modifier in the
   provider's schema, or the plan never settles. The provider's acceptance tests find
   them: an unchanged configuration must plan empty right after apply.
3. **Deleting something missing answers 204.** The provider detects removal by the read,
   which answers 404.
4. **Four of the five integration writes answer 204 with only an `ETag`.** The provider
   reads after each write to fill its state.

## Phase 1: what the API already supports

Resources: `mcp_server`, `connector_instance`, `model_provider`, `channel`, `policy`,
`agent`. Data sources: `agents`, `connector_catalog`, `mcp_recipes`.

Servers and providers configured by flag or environment variable come back with
`managed: false`. The provider refuses to import or manage them: their source of truth
is the deployment, not the API.

### Concurrency

The provider stores the `ETag` from its last read and sends it as `If-Match` on every
update and delete. A 412 is a person having changed the resource since the plan; the
provider reports it and tells the operator to plan again. It never retries without the
header.

### Secrets

Secrets are write-only attributes, available in Terraform 1.11 and OpenTofu 1.11: they
are sent and never stored in the plan or the state. Each one pairs with a
`<name>_wo_version` attribute that is stored, because a write-only value cannot be
compared; changing the version is how the configuration says "send the secret again".

The API already never returns a secret. Only one schema field says so
(`MCPGitHubApp.privateKey` is `writeOnly: true`); the others say it in prose. Phase 1
marks every secret field `writeOnly` in `api/openapi.yaml`, so a client generated from
it, the provider included, knows which fields they are.

### Agents

An agent is published with `PUT /agents/{id}/versions`. Publishing identical content is
a no-op, and the version is the digest of the rendered definition.

- **Managed:** the definition fields `AgentDefinition` accepts. The state also records
  the agent's `ETag` from the last read.
- **A precondition, which the API lacks today.** Neither `PUT /agents/{id}/versions` nor
  `PUT /agents/{id}/retired` takes `If-Match`, so a version published in the console
  between plan and apply would be replaced without anybody seeing it. Phase 1 gives the
  agent an `ETag` and makes both calls honour `If-Match`:
  - The `ETag` covers the current version and whether the agent is retired, the two
    things these calls change. A publish moves it; an identical publish does not.
  - `If-Match` is judged under the agent's row lock, in the transaction that writes the
    version or the retirement, where `refuseIfStage` is already judged. A check made by
    an earlier read would leave the same window it exists to close.
  - A stale tag answers 412 with the current `ETag`, as the other resources do.
- **Drift:** a read whose `ETag` differs from the recorded one means somebody published
  or retired since, and the plan shows it.
- **Read-back gap:** `GET /agents/{id}` does not return the rendered definition, and the
  provider needs every field it manages to come back. Phase 1 checks which
  `AgentDefinition` fields `AgentDetail` lacks and adds them to the read, rather than
  the provider keeping its own copy.
- **Never on an unsupervised agent.** Every publish sends `refuseIfStage=autonomous`.
  The stage is judged under the agent's row lock, so a promotion that lands while the
  apply runs is seen and the publish is refused with 409 `fuseone:stage-refused`.
  Changing what an autonomous agent runs stays a person's decision in the console.
- **A new agent is paused.** Authoring starts nothing, so a first apply puts nothing to
  work.
- **`destroy` retires, and that is the one exception to "nothing pauses".** Versions are
  the only explanation of the runs that used them, so the provider retires the agent
  (`PUT /agents/{id}/retired`, with `If-Match`), which pauses it and is reversible. The
  plan says *retire*, not *delete*, so the reviewer of the pull request sees that the
  agent stops. The OpenAPI text for retiring promises a line in the trail, and none is
  written today. Phase 1 writes it.

### Deleting what something depends on

No resource refuses a delete today. Deleting an MCP server removes its tools from the
catalogue, and the agents that used them stay published, naming tools that no longer
exist; only a new publish is checked against the catalogue.

Phase 1 makes `DELETE` on an MCP server, connector instance or model provider answer
409 `fuseone:in-use` while the current version of an agent that is not retired refers
to it, and the problem lists those agents. The console gets the same answer. Channels
and policies are not covered: an agent definition names neither (a channel names agents,
not the other way round), and deleting a policy already takes a new snapshot.

**The check needs a lock both sides take.** Today the publish judges the catalogue before
the transaction that writes the version (`internal/httpapi/publish.go`, `rulingsFor`),
and the delete runs in a transaction of its own (`RemoveMCPServer` in
`internal/admin/integrations.go`). A publish and a delete running together can both
pass their checks and leave an agent naming something that is gone. Phase 1 closes it
by locking the row of the resource itself, first, then judging:

- The delete takes `FOR UPDATE` on the resource's row before anything else, then looks
  for referring agents in the same transaction.
- The publish takes `FOR SHARE` on the row of every resource the definition names (the
  servers and connector instances behind its tools, its model provider, its area), in
  the transaction that writes the version, and judges existence after the lock is
  granted. Today the publish does not check the model provider at all; phase 1 adds
  that check, or the fence would hold on one side only.
- Whichever commits first wins, and the other sees it: a delete that waited finds the
  new version and answers 409; a publish that waited finds the row gone and answers
  400.
- Servers and providers configured by flag or environment variable have no row to lock,
  and the API cannot delete them either.

**No force flag.** An emergency, such as a compromised MCP server, goes in this order:

1. Disable the resource (`enabled: false`) and revoke or rotate its credential. Both are
   updates, not deletes, and the 409 never applies to them.
2. Edit or retire the agents that use it.
3. Delete it.

The manual documents the order, and a test proves the 409 does not block step 1.

## Phase 2: companies, areas and classification

The API gains what the provider needs, for every client.

**Companies.** `GET /admin/companies/{company}` and `PUT /admin/companies/{company}`,
idempotent, with `ETag` and `If-Match`. A company is never deleted: `destroy` archives
it, and applying it again restores it, which the existing `archived` flag already
models. Creating a company still grants the creator `curator` on it.

**Areas.** `GET` and `PUT /admin/scopes/{company}/{area}`, idempotent, with `ETag` and
`If-Match`. Two gaps close at the same time:

- Registering and deleting an area write nothing to `admin_events` today. Both will.
- Deleting an area leaves the agents, ceilings and policies that name it in place. The
  delete answers 409 `fuseone:in-use` while an agent that is not retired is published
  there, behind the same fence as phase 1: the delete takes `FOR UPDATE` on the area's
  row first, and the publish already holds `FOR SHARE` on it.

The `PUT` takes the area id as given. Today's `POST` folds a name into an id (lowercase,
no accents, hyphens); a provider resource keyed by a folded value would plan a
replacement every time. The `PUT` refuses an id that folding would change, and the
`POST` stays as the console's convenience.

**Tool classification.** `GET /admin/tools/{id}/classification`, `ETag` and `If-Match`
on the `PUT`, and an identical `PUT` becomes a no-op. Today every write records a
`tool.classified` event, so an unchanged `apply` would add a line to the trail each time.

The classification carries the digest of the tool definition it was judged against, and
the provider makes `digest` a required attribute. A curator approving a tool approves
one definition; if the provider filled in whatever digest is current, a server that
changed its tool would be approved by the next apply with nobody reading it. When the
definition changes, the plan shows the mismatch and a person updates the digest in a
pull request. A data source exposes the current digest to read from.

Two consequences, documented rather than hidden:

- A tool exists only after its server is discovered, which workers do asynchronously.
  Adding a server and classifying its tools is two applies.
- Classifying ahead of discovery without a digest stays possible in the API and the
  console. The provider does not offer it.

## Phase 3: access to an area

Today only a person with `identity:write` on the installation can grant access, and the
grant call replaces every hand-set grant a principal holds, including grants other
operators made.

Phase 3 adds a grant as a resource of its own:

`GET`, `PUT` and `DELETE /admin/grants/{principal}/{company}/{area}/{role}`

- **`PUT` and `DELETE`** add or remove exactly one hand-set grant and touch nothing else.
- **`GET`** answers 200 when that hand-set grant exists, with who made it and when, and
  404 when it does not. A grant asserted by an identity provider with the same principal,
  scope and role does not count: the provider manages only what a hand made, and import
  reads with the same path.

A person with `identity:write` on the installation may use all three without limits, as
today. Every other caller, a person or a service account, goes through the same rules:

1. **Delegation is a permission, held where it is used.** The caller holds
   `access:delegate` in a grant whose scope contains the target scope. A curator of
   area X can act in X and not in Y.
2. **The only role it grants is `author`.** An allowlist, not a list of exceptions.
   Allowing any role whose permissions the caller holds would let a curator create
   another curator, who receives delegation, policy and provider writes and erasure in
   turn. `admin`, `approver`, `curator` and `auditor` stay with `identity:write`.
3. **Only to a person.** Never to itself and never to a service account, so tokens
   cannot widen each other's reach.
4. **Recorded.** Each grant and revocation writes `grants.added` or `grants.removed` to
   `admin_events` with the caller's id, the grant and, for a token, the token's id. The
   request context does not carry the token's id today; phase 3 adds it, so the grants a
   revoked token made can be found.

The same rules decide a read: a caller may read a grant it could make. A caller may
revoke only a grant it could have made, so a delegating caller can remove an `author`
grant in its scope and nothing else. Grants asserted by an identity provider are never
touched.

`access:delegate` goes into the `admin` and `curator` roles. It is not barred for
tokens; the rules are what make that safe. For people it is new: a curator of an area
can now give `author` there without installation-wide `identity:write`, which is what
"a team looks after its own agent" in the scopes manual needs.

Four facts about grants today shape the resource:

- **A person must have signed in once.** A principal row is created at first sign-in,
  and a grant for a principal that does not exist answers 404.
- **Nobody but `identity:write` can find a person today.** `GET /admin/people` needs it,
  so a token cannot turn an email into a principal id. Phase 3 adds an exact lookup by
  email for a caller holding `access:delegate`, answering one id or 404 and never a
  list. That stops listing everybody, but it is still an oracle: a caller with a list of
  candidate addresses learns which ones have an account. Two mitigations bound it:
  - **A rate limit per caller**, configurable, with a low default; past it the lookup
    answers 429.
  - **Every lookup is recorded** in `admin_events` with the caller and the outcome. The
    row holds the principal id when one was found and a keyed digest of the address when
    none was, never the address itself, so the record of a probe carries no personal
    data.

  The provider's `person` data source uses the lookup, and a plan for somebody who has
  never signed in fails with that reason.
- **A hand grant equal to an asserted one is dropped today.** The key is principal,
  company, area and role, without the source, and the insert ignores a conflict. The
  hand grant vanishes when the identity provider stops asserting it, and the provider
  would report drift it cannot fix. Phase 3 makes the source part of the key, so the two
  are separate rows.
- **The role and the scope are not checked today.** A bad role reaches the database
  constraint and surfaces as a 500, and a grant for a company or area that does not
  exist is accepted. The new call refuses both with 400.

Group mappings, which grant access to everybody an identity provider puts in a group,
remain `identity:write` and stay with a person. They grant access to people who have
never signed in, which the rules cannot judge.

## The provider

- **Framework:** HashiCorp's `terraform-plugin-framework`, in Go.
- **Client:** generated from `api/openapi.yaml` at a pinned server release with
  `oapi-codegen` in client mode. Bumping the pin regenerates the client, and CI fails if
  the generated code differs from what is committed.
- **Compatibility:** the provider reads the server version from `GET /api/v1/readyz` at
  configure time and refuses a server older than the minimum its release names. Breaking
  changes follow the API's compatibility statement in
  [automating-configuration.md](manual/en-US/automating-configuration.md#what-the-api-promises-not-to-break).
- **Distribution:** signed releases on the Terraform registry and the OpenTofu registry.
  Air-gapped installations get a documented filesystem mirror.
- **Documentation:** pt-BR and en-US, with a worked example that builds an area, its MCP
  server, the server's classifications and an agent.

## Rollout

Each phase is a FuseOne release followed by a provider release that requires it.

- Phase 1's 409 on delete changes console behaviour too. It ships with the console
  showing the agents that block the delete.
- A new `If-Match` is honoured only by a replica that knows it. The provider's minimum
  version covers this for the provider, and the console sends `If-Match` on a new
  resource only after a release in which every replica supports it, as in #268.
- The agent's `If-Match` is judged only by a replica that knows it. An old replica
  ignores the header and publishes, so the provider's minimum server version is the
  phase 1 release, and the console sends `If-Match` on publish and retire only after a
  release in which every replica supports it.
- The fence on delete holds only when both sides take the lock. A replica still on the
  previous release publishes without `FOR SHARE`, so the 409 is a guarantee only once
  every replica runs phase 1. Until then it is a check, and the release notes say so.
- `access:delegate` is a new permission in a role. Rolling back below phase 3 removes it,
  and the grants it made stay valid: they are ordinary grants.

## What this is not

- Not a way to create service accounts, tokens or identity providers.
- Not a way to change an agent's stage, resume it or approve anything. Destroying an
  agent retires it, which pauses it; that is the one exception, and the plan names it.
- Not custom roles. The five roles cover the cases asked for; an editable permission
  matrix is a separate feature.
- Not a replacement for the console. Both write the same resources, and the plan says
  when they disagree.

## Tests that must exist

Server side, each written before its change:

- A `DELETE` of an MCP server used by an agent that is not retired answers 409 and lists
  it; once the agent is retired, the delete succeeds. The same for a connector instance,
  a model provider and, in phase 2, an area.
- `enabled: false` and a credential rotation on a resource in use succeed: the 409
  never blocks the first step of the emergency order.
- Publish and delete racing, in both orders, for an MCP server and for an area. Each
  side is held at its lock with a test hook, the other runs, and the outcome is either a
  409 on the delete or a 400 on the publish, never both succeeding.
- A publish naming a model provider that does not exist answers 400.
- A publish with a stale `If-Match` answers 412 and writes no version; the same for a
  retirement. A publish landing between the read and the write with the old tag is the
  case the test uses, not a made-up tag.
- An identical publish keeps the agent's `ETag`.
- Retiring an agent writes a trail line.
- Every secret field in `api/openapi.yaml` is `writeOnly`, found by walking the schema,
  so a new secret field without the mark fails the test.
- An identical classification `PUT` writes no event and keeps the `ETag`.
- An area `PUT` with an id folding would change answers 400.
- For grants by a caller without `identity:write`, one test per rule, each using the
  input the broken version would accept:
  - a curator person granting `curator`, and a curator token granting `curator`;
  - a curator granting `auditor`;
  - a curator of area X granting in area Y;
  - a token granting `approver`;
  - a token granting to itself;
  - a token granting to another service account;
  - a token revoking an `admin` grant.
- The grant `GET` answers 404 for a principal holding only the asserted twin of a grant,
  and 200 once the hand-set one exists.
- A hand grant equal to an asserted one survives the identity provider withdrawing its
  assertion.
- The email lookup answers one id or 404 for a caller with `access:delegate`, 403
  without it, and never a list.
- The lookup answers 429 past the caller's limit.
- A lookup that found nobody records a row that contains no address, checked by
  searching the row for the address that was asked.
- A grant made by a token records that token's id.

Provider side, in its repository, against a real server and database:

- For every resource: create, read, update, import and delete.
- An unchanged configuration plans empty right after apply and again later.
- A change made through the API outside Terraform appears as drift on the next plan.
- A test reads the state file after apply and finds no secret.
- An apply against an autonomous agent is refused and changes nothing.

## Decided

Decided by Kleber on 2026-11-11:

1. **Scope:** three phases, as above.
2. **Access may be delegated**, with `identity:write` still barred for tokens.
3. **Repository name:** `fuseone-io/terraform-provider-fuseoneagents`.

Decided by the reviewer on 2026-11-11:

4. **Delegation by people: yes.** One rule for people and service accounts, and the
   only role it grants is `author`.
5. **No force on delete.** An emergency disables the resource and revokes or rotates its
   credential first, then edits or retires the agents, then deletes. The 409 never
   blocks disabling.
