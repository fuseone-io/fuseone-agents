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
- **An area's MCP server is remote** (decided by Kleber on 2026-11-13, while building
  phase 2's writers). A stdio server is a process the platform's worker starts on its
  own host, with its command, arguments, environment and configuration file. Registered
  by an area, it would run that area's commands on infrastructure every area shares.
  So a company's or an area's server is HTTP, and stdio servers stay the platform's.
  The rule is held three times:
  - **by the database**, migration `0110`, so every binary is bound;
  - **by the door**, which answers 400 to an area's write naming a local server;
  - **by the worker**, which starts no local process for a server outside the
    installation.

  Keeping stdio the platform's prevents running an area's code on the platform's
  host. Where a remote server may send requests is a separate rule, below: only the
  origins the platform allows the server's scope.
- **An area may create conversations and its own channel connections.** It can use the
  platform's Slack connection, or register another app or workspace of its own.
- **A server outside the installation reaches only the origins the platform allows its
  scope** (decided 2026-11-13, after review of the writers). `netguard` refuses cloud
  metadata and link-local addresses only; private addresses stay reachable on purpose,
  because on-premise MCP servers are normal. Delegating servers to areas would delegate
  reaching any endpoint the worker can, with the area's credential. So:
  - The platform keeps one policy per company or area: a list of exact origins (scheme,
    host and port, no wildcards), stored at the installation and written only by the
    platform's administration. An area can neither create nor widen it.
  - A server of an area is held to its area's policy, or else to its company's, never to
    both together. With neither, it reaches nothing. An empty list is a deny-all that
    overrides the company's; removing an area's policy inherits the company's again.
  - A server outside the installation that reaches the worker with no policy at all,
    a field lost on the way, reaches nothing too: the worker fails closed.
  - The worker refuses a server whose URL is not listed before sending anything, and
    checks every request as it leaves: redirects, and the endpoints that refresh an
    OAuth grant or mint a GitHub App token. A changed policy reconnects the server.
  - The writers' door checks the server's origins at the PUT too: its URL, its OAuth
    token endpoint and its GitHub App API base (`api.github.com` when none is named).
    It reads the policy inside the write's transaction, holding the policies' lock
    shared until it commits; a policy write holds that lock exclusively. So a policy
    narrowed meanwhile is either waited for or the one the server is judged by. A name
    another scope holds answers 409 before the policy is judged.
  - The installation's servers are not held to any list. `netguard` keeps refusing
    metadata and link-local for every server, and NetworkPolicy remains a defence in
    depth, not a substitute.
  - The enforcement ships in its own release, on every replica, before the writers. Once
    an area's server exists, that release is the rollback floor.

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

### Three scopes for one row

A row has three scopes, and the code converts between them in one place:

| Stored as (`scope_kind`, company, area) | Runtime scope (what a run's scope must be inside) | Authorization scope (what a writer must hold) |
|---|---|---|
| `installation`, `''`, `''` | `{Company: Installation}`, which contains every scope | `adminScope` (`default/platform`), unchanged |
| `company`, `acme`, `''` | `acme` | `acme`, or `adminScope` |
| `area`, `acme`, `cx` | `acme/cx` | `acme/cx`, or `adminScope` |

- The stored and runtime columns are what `connectorScopes` does today for connector
  instances (`internal/admin/connectors.go:41-59`). It becomes the one conversion every
  kind uses.
- **A row of the installation keeps today's check.** Writing it still needs the
  permission at `adminScope`, so nobody who configures integrations today loses that
  right. Applying `Require(target)` literally would ask for the permission at the
  installation, which only root administrators hold.
- **The platform's administration governs every area.** A grant at `adminScope` writes
  any company's or area's row, as it writes every row today. `adminScope` does not
  contain `acme/cx` by `Scope.Contains`, so this is an explicit second condition, not a
  consequence of the hierarchy.

### The rule itself

One rule, for every kind:

1. **A resource belongs to the scope it is stored at.** An area's resource is stored at
   `area` with its company and area, and a company's at `company`. The platform's stays
   at `installation`.
2. **Writing it needs the permission at its authorization scope**, from the table above.
   `configurer()` takes the target and checks `Require(provider:write, target)`, or the
   permission at `adminScope`. A grant on `acme` writes `acme` and every area under it,
   and a grant on `acme/cx` writes only `acme/cx`. That is `Scope.Contains`, as for
   agents (`internal/domain/ids.go:80-90`).
3. **A run uses only resources whose runtime scope contains the run's scope.**
   Connectors and conversations already do this. MCP servers, providers and channel
   connections gain the same check, at every door (see [Enforcement](#enforcement)).
4. **A configuration listing shows what the caller configures, and nothing to anyone
   else.** The configuration listings (`ListIntegrations`, `ListConnectorInstances`,
   `ListChannels`, and the `Get` of each kind) carry addresses: an MCP server's URL or
   command, a connector's hosts and databases, a provider's base URL, a channel's
   settings. They are not for authors.

   `author` holds `tool:read` (`internal/domain/access.go`, `RoleAuthor`). #324 opened
   only the tool catalogue to `tool:read`, because it names no address. So `V` is the
   scopes where the caller holds `provider:write` (`auth.VisibleScopes(ctx,
   PermProviderWrite)`), which `admin` and `curator` hold and `author` does not. A row is
   listed when either holds:
   - **Inherited:** its runtime scope contains a scope in `V`. An area administrator sees
     the company's rows and the platform's, marked inherited, and read-only unless rule 2
     lets the caller write them.
   - **Governed:** a scope in `V` contains its runtime scope, or the caller holds
     `provider:write` at `adminScope`. A company administrator sees every area of the
     company, and the platform's administration sees every row.

   Nothing else is listed. An area never sees a sibling area's rows. Tests cover an area,
   a company and a platform caller, and an author, who is refused 403.
5. **What an author picks from is a reduced answer, on `tool:read`.** Building an agent
   needs names, not addresses:
   - **Tools:** `ListTools`, open to `tool:read` in any scope since #324, now asked for a
     target as below.
   - **Providers and their models:** a new `GET /providers`, which answers name, kind,
     models and whether each is usable. It never answers a base URL or anything sealed.
     Usable is what the serving process's registry answers (`Registry.Usable`: switched
     on, credential opened, address whole) for a provider switched on; with no registry
     wired, a provider switched on. Providers are stored at the installation today, so
     every reachable target is answered the installation's; the filter is the same
     containment rule, ready for an area's providers.
     The agent editor reads providers from `ListIntegrations` today
     (`web/src/features/agents/agent-serving-fields.tsx`, `agent-fallback-fields.tsx`,
     `web/src/features/experiments/experiment-config-fields.tsx`). That listing needs
     `tool:read` at `adminScope` (`internal/httpapi/connections.go:65-66`), so an area
     author cannot load providers at all. The editor moves to the reduced answer.

   **Each reduced answer is asked for one target scope**: the scope of the agent being
   edited, or chosen for a new one. A union of the caller's scopes would be wrong in
   both directions. An author of `cx` and `finance` editing a `cx` agent would be offered
   `finance`'s tools, which `checkScope` then stops. And a company author editing an
   agent of `acme/cx` would see only the rows that contain `acme`, and miss `acme/cx`'s
   own.

   - `ListTools` and `GET /providers` take a `scope` parameter, `company` or
     `company/area`.
   - **Authorization:** the caller holds `tool:read` at a scope that contains the target.
     An `acme` grant asks for `acme/cx`, and an `acme/cx` grant cannot ask for
     `acme/finance`. Otherwise the answer is 404. A 403 would confirm that an area the
     caller cannot see exists.
   - **The answer:** every row whose runtime scope contains the target, by rule 3, and
     only those. That is exactly what an agent stored at the target can run, so what the
     editor offers and what `checkScope` admits are the same set.
   - **Without `scope`**, an answer holds only the installation's rows, which every
     scope inherits, and nothing of any area is answered without naming it.
     **Exception, decided in part B2a:** a caller holding `tool:read` at `adminScope` keeps
     every row without `scope`. The platform's tools screen lists every tool and every
     owner's ruling on it (B1's `rulings`), and is where the ceiling is ruled and
     withdrawn; answering it only the installation's tools would hide every area's tool
     from the one screen that rules over them. The platform's administration already
     sees every row (rule 4), so nothing is answered it could not otherwise read. The
     same reader may also name any target, since `adminScope` governs every area but
     does not contain one by `Scope.Contains`.
   - **400 before 404.** A target that is not a real company or area as stored (an
     identifier in another form, a missing part, the installation's `*`) answers 400,
     judged as part A's `realScope` judges a write's scope. The form of a request says
     nothing about which scopes exist; only then is the caller's reach judged.
   - **The worker's rulings stay whole.** The filter lives in the listing door, never in
     `Curator.List`, which the worker's `Catalog.Sync` reads with the empty scope: Sync
     resets every tool no ruling decides (B1), and a narrowed source would reset area
     tools their owners ruled on. A journey test pins it.
   - The agent editor, the fallback fields and the experiment form pass the agent's
     scope. The experiment form passes the experimented agent's scope.
   - **What a curator governs is a third answer, decided in part B2b:**
     `ListTools?classifiable=true`. A target answer runs upwards (rows whose scope
     contains the target), so a company curator asking for `acme` never sees
     `acme/cx`'s tools, which an `acme` grant may classify. This answer runs downwards:
     the tools whose server is stored at a company or an area that a scope where the
     caller holds `tool:classify` contains, as the owner's door (`ClassifyTool`,
     `level: owner`) authorizes it. A company's grant answers its own and its areas';
     an area's only its own; never a sibling's, never the installation's. The
     platform's curator (`tool:classify` at `adminScope`) gets every tool. A caller
     holding `tool:classify` nowhere is answered 403, as every classify door answers
     them; asked together with `scope`, 400, since the two answers run in opposite
     directions and a combination would have to invent which one wins. The console's
     queue of tools waiting asks this once.

### Names are unique across the installation, in the database

A tool id (`server.tool`, `connector.instance.operation`) has no scope in it, and
neither does the name an inbound message is matched by (`settings.Store.Named`,
`internal/settings/secrets.go:111-116`). Putting a scope into every id would touch every
pack, policy, ruling and ledger step ever recorded. So the name stays the identity, and
it is unique per kind across the installation.

- **The settings primary key already makes a name unique within one scope**, whatever
  the connector. A connector instance is stored by its name alone, with its connector
  type in the value (`internal/admin/connector_write.go:112`). So `jira.prod` and
  `sql.prod` cannot both exist in one scope today.
- **For connector instances, the name alone becomes unique across the installation,
  not `(connector, name)`.** This is a decision, and it is stricter than the tool id
  needs: `jira.prod` in one area and `sql.prod` in another do not collide as tool ids.
  The name is chosen because the name alone is already the identity elsewhere:
  - within one scope, by the primary key;
  - in bindings, where `boundVault` finds the bound Vault by name alone across every
    scope that contains the consumer, and only then checks it is a Vault
    (`internal/connectortools/sql_config.go:169-190`). A same-named instance of another
    connector in a containing scope makes the binding ambiguous today.

  **Incompatibility, stated:** two instances of different connectors may no longer share
  a name in different scopes. Phase 0's migration check names any that exist, and the
  release notes say so.
- **The database enforces it.** Phase 0's migration creates a partial unique index:

  ```sql
  create unique index settings_integration_names on settings (kind, name)
  where kind in ('mcp_server', 'model_provider', 'connector_instance',
                 'channel', 'channel_conversation');
  ```

  - It binds every binary, the previous one included. An older replica that writes a
    taken name, during the rollout or after a rollback, gets a unique violation.
    It never writes a duplicate.
  - Two writers racing for one name cannot both commit: the index serialises them.
  - Kinds that legitimately repeat a name across scopes stay out of it: classifications
    (see [Classification](#classification-under-the-platforms-ceiling)), personal MCP
    credentials, channel identities and budgets.
- **A refused write** answers 409 `fuseone:integration-name-taken`. The new binary
  maps the unique violation of that index to the 409. The refusal says the name is
  taken, and never by whom: another area's names are not the caller's to read.
- **Existing rows.** The migration first counts names held in more than one scope. If
  there are any, it raises an error naming each duplicate's kind and name, and creates
  no index, so the upgrade stops before anything changes. A read-only query does the
  same count ahead of the release against every installation that will take it, so a
  stopped upgrade is never the first anyone hears of it. The platform's administration
  renames one of the two.
- **Rollback.** The index outlives any rollback, because migrations do not go back.
  Below phase 0, a binary that writes a connector instance under a taken name gets
  400 `fuseone:not-stored`, which is how that handler answers a failed write, rather
  than the 409. Nothing is written, so it fails closed. Phase 0's release notes say so.

The collision above therefore cannot happen. As a guard, the connector layer refuses
a set that holds one name twice, judged by the name alone, as the index does, and keeps
serving the set it had; it never overwrites. Its key stays `(connector, name)`, only to
find an instance from a tool id, which carries no scope: keying by scope would give no
lookup anything to match.

## Kind by kind

### Connector instances

The instance already has the scope. Only the doors change:

- `PutConnectorInstance`, `DeleteConnectorInstance` and `GetConnectorInstance` check
  `provider:write` at the instance's scope, not at the platform's.
- `ListConnectorInstances` is filtered by rule 4, on `provider:write`.
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
    only the tools usable from the target scope it is asked for, by rule 5: each entry
    carries the runtime scope of the server or connector instance that declares it (the
    installation's for a `--mcp` flag's server and the platform's own tools), and a tool
    is answered when that scope contains the target. Without a target the platform's
    administration keeps every tool, and anyone else gets the installation's (rule 5's
    exception).
  - Before part B2a it filtered only who declares each tool (`declaringTools`).
- **Personal credentials** (`mcp_user_credential`) stay keyed by server and person
  (`internal/admin/integrations.go:752`). A person stores one only for a server they
  can see.

#### Classification under the platform's ceiling

A tool has at most **two rulings**, both of kind `tool_classification` and named by
the tool id. They are told apart by the scope they are stored at, which the `settings`
key already allows.

| Ruling | Stored at | Written by | Door |
|---|---|---|---|
| **Owner's** | The server's own scope (`company` or `area`) | `tool:classify` at the server's authorization scope | `ClassifyTool` with `level: owner` |
| **Platform's**, the ceiling | `installation`, `''`, `''`, as every ruling is today (`internal/admin/curator.go:83`) | `tool:classify` at `adminScope` | `ClassifyTool` with `level: platform`, today's behaviour, the default |

- For a server of the installation, the two are the same row. There is one ruling, as
  today.
- **The owner's door** refuses a tool whose server is not stored at a scope the caller
  may classify. That covers the platform's own servers and a sibling area's servers.
  `level: owner` on a server of the installation is the platform's door, and needs
  `adminScope`.
- **Withdrawing the ceiling.** A new door, `DELETE /admin/tools/{tool}/classification`
  with `level: platform`, needs `tool:classify` at `adminScope` and removes the
  platform's row. Nothing lets an owner write or remove the platform's row.

**Precedence, exactly.** `Catalog.Sync` resolves each tool on its own:

1. If a platform row exists for the tool, it is the ruling. The owner's row is ignored
   for that tool, and listed as "overridden by the platform".
2. Otherwise, the owner's row is the ruling.
3. Otherwise, there is none, and the tool never executes, as today
   (`internal/gate/checks.go:58-60`).

The ruling chosen in step 1 or 2 is then checked against the current definition digest
(`internal/tools/classify.go:36-77`). If it is stale, the tool is stale. It does **not**
fall back to the other row: a ceiling the platform set against an older definition
stops the tool until somebody re-rules. It never quietly hands the decision back to the
owner.

- `Curator.List` returns each row's stored scope, which it drops today
  (`internal/admin/curator.go:114-118`). The resolution needs it.
- **Against the previous binary.** An older reader sees two rows with one name, and
  applies whichever its query returns last. So no owner row is written before every
  replica resolves precedence: phase 2's readers' release ships first (see
  [Rollout](#rollout-readers-a-release-before-writers)).
- **Policies do not move.** `policy:write` stays at the platform's scope. Every policy,
  approval rule and standing approval applies to an area's tool exactly as to the
  platform's: the Gate does not ask who classified the tool.

The risk the decision accepts: an area can call `read` a tool that writes. The ceiling
limits who is exposed. The tool can only be reached by the area's own agents, so the
mistake stays inside the area that made it. It also stays visible: the ruling names its
author, the platform's screen lists every owner ruling, and the platform can overrule
any of them.

### Model providers

- A provider is stored at the caller's scope, its key sealed in the row.
- **The registry knows each provider's scope**, and `Planner` takes the run's scope. A
  provider whose scope does not contain the run's is not found. The error names the
  provider, never its key or its owner. `Usable` (`internal/model/registry.go:157`)
  takes the scope too.
- **Every door that names a provider checks it against the scope it will run in.**
  Publication is not enough, because an experiment can replace the provider and a judge
  names its own:

  | Door | Where | What it checks |
  |---|---|---|
  | Publication | `Publisher.Publish`, the HTTP route, the disk sync | The main pair and the fallback (`Spec.Fallback`), against the agent's scope |
  | Starting an experiment | `StartExperiment` → `unservedPair` (`internal/httpapi/experiments_intention.go:115-133`) | The provider the intention runs on, which may be `intent.Config.Provider` instead of the version's, against the agent's scope |
  | Planning an experiment run | `ResolveExperiment` (`internal/spec/resolver_experiment.go:24`, where `config.Provider` replaces the pair's) | The same, against the run's scope, through `Planner` |
  | Opening a judgment | The writer of #298's release 2, not built yet | The judge's provider, against the experiment's scope. That release cannot ship without this check |
  | Planning a judge run | `ResolveJudge` (`internal/spec/resolver_judge.go:39-51`) | `opening.Provider`, against the run's scope, through `Planner` |

  The doors at start and opening answer early and with a reason. The checks in the
  resolvers hold against a row that reached the ledger any other way.
- **Cost.** Runs are already charged to their own scope, so an area provider's spend is
  the area's. Prices (`/admin/prices`) stay the platform's.

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
| Gate | A new check, `checkScope`, beside `checkContract` (`internal/gate/gate.go:73`). The Gate cannot judge scope today: its `Request` carries the run's scope and the tool's effect, but not where the tool belongs (`internal/gate/pack.go:57-70`). `Request` gains `ToolScope`, and the check stops the call unless `ToolScope` contains `Scope`. That stop is recorded in the ledger as a `blocked` verdict with its reason. Where `ToolScope` comes from is the next section |
| Model resolution | `Planner` with the run's scope, so a provider out of scope is not found |
| Channel delivery | `Configured.For` as today, plus the connection's scope |
| Classification sync | The precedence in [Classification](#classification-under-the-platforms-ceiling): the platform's row, else the owner's, never a fall-back on stale |

### Where a tool's scope comes from

The engine reads a tool's effect through the layer chain the worker builds
(`cmd/agentd/worker_parts.go:231-247`): `workitem` → `memory` → `contextshare` → the
native connector layer → the MCP catalogue. Each layer answers for the ids it claims and
passes the rest down. `ToolScope` travels the same way, through a `ScopeOf(tool)` beside
the effect lookup (`internal/engine/workitem.go:56`, `act.go:118,178-184`):

| Layer | Tools | `ToolScope` |
|---|---|---|
| `workitem`, `memory`, `contextshare` | The platform's own tools they serve: claims and links, memory, shared context | The installation's runtime scope `{Company: Installation}` |
| Native connector layer | The platform's own `$fuseone.ticket.answer` (`internal/connectortools/layer.go:123-125`) | The installation's runtime scope, as for the three above |
| Native connector layer | `connector.instance.operation` | The instance's runtime scope |
| MCP catalogue | `server.tool` | The server's runtime scope |
| None claims the id | — | Zero, which contains nothing (`internal/domain/ids.go:80-90`), so the call is stopped |

**The platform's tools are the explicit set that already exists**, not a list per layer.
`ToolID.PlatformOwned()` names them (`internal/domain/ids.go:22-31`):
`ToolContextRead`, `ToolTicketAnswer`, `ToolMemoryFind`, `ToolMemorySuggest`,
`ToolWorkItemClaim` and `ToolWorkItemLink`. The set is spread across layers: the ticket
answer is served by the connector layer, not by a platform layer of its own.

- **The set becomes a list.** `domain.PlatformTools()` returns it, and `PlatformOwned`
  reads from it, so the test below and the method cannot disagree.
- **Each id in the set is answered by the layer that serves it**, with the
  installation's runtime scope, explicitly. No layer derives it, and no default supplies
  it. A zero-value fallback would stop every one of them the day `checkScope` ships. A
  "no scope means everywhere" default would let an unclaimed id through.
- **An id outside the set** gets the scope of the server or instance that declares it,
  or zero.
- **The anchor test in phase 0 iterates `PlatformTools()`.** It does not use a
  hand-picked sample. It builds the worker's real chain (`workerParts.deps`), and for
  every id in the set asserts that `ScopeOf` is the installation's and that the Gate
  admits it from an `acme/cx` run. A tool added to the set later is covered without
  editing the test.
  - Sabotage: drop the ticket answer's declaration from the connector layer. The test
    must name `$fuseone.ticket.answer`, stopped by `checkScope`.
  - A second test asserts an unclaimed id is stopped, so the anchor cannot pass by
    `checkScope` being absent.

The Gate's check is the one the ledger shows. The check at invocation is a second wall
beneath it. Neither replaces the other: a test of one does not prove the other.

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
| 0 | The unique index on names, with its duplicate check and the 409. The connector layer refusing one name configured twice. The Gate's `checkScope`, for every tool, with `ToolScope` declared by every layer and the platform's own tools anchored, and the built-in checks named on every decision | One release. MCP servers, providers and channel connections are the installation's, which contains every run. Connector instances may already be stored at a company or area: a run reaching one outside its scope was already refused by the connector layer, and is now stopped earlier, by the Gate, as a recorded block. Its notes state that a binary below it answers 400 `fuseone:not-stored` on a taken connector name |
| 1 | Connector instances: doors at the instance's scope, filtered listing, catalogue on `tool:read` | Writers only. The readers already check scope |
| 2 | MCP servers and classification | Readers, then writers |
| 3 | Model providers | Readers, then writers |
| 4 | Channel connections | Readers, then writers. Conversations change only their doors, like phase 1 |
| 5 | Console: gate, scope field, inherited marks | With each writers' release, per kind |

Phase 1 is the smallest step to something an area administrator can use: connecting
their own Jira, Elasticsearch or SQL from their own area.

## Tests that must exist

- **The write check is at the authorization scope.**
  - An `acme/cx` administrator writes an `acme/cx` instance. The same administrator is
    refused 403 for `acme/finance` and for the installation.
  - A platform administrator, whose grant is at `adminScope`, writes the installation's
    instance and `acme/cx`'s.
  - Sabotage, three runs: check `Require(target)` literally, which loses the platform
    administrator on the installation row; drop the `adminScope` condition; check at the
    caller's first visible scope. Each time, the test must name the write it refused or
    the one it leaked.
- **The Gate stops an out-of-scope tool and the ledger says so.**
  - An agent of `acme/finance` whose pack names `acme/cx`'s MCP tool runs. The row was
    planted directly, bypassing publication.
  - The run's ledger holds a `blocked` decision from `checkScope`, with the reason, and
    the server is never contacted.
  - Sabotage: drop `checkScope` and keep the check at invocation; the test must find no
    `blocked` verdict. Then drop the check at invocation and keep `checkScope`; a
    separate test calling the tools layer directly must find the call made.
- **A run never reaches another area's provider.** Each provider door in the table
  above answers for its own test:
  - publication of the main pair, and of the fallback;
  - `StartExperiment` with `intent.Config.Provider` naming `acme/cx`'s provider for an
    `acme/finance` agent;
  - `ResolveExperiment` for an experiment run whose started payload names it;
  - `ResolveJudge` for a judge run whose opening names it.

  For the two resolvers, the payload is written to the ledger directly. Sabotage: check
  at publication only; the test must name each door that let it through.
- **Names, in the database.**
  - A second scope taking a name is refused 409. The body names neither the holder nor
    its scope.
  - Two writers racing for one name, through the contention fixture: exactly one commits.
  - The previous release's exact `settings` upsert, run after phase 0's migration with a
    taken name, is refused by the index.
  - The migration over a database holding a duplicate stops, names the duplicate, and
    leaves no index. One of its cases is `jira.prod` in one area and `sql.prod` in
    another: the decided incompatibility.
  - Sabotage: drop the index and keep the binary's 409 mapping; the old statement must
    get through, and the test must name it.
- **Configuration listings, for four callers.**
  - An area administrator sees its own rows and the inherited ones, marked, and none of
    a sibling area.
  - A company administrator sees the company's rows, every area's under it, and the
    platform's.
  - The platform's administration sees every row.
  - An author, who holds `tool:read` and not `provider:write`, is refused 403 on every
    configuration listing and `Get`. The same author reads `GET /providers` and
    `ListTools`, and neither holds an address. Sabotage: list on `tool:read`; the test
    must name the address the author read.
  - Sabotage: keep only the inherited relation; the test must name the descendant row the
    company administrator lost. Then return every row; the test must name the sibling's
    row the area saw.
- **Reduced answers, per target.**
  - An author granted on `acme/cx` and `acme/finance` asks `ListTools` and
    `GET /providers` for `acme/cx`. The answer holds the area's, the company's and the
    installation's rows, and none of `acme/finance`.
  - An author granted on `acme` asks for `acme/cx`, and the answer includes `acme/cx`'s
    own rows.
  - An `acme/cx` author asking for `acme/finance` gets 404.
  - Without `scope`, only installation rows, except for a reader at `adminScope`, who
    keeps every row.
  - Sabotage: answer the union of the caller's scopes; the multi-area test must name
    `finance`'s tool. Then filter by the caller's grants instead of the target; the
    company test must name the area's row it lost.
- **Classification precedence.**
  - An area curator classifies its own server's tool with `level: owner`, and is refused
    on a platform tool and on a sibling's.
  - With both rows present, the platform's applies. A later owner ruling changes nothing
    in effect, and is listed as overridden.
  - Withdrawing the platform's row makes the owner's apply.
  - A stale platform row stops the tool, and does not fall back to the owner's.
  - Sabotage: resolve the owner's row first; then fall back on stale. Each time, the test
    must name the tool whose effect changed.
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
