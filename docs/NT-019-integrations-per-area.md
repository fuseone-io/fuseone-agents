# NT-019 — Integrations per area

Status: proposed · 2026-11-12 · extends [NT-009](NT-009-governed-connectors.md) and
[NT-005](NT-005-interaction-channels.md) · follows #324

## What this is

An area is meant to be isolated and autonomous: the people who look after it build its
agents, and nothing of theirs leaks into another area. Integrations break that today.
The administrator of `acme/cx` can author and publish agents in `acme/cx`. That
administrator cannot connect a single tool server, model provider, connector instance or
channel for them. Every one of those writes is reserved to the platform's administration.

#324 stopped the console from offering those writes to people the server refuses. This
note gives an area its own integrations.

**Who does what.** A person holding `provider:write` on an area (the `admin` and
`curator` roles, granted on that area) creates, changes and removes the area's MCP
servers, model providers, connector instances, channel connections and conversations,
in the console or through the API. A person holding `tool:classify` on the area
classifies the tools of the area's own servers. Agents of the area use the area's
integrations and the ones above it. Nothing here is done by an agent.

## Decided by Kleber on 2026-11-12

- **Each area has its own integrations.** Areas need isolation and autonomy, so integrations
  are per area and are not mixed between areas.
- **Classification belongs to the area, under a ceiling set by the platform.** The
  curator of an area classifies the tools of the area's own servers. A platform
  ruling on the same tool wins, the platform's policies keep applying to every run, and
  an area never classifies a tool of a server that is not its own.
- **An area may register its own model provider, with its own key.** The cost is the
  area's. Nothing new is needed for that: a run's spend is already measured against the
  run's scope (`budget.Watcher.check` reads `SpentSince(ctx, ceiling.Scope, …)`,
  `internal/budget/alerts.go:121`).
- **An area may create conversations and its own channel connections.** It can use the
  platform's Slack connection, or register another app or workspace of its own.

## Where things stand

All of these live in the one `settings` table. Its primary key already carries a
scope: `(scope_kind, company_id, area_id, kind, name)`, with `scope_kind` one of
`installation`, `company`, `area` (`internal/ledger/sql/0004_identity.sql:123-136`).
Each secret is sealed with that scope as associated data (`internal/settings/settings.go:166,268-271`).
So a row of an area is already possible, and its secret is already bound to it. What is
missing is everything around the row.

| Kind | Stored at | Written by | Runtime respects scope |
|---|---|---|---|
| Connector instance | Caller's choice: installation, company or area (`internal/admin/connectors.go:41-59`) | `provider:write` at `default/platform` | Yes. `Layer.Reserve` refuses `ErrOutOfScope` (`internal/connectortools/layer.go:311`) |
| Channel conversation | Company or area (`internal/admin/channel_installation.go:324-329`) | Same | Yes. `Configured.For` keeps the conversations whose scope contains the run's (`internal/channel/configured.go:417`) |
| Channel connection | Installation, hard-coded (`internal/admin/channels.go:327`) | Same | No scope |
| MCP server | Installation, hard-coded (`internal/admin/integrations.go:305`) | Same | No. One catalogue per process. The Gate checks only the agent's pack (`internal/gate/checks.go:45-55`) |
| Model provider | Installation, hard-coded (`internal/admin/providers.go:172`) | Same | No. `Registry.Planner(providerName, …)` takes no scope (`internal/model/registry.go:227`) |
| Tool classification | Installation, literal `'installation','',''` (`internal/admin/curator.go:83`) | `tool:classify` at `default/platform` | Global. `Curator.List` ignores its scope argument (`curator.go:114-118`) |

The write check is the same everywhere. `configurer()` is `auth.Require(provider:write,
adminScope)` (`internal/httpapi/integrations.go:18,236-242`, `admin.go:214-220`). It
asks for the permission at `default/platform` whatever scope the body names. So an
`acme/cx` administrator cannot write even the `acme/cx` connector instance that the API
already accepts. Every listing (`ListIntegrations`, `ListConnectorInstances`,
`ListChannels`) returns every row of every scope (`internal/httpapi/connections.go:86`,
`connectors.go:66`, `channels.go:72`).

**A latent collision.** `Layer.SetInstances` keys connector instances by
`(connector, name)` alone (`internal/connectortools/layer.go:51-54,114`), and a tool id
is `connector.name.operation` with no scope (`internal/connectortools/types.go:78-97`).
Two areas each holding `jira.support` would overwrite each other in memory. Today only the
platform writes, so nobody has done it. Once areas write, somebody will.

## The rule

One rule, for every kind:

1. **A resource belongs to the scope it is stored at.** An area's resource is stored at
   `area` with its company and area, and a company's at `company`. The platform's stays
   at `installation`.
2. **Writing it needs the permission at that scope.** `configurer()` takes the target
   scope, and `auth.Require(provider:write, target)` replaces the check at
   `adminScope`. A grant on `acme` writes `acme` and every area under it, and a grant on
   `acme/cx` writes only `acme/cx`. That is `Scope.Contains`, as for agents
   (`internal/domain/ids.go:80-90`).
3. **A run uses only resources whose scope contains the run's scope.** Connectors and
   conversations already do this. MCP servers, providers and channel connections gain the
   same check, at every door (see [Enforcement](#enforcement)).
4. **Reading shows a caller what they can use, and nothing beside it.** A listing
   returns the resources whose scope contains a scope the caller may read, as
   `auth.VisibleScopes` does for agents. An area sees its own resources, its company's
   and the platform's. The resources above it are marked inherited and are read-only for
   the area. It never sees another area's.

### Names are unique across the installation

A tool id (`server.tool`, `connector.instance.operation`) has no scope in it, and
neither does the name an inbound message is matched by (`settings.Store.Named`,
`internal/settings/secrets.go:111-116`). Putting a scope into every id would touch every
pack, policy, ruling and ledger step ever recorded. Instead:

- **The name of an MCP server, a provider, a channel connection and a conversation is
  unique per kind across the installation.** So is a connector instance's
  `(connector, name)`. A write that would take a name another scope already holds is
  refused with 409 `fuseone:integration-name-taken`. The refusal says the name is taken.
  It never says by whom: another area's names are not the caller's to read.
- The check runs under a lock on the kind, in the write's own transaction. The
  `settings` primary key cannot enforce it, because it includes the scope.
- **Existing rows.** Before phase 1 ships, a read-only query counts names held in more
  than one scope today. It runs against every installation that will take the release.
  A duplicate is reported to the platform's administration, who renames one of the two.
  The release does not ship over a duplicate.

The name stays the identity, so the collision above cannot happen. The layer is still
re-keyed by scope as a guard, and a duplicate found while loading the rows is refused
rather than overwritten.

## Kind by kind

### Connector instances

The instance already has the scope. Only the doors change:

- `PutConnectorInstance`, `DeleteConnectorInstance` and `GetConnectorInstance` check
  `provider:write` at the instance's scope, not at the platform's.
- `ListConnectorInstances` is filtered by rule 4.
- The connector catalogue (`ListConnectorCatalog`) needs `tool:classify` at the
  platform's scope today (`internal/httpapi/connectors.go:30`). It names connector types,
  not instances or credentials, so it opens to `tool:read` in any scope, as #324 did for
  tools.
- **Bindings stay inside the scope.** A SQL or Gravitee instance of an area may bind
  only a Vault instance whose scope contains its own. `boundVault` already checks this
  (`internal/connectortools/sql_config.go:169-173`). A test pins it from the area's door.

### MCP servers and their tools

- A server is stored at the caller's scope. Its credentials, headers, OAuth and GitHub
  App key are sealed in that row, as today, and so bound to that scope.
- **Every reveal, probe, rediscovery and removal reads at the row's own scope.** Each one
  hard-codes the installation today (`internal/admin/integrations.go:536,803,1024,1036`,
  `internal/admin/mcp_rediscovery.go:33,116`). A reveal at a scope that does not hold the
  row finds nothing, and the AAD makes that a cryptographic fact, not only a query.
- **The catalogue knows each tool's scope.** A tool entry carries the scope of the
  server that declares it.
  - `ListTools` already returns tools to `tool:read` in any scope (#324). It now keeps
    only the tools a caller's scopes may use, by rule 4.
  - Today it filters only who declares each tool (`internal/httpapi/admin.go:79-84`).
- **Personal credentials** (`mcp_user_credential`) stay keyed by server and person
  (`internal/admin/integrations.go:752`). A person stores one only for a server they
  can see.

**Classification, under the platform's ceiling.**

- A ruling is stored at the scope of the server whose tool it classifies. The
  `settings` key already allows it. `Curator.Classify` writes the literal installation
  today, and it writes the server's scope instead.
- **Classifying needs `tool:classify` at the server's scope.** An area curator therefore
  classifies the area's servers and nothing else. Classifying a tool of a server the area
  does not own is refused, and so is a tool of the platform's.
- **A platform ruling on a tool wins over the area's.** `Catalog.Sync` resolves the
  ruling for each tool from the installation down. The platform's administration can
  therefore set any area tool to a stricter effect, and the area cannot undo it.
  The platform's screen lists the area rulings, with who made them, so they can be
  reviewed.
- **Policies do not move.** `policy:write` stays at the platform's scope. Every policy,
  approval rule and standing approval applies to an area's tool exactly as to the
  platform's: the Gate does not ask who classified the tool.
- **What stays the same.** An unclassified tool still never executes
  (`internal/gate/checks.go:58-60`). A ruling still carries the definition digest it was
  made against, and goes stale when the server changes the tool (`internal/tools/classify.go:36-77`).

The risk the decision accepts: an area can call `read` a tool that writes. The ceiling
limits who is exposed. The tool can only be reached by the area's own agents, so the
mistake stays inside the area that made it. It also stays visible: the ruling names
its author, and the platform can overrule it.

### Model providers

- A provider is stored at the caller's scope, its key sealed in the row.
- **The registry knows each provider's scope**, and `Planner` takes the run's scope. A
  provider whose scope does not contain the run's is not found. The error names the
  provider, never its key or its owner.
- **Publication refuses an agent naming a provider its scope cannot use.** That covers
  the main pair, the fallback (`Spec.Fallback`) and a judge's model
  (`internal/spec/resolver_judge.go:51`).
- **Cost.** Runs are already charged to their own scope, so an area provider's spend is
  the area's. Prices (`/admin/prices`) stay the platform's. An area provider whose model
  has no price is recorded at the unknown price, exactly as a platform provider would be.

### Channels

- A channel connection is stored at the caller's scope. Its secret is sealed in the row,
  and `ChannelDoor.Secrets` reads it at that scope (`internal/admin/channel_identity.go:444-448`).
- **A conversation names a connection whose scope contains its own.** A conversation of
  `acme/cx` may use the platform's Slack connection or `acme/cx`'s own, and never one of
  `acme/finance`.
- **Inbound traffic** finds the conversation by its unique name, as today. It is then
  dropped unless the connection it arrived on is the one the conversation names. An
  event cannot reach an area's conversation through another area's app.
- **What stays at the platform:**
  - **Channel identities.** These are the bindings of a person to their Slack user
    (`internal/admin/channel_identity.go:161`), and they belong to the person, not to an
    area.
  - **Installation-wide conversations**, which need `company:write` at the installation
    (`internal/httpapi/channels.go:228-233`).
  - **Choosing a `runAs` user**, which needs `identity:write` (`channels.go:361`).

## Enforcement

A scope check only at the doors that write would be a check in the console. A run must
not reach another area's integration however the agent was published, restored or
imported. No effect bypasses the Gate.

| Door | Check |
|---|---|
| Publication (console, API, Terraform, disk sync) | Every tool in the pack and every provider the agent names is usable from the agent's scope. Otherwise the answer is 422, naming the tool or provider |
| `tools` invocation | A call to a tool whose server's scope does not contain the run's is refused `ErrOutOfScope` before the server is contacted. This mirrors `Layer.Reserve` (`internal/connectortools/layer.go:311`) |
| Gate | An out-of-scope refusal is a `blocked` verdict with its reason in the ledger, never a silent skip |
| Model resolution | `Planner` with the run's scope, so a provider out of scope is not found |
| Channel delivery | `Configured.For` as today, plus the connection's scope |
| Classification sync | Rulings resolved from the installation down, scoped to the server |

The check at invocation is what holds against a stale definition. If an agent was
published while a tool was in scope, and its server moved or was removed, the next call
is refused there.

## Rollout: readers a release before writers

A rule only in the new binary is not yet a guarantee. Area rows written while an older
replica serves would be loaded by it without a scope check. Its single catalogue would
offer an area's MCP tools to any run whose pack names them, and its registry would hand
an area's key to any agent naming the provider.

So it ships in two steps per kind:

- **Readers first.** Every reader of the kind learns scopes and enforces rule 3. A row
  stored at a scope it does not know how to check is refused, never treated as the
  platform's. No door writes an area row yet. Nothing changes for any installation,
  because every row is still the platform's.
- **Writers next**, only after the readers' release runs on every replica: the doors
  accept area scopes, the listings filter, and the console opens the pages.

Rolling back from a writers' release to its readers' release is safe. Below the readers'
release it is not: an older binary would treat area rows as global. The release notes of
each readers' release state that floor.

## Console

- Integrations stops being a page only the platform opens. `IntegrationsGate`
  (`web/src/features/integrations/integrations-gate.tsx`) and the `platformPermission`
  on the nav item (`web/src/components/layout/nav.ts:121-127`) give way to
  `provider:write` in any scope, from `/me.can`.
- `provider:write` and `tool:classify` leave `platformOnly`
  (`internal/httpapi/httpwrite.go:84-89`), each in the change that moves its check to
  the resource's scope, as that list's comment requires.
- Every form gains a scope field. It defaults to the caller's own area, and offers only
  the scopes where the caller holds the permission.
- Inherited resources are shown with their origin ("da plataforma", "da empresa") and
  without edit actions.

## Audit

`admin/write.go` records each write in `admin_events` at the scope its caller hands it
(`internal/admin/write.go:50-58,83-101`). Today every integrations handler hands it
`adminScope` (`internal/httpapi/integrations.go:104,177,222`). They hand it the
resource's own scope instead. An area's trail then shows its own integrations, and the
platform's trail shows every scope.

## Phases

| # | What | Released as |
|---|---|---|
| 0 | Names unique across the installation, the duplicate count, the connector layer keyed by scope | One release |
| 1 | Connector instances: doors at the instance's scope, filtered listing, catalogue on `tool:read` | Writers only. The readers already check scope |
| 2 | MCP servers and classification | Readers, then writers |
| 3 | Model providers | Readers, then writers |
| 4 | Channel connections | Readers, then writers. Conversations change only their doors, like phase 1 |
| 5 | Console: gate, scope field, inherited marks | With each writers' release, per kind |

Phase 1 is the smallest step to something an area administrator can use: connecting
their own Jira, Elasticsearch or SQL from their own area.

## Tests that must exist

- **The write check is at the target's scope.** An `acme/cx` administrator writes an
  `acme/cx` instance, and is refused 403 for `acme/finance` and for the installation.
  Sabotage: check at `adminScope`, or at the caller's first visible scope; the test must
  name the refused or the leaked write.
- **A run never reaches another area's resource.** For each kind, an agent of
  `acme/finance` whose pack names `acme/cx`'s tool, or whose definition names its provider,
  is refused at invocation even when the row was planted directly, bypassing publication.
  Sabotage: drop the check at invocation and keep the one at publication; the test must
  find the call made.
- **Names.** A second scope taking a name is refused 409. The body names neither the
  holder nor its scope. Two writers racing for one name: exactly one succeeds.
- **Listing.** An area administrator's listing has its own rows and the inherited ones,
  marked, and none of a sibling area. The sabotage returns every row; the test must
  name the sibling's row.
- **Classification.** An area curator classifies its own server's tool, and is refused
  on a platform tool and on a sibling's. A platform ruling on an area tool wins, and the
  area's later ruling does not undo it.
- **Reveals read at the row's scope.** A reveal at the installation of an area server
  finds nothing, and opening an area row's sealed value under another scope's AAD fails.
- **Against the previous binary.** After the readers' release, an area row is refused,
  not loaded as global. Sabotage: the reader skips the scope check for rows it does not
  recognise; the test must find the area's tool in the catalogue of another area's run.

## What this does not change

- Budgets, prices, policies, approvals, branding, audit export and erasure stay with the
  platform's administration.
- Per-user MCP credentials stay per user.
- **NT-017 (Terraform).** The provider in #321 manages integrations through the same API,
  so each resource gains the scope attribute the API gains. That note is revised when
  this one is approved, not before.
