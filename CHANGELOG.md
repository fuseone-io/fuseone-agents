# Changelog

What changed between versions, written for the person who has to decide whether
to upgrade an installation that is currently working.

Versions are [semantic](https://semver.org). A release is a `v`-prefixed tag,
and pushing one builds the image and chart that carry that version — nothing is
released by merging. The cut is a `docs(changelog): cut X.Y.Z` pull request.
`make release V=X.Y.Z` tags whatever commit is checked out, so run it with the
cut's merge commit on `main` checked out, and nothing after it; a later commit
would be released without its notes (see [Releases](README.md#releases)).

## What each section is for

**Upgrade notes** comes first when it exists, and it is the only section that
is not optional. This platform refuses by default: a permissive value standing
in for "nobody has said" is a bug here, so several changes deliberately *stop*
things that used to work until somebody decides. An operator who meets that
without warning reads governance as breakage and works around it, which is the
one outcome worth writing a changelog to prevent.

**Added**, **Changed**, **Fixed**, **Security** describe what a reader would
notice. A refactor nobody can observe does not belong in any of them.

Entries say what happens, not what was implemented. "Local tool servers stop
until somebody accepts running them" is useful; "added AcceptsLocalExecution
field" is a commit message.

---

## [Unreleased]

### Added

- **An account made here can be removed** (`DELETE /admin/people/{principalId}`).
  Removing deactivates rather than deletes: the account stops signing in, every
  session it holds is revoked and every grant it holds is taken away, in one
  transaction, and `person.removed` is recorded with the two ids. The person leaves
  the people list; audit and run views that name them by id keep reading. Only
  accounts created with a password or by the setup token: somebody an identity
  provider vouched for is refused (`fuseone:person-not-local`), since their next
  sign-in would bring them back — change the group in the provider. Nobody can
  remove themselves (`fuseone:person-is-you`), nor the last active holder of
  administration over the whole installation (`fuseone:last-administrator`).
  Requires `identity:write` over the installation, like the rest of people
  management. The username stays with the removed account and cannot be given to a
  new one. In the console, **Remove** appears on such a person's row, after a
  confirmation that names them; never on your own row or a provider's person.

### Changed

- **A grant names its company and area from lists** rather than typed text, in a
  person's access editor and in a service account's "What it may do":
  the companies registered (a withdrawn one is not offered for a new grant) and the
  chosen company's areas, or the whole company. A grant that already names a
  withdrawn company, or a scope from before areas were registered, still shows and
  saves unchanged.

## [0.110.0] — 2026-11-13

NT-019 phase 2, the writers' release (#335, #340, #341, #342, #343): the administrator of an area configures
that area's own MCP servers, and the area's curator classifies their tools under the
platform's ceiling. The servers are remote; local servers stay the platform's. What an
author picks tools and providers from is asked for the agent's scope. Adds migration
`0110_area_mcp_servers_are_remote`.

### Upgrade notes

- **Every replica must already run 0.109.0**, the egress barrier, before this one. An
  older worker either joins a server's configuration and credential by name alone
  (before 0.108.0) or does not hold an area's server to the platform's egress policy
  (before 0.109.0).
- **Rollback floor:** once an area's MCP server exists, never go below 0.109.0. Rolling
  back to 0.109.0 is safe.
- **The integrations listing (`GET /admin/integrations`) changes who may read it.** A
  reader of the platform reads it as before. Whoever configures in some scope reads it
  too, with their own servers, the ones they inherit, and those of the areas they
  govern; never a sibling area's. Each server says where it is stored and whether the
  caller may change it (`editable`).
- **The tool catalogue (`GET /admin/tools`) answers less without a target.** A reader of
  `tool:read` at the platform's administration still gets every tool. Anyone else
  without `scope` now gets only the installation's tools; an area's or a company's
  tools are answered when the request names a target they reach (`?scope=acme/cx`).
- **Withdrawing the platform's ruling on a tool needs every worker on this release.** A
  worker on 0.109.0 or older keeps applying a withdrawn ruling that has no owner's ruling
  under it until it restarts; this release's worker stops the tool at its next sync.
  Owner rulings themselves are safe from 0.107.0, whose readers already resolve
  precedence.

### Added

- **An area's own remote MCP servers.**
  - Writing one needs `provider:write` at its scope, or at the platform's
    administration. Reading, probing, rediscovering and removing it need the same, at
    the scope where it is stored.
  - A server's scope does not move: a name another scope holds answers 409
    `fuseone:integration-name-taken`.
  - A local (stdio) server from an area is refused, by the door with 400, by the
    database, and by the worker, which starts no such process. It would run that area's
    commands on the platform's host.
  - A remote server reaches only the origins the platform's egress policy allows its
    scope (0.109.0). The door checks the server's URL, its OAuth token endpoint and its
    GitHub App API base (`api.github.com` when none is named) against that policy, and
    answers 400 `fuseone:egress-not-allowed` for an origin outside it and for a scope
    the platform allows nothing; the problem's `detail` is the canonical origin alone. A
    policy changed while the write is in flight is either waited for or the one it is
    judged by. A name another scope holds still answers 409 first.
  - The scope a write names must be a real company or area: the installation's `*`,
    an identifier in another form (upper case, spaces), or a company or area without
    `scopeKind` answers 400.
  - A door acts on the server it authorized and no other. A server removed and stored
    again under the same name in another scope meanwhile is not removed, probed or
    rediscovered by a request authorized for the first, and its rediscovery status
    shows nothing of the first.
  - Writing and removing a server of one name take turns, whatever the scope: a
    removal never clears what a concurrent creation of the name in another scope
    stored.
- **A personal MCP credential is held only for a server the person's scopes reach.**
  Checked under the server's row when the credential is written, and recorded for that
  row. Reach goes both ways: a server the person's scopes inherit, and one stored in a
  scope a grant of theirs contains, such as an area of their company. A server out of
  reach answers as one that does not exist.
- **An area classifies the tools of its own MCP servers, under the platform's ceiling.**
  - `PUT /admin/tools/{tool}/classification` takes `level: owner | platform`. Without
    one it is `platform`: today's ruling, stored at the installation, needing
    `tool:classify` at the platform's administration.
  - `level: owner` stores the ruling at the scope where the tool's server is stored,
    and needs `tool:classify` there, or at the platform's administration. On a server
    of the installation it is the platform's row and needs the platform's
    administration.
  - A tool whose server the caller may not classify (the platform's own servers, a
    sibling area's, or a tool nobody published) answers one 403 naming the platform's
    administration, whichever it was. The platform's curator gets 400 for a tool whose
    server is not stored at any scope, such as one a `--mcp` flag connected.
  - The ruling lands on the server row the door authorized, or nowhere: a server
    removed and stored again in another scope meanwhile answers 409, and a removal in
    flight is waited for.
  - `DELETE /admin/tools/{tool}/classification?level=platform` withdraws the platform's
    ruling and needs `tool:classify` at the platform's administration. The owner's
    ruling, if any, is then in force; with none the tool never executes. Nothing lets
    an owner write or remove the platform's row.
  - `GET /admin/tools` gains `rulings` on each tool: the platform's and the owner's,
    each with its `level`, `scope`, `effect`, `untrusted`, `compensatedBy` and
    `dedupe`, the one `inForce`, and an owner's ruling under a platform ruling marked
    `overridden`. Each row carries its own, so editing one level starts from that
    level's word.
  - `GET /admin/tools?classifiable=true` answers what the caller may rule on as the
    owner: the tools whose server is stored at a company or an area contained by a
    scope where they hold `tool:classify` (a company's contains its areas'), every
    tool for the platform's curator, never a sibling's or the installation's, and never
    a native connector tool, which belongs to no stored MCP server. 403 for a caller
    who classifies nowhere; 400 together with `scope`.
  - The console offers it per tool, on a server's page and in the administration's
    queue of tools waiting: as the owner on a company's or an area's tool contained by
    a scope where the caller holds `tool:classify` (`/me`'s `classify`), never on the
    installation's or a sibling's. The queue asks `classifiable=true`, so a company's
    curator sees their areas' tools. The dialog starts from the listed ruling of the
    level being recorded, never the one in force, and opens blank on a level nobody
    ruled at. The platform's curator keeps recording the ceiling by default,
    may record the owner's ruling instead on a company's or an area's tool, and alone
    is offered withdrawing the ceiling. The dialog lists the tool's rulings, the one in
    force, and an owner's ruling overridden by the platform.
- **What an author picks from is asked for one target scope.**
  - `GET /admin/tools` takes `scope`, `company` or `company/area`, and answers every
    tool an agent stored there can run: those whose server's (or connector instance's)
    runtime scope contains the target, the target's own, its company's and the
    installation's, never a sibling area's. It is the Gate's scope check over the same
    scopes. Every other field of an entry, `rulings` included, is unchanged.
  - The caller holds `tool:read` at a scope containing the target, or at the platform's
    administration; otherwise 404, so the answer does not confirm a scope the caller
    cannot see. A target not stored as given (upper case, spaces, a missing part, the
    installation's `*`) answers 400.
  - Without `scope`, the platform's administration keeps every tool, the screen the
    ceiling is ruled from; anyone else gets the installation's tools.
  - A new `GET /providers`, on `tool:read`, answers each provider's name, kind, models
    and whether this installation would ask it now (`usable`), never an address, a key
    or anything sealed. It is asked for a target by the same rule; providers are the
    installation's today, so every reachable target is answered them.
  - The worker still reads every ruling whatever a listing asked for: the filter is in
    the listing, never in the rulings a sync applies.
  - Each listed tool says where its server is stored (`scope`), and a tool's scope is
    read in the same snapshot as the tool, so a server moved meanwhile never hands one
    area's tool to another.
  - `/me` tells an area curator where it classifies (`classify`), for the console that
    offers an owner's ruling per tool; `tool:classify` leaves the console's
    platform-only list with that console.
  - The console asks for them: the agent editor's tool picker and its provider and
    fallback pickers, and an experiment's provider picker, send the agent's scope
    (a new agent's once its area is chosen) and read providers from `GET /providers`.
    A target out of the author's reach reads as such, and a provider with `usable:
    false` is marked unavailable and refused as a new choice (kept when already
    chosen). An existing agent's capabilities, guide and read-back ask for the agent's
    scope too, and the agents list asks once per distinct scope on the page.

### Fixed

- **The ticket answer is listed as the installation's.** The runtime served it to every
  scope, but its catalogue entry carried no scope, so the flow check left it out of the
  rulings it reads.
- **A tool no ruling decides reads unclassified at the worker's next sync.** The worker
  applied the rulings it read and left every other tool as it was, enough while rulings
  were only ever replaced. A withdrawn platform ruling, or an owner's ruling whose server
  moved to another scope, now stops deciding the tool.
- **A connector instance's scope must be a real company or area.** Its writes shared
  the check that let `company="*"` through, a row every reader takes for the
  installation's, and an area typed in another form, a row no registered area reaches.
  Both now answer 400.

## [0.109.0] — 2026-11-13

NT-019 phase 2, the egress barrier: before areas may register MCP servers, the platform
decides where those servers may send requests. No door writes an area's server yet, so
nothing changes for an installation today. No migration.

### Upgrade notes

- **Every `serve` and `worker` replica must run this release before the writers'
  release.** An older worker does not know the egress policy, and would let an area's
  server reach any endpoint the worker can, with that area's credential.
- **Rollback floor, from the writers' release on:** once an area's server has been
  written, never go below this release. Rolling back to 0.108.0 before then is safe.
  The floors already set still hold.

### Added

- **An egress policy for MCP servers outside the installation** (#338). The platform's
  administration keeps one per company or area, a list of exact origins, through
  `GET`, `PUT` and `DELETE /admin/integrations/mcp-egress-policies`. An area's
  administrator can neither read nor change it.
  - A server of an area is held to its area's policy, or else to its company's, never
    both. With neither, it reaches nothing. An empty list is a deny-all that overrides
    the company's; removing an area's policy inherits the company's again.
  - A server outside the installation whose policy is missing for any reason reaches
    nothing: only the installation's servers, and those no row describes, carry
    none.
  - The worker refuses a server whose URL is not listed before sending anything, and
    every request as it leaves: redirects, and the endpoints that refresh an OAuth grant
    or mint a GitHub App token. A changed policy connects the server again.
  - Origins are scheme, host and port, with no path or wildcard. One that `netguard`
    would never reach (cloud metadata, link-local) is refused.
  - The installation's servers are not held to any policy, and keep their reach.

## [0.108.0] — 2026-11-13

NT-019 phase 2, a second readers' release: what a worker reads and writes about an
MCP server, it reads and writes of the server's row, never of whatever holds the name
now. No door writes an area's server yet, so nothing changes for an installation
today. Adds migration `0109_mcp_requests_name_their_row`.

### Upgrade notes

- **Every `serve` and `worker` replica must run this release before the writers'
  release.** An older worker joins a server's configuration and its credential by name
  alone: once areas write servers, a name removed from one area and stored again in
  another could have its token sent to the first one's URL.
- **Rollback floor, from the writers' release on:** once an area's server has been
  written, never go below this release. Rolling back to 0.107.0 before then is safe.
  The floors already set still hold: never go below 0.103.0.

### Fixed

- **A worker opens the credential of the server row it read** (#336). A row gone from where
  it was read, or revised since, is read again on the next pass. It is not reported
  as the server failing.
- **A person's credential is sent only to the server it was stored for.** Stored again
  for a server of the same name stored elsewhere, it keeps nothing of the old
  credential. One stored before this release belongs to the installation's server.
- **A refreshed OAuth grant is written back only to the row it was refreshed for,**
  for the server's own grant and for a person's.
- **A probe or rediscovery request is answered only by the server it was asked of.**
  It stores that server's row, and a worker holding a server of the name stored
  elsewhere neither runs it nor answers it by connecting. Receipts name the row they
  answer, and a server's rediscovery status shows only its own request and receipts.

## [0.107.0] — 2026-11-12

NT-019 phase 2, the readers' release: every reader of an MCP server and of a tool's
classification learns where it is stored. No door writes an area's server or ruling
yet, so nothing changes for an installation today. No migration.

### Upgrade notes

- **Every `serve` and `worker` replica must run this release before the next one.**
  The next release opens the doors that write an area's MCP servers and rulings. An
  older replica would read an area's server as the platform's: its tools in every
  area's runs, and its credential opened at the wrong scope.
- **Rollback floor, from the next release on:** once an area's server or ruling has
  been written, never go below this release. Rolling back to 0.106.0 before then is
  safe. The floor 0.104.0 set still holds: never go below 0.103.0.

### Changed

- **An MCP server is read where it is stored** (#333). That covers its credential, a probe,
  a rediscovery, its removal, and its OAuth grant's refresh, which is written back to
  the row it came from. Its tools carry its scope into the catalogue, and the Gate
  stops them for a run outside that scope.
- **A tool's classification is chosen the same way everywhere.** The platform's ruling
  wins over its owner's, an owner's ruling counts only at the scope of the tool's
  server, and a ruling stored anywhere else is ignored. The worker's catalogue and the
  console's listing use one rule.
- **A ruling judged against an older definition stops its tool.** The tool returns to
  how discovery leaves it: no effect, untrusted, nothing to compensate with, no dedupe.
  Before, the entry was only marked stale, and what an earlier ruling had set stayed in
  force.
- **An area's server keeps its tools in the shared catalogue.** Publications are judged
  against the configured servers at every scope.

## [0.106.0] — 2026-11-12

NT-019 phase 1: the administrator of an area configures that area's own connector
instances. MCP servers, model providers and channels are still configured by the
platform's administration. No migration.

### Upgrade notes

- **The connector instance listing now needs `provider:write`.** It carries hosts and
  databases, so it is listed to whoever configures connectors in some scope, and no
  longer to a reader of the platform. A platform auditor sees the catalogue of
  connector types, and no instance.
- **`/me` lists `provider:write` in `can` wherever it is held.** A new `configure` list
  names the scopes. MCP servers, providers, channels and the assistant's provider stay
  behind `platform`, so a client that hid them on `can` alone should read `platform`
  for them.
- **Rolling back to 0.105.0 is safe.** Instances an area wrote stay where they are, and
  0.105.0's Gate and connector layer already keep each to its own scope. Only the doors
  close again: an area's administrator can no longer change them. The floor 0.104.0 set
  still holds: never go below 0.103.0.

### Added

- **An area's own connector instances** (#331).
  - Writing, reading and removing an instance needs `provider:write` at the instance's
    own scope, or at the platform's administration, which governs every area. The
    installation's instances keep today's check.
  - The listing shows an area's administrator its own instances and the ones it
    inherits, from its company and the platform. Each item says whether the caller
    may change it (`editable`). Nothing of a sibling area is listed.
  - An area's SQL or Gravitee instance binds only a Vault instance its scope can reach.
  - The connector catalogue is read with `tool:read` in any scope. It names connector
    types, never an instance or a credential.
- **Console.** An area's administrator opens Integrations on the connectors and on
  nothing else. A new instance starts in their own area, and an inherited one is marked
  "Da plataforma" or "Da empresa" and offers no change.

## [0.105.0] — 2026-11-12

NT-019 phase 0: the names of integrations become unique across the installation, in the
database, and the Gate stops a tool its run's scope does not contain. Adds migration
`0108_integration_names_unique`. MCP servers, providers and channel connections are
written at the installation only, so for them nothing changes. Connector instances and
conversations may already be stored at a company or an area; see below.

### Upgrade notes

- **The upgrade stops if two scopes already hold one integration name.**
  - The kinds are MCP servers, model providers, connector instances, channel connections
    and conversations.
  - The migration names each duplicate's kind and name, changes nothing, and the
    upgrade does not proceed.
  - Run this read-only query against the installation's database before upgrading. It
    must return no rows:

    ```sql
    select kind, name, count(*) from settings
    where kind in ('mcp_server', 'model_provider', 'connector_instance',
                   'channel', 'channel_conversation')
    group by kind, name having count(*) > 1;
    ```

  - Rename one of each pair, then upgrade.
- **Connector instances: the name alone is unique.** Two instances of different
  connectors can no longer share a name in two scopes (for example `prod` for Jira in
  one area and for SQL in another). The query above lists any that exist.
- **Rolling back.** The index stays after a rollback, because migrations do not go back.
  Below this release, writing a connector instance under a taken name answers 400
  `fuseone:not-stored` instead of 409. Nothing is written, so it fails closed.
  - The floor 0.104.0 set still holds: once `0107` has run, never go below 0.103.0. The
    floor 0.101.0 set holds too: once the first experiment exists, never go below
    0.100.0.
- **Replays of earlier decisions.** A decision is recorded with the built-in checks that
  made it, now `builtin/v7`. Replaying a decision recorded under other built-in checks
  reports it as not comparable, with the reason. Decisions recorded before this release
  carry no such identity, and are replayed under today's checks as before.

### Changed

- **A connector instance name another scope holds answers 409**
  `fuseone:integration-name-taken` (#329). The answer never says which scope holds it.
- **The Gate stops a tool its run's scope does not contain** (#329), with the rule
  `scope`, recorded in the run's trail as a block.
  - MCP tools and the platform's own tools belong to the installation, which contains
    every run, so none of them is stopped.
  - A connector instance stored at a company or an area was already refused to a run
    outside that scope, by the connector layer (`ErrOutOfScope`). That run is now
    stopped earlier, by the Gate, and the trail says why.

### Design decisions

These are decisions recorded in a note. Beyond phase 0 above, nothing below is
implemented yet.

- **NT-019: integrations per area** (#328). The administrator of an area connects the
  area's own MCP servers, model providers, connector instances and channels, and its
  curator classifies its own servers' tools under the platform's ceiling. An area's
  provider is paid for by the area. Names stay unique across the installation, and
  every reader learns scopes a release before any door writes an area's row.
- **NT-017: a Terraform provider** (#321). Agents, areas, access and integrations are
  managed as code through the same API. Every create sends `If-None-Match: *`, an
  agent's scopes are authorized under the publication lock before any precondition is
  judged, and integrations carry their scope. Deleting an area is refused while it
  still holds configuration, behind a fence that ships a release before the delete.

## [0.104.0] — 2026-11-11

Release C2 of version claims (NT-018): the database itself refuses a version whose digest
it cannot prove, whatever binary sends it. Adds migration `0107_version_claims_enforced`.
No behaviour changes for a 0.103.0 installation.

### Upgrade notes

- **Every `serve` and `worker` replica must run 0.103.0 before upgrading.** This release
  assumes every writer already claims its versions. Coming straight from 0.102.0 or
  earlier, the old replicas have every publication refused until the rollout ends. Go to
  0.103.0 first, and wait until every replica reports it.
- **Rollback floor: never below 0.103.0 once `0107` has run.**
  - A 0.102.0 replica has every publication refused by the database. That fails closed,
    and no wrong row is written, but publishing stops until 0.103.0 or later is back.
  - Rolling back to 0.103.0 is safe, because it claims every version it writes.
  - The floor 0.101.0 set still holds: once the first experiment exists, never go below
    0.100.0.
- **What the migration does to existing data.** A version a 0.102.0 replica wrote during
  the 0.103.0 rollout, without a claim, is claimed without a digest, like every version
  0.103.0's migration found. It stays where it is, and stays current if it is current.
  Republishing it identically is refused, as for every version published before 0.103.0.

### Changed

- **The database enforces version claims** (#326).
  - An insert into `agent_specs` must present the digest its claim holds. A writer that
    presents none, or another one, is refused before `on conflict` is considered, so the
    transaction that would have made that row current aborts with it.
  - An agent's current version can only move to a version claimed with a digest.
  - Pausing, stages, retirement and naming the version already current are unaffected.

## [0.103.0] — 2026-11-11

Release C1 of version claims (NT-018): every version id is bound to the full digest of its
bytes before anything is judged. Administrators of one area are now shown only what the
server lets them do, and can pick tools for an agent. Adds migration `0106_version_claims`.

### Upgrade notes

- **Republishing a version published before 0.103.0 is refused.**
  - The migration claims every existing version without a digest, because its bytes were
    never kept.
  - Publishing the identical bytes of such a version used to make it current again. It now
    answers 409 `fuseone:version-digest-unknown`.
  - To put the definition live again, change it: a change almost always gives another
    version id. If the new id lands on another claim, it is refused the same way, and the
    definition is changed again.
  - Versions that are current today stay current, and runs are unaffected.
- **The disk sync skips such a version instead of failing the boot.**
  - It logs `agent version skipped: published without a digest`, with the agent and the
    version, and the boot line reports a `skipped` count.
  - The row stays as it was, and so does its current state.
  - Cron schedules and webhook paths are reconciled from the version the boot left current,
    not from the newest file. An agent whose every file was skipped keeps its triggers.
- **Every `serve` and `worker` replica must run 0.103.0 before release C2.**
  - C2 installs the database triggers that refuse an unclaimed version.
  - Until then, a 0.102.0 replica still writes versions without a claim. 0.103.0 refuses to
    claim those rows later, and C2 claims them without a digest.
  - Merged or published is not enough: C2 waits until every replica reports 0.103.0.
- **Rolling back to 0.102.0 is supported, with a consequence.** Versions it publishes while it
  runs carry no claim. After coming back to 0.103.0, they cannot be republished identically.
  The floor 0.101.0 set still holds: once the first experiment exists, never go below
  0.100.0.

### Added

- **Version claims** (#323).
  - A new insert-only table, `agent_version_digests`, binds `(agent_id, version_id)` to the
    full SHA-256 of the definition's bytes.
  - `agent_specs` gains the digest column an insert presents. It is not a second record:
    release C2 will require the two to be equal.
  - Every writer claims the version under the event graph lock before it judges anything:
    the console and the API, and the disk sync.
  - The digest is the one `Parse` computed from the bytes it read, and a definition that
    does not carry its own digest does not publish.
- **Two problem types on publication** (#323).
  - `fuseone:version-collision`: the version id already names other bytes.
  - `fuseone:version-digest-unknown`: the id was published before digests were kept.
  - A row found without a claim is refused too. The digest offered is never adopted for it.

### Fixed

- **Authors and administrators of an area can pick tools for an agent** (#324). The tool
  catalogue the agent editor reads required `tool:read` at the platform's administration
  scope, so anyone in an area got 403. It now reads with `tool:read` in any scope.
  - It names tools, their servers and effects, never an address or a credential.
  - Who declares each tool is filtered by where the caller may read agents.
- **Administrators of an area are no longer offered what the server refuses** (#324).
  - Eight permissions are checked only at the platform's administration scope:
    `provider:write`, `tool:classify`, `budget:write`, `policy:write`, `approval:grant`,
    `audit:export`, `brand:write` and `data:erase`. `/me` now lists them in `can` only
    when held there.
  - A new `platform` list says what the caller holds at that scope.
  - The console hides Integrations, and the administration menu for an area auditor, unless
    `platform` allows them.
  - A link into integrations says they are configured by the platform's administration,
    instead of a page of 403s.
  - Integrations configured per area are not in this release; they need their own design
    note.

### Design decisions

- **NT-018: the authoring workbench** (#322). One screen to build an agent by conversation,
  rehearse it live on one case, correct it into a draft change and a dataset case, and run
  the experiment in the same thread.
  - The conversation is free, and the schema is fixed: nothing reaches the draft unless a
    person accepts a validated operation. This revises NT-003 §4 and PRD §6.1.
  - In a rehearsal the author answers for the world, and nothing reaches a real system.
  - Candidates are sealed versions held apart from `agent_specs`. Activating one repeats
    every guarantee of a publication, in the publication's lock order.
  - Version claims are rolled out in two releases, C1 and C2; this release is C1. Nothing of
    the workbench itself ships yet.

## [0.102.0] — 2026-11-11

Release 1 of judge evaluators (#298, NT-016): every replica learns to recognise a judge run
and to run one. Nothing in this release starts a judgment. It adds no migration.

### Upgrade notes

- **There is no judgment writer, and no judgment API, in this release.**
  - Nothing opens a judge run, and no route, console screen or manual page starts, reads
    or compares a judgment.
  - What ships is what every replica must know before anything writes one: the readers,
    the fences, and the simulation pool's ability to run a judge.
- **Every `serve` and `worker` replica must run 0.102.0 before release 2, which adds the
  writers.**
  - A 0.101.0 worker that claims a judge run resolves it as an ordinary experiment run. It
    would plan the judged agent itself on the case input and spend money on an answer
    nobody asked for.
  - A 0.101.0 server would count judge runs among an experiment's runs and cases.
  - Merged or published is not enough: the release that writes judgments waits until
    every replica reports 0.102.0.
- **Rolling back stays as before.** Nothing writes a judge run yet, so going back to 0.101.0
  is safe. The floor 0.101.0 set still holds: once the first experiment exists, never go
  below 0.100.0.
- **Every turn reads one more row.** Both pools now read a run's first step before
  resolving it, by primary key, whatever the length of the run. This adds one query per
  turn. The run's other steps are not read again.
- **Reading a judgment's id as a simulation answers 404.** No such id exists yet.

### Changed

- **Readers and workers know judge runs** (#318).
  - An agent's experiment list leaves out runs that judge an experiment: they count as
    neither runs nor cases of it, and none is its first.
  - No worker plans the judged agent on a judge run, in the simulation pool or the real
    one. The real pool parks one as an unresolved specification. One outside an experiment
    stays refused whatever the worker can do.
  - The doors that guard the start gate, the battery and FU-10 already left judge runs out
    on every replica since 0.100.0, and this is now asserted.
- **The simulation pool runs judge runs** (#319).
  - **What the judge is sent.** The rubric is the system prompt. The case and the judged
    output are the only user turn, each in a block marked with a boundary derived from the
    run's salt. The pack is empty, the ceiling is the judgment's, and the pair is the judge's.
  - **What is checked first.** Every object the opening names is checked against its digest
    before any of it is sent: the rubric, the salt and the copy of the output.
  - **What counts as an answer.** The judge answers with a fixed-width option code, read
    from the finish call as the provider returned it, before the model adapter trims a
    summary or drops an empty headline. With the reason off, anything beyond the code is a
    failed judgment (`judge_unparseable`), never trimmed and never cut to a valid prefix.
    These are failed judgments too:
    - any other key in the call, empty or of spaces;
    - `summary` written twice, or written as null;
    - text beside the call;
    - another tool call in the same response, before or after the finish;
    - anything after the arguments' object but the end, such as `{"summary":"B"} x`.
  - **What a rubric may declare.** A field's presence is what was declared: a zero is a
    value, and a key written twice or as null is refused. A score is any range of integers
    with at most eleven values. Its width is computed without overflow, and its options are
    walked by count, so a range ending at the largest integer ends there.
  - **What is stored.** The judgment and the answer as written are stored salted, at one
    fixed size each, and nothing else of the judge's is stored. An answer over the size is
    `judge_oversized`, never truncated.

### Design decisions

- **NT-016: judge evaluators** (#316). A judge applies a rubric to the outputs of a
  settled experiment. Its result is an opinion with the rubric digest, judge model and
  cost, shown beside held and broken and never counted in them. This release implements
  its first rollout step, and the rest waits for release 2.
  - Each judgment is a set of simulated runs that name the experiment.
  - The value and the reason are content, under retention and erasure. A rubric can
    classify a person, so the chain holds only references, digests and provenance.
  - Every object about a verdict is salted, fixed in size and answered as a fixed-width
    code, so no digest, length or token count left after erasure gives the value back.
  - A judgment requires the whole experiment to be materialized, checked under the corpus
    fence. One idempotency key names one judgment, as it does for an experiment.
  - The platform guarantees structural isolation from the judged output and measures the
    judge's resistance to persuasion; it does not promise it.
  - A reason is off unless the rubric declares it. Declared or not, it is part of the
    rubric's digest, so judgments with and without one are not compared.
  - Broken runs are not judged. Two experiments compare on quality only under the same
    rubric and judge, and only over the cases held on both sides.
  - The issue NT-016 requires before the writers exists (#317). It is for showing
    measurement spend apart from operating spend, and it does not change the phase 3 budget.

## [0.101.0] — 2026-11-10

Experiments (#297), and three reads that crossed a scope boundary. It adds no migration.

### Upgrade notes

- **Every `serve` and `worker` replica must have run 0.100.0 before this release.** This is
  the release that writes experiments. 0.100.0 is the one that taught every replica to tell
  an experiment apart from a battery; a replica on anything older would read an experiment
  as evidence about a version that was never simulated as published.
- **Once the first experiment exists, rolling back below 0.100.0 is not safe.** A replica on
  an older version reads the recorded experiments as batteries and simulations: an
  experiment could then start or stop a version, reset the nightly battery's clock, or let
  an agent leave Draft.
- **Rolling back to 0.100.0 itself stays safe.** It is the compatible floor: it keeps every
  recorded experiment out of batteries and simulations. What it lacks is a way to start, read
  or compare them; the experiments stay in the ledger, and this release reads them again. Let
  running experiments settle first: a 0.100.0 worker does not know an experiment's
  configuration, and plans any run it still has to advance on the version as published. The
  report shows the model each run actually used.
- **Three reads now answer 404 where they used to answer.** Reading a simulation by an id that
  names nothing, another agent's simulation, or one recorded under a scope the caller may not
  read is a 404; the first used to be an empty report and the others the report itself. A
  version comparison naming, or falling back to, a version published under a scope the caller
  may not read is a 404 as well, and so is reading such a version of an agent; the agent's
  history no longer lists it. A client that read an unknown simulation id as "no runs yet"
  must expect a 404.

### Added

- **Experiments** (#309, #311, #314). An experiment runs one agent version against a dataset
  version under a configuration: another provider, model or effort, each optional and the
  version's own when left out. Each case runs 1 to 5 times. Any change, even effort alone,
  runs without the version's declared fallback, so a result never reports one model while
  measuring another; an experiment that changes nothing keeps it. An experiment is never a
  battery and changes nothing about the agent.
  - **Console:** **Experiments** in an agent's menu lists the newest ones, starts one with the
    cost stated first (cases × repetitions = runs), and compares two.
  - **Report:** in a fixed order. First what ran: dataset version, agent version,
    configuration, cases run out of the cases in the version, erased cases and repetitions.
    Then what was found: held, broken and cost. A case broken in any repetition is broken,
    and says in how many: `1 of 5`.
  - **Comparison:** on one dataset version, every case is compared. Across two, only the
    cases both have are listed, the screen says so, and it counts what was left out of each
    side. Cost moves per run.
  - **API:** `POST` and `GET /agents/{agentId}/experiments`, `GET
    /agents/{agentId}/experiments/{experimentId}` and `GET
    /agents/{agentId}/experiment-comparison`. The `Idempotency-Key` names the experiment. A
    retry reaches its runs whatever has changed since and completes an interrupted start; a
    retry that asks for something else is a 409.
  - **Manual:** a new page, *Experiments*, in pt-BR and en-US.

### Security

- **A simulation is read only under its agent and where it was recorded** (#312). Reading
  one folded whatever id the caller typed, under the route's agent and the scope that agent is
  published in today. That showed another agent's simulation, and runs recorded before the
  agent was republished under another scope.
- **A version comparison reads each side only where it was published** (#313). An agent
  republished from one area to another keeps its old versions there, and anybody who may read
  the new area could compare against one, named or picked as the version before.
- **An agent's version is read only where it was published** (#314). Reading an agent by
  version checked only the scope of its current version, so after it was republished under
  another scope anybody who could read the new one could read an old version's whole
  definition, and its history listed every version. A version the caller may not read is now a
  404, and the history lists only the versions they may read.
- **An experiment is read, listed and added to only where its runs were recorded** (#311,
  #314). Every door answers 404 before any other answer, so a retry cannot confirm an
  experiment it has no claim on.

### Fixed

- **A version comparison no longer reports a saving on a case one side never ran** (#311). A
  battery stands in for a corpus case that opened no run with a row that costs nothing, and
  the comparison read it as a run that got cheaper. The case still counts as broken on that
  side, and moves no cost or steps.

---

## [0.100.0] — 2026-11-09

A compatibility release for experiments (#297). There is no experiment endpoint and nothing
writes an experiment yet. This version only teaches every replica to tell an experiment
apart from a battery, so that the release which starts writing them is safe to roll out.
It adds no migration.

### Upgrade notes

- **Every `serve` and `worker` replica must run 0.100.0 before the release that writes
  experiments.** From this version, a simulated run that names an experiment never counts
  as the version's battery: it does not start or stop the version, does not reset the clock
  of the nightly battery, and does not let an agent leave Draft. An older replica counts any
  simulated run, so a single one left behind would read an experiment as evidence about a
  version that was never simulated as published.
- **Once the first experiment exists, rolling back below 0.100.0 is not safe.** A replica
  on an older version would read the recorded experiments as batteries and simulations.

---

## [0.99.1] — 2026-11-09

A fix on top of 0.99.0, and nothing else. It adds no migration.

### Fixed

- **Saving a configuration behind a compressing proxy no longer fails as stale.** A proxy that
  compresses responses marks the `ETag` it forwards as weak (`W/"…"`), and the console sends
  that tag back in `If-Match`. Weak tags never matched, so every save of an MCP server, model
  provider, connector, channel or policy was refused with *Somebody changed this while you
  were editing*, even when nothing had changed. A weak tag now matches the revision it
  weakens. A weakened tag of another revision is still refused.

---

## [0.99.0] — 2026-11-09

### Upgrade notes

- **A worker still on an older version ignores rediscovery requests and records no answer,** so
  during the rollout it does not appear in the list at all. Once it runs this release it records
  an answer when it connects. Migration `0105` only adds two tables.

### Added

- **Rediscovery on every worker.** When a connected tool server gains a tool, an administrator
  asks every worker to look again from the server's page, or a pipeline with
  `POST /admin/integrations/mcp-servers/{name}/rediscovery`. Each worker asks once, on the
  session it already holds: nothing reconnects and no call is interrupted. New or changed tools
  arrive unclassified; unchanged ones keep their classification. The page lists the answers
  workers have recorded: answered, failed or not answered yet. It is not a record of which
  workers are running; a stopped worker keeps its last answer. The request
  is on the admin trail and does not change the server's configuration. Turning the server off
  and on is no longer needed for this.

### Fixed

- **A tool catalogue publication that failed is sent again.** A worker that could not publish
  what it discovered logged it and waited for an unrelated change, so a tool could be offered
  to agents while no screen listed it for classification. The worker now publishes again on its
  next pass, about 30 seconds later, until one is taken.

---

## [0.98.0] — 2026-11-08

The console side of conflict detection, and the API's compatibility statement. No migration,
and nothing changes on the server.

### Upgrade notes

- **Coming from 0.96.x, the 0.97.0 notes apply in full,** including that every pod must be on
  0.96.0 or 0.96.1 first and that migration `0104` erases. During that rollout, a console
  served by a new pod sends `If-Match`, and a pod still on 0.96.x ignores it and can overwrite.
  Upgrading to 0.97.0 first, and letting it finish, closes that window.
- **A console tab opened before the upgrade keeps the old behaviour until it is reloaded.** It
  sends no `If-Match`, so its saves are last-write-wins, as before.

### Added

- **A manual page for automating configuration.** *Automating configuration*, under
  Reference, says how a pipeline reads a policy, tool server, model provider, channel or
  connector instance, tells whether it drifted, and writes it with `If-Match`. It also says
  what counts as a breaking change to the API and how one is announced.

### Changed

- **The console no longer overwrites a change it has not seen.** Saving or removing a policy,
  tool server, model provider, channel or connector instance sends the revision the console
  read. When somebody else changed it first, nothing is written, the edit stays on the screen,
  and it is replaced only when the person asks to reload. A removal reads the revision again
  each time its confirmation opens. An editor opens only on a fresh read that came back with a
  revision; a resource that is gone says so instead of showing an old copy.

---

## [0.97.0] — 2026-11-08

### Upgrade notes

- **Every running pod must be on 0.96.0 or 0.96.1 first.** This release turns dataset versions
  on, and migration `0104` needs pods whose writers already declare themselves. From 0.95.0 or
  earlier, upgrade to 0.96.1 and let it finish rolling out before this one. Skipping it fails
  safely, but corrections from older pods fail during the rollout. Coming from before 0.96.0,
  the notes of every version in between still apply.
- **`0104` erases data, and it cannot be undone.** It clears the note and expectations of every
  correction whose copy of the input is nowhere. Such a case has nothing to run, and nothing would
  ever age its words out. It stays as a gap, where before it refused the whole corpus. Count them
  first:

  ```sql
  select count(*) from regression_cases c
  where not exists (select 1 from run_content i where i.ref = c.input_ref)
    and (c.note <> '' or c.expectations <> '[]'::jsonb);
  ```

- **Deleting a case no longer erases its data.** It records a new version of the corpus, and the
  case's copy of the input and its replays stay under the retention they already had. Subject
  erasure still takes them.
- **During the rollout, `detail.objects` on an erasure from a 0.96.x pod is only a floor.** That
  pod's SQL cannot count what the trigger takes. The objects do go; the count is short only as
  long as such a pod is running.
- **The drift notice is heard for the first time.** The `drifted` event is on by default and never
  reached anyone before. A correction that held in one battery and broke in the next, on the same
  agent version, is now announced to the agent's conversation. The first sweep compares the last
  two batteries already recorded, so an old regression between them can be announced. A restart
  can repeat a notice.
- **Readiness can drop.** When everything a battery ran held but cases were added or changed
  since that can still run, the trust center reports `simulation_not_checked` with status
  `unknown`, where it read as ready; running the battery again clears it. A case added or
  changed since whose input is already gone is coverage lost: that reads as
  `simulation_incomplete` with status `bad`, and running the battery again does not bring it
  back.
- **During the rollout, a scheduled battery window can run each case twice.** An old and a new
  pod key a corpus run differently. Those runs are paid twice, and the case is judged on the
  one that ended worst.
- **Saving a configuration with no change now changes nothing.** This applies to policies, tool
  servers, model providers, channels and connector instances. No `updated_at`, no `updated_by`,
  no trail line and no new policy snapshot. **Saving again is no longer a way to reconnect.** A
  tool server's **Try now** reaches the one worker that claims it. To make every worker
  rediscover a server's tools, turn it off and on, which makes its tools unavailable for a
  moment. The first save of a row written by an older version can be a real write, once.
- **`If-Match` is not honoured by a pod still on an older version.** During the rollout such a
  pod ignores the header and can overwrite. Send it only once every replica runs this release.

### Added

- **Conflict detection on configuration.** A `GET` of a policy, tool server, model provider,
  channel or connector instance answers an `ETag`, and `PUT` and `DELETE` take `If-Match`. A
  stale one is refused with 412, `fuseone:precondition-failed`, and writes nothing. The 412
  carries the current `ETag` when the resource exists, and none when it does not. A malformed header is a 400. Without the header, the last write wins, as
  before. A channel's `ETag` also moves when a conversation or an identity is attached to it,
  and a tool server's when somebody pastes an OAuth grant, but not when a worker rotates one.
- **Dataset versions are on.** Every change to an agent's corpus records a version, and a
  battery names the version it ran and is read against it, so a past result stops moving when
  the corpus changes.
- **A battery says what it has not checked.** The report says which corpus version it read and
  lists cases added or changed since, and those already lost. `SimulationReport` gains
  `datasetVersion`, `unpinned`, `notChecked` and `erasedSince`, all additions.
- **Service accounts in the console.** Under **Administration > Service accounts**, an
  administrator creates an account, grants access, issues and rotates tokens, revokes them and
  disables the account. A token's secret is shown once, and its validity is required.

### Changed

- **The start gate refuses only on a case that ran against this agent version and broke.** A
  case added after the last battery no longer stops a start by itself; a case that broke and was
  deleted since still does.
- **Comparing two batteries across corpus versions** marks a case only one side had, or that
  expected something else on each, as `not_comparable`. It counts as neither fixed nor
  regressed.
- **Policy writes are serialised,** so two writes to different codes can no longer each snapshot
  a set missing the other.

### Fixed

- **The drift watcher never announced a regression** since it shipped in August. It now judges a
  battery before comparing it.
- **A battery could run another case's run back,** when two workers read different versions of
  the corpus. A corpus run is now keyed by the case.

### Security

- **A service account cannot hold a password or a session.** Shipped first in 0.96.1, and
  included here.

---

## [0.96.1] — 2026-11-07

A security fix on top of 0.96.0, and nothing else. It adds no migration and does not turn
dataset versions on. It counts as 0.96.0 for what the next release requires of a running pod.

### Upgrade notes

- **Coming from before 0.96.0, the notes of every version in between still apply.** That
  includes the erasing migrations in 0.95.0 and their preview queries.
- **Find out whether a machine already acquired a sign-in.** Any installation that has run
  0.95.0 or 0.96.0 with a service account should look. Upgrading makes what these find stop
  working, and does not remove it, so the row stays as evidence:

  ```sql
  select principal_id, kind, password_set_at, last_seen_at from principals
  where kind <> 'user' and password_hash is not null;

  select s.principal_id, pr.kind, count(*) as sessions, max(s.last_used_at) as last_used
  from sessions s join principals pr using (principal_id)
  where pr.kind <> 'user'
  group by 1, 2;
  ```

  Setting a password is not on the administration trail, so these rows are the record. A row
  here means a token or an administrator set one, and that machine could sign in whatever
  became of its tokens. Treat it as a credential that was out of your control.

### Security

- **A service account can no longer hold a password or a session.** In 0.95.0 and 0.96.0 a
  token could set a username and password on its own account, through the route that lets
  somebody change their own, and an administrator could set one on its behalf. The machine
  could then sign in and keep working after every token was revoked. That route now refuses,
  with a 403 to the token and a 400 to an administrator. Signing in with a password left
  behind is refused, and a session of a machine stops working on its next request.

---

## [0.96.0] — 2026-11-07

### Upgrade notes

- **Do not skip this version when upgrading to the one that turns dataset versions on.** A
  later release will start recording a version of an agent's corpus on every change, and it
  needs every running pod to already know how. Going straight past this release makes corrections
  from older pods fail during the rollout, without changing the corpus. A subject erasure on
  such a pod can also miss a correction a newer pod has just deleted, if the run's input was
  too large to keep whole.

### Added

- **The groundwork for versioned corpora, switched off.** Migration `0103` adds the tables and a
  switch that nothing in this release can turn on. Recording, deleting and running corrections
  work exactly as they did, and no version is written yet.

---

## [0.95.0] — 2026-11-07

### Upgrade notes

- **This upgrade erases data, and it cannot be undone.** Migration `0102` clears what a
  correction expected wherever its input is already gone, and adds a trigger that does the
  same whenever a copy is erased from now on. Look at what it would take first:

  ```sql
  select count(*) from regression_cases c
  join run_content rc on rc.ref = c.input_ref
  where rc.erased_at is not null and c.expectations <> '[]'::jsonb;
  ```
- **This upgrade removes identity bindings left behind.** Migration `0100` removes bindings
  whose connection no longer exists, each on the trail with who it spoke for, so anyone who
  still needs one can be bound again on purpose. To count them first:

  ```sql
  select count(*) from settings b
  where b.kind = 'channel_identity' and not exists (
    select 1 from settings c
    where c.scope_kind = 'installation' and c.kind = 'channel'
      and c.name = left(b.name, length(b.name) - position('/' in reverse(b.name))));
  ```
- **Some agents will read as needing review after this.** A simulation with an erased case
  now reads as incomplete, where it could read as ready. Nothing about those agents changed:
  the coverage was already gone, and running the battery does not bring it back. Recording
  the case again does.
- **A past battery can report fewer judged cases.** A case that ran and has since lost its
  input and expectations now reads as erased, neither held nor broken.
- **Service and agent principals can no longer approve or manage identity,** whatever role
  they hold. If something approves with a token today, it stops, and a person has to do it.
- **Binding an identity on a connection that does not exist is now refused** with a 400. It
  used to succeed, and the binding waited for the next connection to take that name.
- **Migration `0101` builds two indexes on `settings`,** blocking writes to that table while
  it runs.

### Added

- **Service accounts and API tokens over the API,** at `/admin/service-accounts`: create, list
  and disable an account, then issue, list and revoke its tokens. A token has to expire, and
  its secret is in the response that issues it and nowhere else. Revoking or disabling takes
  effect on the next request. The console screen is not in this release.
- **One MCP server, model provider, channel or policy can be read back by name,** which is what
  a client needs to compare what it wants with what is there. Credentials never come back.
  A fingerprint says whether one changed, and it moves when the master key does, so after a
  key rotation every fingerprint reads as changed once. A read is one instant, so a write
  landing during it cannot give an answer that mixes the two.

### Changed

- **Changing someone's grants is now on the trail,** for people and machines alike.

### Fixed

- **Erasure now takes what a correction expected.** A subject erasure and retention cleared its
  note and left its expectations, which are free text. After erasing a person's run, their
  address could stay in the row. This also holds during a rolling update, when a process on
  the previous version is still the one erasing.
- **A simulation with an erased case no longer reads as ready.** The evidence says how many
  cases were erased, and counts each one once.
- **A credential is now read together with its configuration,** everywhere the platform opens
  one. A write landing between the two reads could hand the new key to the old address.

### Security

- **Deleting a connection now takes its identity bindings.** A binding hands whoever holds that
  account somebody's authority, including deciding approvals. A new connection taking the name
  used to inherit them, with nobody having decided it.

## [0.94.0] — 2026-11-06

### Upgrade notes

- **This upgrade erases data.** Migration `0098` erases corpus copies that no correction
  names any more, left behind by deleting a correction before this release, along with
  the content of the same agent's simulated runs holding those bytes. Their steps and
  digests stay, and it records a `content.erased` admin event when it took anything.
  It has only been run on disposable databases, so look at what it would take first:

  ```sql
  with orphans as (
    select rc.ref, rc.digest, rc.run_id as agent_id, rc.erased_at from run_content rc
    where rc.owner_kind = 'regression'
      and not exists (select 1 from regression_cases c where c.input_ref = rc.ref)
  )
  select
    (select count(*) from orphans where erased_at is null) as copies,
    (select count(distinct held.run_id) from run_content held
     join orphans o on o.digest = held.digest
     join runs r on r.run_id = held.run_id and r.agent_id = o.agent_id
     where held.owner_kind = 'run' and held.erased_at is null and r.simulated) as replay_runs;
  ```
- **Migration `0097` builds an index on `run_content`** without blocking writes. It can
  take a while on a large installation.
- **Count four more PostgreSQL connections** per `serve` and `worker` replica when you
  size `max_connections`. They are outside each process's pool, and only open while
  content is being erased or a corpus is being copied.
- **Recording a correction from a run in another scope, or of another agent, now fails**
  as an absent run. Those used to succeed, which copied the run's input across the
  boundary.
- **A battery can now report fewer cases than the corpus holds.** A case whose input was
  erased no longer stops the battery. It is reported as a gap, neither held nor broken.
- **The `evidence` connector has still not run against a real source.**

### Fixed

- **Erasing a run on a person's request now reaches the regression corpus.** This closes
  the gap the 0.93.0 notes described. Within the same agent, it erases every correction's
  copy holding that run's input, those corrections' notes, and the content of the simulated
  runs holding it. It follows the digest the run recorded, so it still works after a
  correction was deleted. The event counts them as `corpusCases`. Input that is trivially
  common within an agent, such as `{}`, takes every copy, note and rehearsal holding it.
  A real run holding the same bytes is not reached unless it is named.
- **Retention counts from when bytes were collected.** When a correction's copy expires, its
  note and the content of the same agent's simulated runs holding those bytes go with it,
  however recent they are. `content.expired` counts them as `corpusReplays`.
- **Deleting a correction takes its copy and the runs that replayed it,** unless another
  correction still holds the same input.
- **Nothing can put back what was just erased.** Recording a correction and opening a battery
  now exclude erasing, retention and deleting a correction, per agent.
- **One erased case no longer stops every battery of an agent.** The rest of the corpus runs,
  the report and console show **Input erased**, and the conversation subscribed to drift
  notices hears which case stopped being checked. If every case is erased, the battery is
  refused and says why. Recording a case again after it aged out now works.
- **A past comparison no longer depends on erased cases.** A case erased on one side is neither
  fixed nor regressed.

### Security

- **A correction can only be made from a run of the same agent, in the scope it is published
  in, by someone who can read that run.** Before, an author could copy the input of a run from
  another scope or another agent into their own corpus. Every refusal reads as an absent run.

## [0.93.0] — 2026-11-06

### Upgrade notes

- **Nothing to do to upgrade.** No migration, no chart change and no new setting.
- **The `evidence` connector has still not run against a real source.** The editor
  was tested with a fake token and no `check` or `collect` reached a source. The
  examples are starting points, so confirm their field names against your own
  console, and run `check` first.
- **The console can now change a source declared through the API.** Whoever can
  configure connectors now sees an edit button on the card. Saving with nothing
  changed sends the stored declaration back as it was.
- **A known gap in erasure, not new in this release and not fixed by it.** Erasing a
  run on a person's request does not reach the copy kept when that run became a
  correction, or that correction's note. Retention does reach the copy, and after it
  does, the agent's battery refuses to run until that case is deleted. NT-015 records
  both, and a later release fixes them. Until then, when you handle an erasure request,
  also delete any correction made from those runs. Deleting removes the case and leaves
  the copy in the content store, where it stays until retention takes it.

### Added

- **An editor for evidence sources.** Under **Integrations → Connectors**, choose
  **New instance → New evidence source**, or **Configure** on the catalogue card. It
  covers the source, paging, window, queries and a token that is sealed and never comes
  back to the screen.
  - Start from one of the manual's two examples, or paste a declaration as JSON. A
    pasted declaration with an unknown key at any level is refused whole, so a misspelled
    filter cannot silently widen a query.
  - Editing opens only on the current declaration. If it cannot be read, the form does
    not open.
  - The manual, in pt-BR and en-US, now starts from the console.

### Design decisions

These are decisions recorded in a note. Nothing below is implemented in this release.

- **NT-015: versioned evaluation datasets and experiments.** It extends NT-006 and
  stays within its rule that this is not a general evaluation platform: a dataset
  belongs to one agent.
  - A dataset version is immutable and holds no free text. Tags are left out.
  - An experiment is never a battery, so it cannot satisfy the start gate, the drift
    sweep or FU-10, even with an empty configuration.
  - Retention wins over the corpus. An erased case stays visible as a gap.
  - A case added after the last battery reads as `not_checked` and does not refuse a
    start by itself.

## [0.92.0] — 2026-11-06

### Upgrade notes

- **Nothing to do to upgrade.** No migration, no chart change and no new
  setting. The `evidence` connector stays out of every agent's tools until an
  administrator creates an instance through the API.
- **The console shows an evidence source and does not edit it.** Declaring one
  is an API call, as with SQL.
- **Try a new source before relying on it.** This connector has not yet run
  against a real source. The manual's declarations are starting points, so
  confirm their field names against your own console, and run `check` first.
- **Read-only is the token's guarantee.** Give each instance a read-only token.

### Added

- **Governed evidence collection, the `evidence` connector.** It collects events
  from a source you declare and answers in aggregate. The model never sees an
  event and never writes a query. Each instance gives an agent three reads:
  - `check` validates the credential and runs the source's positive control.
  - `collect` runs a named query over a window and returns a manifest and a
    reference to the events, which stay in the content store under the run's
    retention. A collection too long for one call answers `partial` and resumes.
  - `analyze` counts, sums and groups what this run collected. It cannot read
    another run's evidence.
- **Every number states its coverage.** A result carries its window, scope, count
  and coverage: `complete`, `partial`, or `floor` when a limit was hit or paging
  stopped advancing, so the number is a minimum.
- **Absence is claimed only when it can be shown.** `analyze` answers `observed`,
  `not_observed` or `not_observable`. `not_observed` needs a complete collection,
  a passing positive control and no unmeasured event in the set, so a source with
  much unmeasured traffic will usually answer `not_observable`.
- **A source's failure is never zero events.** A refusal inside a 200 response, a
  denied credential or a body of the wrong shape is a failure. A value a model
  supplies that does not match its declared pattern is refused, never escaped.
- **A manual page and a reference agent.** The manual, in pt-BR and en-US, covers
  how to read coverage and outcomes, with two example declarations.
  `agents/evidence-investigator.agent.md` reports window, scope, counts and
  coverage before any interpretation.

## [0.91.0] — 2026-11-03

### Upgrade notes

- **Nothing changes in agentd itself.** This release changes the software
  factory's tool server image, `ghcr.io/fuseone-io/fuseone-agents-factory-mcp`,
  and the factory's texts. An installation that does not run the factory has
  nothing to do.
- **For installations running the factory, the Merger is ready to set up.**
  Start it only once all of these hold, in this order:
  - its own GitHub App exists;
  - `main` carries the checks ruleset beside the minimal one;
  - the `FACTORY-MERGE` policy is stored and enforced. Without it, the Gate
    blocks every merge, and no approval releases a block;
  - the factory server's environment lists `merger`, with
    `FACTORY_REVIEWERS`, `FACTORY_CHECKS` and `FACTORY_CHECKS_APP_ID`;
  - `factory-merger` is registered and classified.

  The factory manual has each step. Until then, a person merges as before.
- **The Merger's `move_card` no longer moves a card from Doing to Done.**
  Only `close_merged` does, and only for a pull request GitHub reports
  merged. No other role's transitions change.
- **`factory-measure report` needs the Merger's bot in `bots.yaml`.** The
  shipped map carries a placeholder for it, as for the Architect and the
  Developer, and the report refuses to run until all three are filled in.

### Added

- **The Merger, the software factory's sixth role.** It reads only
  `list_ready_prs`, which returns numbers, commit SHAs, item ids and reasons,
  never text anyone wrote, so nothing it reads taints the merge.
  - It proposes `merge_if_ready` on the listed head, and a person approves
    each call. The tool re-checks, on that head, the Reviewer's approval, QA's
    `factory/qa` and every expected check before it merges.
  - A pull request it does not merge goes back through `send_back`. That
    tool checks the cause itself and writes a comment of fixed text. It then
    moves the label to the role whose signal is missing, or, when only a
    person can clear the cause, sends the card to Travado.
- **`close_merged`.** It finishes a merge whose card never reached Done: a
  run that stopped between the merge and the move, or a person who merged by
  hand.
  - `list_ready_prs` lists those pull requests under `merged`.
  - `close_merged` moves the card to Done and only then drops
    `factory:ready-to-merge`, so any failure leaves the pull request listed
    for the next run.

### Changed

- **`factory-measure` gives ready-to-merge time to the Merger, not to a
  person.** This covers an open pull request carrying the label, and a
  merged one still labelled, until `close_merged` finishes it. A pull request
  closed without a merge stays a person's.
- **The factory's rules distinguish a direct merge from `merge_if_ready`.**
  No role merges directly or holds `merge_pull_request`. The Merger's one
  merge is `merge_if_ready`, which a person approves on every call. A person
  may still merge by hand.

## [0.90.0] — 2026-11-03

### Upgrade notes

- **An approval note may no longer start with `standing approval `.** The
  approvals route answers 400 to such a note, in any letter case. That prefix
  is now reserved for the engine. Whether an approval was standing is recorded
  in its own field, so a person's note can no longer count as one.
- **A second image ships with this release:**
  `ghcr.io/fuseone-io/fuseone-agents-factory-mcp`, the software factory's own
  tool server. Check that the new package's visibility matches the main
  image's before pulling it, or the cluster needs a pull secret. Nothing runs
  it unless you add it.
- **The chart gains `worker.extraContainers`, empty by default.** With it
  unset, the rendered worker is unchanged.
- **The software factory's setup changed**, for installations running it:
  - every role moves cards with the factory server's `move_card` instead of
    the GitHub server's `projects_write`, which no role holds any more;
  - QA records its verdict as a `factory/qa` commit status instead of a
    review. Its App needs Commit statuses: write, and Pull requests only read;
  - the factory server is a worker sidecar on loopback that acts under each
    role's own App token. Run the setup canary in the factory server's README
    with each role's token before starting the roles.

### Added

- **Per-role, per-week measures.** `GET /api/v1/work-items/measures` and a
  "Per role, per week" panel on the work-items page count, for each agent and
  ISO week (Monday 00:00 UTC):
  - runs and cards;
  - proposals: requested, approved, refused, pending and abandoned, plus
    revised or repeated after a refusal;
  - cost on cards, inherited and off any card, which add up to the week's
    total.

  The answer says that whether a person edited a proposal is not recorded,
  rather than reporting it as zero. Only runs the caller can read appear, and
  cost only with `cost:read`.
- **Board measures for the software factory.** A read-only CLI,
  `factory/measure`, reads the GitHub board's history. It reports time in
  each lane and the signals a later stage caught, per role and per week. It
  refuses to run until its canary has passed and every role's bot is named.
- **The software factory's own tool server.** `move_card` and
  `set_card_fields` change only the factory board's Status, Work type and
  Size. Each role gets exactly the tools and lane moves its role allows,
  judged from the calling App's identity, and nothing ever leaves Travado.

### Changed

- **Standing approvals carry their origin in the record**, instead of a note.

## [0.89.0] — 2026-11-02

### Upgrade notes

- **0.88.0 must be running on every worker first.** This version writes the
  `work_item_linked` step that 0.88.0 taught every reader. A worker older than
  0.88.0 fails a run that carries it.
- **The built-in Gate policy is now `builtin/v6`.** It adds one coordination
  exemption: `$fuseone.work_item.link` passes without approval only when the
  run's own `create_pull_request`, answered by the hosted GitHub MCP server
  under a GitHub App, opened the pull request it links. Any other link waits
  for a person like any other write. Leave `create_pull_request`'s result
  classified untrusted, as it is by default; the link does not need it ruled
  trusted.
- **Webhook deliveries attribute a card only from github.com.** A GitHub
  Enterprise delivery still opens its run, with no card on it.

### Added

- **A card's trail joins its issue and its pull requests.** A Developer links
  each pull request it opens to the issue it holds, and the trail of either
  shows every role's runs on the card. Asking by the pull request answers with
  the issue's trail, and says it resolved. A pull request links to one card
  only; linking it to another is refused.
- **Runs inherit a card for attribution.** A run opened by another run's event
  carries the card that run held, and a signed GitHub delivery carries the issue
  or pull request it is about. Inherited runs show on the card's trail with
  their approvals, but never hold the card and never add to its cost.

### Changed

- **The trail says "joined the trail" and "first attribution"** instead of
  "claimed", because an inherited run never claimed the card.

## [0.88.0] — 2026-11-02

### Upgrade notes

- **Every worker must run 0.88.0 before any later version that links a pull
  request to a card is deployed.** This version teaches every reader the
  `work_item_linked` step; nothing writes it yet. A worker older than 0.88.0
  that meets that step fails the run. Finish rolling out 0.88.0 everywhere
  first, then upgrade further.
- **A software factory Developer needs a ruleset on `main` before it starts.**
  The manual's software factory page gives it: a pull request required, force
  pushes and deletion blocked, and no bypass for anyone, people included. With
  it in place, a release's changelog cut also reaches `main` through a pull
  request.

### Added

- **A card's trail.** `GET /api/v1/work-items/trail?ref=` and a console page
  list every run that claimed a card, per role: runs, approvals asked,
  approved and refused, and cost. A run claim in the run trail links to it.
  Only runs the caller can read appear, and cost only with `cost:read`. Until
  pull requests are linked to their issue, an issue and its pull request show
  as separate trails.
- **Reference definitions for a Developer (Level 1) and an Architect**, beside
  Triage, Reviewer and QA in `factory/agents/`. The Developer makes small
  changes through the GitHub API and leaves a card that needs a build labelled
  `factory:needs-runtime`. The Architect writes a plan verified in the code
  and acceptance criteria on the issue. The setup creates every `factory:*`
  label the roles use.
- **The run trail shows when a run linked a pull request to a card.**

### Changed

- **A factory role that blocks on a person now records the block before its
  run ends.** Each role says in its last comment what unblocks the card and
  which lane it returns to, and the board operator moves it back once that is
  done. A closed card goes to Done instead.
- **Factory roles list only open items on the board.**
- **`make release` tags only a clean checkout of `origin/main`.** It refuses
  staged, unstaged or untracked changes, and any commit that is not the merged
  head of `main`.

### Fixed

- **A claim whose recorded agent or scope differed from its run's no longer
  counts as another agent's.** Such a claim is now refused, and a card held
  under an older one still blocks a second run of the same agent.
- **A claim can no longer be recorded for a run that was never started.**

## [0.87.0] — 2026-11-01

### Upgrade notes

- **Tools of a `--mcp` server published before this version leave the
  catalogue 24 hours after the upgrade**, unless a worker running this
  version still runs that server and renews it. During the rolling update,
  workers on the previous version keep serving their `--mcp` tools for that
  grace period. Servers configured in the console are not affected.
- **Migration 0094 builds the event feed's new index concurrently.** It does
  not block writes to `runs` while it scans the table, so an upgrade does not
  pause the workers still serving. The previous index stays, so rolling back
  to 0.86 remains safe; a later version removes it.
- **`pg_read_all_stats` is not needed, and we recommend not granting it.**
  Without it, the Runtime page and the metrics say a transaction is holding
  event dispatch back but not for how long. The grant is cluster-wide: it
  exposes every session's queries in every database on the server. The
  monitoring page of the manual describes a narrower alternative for an
  operator's own monitoring role.

### Added

- **Event dispatch says how far behind it is.** The worker's metrics and the
  Runtime page show how many finished runs are waiting, how long the oldest
  has waited, and whether an open transaction is holding dispatch back. The
  metrics are the `fuseone_event_dispatch_*` series. Dispatch shows as
  degraded, never failing, after five minutes of lag, or when more than a
  thousand runs are waiting. A new manual page, "Monitoring", covers it.
- **A publish that closes an event cycle warns.** The version is still
  published, and the Studio names every agent in the cycle and recalls that a
  chain stops after eight hops. An agent the author cannot read appears as
  "an agent outside your access". When the graph is too large to check in
  full, the warning says so.

### Changed

- **Publishes and changes of an agent's current version run one at a time**,
  so two publishes that only close a cycle together cannot both miss it.

### Fixed

- **A `--mcp` server's tools leave the catalogue once no worker runs it.**
  Each worker renews a short lease on the `--mcp` servers it really has
  connected. When the last one stops, or its session dies and cannot be
  started again, the tools leave within about two and a half minutes. Their
  classifications are kept for when the server comes back.
- **A `--mcp` server that fails at startup is retried on every pass**, instead
  of never again.
- **Simulated runs no longer slow event dispatch.** The dispatcher's index now
  holds only the runs it can offer, so a large number of simulations past its
  position no longer makes every read walk through them.

## [0.86.0] — 2026-11-01

### Upgrade notes

- **An event chain stops eight hops from its root.** During the rolling
  update to this version, a chain of agent events may continue under workers
  still running the previous version, uncounted, as it did before. Once the
  last of them is gone, the first event-opened run without a recorded chain
  that the new version sees publishes no further events. Its trail says the
  chain is unknown only if the new version closed that run. A chain in flight
  during the update may therefore run past the limit while the versions
  overlap, or stop early once, after; if it should continue, start its next
  step by hand.
- **Runs that finished in the hour before the upgrade are offered to event
  listeners once more.** Migration 0092 gives them a dispatch position,
  because the previous dispatcher read only the newest 30 and may have missed
  some. Any that were already dispatched reach the run they opened, so no
  listener starts twice. Older runs are not replayed.
- **A disabled tool server's tools leave the catalogue.** They come back,
  with their classifications, when the server is enabled again.

### Added

- **A webhook can come from GitHub.** A webhook trigger with `from: github`
  and the `events` it starts on verifies GitHub's signature over the raw body,
  uses GitHub's delivery id, and answers `ping`. Rotating a GitHub hook needs
  the master key, because GitHub signs with the secret and the door must read
  it back; without one the hook stays closed. Only the shape of an event name
  is checked, so a misspelt one publishes and never fires.
- **The run that reaches the end of an event chain says so.** Its trail
  records that it published no further events, and why: the hop limit, or a
  chain it could not establish.

### Changed

- **The tool catalogue follows what the servers offer.** Narrowing a
  server's toolsets, deleting it or switching it off removes its tools from
  the console and from the publish check. A tool that comes back with the same
  definition keeps its classification. A server that is down keeps its last
  discovered tools.
- **The Triage reference may read card titles before its claim, and nothing
  more.** The manual and the factory references now say that every GitHub
  write after the claim waits for a person, and that the claim itself does
  not.

### Fixed

- **Every finished run is dispatched to its event listeners.** The
  dispatcher read only the 30 newest finished runs on each sweep, so a burst
  larger than that lost the rest. It now reads forward from a durable cursor.

## [0.85.0] — 2026-10-31

### Upgrade notes

- **Headers saved beside a GitHub App are now an allowlist.** Only the
  hosted GitHub MCP server's configuration headers are accepted —
  `X-MCP-Toolsets`, `X-MCP-Tools`, `X-MCP-Readonly`, `X-MCP-Lockdown` and
  `X-MCP-Insiders`. Saving any other header next to an App is refused, and a
  registration stored with one keeps working but stops sending it.
- **A header name given twice is refused.** Two spellings of the same name
  (`X-MCP-Readonly` and `x-mcp-readonly`) are refused at save, in every mode.
  A registration already stored that way fails its calls without sending
  anything until somebody saves it again with the name given once.

### Added

- **Reference definitions for a Reviewer and a QA role**, beside the Triage
  one in `factory/agents/`. The definitions declare the toolsets and the
  GitHub App permissions each role needs. QA only proceeds on the Reviewer's
  most recent decisive review, and it must be on the pull request's current
  head. A test keeps all three references parseable and keeps each one at
  `standing_approvals: forbidden` with its claim line.
- **The manual documents the GitHub App credential and the software
  factory.** "MCP and credentials" covers the App mode and the header rules.
  A new "Software factory" page walks an operator from creating the Apps to
  the roles running at copilot.

### Security

- **Removing a GitHub App also removes the headers stored beside it.** They
  can no longer turn into the server's credential once the App is gone.

## [0.84.0] — 2026-10-31

### Upgrade notes

- **Migrations 0089 and 0090.** 0089 creates `run_work_items`, a new
  append-only table, empty on upgrade. 0090 adds `standing_approvals` to
  `agent_specs` with a default of empty, which is how every version
  published before it behaves: standing approvals keep releasing writes
  exactly as they did.

### Added

- **MCP servers can authenticate as a GitHub App.** A registration takes an
  App id, an installation id, the App's private key (pasted or uploaded as a
  `.pem`) and optionally a list of repositories. The worker mints
  installation tokens and keeps them only in memory. It refreshes them before
  they expire, and after a failed refresh it waits briefly before asking
  GitHub again. A server set up this way always acts as the App, never as
  the person who started the run, and no personal credential is looked up for
  it. The key is sealed. The console only ever shows that a key is
  configured, and the admin trail records that the credential changed,
  without the key. Configuration headers can sit beside the App: the hosted
  GitHub server needs `X-MCP-Toolsets` to offer more than its default sets.
- **An agent can claim the work item it is working on.** An agent that
  declares `$fuseone.work_item.claim` records which card a run took, as a
  step in the trail. One card per run; while a run is open, no other run of
  the same agent can take the same card. Nothing has to release a claim: a
  run stops holding its card when it finishes or fails, however that
  happens. The run page names the card.
- **An agent definition can forbid standing approvals.** A version that
  declares `standing_approvals: forbidden` never has a write released by a
  standing approval, and no new grant can be created for that agent. Every
  write needs a person, every time.
- **Publishing can refuse to replace an autonomous agent's version.** A
  publish may ask to be refused if the agent is at a given stage. The stage
  is judged under the same lock that writes the version, so a promotion
  cannot slip in between.
- **Reference material for running a software factory on the platform**, in
  `factory/` and design note NT-013: roles configured in the console, one
  GitHub App per role, every role at copilot and a person merging.

### Security

- **A GitHub App secret never travels without TLS or to another origin.**
  With the App mode on, the API and server URLs must be HTTPS (plain HTTP
  only to loopback). Minting does not follow redirects, and a redirect that
  changes scheme, host or port is refused before a token is attached. The
  admin refuses an `Authorization` header beside the App, and the transport
  strips any such header before setting the App's token.

## [0.83.0] — 2026-10-30

### Upgrade notes

- **Migrations 0087 and 0088.** 0087 adds the fallback columns to
  `agent_specs`. 0088 creates a partial index on `run_steps` for the
  fallback marker. It is built on the partitioned parent, inside the
  migration's transaction, not concurrently: it reads the ledger once and
  can hold appends while it builds. On a large ledger, plan the deploy for
  that window.

### Added

- **An agent can declare a fallback model for when its provider stays
  down.** A version may name `fallback: {provider, model}`. When the
  declared provider keeps failing in a way a retry is meant to outlast —
  overloaded, rate limited, unavailable — a draft or copilot run switches
  to the fallback after the normal attempts and stays on it for the rest of
  the run. Authentication and bad-request failures still stop the run. The
  trail records the switch with the failure that caused it, the run page
  shows it, and cost lands on the pair that answered. Without a declared
  fallback nothing changes.
- **An autonomous run never uses an unrehearsed fallback.** It was promoted
  on evidence gathered against its declared model; it parks instead, with
  the reason `fallback_unrehearsed` beside the provider's own failure —
  including a run promoted after it had already switched.
- **Runs that fell back do not count as promotion evidence.** Agreement,
  gate blocks and promotion suggestions leave them out, so a copilot is not
  promoted on decisions another model made. Activity and cost still count
  them.

## [0.82.0] — 2026-10-30

### Upgrade notes

- **Migration 0086 adds `branding_assets`.** Uploaded logos and icons live
  in the database, so a database backup now includes them.

### Added

- **Logo and icon can be uploaded to the installation.** An air-gapped
  installation no longer depends on an external URL. Images are stored by
  content hash (PNG, JPEG or WebP, detected from the bytes, up to 512 KiB;
  SVG is refused), served from the installation — the sign-in screen
  included — and a replaced image is removed once no brand references it.
  The trail records which image, never its bytes. External URLs still work,
  with the same warning.
- **The brand colour can be picked visually**, beside the hex field it
  stays in step with.
- **A run can be opened again from another run's recorded input.** "Run
  from this input" opens a new run, with its own id and chain, seeded with
  the source run's trigger input; the source is never written. Labels such
  as untrusted survive, nothing else carries over — no approvals,
  reservations or reply thread — and the trail names the source run. It is
  not Replay, which still only re-checks recorded decisions. Context
  artifacts and a version picker in the console follow in #188.

### Changed

- **The console loads each screen on demand and is served precompressed.**
  The sign-in screen downloaded 2.1 MB of uncompressed JavaScript; it now
  gets about 238 KB, brotli or gzip by what the browser accepts.

### Fixed

- **Postgres test packages no longer interfere when run in parallel.** Each
  gets a database of its own, so the full suite runs in parallel and green.

## [0.81.0] — 2026-10-30

### Changed

- **An agent's per-run cost ceiling is typed as money.** The field asked
  for micros — 500000000 for 500 — so a zero too many or too few changed
  the ceiling tenfold. It now reads "Cost per run" in the installation's
  currency and takes `500` or `0,25`. Definitions still carry integer
  micros.

### Fixed

- **The authoring assistant's daily ceiling accepts a fraction.** Typing
  `1,` turned into `1` and the separator vanished.
- **Money fields show only what they save.** Text that is not an amount is
  refused rather than displayed beside a different saved value; an amount
  finer than a micro or too large to hold exactly is refused rather than
  rounded, and a price rate with seven decimals is no longer saved as zero.

## [0.80.0] — 2026-10-30

### Upgrade notes

- **Publishing refuses tools the agent cannot use.** A definition is now
  refused when a step reaches a tool outside the agent's `tools`, when a
  step anchors a platform tool, or when `tools` names something this
  installation does not offer — unknown to the catalogue, or kept off the
  surface. These used to publish and stop at the first call. Agents
  already published keep running; an affected one meets the refusal at
  its next publish, which names the step and the tool. A tool on the
  surface the Curator has not classified yet still publishes.

### Fixed

- **A step could widen what an agent may call.** A run's envelope is built
  from its steps, and a step reaching a tool outside `tools` extended it
  past the list an author and a reviewer read. Refused at publish, above.
- **The run page shows where the tokens went.** It put every token under
  input and reported output and cache as zero, while the money, the run
  list and the planning spend were right. It now carries the breakdown its
  steps were billed for.
- **A design note in `docs/` can no longer be missing from the
  documentation site silently.** NT-007 on standing approvals was; the
  build now names any note it was not told about.

### Changed

- **The policy editor's choices are radio groups.** Effect, enforcement and
  reach can be chosen with the keyboard and are announced to screen
  readers; the draft is validated by one schema rather than by the save
  button.
- **NT-002, the remaining-work ordering, is no longer on the documentation
  site.** It stays in the repository, and links to it point there.

## [0.79.0] — 2026-10-30

### Fixed

- **An agent whose own ceiling did not bind re-wrote its whole prompt to
  cache on every call.** The cached prefix stated the run's budget, which
  is the agent's ceiling narrowed by its scope's headroom — and the
  headroom falls with every call any agent in the scope makes. When the
  agent's ceiling was at or above the scope's, no two calls shared a
  prefix: every call paid the full prompt at the cache-write rate, about
  ten times what a comparable agent paid. The prefix now shows the agent's
  own ceiling, which only a new version changes; enforcement still uses
  the narrowed budget.
- **A step change no longer re-writes the agent's instructions to cache.**
  Tools, instructions and the platform contract are now cached on their
  own, ahead of the note that names the step, so moving to a new step pays
  only for that note and the history.

### Changed

- **The ceiling line the model reads says whose ceiling it is.** It reads
  "Agent-configured budget ceiling: X. Scope limits may enforce a lower
  amount." instead of "Budget ceiling for this run": a scope can enforce
  less, and an OpenAI-compatible provider has no other line to say so.

### Added

- **A planned step fingerprints the tools and stable system blocks it
  sent.** `prompt.digests` on each planned step holds a short hash of the
  tools and of every system block, exactly as they went on the wire.
  Comparing two consecutive planned steps names the block that changed —
  the question an agent that never reads its cache raises, and that equal
  sizes could not answer. Diagnostic only; message history and request
  parameters such as `tool_choice` or thinking are not covered.

## [0.78.0] — 2026-10-30

### Added

- **A scheduled run knows the moment it was fired for.** A cron run used
  to open with no input, so an agent asked to sweep "the window this
  firing is about" derived it from the wall clock — and a run that
  started late or was resumed computed a different window than the
  moment that named it. The run's input is now
  `{"trigger":"cron","moment":"…","schedule":"…"}`, the same instant its
  idempotency key comes from. A prompt can say "end the window at the
  `moment` in the input" instead of guessing.
- **The manual shows the screens it describes.** Twelve screenshots,
  cited under the sections they illustrate in both languages, render on
  GitHub, in the docs site and inside the console. The console serves
  them from its own embedded copy and renders no image from anywhere
  else, so an air-gapped installation never reaches out to draw a page.

### Fixed

- **A refusal's cost is counted.** A provider bills a refusal — it read
  the whole prompt before declining — but the run dropped the figure, so
  it never reached the trail, the budget or FinOps. It is now recorded
  like any other planning call, on Anthropic and on OpenAI-compatible
  providers and gateways alike. Where the run goes after a refusal is
  unchanged.
- **A duplicate-effect check no longer fails under contention.** When two
  workers raced to reserve the same effect, the one that waited could get
  an error instead of the other's reservation, failing its turn and
  retrying for nothing. It now receives the existing reservation, which
  is the answer the check exists to give.
- **The in-memory demo starts and can be browsed.** `serve --demo`
  without a database panicked before serving. It now seeds and serves,
  and — having nobody to sign in as — acts every request as a named
  installation administrator, so runs list and their answers open. An
  installation with a database is unaffected and always authenticates.

## [0.77.1] — 2026-10-29

### Fixed

- **The serving form's selects stay inside their columns.** In the agent
  Identity form, choosing a long option — the one-hour prefix cache, a
  long model id — grew the select over its neighbour instead of
  truncating. Every serving select now fills its own column and clips its
  label; the full text remains readable in the open list. A custom model
  id absent from the provider's suggestions now appears in that list too,
  wrapped rather than cut off, so a value the button truncates can always
  be read in full by opening the field.

## [0.77.0] — 2026-10-28

### Added

- **A finished answer can declare a headline, and the channel threads the
  rest.** An agent that reports hourly wallpapered its channel with the
  full report. The finish tool now takes an optional one-line `headline`;
  when a run declares one, the announcement posts that line at the top of
  the channel and the full answer as a reply in its thread. Without it,
  nothing changes. Delivery stays with the channel layer — same bot, same
  rooms, same tracking, ceiling and erasure; the agent only formats. The
  headline is bounded at both ends and stored beside the answer in the
  content store, where retention and erasure reach it.

## [0.76.0] — 2026-10-28

### Fixed

- **A long run's final answer no longer goes blind to results it already
  read.** The transcript's working memory for tool results evicted the
  oldest first when it filled, which in a checklist run meant the small
  verdicts paid while the large, already-analysed evidence survived — the
  closing report then declared kilobyte results unreadable. Eviction now
  takes the largest results first (ties to the oldest, the newest kept
  whole), a result that alone exceeds the whole budget becomes a receipt
  immediately instead of making every earlier result pay first, and a
  small result is only ever evicted by the hard bound, never to buy
  headroom.

### Changed

- **The transcript result budget doubles to 128 KiB.** The old 64 KiB was
  sized when every planning turn repaid those bytes at full input price;
  under provider prefix caching the resend costs a tenth, and the old
  ceiling could not hold even one disciplined multi-check run. Agents
  whose runs carry many results may see a modest input-token increase,
  almost all of it at the cached rate.

## [0.75.0] — 2026-10-28

### Fixed

- **A run with steps no longer finishes at the first step boundary.** The
  prompt told the model which step it was at and that finishing was "the
  only way to finish", and nothing said finish ends the whole run — so a
  careful model, whichever provider served it, completed its first step's
  work and finished there, with a summary promising later steps that
  nothing would ever execute. The step note now states that finish ends
  the run and that a completed step with steps remaining means moving
  forward. Agents without steps keep their prompt byte for byte.
- **Regenerating steps from the interview no longer produces envelope
  traps.** The generator anchored platform tools in steps (the trap fixed
  in 0.74.0) and anchored fallback tools only in the step that mentioned
  them — unreachable the moment the forward-only run advances. Platform
  tools are now stripped from generated steps whatever the model answers,
  and the generator's instructions, in both languages, teach the
  forward-only rules: platform tools are ambient, a tool a later step may
  need goes in every step that may need it, and few broad steps beat many
  narrow ones.

### Added

- **A definition can pin the language its agent answers in.** An agent
  authored in one language could report in another: the platform's own
  prompt scaffolding is English and sits closer to the generation than
  anything the author writes. The new `reply_locale` field injects the
  author's choice as the last stable text before the generation, where
  position makes it hold. Absent, nothing changes. The Studio exposes it
  beside the model and effort.

## [0.74.0] — 2026-10-27

### Fixed

- **Consulting memory no longer jumps a run past its steps.** With steps
  declared, a run advances by using a step's tools, and the envelope only
  moves forward. A platform tool — memory, context, the ticket answer —
  anchored in a late step turned the model's opening memory lookup into
  that advancement: every earlier step closed silently, with no park and
  no note, and the run finished reporting its tools "not available at this
  step". Platform tools no longer advance the step, and no longer advance
  the per-step model and effort either; where an author anchored one, it
  still becomes reachable when its step opens.

### Added

- **The trail narrates forward movement.** When advancing past a step
  closes tools, the planning step records them and the trail reads "the
  run has moved to step X; earlier steps' tools are behind it" — so a
  deliberately narrowed envelope no longer looks like a tool outage. A
  genuine outage still outranks this line.

## [0.73.0] — 2026-10-27

### Fixed

- **The trail now says which model actually answered.** A gateway that
  reroutes a call — a fallback policy serving a different model than the
  one requested — was invisible: the planning step recorded only the
  requested model, and every cost figure was priced at its rate. The step
  now records the served model whenever the provider reports a different
  one, the trail reads "Requested X, but the provider served Y", FinOps
  attributes the spend to the model that produced it, and the call is
  billed at the served model's configured rate — falling back to the
  requested model's rate rather than zero when the served name has none.
  Failed-but-billed calls carry the same record.

## [0.72.0] — 2026-10-27

### Fixed

- **A run no longer plans silently without the tools its worker cannot
  offer.** A worker whose MCP session died used to plan runs as if the
  agent had never been given those tools: the model could not propose
  them, the Gate saw nothing, and the run finished "clean" having been
  unable to do its work — intermittently, depending on which worker
  claimed the run. Now a turn whose envelope needs a server that is
  configured, enabled and not connected on the claiming worker refuses to
  run before any model call is paid for, backs off, and parks as "tool
  server unavailable" — naming the server — only if the outage outlives
  the retries; a reconnection resumes it on the next attempt. A tool
  absent for any deliberate reason — server removed or disabled, tool
  taken off the surface — keeps the run going, with the model told
  plainly which pack tools are not available and the trail recording the
  same list on the planning step. Native connector tools are never read
  as MCP tools, even under an exactly colliding name.
- **The exhausted-budget banner no longer outlives the budget it measured.**
  A ceiling raised after a crossing kept its old "spent the budget" banner
  on the cost screen indefinitely, reporting an amount nobody configured
  any more — and suppressed the new ceiling's own warnings for the rest of
  the window. An alert is now served only while the window, the amount and
  the budget it froze are still the configured ones, a changed ceiling
  starts the 50/80/100% warnings again, and the banner says which window
  it measures: today's budget or this month's.

## [0.71.0] — 2026-10-27

### Added

- **A provider can declare the models that must run under tool_choice
  "auto".** The platform learns this downgrade from a refusal — but a
  gateway with fallback routing swallows the refusal and silently serves
  every call with another model, at that model's price. The provider screen
  now takes the list directly (one per line, Anthropic-protocol providers
  only); declared models run under "auto" from the first call, with the
  same free-text guard, surviving restarts, and an edit reaches already-
  running workers within the configuration refresh.

## [0.70.0] — 2026-10-27

### Upgrade notes

- Compensation now writes a new step kind (`compensation_started`) into the
  trail, and a binary from an earlier version cannot read a run that carries
  it. Complete the rollout before abandoning runs with compensation; a run
  compensated during the brief window where old and new pods overlap may
  park and need a resume once the rollout finishes.

### Changed

- **An undo whose outcome is unknown is never sent again.** Compensation
  records its intent in the trail before the external call fires. After a
  crash, an attempt with no recorded result means the remote outcome is
  unknown: the run says so explicitly ("manual reconciliation required")
  instead of silently refunding or cancelling a second time. A failure
  proven to have happened before anything left the platform — a Gate
  refusal, a missing reservation — stays safely retryable, picking up the
  catalogue's current compensator.

### Fixed

- **An erasure either happens whole or not at all.** A subject erasure and
  the retention sweep now commit the destroyed bytes, the operational
  records, the memory marks and the administrative record together. A
  failure anywhere leaves everything in place, instead of bytes destroyed
  with no record that it ever happened.

## [0.69.0] — 2026-10-26

### Fixed

- **A worker that loses its run can no longer act on it.** A worker
  suspended past its lease — a long GC pause, a frozen node — used to wake
  up and keep going: releasing the run from under its successor, parking it
  over a human's abandonment, sealing stale steps onto a history it never
  saw, and firing tool calls beside the successor's. Every claim now
  advances a fencing token that every write and every outgoing effect must
  present, leases are renewed while a turn runs and judged dead by the
  database's own clock the moment they cannot be proven held, and a fenced
  worker stands down quietly: no park, no failure counted, the run left to
  whoever holds it now. Billed model calls caught on the wrong side of the
  fence still reach the ledger, exactly once.
- **Parking no longer overwrites what a person decided.** An abandonment
  landing before or during the supervisor's parking is honoured: the
  compensation stays claimable instead of being buried under a parked run,
  and a run that already ended is not relabelled as parked.
- **An oversized request is told so.** A body over the 2 MB ceiling now
  answers 413 instead of a generic 400, and a request stops waiting for
  ever on a client that never finishes sending (60s read timeout; event
  streams are unaffected). The ceiling comfortably holds the largest
  legitimate simulation and regression uploads.

### Security

- **Identity-provider traffic goes through the egress guard.** OIDC
  discovery and token exchange now run behind the same guard as every
  other outbound call, refusing metadata and link-local destinations and
  the environment's proxy.
- **A changed identity provider needs its own client secret.** Pointing an
  existing connection at a different issuer or client id while silently
  keeping the stored secret is refused, so a secret can no longer be
  replayed against a provider it was never issued for.
- **Erasure reaches exactly the run it names.** Stored content is erased
  by owner kind and id, so a run's erasure can no longer touch records
  that merely share its identifier.

## [0.68.0] — 2026-10-26

### Fixed

- **Free text no longer ends a run on the newer models.** A model that
  refuses the required-tool mode runs under auto, where a polite status
  line mid-run used to read back as the run's final answer — a scan
  once ended at its memory step reporting its own work as pending. Such
  text is now handed back once with the instruction to continue; only
  the finish tool ends the run. A model that answers text twice has
  said its piece, and the second text stands.
- **A turn that spends before failing leaves the spend on the record.**
  When the follow-up call fails, the billed first call now reaches the
  ledger, the budget, FinOps and the pool metrics — cost, pair, price
  and prompt breakdown — exactly once per attempt, before the run is
  released for retry.

## [0.67.0] — 2026-10-26

### Added

- **Each tool server carries its own call ceiling.** Sixty seconds was
  the one timeout every tool call lived under, and a log store
  aggregating an hour of the busiest stream has different physics than
  a CRM lookup. The server's configuration gains a per-call ceiling (5
  to 600 seconds) that binds the call itself on every transport;
  connect and discovery are never strangled by a short one, zero resets
  to the default, and a server that chose nothing keeps today's
  behaviour.

## [0.66.0] — 2026-10-26

### Added

- **A model that refuses the required-tool mode is learned, not worked
  around.** The planner requires one tool use per turn; a newer model
  answering 400 to that requirement used to be silently swapped by the
  proxy in front — the definition said one model, the behaviour and the
  invoice said another. The refusal now downgrades that one model to
  auto, remembered so the next turn pays no second 400, scoped to the
  model that said it. Agents can now run the newer models exactly as
  their definitions name them.

## [0.65.1] — 2026-10-26

### Security

- **Every open dependency advisory is closed.** The one that ran in
  production — the OpenAPI library behind the HTTP layer, carrying four
  highs — moves to its patched line. The console bundle's schema
  validator moves past a memory-exhaustion advisory, the docs site's
  lock catches up with fixes its ranges already allowed (two criticals
  among them), and the build toolchain's pair of criticals goes with
  the test runner's patched major. Audit reports zero across all three
  manifests.

### Fixed

- **A thorough "Never" block no longer fights the step editor.** The
  drift warning judged a word by its length, so careful prose made
  ordinary stage descriptions read as conflicts and operators reworded
  cards to appease a word list. Strength now comes from the
  prohibition's shape — identifiers, hammered words, short items — and
  every true conflict the suite pins still fires.

## [0.65.0] — 2026-10-26

### Added

- **The cache window is editable where agents are edited.** The identity
  section gains a "Prefix cache" choice beside effort — provider default,
  or one hour for agents that run at least hourly — and the value rides
  the whole console path: saved, shown back, and named in the version
  diff when it changes. Until now the window could only be set by
  editing the definition text directly.

## [0.64.0] — 2026-10-26

### Added

- **A proxy alias inherits its canonical market reference.** Behind a
  gateway the models that bill are the proxy's own names, which the
  bundled price list cannot know; now every model a provider declares
  inherits the reference of the canonical id it carries, shown under the
  alias's own spelling so saving it configures the pair the runs use.
  References remain suggestions — only a saved rate feeds cost.

## [0.63.0] — 2026-10-26

### Added

- **The hour cache window is priced at its own rate.** The prices screen
  gains "cache write 1h", the provider's own usage breakdown decides
  which writes pay it — a partially stripped breakdown bills upward,
  never silently down — and the hour subset is recorded and sealed on
  the step like every other financial fact. Market defaults follow the
  published multipliers; left unconfigured, hour writes keep billing at
  the short rate, visible on the screen. Configure the fifth rate on any
  installation whose agents use cache_prefix_ttl.

## [0.62.0] — 2026-10-26

### Added

- **An agent's requests share a named cache on OpenAI-style providers.**
  Each agent's calls carry its own cache key where the provider accepts
  one — steadier cache hits under load, and the provider's cache
  accounting can tell agents apart. Sent only to providers that declare
  support (the OpenAI preset does); endpoints fronting other vendors'
  models never see the unknown field.

## [0.61.0] — 2026-10-26

### Added

- **An agent can keep its stable prefix cached for the hour.** The
  definition chooses it — `cache_prefix_ttl: 1h` — and an agent triggered
  on the hour then reads its instructions and tool schemas back from the
  provider's cache instead of re-writing them every run, which in a real
  installation was half the model bill. The conversation itself keeps the
  short window, and agents that run less often should leave the field
  unset: they would pay the doubled write rate and never collect the
  read. The manual's cost page explains when each window pays.

## [0.60.2] — 2026-10-16

### Fixed

- **A transient failure dressed as a 400 retries instead of stopping the
  run.** A proxy in front of the model (LiteLLM, most commonly) sometimes
  wraps a timeout or an upstream outage in a 400, which read as "bad
  request" and parked the run on the spot. The error body is now read for
  explicit evidence of transience — the proxy's own exception class, or the
  upstream status it carries — and with evidence the run retries with the
  usual backoff. A genuine bad request still stops immediately, and the
  original status stays on the record either way.

## [0.60.1] — 2026-10-13

### Upgrade notes

- **The first boot of this version stamps your migration history.** Every
  applied migration's SHA-256 digest is recorded, and from then on the
  process refuses to start when an applied migration's bytes no longer match
  what the release ships — the error names the version and both digests,
  never the SQL. An installation whose `migrations/` files were ever edited
  in place will stop here: restore the canonical files, or correct the
  schema with a new migration, never by editing an applied one.
- **Rolling back to an older release still boots.** A migration recorded by
  a newer release than the running binary is logged loudly and left
  untouched. The refusal is reserved for genuine anomalies: an unknown
  version inside the known history, a name that is not a migration, or a
  newer row with work still pending beneath it — each of those means the
  history diverged, and the process stops instead of papering over it.

### Fixed

- **Migration history is verified, not assumed.** (#79) The run ledger's
  guarantees start at the schema that holds it; now the schema's own history
  is held to the same standard — append-only, tamper-evident, corrected
  forward.

## [0.60.0] — 2026-10-09

### Added

- **A governed Elasticsearch window.** Historical traffic questions — the
  busiest addresses on a path over days, one path's recent requests — as
  named queries over one fixed index pattern. The model supplies values,
  never structure: an unknown argument is refused, the index and the field
  mapping are instance configuration, every window is capped, and results
  carry only the projected fields, labeled untrusted. Configured in the
  console like every connector, with the password sealed.

## [0.59.1] — 2026-10-08

### Fixed

- **The standing-approval form's scope suggests, and its ceiling row
  aligns.** Company and area are picked from the declared scopes like every
  list here, and the daily-ceiling field gained the hint that keeps its row
  level with the expiry's.

## [0.59.0] — 2026-10-08

### Added

- **The standing-approval form offers what the platform governs.** Tool and
  agent are picked from the governed listings — a mandate demands an exact
  id, which is exactly where typing from memory fails — and typing stays
  valid for a tool an operator is about to configure.

### Fixed

- **The `@` tool popover opens at the caret.** In a long instruction block
  it used to land past the end of the form, behind a scrollbar: the hidden
  mirror that positions it wrapped with a different font size than the
  textarea (the ui component's own won), and the textarea scrolled inside
  itself while the mirror never does. The mirror now copies the textarea's
  computed typography — class strings drift, computed styles cannot — and
  the textarea sizes itself to its content. Measured in the browser: the
  anchor lands within one pixel of the caret line, end or middle of text.


## [0.58.1] — 2026-10-08

### Security

- **Unblocking always carries a decision's identity.** An allow policy that
  lowers the Gate's ladder could have let a misled run strip the block list
  unattended; `unblock_ip` now refuses in code any call no click and no
  standing mandate released, so each removal is named, counted and capped.
  Results that echo stored list comments are labeled untrusted, closing a
  cross-run taint-laundering path the adversarial review of 0.58.0 found;
  and the catalog guarantee now says what the daily ceiling truly bounds.

## [0.58.0] — 2026-10-08

### Added

- **`cloudflare.unblock_ip`.** The other direction, under the strictest
  guard of all: it removes only entries the connector itself wrote — a
  person's entry, or a person's wider range covering the address, is a
  person's decision and is refused. An absent address succeeds saying so.

## [0.57.0] — 2026-10-08

### Added

- **The policy page offers the governed tools.** The Tool field suggests
  the ids the platform already governs — effect beside each — and typing
  stays just as valid: a glob is a legitimate rule, and no listing offers
  every id a policy may name.

### Fixed

- **A Cloudflare upstream refusal says which class it was.** A wrong list id
  used to read as "the remote is down"; now the run says authorization,
  not-found or failure, and the console hints that account and list are ids,
  not names.

## [0.56.0] — 2026-10-08

### Added

- **Standing approvals.** A human approval given ahead of time: one person
  pre-approves one exact tool for one agent in one scope, with a daily
  ceiling and an expiry of at most 90 days. A run that parks on approval
  under a covering mandate is decided immediately with the grant owner's
  name on the decision — the trail, the transcript and the card all read an
  ordinarily approved run. Coverage and spending a use are one atomic step;
  expiry, revocation, a spent ceiling or a store error all leave the park
  waiting for a click. Granting needs its own permission (approval:grant,
  Admin), and creation, revocation and every use land in the trail.
  NT-007 records why a policy-side taint waiver was rejected instead.

### Fixed

- **A run's approval card follows the thread that opened it.** A run opened
  from a message used to have its approval request posted at the channel's
  top level while the conversation happened in the thread — a question
  posted where nobody was looking. The card, and every announcement about
  that run, now lands in the origin thread of its own conversation; other
  configured rooms, ticket rooms and unasked runs are untouched.


## [0.55.0] — 2026-10-08

### Added

- **A Cloudflare block list connector.** An agent that catches an attacker at
  3 a.m. can now act: `cloudflare.block_ip` adds one literal address to one
  configured Cloudflare IP list that a firewall rule blocks at the edge — and
  nothing else. The guards live in code, not in prompts: private and reserved
  ranges are refused unconditionally, the instance's protected ranges (the
  installation's egress, shared anonymizer ranges) refuse the same way, a
  daily ceiling counted from the list itself bounds a misled night, and an
  address already listed succeeds without a duplicate. Every entry carries a
  timestamped `fuseone:auto:` comment for the operator's external expiry job,
  so blocks are temporary by design. Account, list, protected ranges and the
  sealed token are configured in the console like every connector.

## [0.54.0] — 2026-10-06

### Added

- **A governed ticket can be worked in a room of its own.** In a help channel
  where a form bot writes the request, a marked thread becomes a ticket: the
  root stays the request, the person mentioned on its first line is the
  requester — the line the form owns, so free text below it never names who
  asked — and the team closes with the reaction it already uses, which only
  ever cancels. The ticket's work moves to its configured room; the support
  thread hears one thing, once: the answer somebody approved.
  `$fuseone.ticket.answer` seals the proposed text as the approval's evidence
  and execution refuses any other text.

### Fixed

- **An approval's evidence is decoded by the kind it declares.** Every
  evidence used to be read as a Gravitee subscription, which refused a written
  answer as unreadable; evidence sealed before this release still decodes as
  before.

## [0.53.1] — 2026-10-05

### Fixed

- **A Slack Socket Mode connection that stops speaking now fails and
  reconnects.** A connection that went away without saying so left the worker
  blocked on a read that would never return: nothing failed, so nothing
  reconnected, and Socket Mode does not redeliver — events were lost for as
  long as the process stayed up while the worker looked healthy. The
  connection now carries a deadline kept alive by Slack's own pings, and the
  backoff restarts after a connection that lasted.

- **Approval evidence reaches the tool through every layer.** The memory and
  context layers wrap every tool a worker offers and did not answer for
  evidence, so the engine's question never arrived: the request went out with
  none and the decision was refused afterwards.

- **A tool schema written as a whole object reaches the provider intact,
  required fields included.** It used to arrive as a property called "type"
  and one called "properties", refused without naming the tool; and the
  fields its author marked required stay required on the wire.

- **A grant above every company reads every company's runs.** Written as a
  filter it was `company_id = '*'`, a company no run belongs to: an
  administrator saw an empty page from a healthy store.

- **A run parked by a provider failure carries the failure as its reason.**
  The card used to say no reason was recorded about a failure the projection
  already had under its code.

### Changed

- `FUSEONE_LOG_LEVEL` turns the log up without a deploy, and the socket door
  says at debug why an event started nothing.

## [0.53.0] — 2026-10-04

### Added

- **A conversation can render an agent's answer as blocks.** A dense report
  used to arrive as one wall of text; with the new option beside Finished, the
  text's own paragraphs, lists and code fences become separate Slack blocks
  with breathing room. Presentation only, and provably so: the formatted
  render contains exactly the same characters as the dense one, partitioned at
  the structure the text already has — no guessed headlines, nothing reordered
  or demoted. Off by default; a conversation that did not opt in is
  byte-identical to before.

## [0.52.0] — 2026-10-03

### Added

- **The policy page offers what the platform already knows.** A condition on
  the agent suggests the agents, an area condition suggests the declared
  scopes, a tool effect is picked from the Gate's own four — and everything
  stays typeable, because a policy may name an agent that is not deployed yet.
  The owner starts as whoever is writing the rule and offers the directory to
  whoever may read it. And the reach the contract always had finally has a
  screen: a policy can say "only these agents" or "only these areas"
  structurally, with the empty set refused before the save button works.
  `scope.company` joins the condition fields the console can write.

## [0.51.0] — 2026-09-30

### Added

- **A conversation can announce only the finishes that carry an answer.** With
  Finished ticked, a run whose agent wrote no closing text posted the card —
  once an hour, forever, for a scheduled agent. The new option beside the
  Finished event keeps those out: a finish with no answer is heard by nobody in
  that conversation, the run is still retired from the sweep, and the console
  still records everything. The agent's instruction becomes the editor: finish
  with text when there is something to say, with none when there is not. An
  answer erased by retention is still announced as erased — "the platform
  removed this" is not "nothing to say".

## [0.50.1] — 2026-09-30

### Fixed

- **One slow tool call no longer takes a whole tool server down.** When a call
  outlived its deadline, the platform told the server it was cancelled; a
  server that refused that notice — Grafana's MCP server does, after a long
  Loki query — made the connection close for good. Every later call to that
  server then failed with `connection closed … "notifications/cancelled": Bad
  Request` until the worker was restarted, while the console still showed the
  server as reachable. A refused cancellation is now ignored, since the call
  was already abandoned. And a connection that dies for any other reason is
  reconnected on the next pass (within 30 seconds) instead of staying dead. The
  call that timed out still fails; queries that routinely take more than 60
  seconds need a narrower range in the agent's instruction.

## [0.50.0] — 2026-09-30

### Added

- **A finished announcement carries the run's own answer.** A conversation that
  ticked Finished used to receive a card saying a run ended, with a link; the
  sentence the agent wrote stayed in the console. The announcement now says the
  answer itself, with one provenance line — agent, area and a link to the
  invocation — when nobody asked for the run: a run opened by a mention keeps
  answering only in the thread that asked, so nothing is ever said twice. Long
  answers are truncated with a notice, an answer erased by retention says so,
  and an operational note: an hourly agent with Finished ticked writes a
  message every hour — the agent's instruction is what decides whether that is
  a report or noise.

## [0.49.1] — 2026-09-13

### Added

- **Gravitee connector instances can now be configured and corrected in the
  console.** The form fixes the gateway, organization, environment, single API,
  expiration bounds and Vault-backed administrative credential location. It
  never accepts a Gravitee token, and editing waits for the configurer-only
  detail so changing one field cannot erase or restore a stale credential
  boundary.

## [0.49.0] — 2026-09-12

### Upgrade notes

- **Enabled Gravitee instances now expose runtime subscription tools.** Before
  upgrading, review each enabled instance's organization, environment, single
  allowed API, expiration bounds, Vault binding and credential permissions.
  The model can supply only a subscription id and requested expiration; the
  configured instance supplies the remote path. A write still stops at the
  Gate and needs an informed human approval.

- **Slack conversations can now admit matching root messages as governed
  tickets.** A ticket conversation reads public-channel messages, so the Slack
  app needs `message.channels` and must be invited to the room. Approval
  buttons need the Interactivity callback, and direct messages to an approver
  need `im:write`.
  Nothing starts until an operator explicitly changes a conversation to
  **Governed tickets** and supplies its admission patterns.

### Added

- **A Slack support thread can govern one Gravitee API subscription.** A linked
  requester opens a matching root message, corrections create durable ticket
  revisions, and a configured bot or app may name who should receive the
  approval. The approver sees the fixed Gravitee subscription snapshot; an
  acceptance proceeds only if that snapshot still matches and one current
  approval wins.

- **Ambiguous Gravitee writes recover without submitting the acceptance
  twice.** The worker records an external attempt before the POST and
  reconciles an orphan from the remote subscription state. Confirmed late
  knowledge is sealed as immutable audit evidence without moving the run's
  current state. If the bounded check window expires, the ticket asks for
  manual verification, keeps its execution claim and continues GET-only
  observation; it never submits a second acceptance. One ticket revision can
  own only one active execution.

### Fixed

- **A saved ticket correction no longer retries forever without telling its
  requester.** When another revision is still executing, FuseOne keeps the new
  revision pending, says once that it will start later, and opens it after the
  current execution releases the ticket. The notice never settles or discards
  the saved Slack event.

- **Manual Gravitee observation can be stopped deliberately.** A subscription
  that remains pending is still observed by GET only, but an operator with
  `run:cancel` can use **Abandon run** after checking Gravitee. FuseOne records
  that decision, stops remote polling and releases the ticket without claiming
  that a possible remote acceptance was undone.

### Security

- **A generated API key never enters FuseOne or Slack.** The connector never
  requests or supplies `customApiKey`, decodes only a fixed safe projection,
  and posts a deterministic outcome containing the subscription, expiration
  and retrieval guidance. The requester retrieves the key from Gravitee
  through the organization's existing protected path.

## [0.48.0] — 2026-09-11

### Added

- **A provider says which models it serves.** A list shipped in the binary
  describes the vendors the platform knows, and behind a proxy those are the
  wrong names: what an installation's LiteLLM or vLLM serves is whatever
  somebody configured there. The models are now part of a provider's
  configuration and are what an agent's author is offered — suggestions and
  never a limit, so a model released last week can still be typed.

### Fixed

- **An address changed in the console reaches a process that is already
  running.** Providers were read once at boot and only prices refreshed, so
  pointing a provider at a proxy changed nothing until somebody restarted the
  worker and the console. Nothing said so: the endpoint the process had started
  with went on answering, authenticated, until the vendor reported a model it
  had never heard of. A provider removed in the console also stayed alive in
  the process for as long as it ran. Both now follow the same 30-second refresh
  that rates already had.

- **One unreadable credential no longer hands its provider to the environment.**
  Registration stopped at the first failure, so in the console process — where
  the master key is optional — a single sealed credential erased every
  configured provider at once. A provider registered from an environment
  variable under the same name then answered in its place, at a different
  address with a different credential. Each provider is now built on its own,
  and one that cannot be built holds its name empty and answers as not
  configured, which is a sentence somebody can act on.

- **A provider's endpoint, protocol and credential reach a run, not just the
  registry.** The resolver rebuilds a published version's planner when the
  provider registry's revision moves, and only a price change moved it — so a
  version that had already run kept speaking to the address it started with
  even after the refresh reached the registry.

- **A proxy configured without a credential is not handed the vendor's.** The
  Anthropic client reads `ANTHROPIC_API_KEY` and `ANTHROPIC_BASE_URL` on its own
  when it is given neither, so an installation's own key could be sent to an
  endpoint nobody chose it for. The platform now refuses those defaults
  explicitly.

- **Changing a provider's protocol no longer risks taking a worker down.** The
  code that needed a provider's kind read it, released the lock and asked for a
  planner built from a second reading; a refresh between the two produced the
  other shape and an unguarded type assertion.

- **A model list has bounds.** Two hundred names of two hundred characters, at
  the door and in the contract. The list is decoded by every process on every
  refresh, so an accidental paste would be paid for twice a minute for ever.

- **The provider name field has an accessible label.** The label pointed at the
  row containing the field rather than at the field, so the input had no
  accessible name.

- **An administrator can be named to be told about an approval.** The list an
  agent's owner picks from asked for the Approver role, and an administrator
  holds the approval act — so the colleague most likely to be named was not in
  the list, and naming them by hand would have been dropped in silence, because
  the delivery narrowed against the announcement list rather than against who
  may decide. Both now ask the permission. What a run is announced to when
  nobody is named is unchanged and still the Approver role: an administrator
  granted at the installation covers every company, and announcing by the act
  would tell every administrator about every parked run there is.

- **The agent editor's scrollbar sits at the edge of the page.** The element
  that measured the prose was also the element that scrolled, so on a wide
  screen the bar floated at the right edge of the text column with the page
  still stretching past it.

## [0.47.0] — 2026-09-10

### Changed

- **A conversation is stored under its connection as well as its id.** The
  second half of a move the previous release prepared: it learned to read both
  shapes and went on writing the old one, because both versions serve during a
  rolling update. Conversations written before this are renamed on upgrade, and
  the restriction that stood in for the key is gone — the same Slack channel id
  on two workspaces, in one scope, is two conversations again.

### Fixed

- **A channel binding stored in the wrong place can be seen and removed.** A
  binding lives at the installation, which is the only position anything reads
  it from — so one at a company or an area, from a restore or a hand-edited row,
  grants nobody anything. The console showed it as an ordinary binding and
  offered a delete that removed nothing: the removal was keyed at the
  installation, matched no row, and reported success. It is marked as being in
  the wrong place now, and the delete reaches it where it sits.

- **A run nothing could be told about is no longer retried twice a minute for a
  day.** Announcements are retried until they land or the run leaves the
  24-hour window, and a run with no destination at all came back every thirty
  seconds — 2,880 attempts written about an installation nobody had finished
  configuring. The wait now doubles from a floor to a ceiling, and it is two
  schedules rather than one: a destination refusing is an incident somebody is
  probably fixing, so the first retry is a minute away; nothing configured to
  hear the run at all is an installation being set up, and waits fifteen. The
  whole schedule still fits inside the window, so waiting longer is never
  giving up.

## [0.46.0] — 2026-09-10

### Added

- **An agent's owner says how its approvals should reach a person.** Under the
  agent's **Governance** tab: also send the approval as a direct message from
  the channel bot, and optionally name who — among the people who may already
  decide — should be told. It needs no conversation and no room: an agent
  published in an area nobody mapped now reaches the people who can answer it,
  which until now produced no card, no message, and no record that anybody
  should have been told.

  The console always has the request; this only decides where else it appears,
  and nothing here can switch approval off. Naming somebody is addressing and
  never authorising: the button is checked against the run's own scope wherever
  it is pressed, so a name that holds no grant is dropped rather than sent a
  message it cannot answer. The choice is versioned with the agent, so a run
  obeys what the version it pinned asked for.

  With more than one channel connection and no conversation covering the run's
  scope, nothing is sent and the reason is recorded — guessing would send one
  company's run into another company's Slack. A conversation in the run's own
  scope answers it, because somebody already chose that workspace for those
  runs.

### Fixed

- **A run nobody could be told about no longer holds up the ones that can be.**
  Announcements are taken oldest-attempt-first so that a broken destination
  cannot starve the rest, and a run with no destination at all had no attempt
  recorded against it — so it counted as never tried, sat at the front of every
  page, and an area that *was* configured never got its turn. Every run nobody
  was told about now records why, which is also what the cockpit needed: no
  connection to send from, nobody holding Approver, nobody with a linked
  account, or nothing configured to hear about it at all.

- **Seven channel failure codes reached the screen as identifiers.** Four of
  them since 0.45.0. They have words now, in both languages.

## [0.45.0] — 2026-09-09

### Added

- **A conversation can speak for the whole installation.** Until now a run
  parked in an area nobody pointed at a channel reached no card and no person,
  and waited for somebody to open the console. Under **Integrations → Channels
  → new conversation**, the context "the whole installation" makes one room
  hear about a run in any company, with the same options as any other
  conversation — including telling the people who may decide privately, which
  still reaches whoever holds Approver in that run's scope rather than whoever
  holds a grant at the installation. It starts nothing: mentioning the bot
  there is refused saying the room only reports, and no watched-message
  automation runs from it. A scope containing every company is the right answer
  for hearing and the wrong one for asking. Configuring or deleting it needs
  Company configurer granted at the installation scope, and the option is
  absent for anybody else rather than offered and refused. Choose "what to
  announce" before saving: the default is parked, failed and drifted, which at
  this scope means every stop, every failure and every drift in every company.

- **"Only reports" is a mode any conversation can be set to.** It is what the
  installation room saves, and it is offered on every conversation in the start
  mode list: a room somebody added the bot to for visibility is not a room
  anybody should be able to start runs from by typing in it. Wherever it is set,
  mentions and watched messages start nothing there, the bound agent is dropped
  — a conversation that starts nothing has none to start — and the console shows
  no fields about starting runs.

- **Drift can be announced in a conversation.** It was in the platform's
  defaults and in none of the console's: the screen offered parked, failed and
  finished, and always sent that list, so the fallback that includes drift was
  reachable through the API and nowhere else. It is an option now, and among
  the ones a new conversation starts with — an agent that quietly stopped
  holding its corrections is the one notice nobody thinks to ask for. Existing
  conversations keep exactly what they were configured with.

### Fixed

- **The same conversation id on two connections no longer overwrites itself
  in silence.** A conversation is stored under the id the vendor gives it, with
  the connection recorded only inside the row — so pointing the same id at the
  same scope on a second workspace replaced the first, with no refusal and
  nothing in the trail saying a conversation had been removed. Cards and
  approvals for that workspace simply stopped. It is refused now, naming the
  connection that already holds it.

  Storing it under the connection as well is the real fix and it is not this
  release: nothing changes shape on upgrade, because both versions serve while
  a rollout runs and a key that moved under the older one would take every
  conversation away from it. This version learns to read the new shape so the
  next one can write it. Until then, the same vendor id on two connections in
  one scope is a configuration the platform declines rather than loses.

- **Nothing inbound can be configured on a connection this version cannot
  reach.** A connection whose delivery mode came from a newer version opens no
  door here, so a mention or watch rule stored under it would do nothing — until
  a version that recognises the mode read it and put it into force, with nobody
  having decided anything in between. Such a connection's conversations may only
  announce, and the console stops offering to add one.

### Changed

- **A conversation's stored mode is handed back as it is stored.** The API used
  to answer with the nearest mode it recognised, so a conversation configured by
  a newer version read as "mentions" — and saving any unrelated edit from that
  reading wrote "mentions" and meant it, turning a room that started nothing
  into one anybody could start runs from. The response field is no longer a
  closed set (the request still is), and a client that does not recognise a
  value must not offer to save that conversation: this console says so and
  refuses to open the form.

## [0.44.0] — 2026-09-09

### Upgrade notes

- **A run that is stopped right now may be announced once more after the
  upgrade.** Announcements are recorded against the step a run stopped on, and
  rows written before this carry no step — the old row cannot say which stop it
  was about, and treating it as covering the current one would restore the very
  silence this release fixes. So a run that is parked or awaiting approval, and
  whose last activity is inside the 24-hour announcement window, gets one
  further message in the conversations that already heard about it. Runs that
  finished or failed are unaffected: those happen once, and their old records
  still match.

### Added

- **An approval can also be sent privately to the people who may decide it.**
  Turn on "also tell the people who may decide, privately" on a conversation
  that is told about parked runs, and the same card goes as a direct message
  from the channel bot to whoever holds Approver in a scope covering the run —
  in addition to the card in the conversation, never instead of it. It grants
  nobody anything: the button is checked against the run's own scope wherever
  it is pressed, so a recipient without the grant is refused in a direct
  message exactly as in a channel. Needs the Slack app's `im:write` scope and a
  linked account for each person; anybody unlinked is simply not reached. If
  more than twenty people may decide, nobody is messaged privately and the
  reason is recorded — telling an arbitrary twenty of them is worse than
  telling none. The cap counts messages rather than candidates, so people
  without a linked account never silence the ones who have one.

- **A card whose approval has been answered stops offering to answer it.** Once
  somebody decides — in Slack, in a private message, or in the console — every
  card posted about that step is rewritten to say what happened and who decided
  it, and the buttons go. This applies to conversation cards too, so an
  approval decided in the console no longer leaves live buttons behind.

### Fixed

- **Two people deciding one approval at the same moment no longer corrupt each
  other.** Reading the run, checking which step it waits on and recording the
  decision were three moments, and two requests that read the same state both
  passed the check and both wrote. A second approval then *destroyed* the
  first — the run returned to running and the agent asked again, with nobody
  told why — and a refusal never removed an approval, so a run approved and
  then refused carried usable permission past its own refusal, with a worker
  free to take the approved action while the refusal was still being written.
  The decision is now sealed onto the request it answers, under the same lock
  that orders the ledger, so the second one is refused and told that the
  question is settled. The same guard covers a decision that arrives while the
  run is being abandoned, which previously put a terminal run back to work.

- **A run that stops a second time is announced again.** An agent that needs
  two tools asks for two approvals, and only the first was ever announced —
  the second reached no channel and no person, and the run sat waiting until
  somebody happened to open the console. The same was true of a run parked
  twice by a budget or by retries that stopped helping. Announcements, the
  per-conversation deduplication and the failure records are all keyed by the
  step now.

## [0.43.0] — 2026-09-05

### Upgrade notes

- **A conversation in "watched messages" or "both" mode now answers mentions
  with the agent it already names, and refuses mentions that name another
  one.** Those conversations were configured with an agent for their watched
  half; the mention half ignored it and let anybody address any agent published
  in the scope. After this upgrade a mention there needs no agent name, and one
  that names a different agent is refused with the name of the agent this
  conversation starts. **Review every conversation in "both" mode where people
  deliberately mention more than one agent** — either give each agent its own
  conversation, or set that conversation to "mentions only" and leave its agent
  unset, which restores exactly the previous behaviour. Conversations in
  "mentions only" mode are unaffected: they had no agent, and an unset agent
  still means the message names one.

- **A conversation naming an agent that is not published in its scope, or that
  never declared the Conversation trigger, is now refused when it is saved.**
  The check existed only for watched messages, so such a configuration could
  already have been stored through the API. Reopening those conversations in
  the console shows what is wrong; nothing stored is rewritten on upgrade, and
  the refusal in the channel already named the agent.

- **A conversation in "watched messages" mode stops answering mentions.** It
  never should have: the mode says only the configured Slack sources start
  agents, and both delivery paths let a mention through regardless. Needing to
  name an agent was the accidental brake, and a conversation that names one no
  longer needs it. If people rely on mentioning the bot in such a channel, move
  it to "both", which keeps each path with its own authority. The refusal says
  what happened, so nobody is left with a silent channel.

- **A mention with no words no longer starts a run.** `@bot` on its own, in a
  conversation that names an agent, would open — and pay for — a run with no
  question in it. It is now refused with a sentence saying what to do, unless
  the mention arrives with something to work on: a thread the platform posted a
  run into, or a thread whose earlier messages were actually read because the
  conversation has "include thread context" on. Being in a thread is not enough
  on its own — a thread nobody read leaves the agent as empty-handed as a bare
  mention.

- **A watched message that says nothing no longer starts a run either.** The
  same rule, and it is now safe to apply because an alert's words are read out
  of its Slack blocks and attachments when the message has no `text` of its own
  — which is how most alerting integrations post. An alert that reaches the
  platform genuinely empty is refused rather than turned into a model call with
  no question in it. If an integration posts content in a shape this does not
  read, its conversation stops starting runs: the delivery is still recorded in
  full, so the payload is there to check.

### Fixed

- **The channel projection stopped depending on an ask having text.** What the
  model sees of a channel ask is a projection that deliberately leaves out
  `asked_by`, the vendor account, the conversation and the thread references.
  It selected records by their text, so an ask whose question is the thread it
  arrived on fell through to a fallback that sent the raw envelope instead. Any
  channel ask that parses is now projected, whatever it turns out to hold.

### Changed

- **Mentioning the bot no longer repeats what the conversation already says.**
  A conversation names the agent it is for in every start mode, and a mention
  there starts that agent without naming it — the whole sentence is the ask.
  The agent selects and does not authorise: the run still acts on behalf of the
  person whose channel account is linked, and an unlinked account is still
  refused. Leaving the agent unset keeps the old arrangement, where the message
  names the agent and any agent published in the scope can be started.

## [0.42.0] — 2026-08-31

### Added

- **Governed SQL connector instances can now be configured and edited in the
  console.** Operators choose a scoped Vault database role, register fixed SQL
  templates and their typed parameters, and set execution limits without ever
  entering a password or connection string. The ordinary connector list still
  withholds query text; only connector configurers can read the authored SQL
  needed by the edit form.

## [0.41.0] — 2026-08-30

### Upgrade notes

- **Enabled SQL connector instances become executable after this upgrade.**
  Review each registered template, its read-only database role, Vault binding
  and Gate policy before upgrading; disable any instance that is still being
  drafted. Existing agent packs only receive the configured native tool, and
  each execution still crosses the Gate before Vault or the database is
  contacted.

### Added

- **Governed SQL templates can use a fresh Vault database credential for each
  execution.** The model chooses only a registered template and its typed
  parameters. FuseOne resolves the scoped Vault binding after approval, opens
  one TLS-verified read-only PostgreSQL connection, bounds rows, bytes and
  time, stores the result by reference, closes the connection and returns the
  lease. The approval is pinned to the registered target, Vault binding,
  Vault endpoint, query, parameter types and limits; changing any of them
  requires a new decision. SQL results are never served from the MCP result
  cache. PostgreSQL enforces the effective execution budget server-side as
  well as through the worker deadline.

### Security

- **Database authority remains private to the worker.** SQL tool schemas and
  governed results contain no Vault path, token, username, password, DSN or
  lease id. Driver and Vault errors are reduced to stable codes, result rows
  that contain the issued username or password are refused, and metrics carry
  only bounded stage and outcome labels. PostgreSQL text is the safe fallback
  for every result type, so an extension or a newly supported driver codec
  cannot expose a driver-specific Go representation. UUIDs, network values,
  exact numerics and `bigint` remain lossless strings; explicitly safe values
  such as JSON stay structured, and binary values remain base64. The connector
  also fixes the session's date, interval, timezone, float and bytea settings,
  so database or role defaults cannot silently change that representation.

## [0.40.0] — 2026-08-28

### Upgrade notes

- **Pending memory suggestions keep the identity they had before this
  upgrade.** New human and agent teaching now shares the platform-derived
  `fact`/subject identity, but existing pending suggestions are not rewritten:
  they remain an exact record of what the agent proposed. If the same fact is
  proposed again after upgrading, the review queue can temporarily show both
  identities until the older suggestion is decided or expires. Existing active
  memory is not changed.

### Changed

- **Teaching a memory now asks for a subject, a claim and a reason.** The kind
  and signature are derived from the subject in both the human form and the
  agent's suggest tool, so one fact has one identity whoever taught it. A
  memory created before this keeps the identity it has, and correcting one
  still accepts its original kind and signature — but only for an identity
  that already exists.

## [0.39.6] — 2026-08-28

### Changed

- **Memory evidence is now chosen from finished runs in the selected scope.**
  The creation form lists recent runs, searches by run or agent identifier and
  marks untrusted origins instead of asking someone to type a ledger id.

### Fixed

- **Changing the memory scope clears evidence from the previous scope.** This
  applies both inside the form and when a preserved draft follows the page's
  active scope, so old evidence cannot be paired with a new destination.

## [0.39.5] — 2026-08-28

### Changed

- **Memory scope is now chosen from the companies and areas the caller can
  reach.** The creation form offers one searchable company/area picker instead
  of two free-text fields, preventing typos and invalid cross-company pairs.

## [0.39.4] — 2026-08-28

### Fixed

- **Memory creation no longer traps people in the form or silently keeps stale
  context.** A draft can be closed, resumed or explicitly discarded; hidden
  drafts stop querying the server, and preserved drafts follow the active
  company and area before they are saved.

## [0.39.3] — 2026-08-28

### Fixed

- **A policy opens with its stored values on the first edit.** The editor now
  waits for the selected policy before creating its draft, instead of keeping
  the empty draft that existed while the first request was loading.

## [0.39.2] — 2026-08-28

### Changed

- **Run investigation now opens directly on the trail.** The trail and its
  summary cards share one tab, while detailed cost and prompt composition live
  in a separate Cost tab. Identity, pending decisions and the run KPIs remain
  visible in both views.

## [0.39.1] — 2026-08-28

### Fixed

- **Agent validation no longer covers every operational tab.** Readiness and
  trust evidence now live in a dedicated Control center tab, while Runs,
  Definition and Steps open directly on their own content.
- **Agent cost now says what it measures.** The header reports average cost per
  run and names the recorded total and run count instead of labeling the total
  as per-run cost.
- **Missing trust evidence explains how it appears.** Version and cost
  comparisons identify the baseline they need, while absent human decisions
  are labeled as not observed and explain that there is no manual action to
  trigger.

## [0.39.0] — 2026-08-28

Teaching an agent a fact no longer means filling a ledger record, and a run that
keeps re-reading the same thing now stops instead of buying another turn.

### Upgrade notes

- **Raise the Helm timeout to at least 25 minutes.** This release adds a
  `reconcile-memory` Job as a `pre-install,pre-upgrade` hook, running after the
  migration. Helm's default is five minutes and it abandons a hook that outlasts
  it, failing the release while the work is still running. Pass
  `--timeout 25m`. On Argo CD the equivalent is `controller.sync.timeout.seconds`
  wherever it is set — its default is `0`, meaning no limit, so an Application
  left at the default waits. The Job itself exits zero when it finds records it
  cannot converge; it reports the counts and does not block the release.
- **Three fields left the memory creation contract.** `POST /admin/memory/assertions`
  no longer accepts `agentId`, `observations`, `confirmed` or `expiresAt`, and
  `namespace` is now required and must be `agent` or `shared`. A client that
  still sends the old shape is refused. The agent is read from the run the
  evidence names, the counters start at one, and memory lasts 30 days from the
  decision that wrote it. Nothing about existing stored memory changes.
- **Memory creation now proves its evidence before writing.** A citation whose
  run cannot be read, or whose bytes are no longer in the content store, is
  refused instead of producing a memory nothing can support. Installations that
  scripted memory creation against runs they later erased will see those calls
  refused.
- **The built-in policy version moved to `builtin/v4`.** Recorded decisions
  carry the version that produced them, so old records stay readable and
  comparable; nothing is rewritten.
- **A run may no longer publish an artifact named `final_answer`.** That name
  belongs to the run's closing answer, and an artifact under it could never be
  cited. It is dropped from what the run publishes; the run still finishes.

### Added

- **"Remember this", from the run that showed it.** A step that a memory can
  cite offers to teach one, for whoever can publish. The citation, the labels
  and the agent come from the ledger and are shown without being editable; what
  is asked for is what only a person knows.
- **What already answers this, before it is taught again.** Both authoring paths
  show the active, shared or pending memory with the same identity before
  saving, and each state offers only what the platform will accept: correcting,
  reactivating, renewing, improving the shared one explicitly, or reading the
  review queue. An erased source is shown and offers nothing.
- **A pending suggestion can be accepted in better words.** The claim is
  editable; the subject, signature and kind are the identity and are not.
- **A disabled memory can be brought back**, with a reason, recorded as its own
  event.
- **A run stops instead of re-reading.** Three consecutive read calls returning
  the same result park the run for a person rather than buying another turn.
- **An architecture drawing in the README and NT-010**, with a test that fails
  if the package layering stops matching what is drawn.

### Changed

- **Prompt caching now works on every provider, not only Anthropic.** Per-stage
  guidance is stable, so an OpenAI-compatible endpoint sees an exact growing
  prefix; the changing budget remainder no longer sits inside it. Long runs keep
  a bounded transcript in stable generations, so older results become receipts
  in steps rather than one per turn.
- **Repeated tool calls are recognised by what they say, not how they were
  spelled.** Argument order and whitespace no longer make one call look like
  two. A completed read may be repeated to observe a changing source; writes
  stay exactly-once, and a call whose outcome never reached the ledger stays
  blocked.
- **Memory identity is canonical.** `"Slack Alerts"` and `" slack   alerts "`
  are one fact, so correcting one no longer creates a second.

### Fixed

- **Policies can be edited and removed from the list again.** Editing works from
  the row and from its icon, removal names the policy and asks first, and the
  list refreshes after.
- **Memory taught from a tainted run keeps the taint.** The closing step now
  carries the labels the run had accumulated, so a fact learned inside a
  poisoned run is no longer remembered as trustworthy.
- **Erased evidence ends the memory that rested on it**, dated when the platform
  found out rather than inheriting the last human decision's timestamp.

### Security

- **Memory that looks like a credential is refused.** A private key or a
  complete recognised token is blocked outright; text long and random enough to
  be a credential warns, and a person with publish permission may override it —
  which marks the assertion with `secret` rather than clearing quietly. Nothing
  repeats the refused value in an error, a log or an event. Automatic
  confirmation has no override: such a proposal stays pending for review.


## [0.38.10] — 2026-08-27

### Changed

- **Console tabs and segmented filters now carry matching icons.** The agent,
  memory, available MCP servers, people and companies screens now use icons
  beside their tab text, including the smaller filters inside those pages. The
  labels stay the same, and the icons use the shared tab sizing so the controls
  scan consistently across the console.

## [0.38.9] — 2026-08-27

### Fixed

- **The tool picker opens where the `@` was typed.** Long instruction blocks
  could anchor the picker near the top of the editor instead of beside the
  marker, especially when the marker was in the middle of a paragraph. The
  editor now measures the typed marker in the same coordinate system as the
  popover, keeps the mirror text complete, and inserts the tool without moving
  another `@` already in the block.

- **`Never` drift warnings stop treating missing context as a conflict.** A
  step that says "stop when information is missing" should not conflict with a
  `Never` block that forbids acting on that subject. The detector now requires
  a stronger signal before warning, preserves real conflicts such as
  "reembolso sem revisão", and keeps warnings actionable instead of training
  authors to ignore them.

### Changed

- **Agent tabs now carry matching icons.** Runs, definition and steps use the
  shared tab icon sizing so the page reads like the rest of the console.

## [0.38.8] — 2026-08-27

### Fixed

- **A rehearsal nobody saved no longer speaks for the corpus.** Every simulated
  run was being counted as a battery, so an ad-hoc rehearsal run after the
  saved cases could become the newest one the Trust Center read — and it
  reported "1 of 1 saved case(s) broke" about cases that had not been checked
  at all. A battery is now only the simulated runs that name a saved case, in
  the store and in the fake alike. With the evidence right, the published
  agent's guide also stops asking for a rehearsal once the corpus has run and
  held.

- **A tool can be cited where the `@` was typed.** The picker used to write the
  identifier at the end of the block, so citing mid-sentence moved the tool to
  the wrong place. It now replaces the marker the person actually typed —
  including when the block already holds another `@`, such as an email address
  — and leaves the cursor after the identifier it inserted. Clicking a block to
  edit it now puts the caret in the text rather than in a row that looks
  editable and is not.

- **Generic verbs stop reading as a conflict with `Never`.** "Consultar",
  "verificar", "analisar" and "prosseguir" are how a step describes itself, not
  a promise to do the thing a `Never` block forbids. A warning that fires on
  ordinary prose teaches people to ignore warnings.

## [0.38.7] — 2026-08-27

### Added

- **The published definition shows the steps it declares.** The prose is still
  exactly what the author wrote and the stages are still exactly what the Gate
  uses, but the read view now names both. A stage could change while the
  definition looked untouched, which made a real edit read as a lost one.

- **A clean rehearsal can be saved as a regression case.** One click records
  the baseline the run already demonstrated, so an agent stops reporting that
  it has no corpus the moment somebody agrees with a result. A case that came
  from the corpus does not offer to be saved again — it is already there, and
  saving it would grow the corpus with copies of itself.

### Changed

- **`@` offers what the agent can actually call.** Inside "How to act" the
  picker lists the agent's enabled pack, because there the gesture is a
  shortcut for a tool the run will really be offered. Everywhere else it still
  lists the whole catalogue, so prose may name a tool the agent does not hold
  and the lint can mark that sentence rather than the editor preventing it.

### Fixed

- **Budget reconciliation stops inventing zeros.** A reconciliation step
  carries what is given back; what was spent is recorded on the tool's own
  step, so a run that reconciled without a cost was being narrated as
  "$0.00 spent". It now says only what it knows: the reconciliation alone, or
  the amount released, or both when the step really carries a cost.

## [0.38.6] — 2026-08-26

### Fixed

- **Memory lookup now starts before the first model turn when learning is
  enabled.** FuseOne records one `$fuseone.memory.find` call from the human
  input before planning, so an agent gets a chance to reuse active memory
  before it suggests the same fact again. The lookup still crosses the Gate,
  appears in the trail and carries returned labels into the run.

- **Channel input is projected to the same request text the console sends.**
  Slack-style channel payloads are no longer shown to the model as wrapper JSON
  when they are ordinary user requests. That keeps the same ticket from
  producing a memory search in one run and a broader document search in another
  just because one started from Slack and the other from the console.

- **Long memory searches no longer drop the identifier that matters.** Search
  terms are bounded, but strong identifiers such as `not_in_channel` or
  `superset.alert.delivery` are kept before filler and ordinary words. When the
  budget still omits terms, the memory tool says which terms were used, how
  many were omitted and why, so a bounded lookup is not confused with missing
  memory.

- **Short identifiers match as their own terms instead of arbitrary
  substrings.** Queries such as `s3`, `db` or `qa` can find memory that names
  those systems, while short words no longer satisfy a search by matching
  inside unrelated prose.

- **Anthropic planning turns must choose a platform tool or finish action.**
  The request now requires a single tool use, preventing free-text assistant
  replies that the ledger cannot record as an action and that previously parked
  the run as `no_finish_action`.

## [0.38.5] — 2026-08-26

### Fixed

- **A suggestion no longer needs approving twice.** Recording a memory
  suggestion is still a write and still crosses the Gate, but it cannot become
  active memory on its own, so the run no longer stops for an approval before
  the review queue gets the item it exists to inspect. The queue is where a
  person decides. Under auto-confirm the relaxation applies only when the
  observation is untrusted, because a clean one can become memory without
  anybody looking.

- **An untrusted observation is never promoted automatically, whatever comes
  after it.** Auto-confirm counted the same assertion across distinct runs and
  weighed the trust of whichever run happened to be last. A poisoned run
  followed by clean ones therefore promoted itself. The decision now reads the
  labels the suggestion has accumulated, so one untrusted contribution keeps it
  in review until a person decides — the same rule the rest of the platform
  already follows: taint never dilutes.

- **An agent that can suggest memory can also look it up.** Learning was
  offered without the lookup beside it, so an agent could record what it
  learned and never read it back.

## [0.38.4] — 2026-08-26

### Fixed

- **Memory suggestions in review mode no longer require a second human
  approval before reaching the review queue.** Suggesting memory is still a
  write, and auto-confirmed learning still goes through the Gate, but review
  mode now treats the memory queue itself as the place where a person decides
  whether the fact should become active.

- **Approved memory can be corrected without changing what it is.** Operators
  can improve the remembered claim while keeping the assertion identity,
  evidence, labels and expiry intact, so a clearer human wording does not turn
  into a new fact.

- **The Memory page now scales as an index and reader instead of a wall of
  cards.** Active, disabled, suggested and all memory entries share the same
  searchable list, with details shown beside the selected item. Untrusted
  origin stays visible in the compact row, because reviewing a suggestion only
  makes sense when the reviewer can see what kind of evidence produced it.

- **A memory suggestion is no longer duplicated when only the wording changes.**
  Existing active memory is matched by kind, subject and signature, so a model
  proposing a better or different claim for the same remembered situation does
  not create another pending item.

- **Channel notices are routed to the conversation plugged into the agent that
  produced them.** Agent-specific Slack conversations no longer receive run
  notices from every agent in the same area. Scope-wide governance alerts remain
  scope-wide, so aggregated policy refusals still reach every relevant
  conversation.

## [0.38.3] — 2026-08-26

### Fixed

- **An agent with memory learning on is now told when to use it.** The tools
  were offered without any guidance, so the model had to infer from a name and
  a one-line description whether to look something up or record something.
  The prompt now says to look memory up early, to treat what comes back as
  evidence carrying its origin rather than as instructions, and to suggest only
  narrow, stable facts — never one-off details, secrets, permissions or
  opinions.

- **Only the versioned learning policy can turn memory writing on.** The
  suggestion tool is removed from the offered set and put back when the policy
  allows it, so a pack that names it by hand cannot enable writing on its own.
  Nothing about this changes what the Gate does: a suggestion is still a write,
  and a run carrying untrusted labels still meets the taint check before
  anything is recorded.

## [0.38.2] — 2026-08-26

### Fixed

- **A long tool description no longer pushes the classify button out of the
  row.** The table of tools waiting for a ruling sized itself from its content,
  so one verbose description could carry the action off the visible area and
  leave a tool nobody could classify. The table now has fixed columns, the
  long fields truncate inside their cell, and the full description is still
  readable on hover.

## [0.38.1] — 2026-08-26

### Added

- **The GitHub Pages documentation site now carries the full product
  documentation.** The online site renders the manual, design notes and Helm
  chart reference with the FuseOne docs branding, so operators do not have to
  browse raw Markdown files to understand the platform.

### Fixed

- **Memory learning no longer sends a double-wrapped tool schema to model
  providers.** Agents with memory learning set to review or auto-confirm mode
  offer `$fuseone.memory.suggest` as provider properties, not as a complete
  JSON Schema nested inside another provider envelope. Anthropic no longer
  rejects those runs with a 400 before the agent can act.

- **Native tool schemas now have a provider-envelope contract.** The tests
  reject top-level `type`, `properties`, `required` and
  `additionalProperties` on FuseOne-owned native schemas while still allowing
  nested object schemas to use those keywords.

## [0.38.0] — 2026-08-26

### Upgrade notes

- **This release adds two migrations, neither on `run_steps`.** 0063 adds the
  versioned memory-learning policy to published agent specs. 0064 creates the
  governed memory suggestion queue and extends the memory event log actions.

- **Existing agents do not start learning automatically.** Published versions
  default to memory learning off. An agent can propose memory only after a new
  version opts into review or auto-confirm mode.

### Added

- **Agents can propose governed memory without writing active memory directly.**
  Review mode records structured suggestions for a person. Auto-confirm mode
  promotes only the same suggestion observed across the configured number of
  distinct runs. The model never chooses company, area, agent namespace or
  confidence. The suggestion tool is still a write effect: tainted runs need
  Gate approval before they can record one.

- **The Memory page now has a suggestion review queue.** Operators can accept
  or dismiss pending suggestions with a reason. Suggestions carry run labels
  and ledger evidence, remain separate from active memory until promoted, and
  become source-erased if their evidence is erased before review.

- **The agent editor exposes memory learning as an explicit governance
  setting.** The policy is versioned with the agent definition and appears in
  the publish diff when it changes.

## [0.37.0] — 2026-08-26

### Upgrade notes

- **This release adds one migration, not on `run_steps`.** 0062 enables
  trigram search for governed memory by building GIN indexes on `subject`,
  `signature` and `claim`. Unlike the recent FinOps and runtime releases, this
  migration does not build indexes on the partitioned run-step ledger.

- **Memory search now requires the PostgreSQL `pg_trgm` extension.** The
  migration enables it with `create extension if not exists pg_trgm` and builds
  GIN indexes on `subject`, `signature` and `claim`. This keeps the existing
  substring search semantics while giving larger memory sets an index the
  runtime can use. If the application database role cannot create extensions,
  the migration fails and the release does not roll out; have a DBA create
  `pg_trgm` in the database before upgrading, and the migration will reuse it.

### Added

- **The manual now explains governed memory and duplicate effects.** Both
  locales document what memory stores, how labels travel through a memory read,
  how retention and erasure affect assertions, and why duplicate effect
  recognition is different from cache or memory.

- **The Connections page can prepare governed connector instances.** The page
  now separates runnable Vault instances from connector shapes that are still
  planned. A Vault instance can be scoped, pointed at an endpoint and mount,
  limited to approved path prefixes, and given a sealed token. The token is
  never returned to the browser; the API reports only whether one is stored.

### Changed

- **Memory reads now have a prompt-size budget and worker metrics.** The memory
  tool records call count, latency, returned assertions and assertions omitted
  by the response budget on the worker metrics endpoint. The labels are fixed:
  no agent, scope, search text, assertion id or claim can become a Prometheus
  label.

## [0.36.0] — 2026-08-26

### Upgrade notes

- **This release adds three migrations, none of them on `run_steps`.** 0059
  creates the cross-run effect dedupe registry, 0060 creates the governed
  memory tables, and 0061 makes the memory event log append-only. Unlike the
  recent FinOps and runtime releases, these migrations do not build indexes on
  the partitioned run-step ledger.

- **Retention and erasure now reach governed memory.** Memory assertion events
  and active assertions are swept by the normal retention job. When an erasure
  removes run content that an assertion used as evidence, the assertion is
  marked `source_erased` and stops being returned to agents. The row is kept
  long enough to explain why the memory disappeared from reads.

### Added

- **Agents can skip repeating the same governed effect across runs.** Tool
  classification can now declare a semantic dedupe key: stable argument paths
  and a window. The platform prefixes that key with company, area, agent and
  tool, so dedupe cannot cross tenants or agents and the operator cannot remove
  those boundaries.

  The Gate stays in the path and still records a decision. Lookup happens before
  the Gate, reservation happens only after the Gate allows the call, success
  confirms the effect, and failure releases the reservation. A duplicate is not
  cache and not a policy block: it means the platform already recorded that
  effect and did not send it again.

- **Governed memory gives agents remembered assertions without remembered
  prose.** A reviewed assertion is structured by kind, subject and signature,
  points at ledger evidence, carries data labels, and can be shared at the
  agent or area level.

  Memory is not remembered text the model promises to treat carefully. It
  carries origin and labels; reading it taints the run, and the Gate refuses the
  next write without asking anyone. That makes the guard mechanical rather than
  prompt-shaped.

- **The console has a Memory page.** Operators with publish permission can add
  reviewed assertions from ledger evidence, see active and retired assertions,
  and disable an assertion with a reason. Lists page visibly instead of cutting
  silently.

### Fixed

- **Pending dedupe waits no longer depend on scheduler timing in tests.** The
  supervision test now keeps another run's reservation pending until the
  deadline, instead of racing a one-millisecond timer against a one-millisecond
  ticker.

## [0.35.0] — 2026-08-25

### Added

- **The governed connector catalogue now includes data and identity shapes.**
  The catalogue lists three additional planned connectors: governed SQL read
  access, object storage, and identity actions.

  SQL is described as read-only and template-based, so a future runtime has a
  contract for database lookups without granting arbitrary query execution.
  Object storage says bytes move through content references rather than inline
  model text. Identity actions distinguish reads from account changes, and
  destructive actions such as disabling a principal or revoking sessions require
  a human decision.

  This is still catalogue only. It creates no credentials, starts no connector
  runtime, exposes no executable tool to an agent, and performs no database,
  object-store or identity-provider call. The screen is a product contract for
  what the governed runtime must later enforce, not evidence that an
  installation can execute those operations today.

## [0.34.3] — 2026-08-25

### Fixed

- **The tool count inside a capability chip sits on its centre line.** The chip
  and its parts inherit the card's line height, which left the count riding
  high inside its box on the agent list.

## [0.34.2] — 2026-08-25

### Fixed

- **Agent cards are the same height, so the list can be compared.** A card used
  to list every tool on its own line, which made an agent with fifteen tools
  three times taller than one with a single tool: the grid broke, and the
  questions a list exists to answer — is it running, is it failing, what does it
  cost — fell below the technical names. Tools are now grouped into one chip per
  integration inside a fixed box, with a counter for what does not fit, and the
  card carries three numbers instead of six. Ceilings, steps and triggers are
  configuration; they belong on the agent's own page.

- **A capability nobody has classified no longer looks safe.** The chip's colour
  is the answer to "what can this touch", so it now has three states rather than
  two: highlighted for write, destructive or financial; plain for tools proven
  read-only; and outlined for tools the catalogue has not classified.

  Before, an unclassified capability was drawn exactly like a proven read-only
  one. The card was careful not to invent risk and invented safety instead,
  which is the worse direction in a console somebody opens to decide what an
  agent may be trusted with. The Gate already refuses to call an unclassified
  tool and the runtime already names it; this was the last surface where its
  absence read as calm.

## [0.34.1] — 2026-08-25

### Fixed

- **The agent overview no longer offers the same next step twice.** When the
  guided path recommends a rehearsal, that recommendation is the visible
  action and the header's Simulate shortcut moves into the overflow menu.
  When the guide points somewhere else, Simulate returns to the header and
  does not also sit in the menu.

  Two buttons side by side pointing at the same screen do not read as a
  shortcut beside a recommendation; they read as one duplicated button, and
  the recommendation is the half that loses. Keeping the shortcut reachable
  rather than removing it leaves a path for somebody who already knows what
  they want.

## [0.34.0] — 2026-08-25

### Changed

- **The agent's Trust Center is judged by the server, not by the browser.** The
  console used to derive its own verdict from whatever fields it happened to
  have. The judgment now comes from `GET /agents/{agentId}/trust`, built where
  the evidence lives, and the page renders what it is told. Two people looking
  at the same agent can no longer be shown two different conclusions because
  one of them loaded a different set of fields.

  The evidence itself is wider than before: how runs under this version ended,
  whether a simulation ran the saved corpus, how its cost compares with the
  previous version, Gate and policy blocks, human decisions, the capabilities
  it was granted, and whether it is published and running.

- **Trust evidence now states the period it covers.** Run, cost, Gate and
  human-decision evidence is read over the last 30 days, and the response
  carries that window so the screen can show it.

  Without a window the numbers were true and mute. "Approved 400 times,
  refused 3" over eight months cannot tell *was bad and got fixed* from *is bad
  now*, and a version whose only blocks happened months ago was being judged
  for them today. Evidence that does not depend on a period — a saved
  regression corpus either exists or does not — is still read whole, and the
  wording says which is which rather than claiming "never" from a 30-day read.

## [0.33.0] — 2026-08-24

### Security

- **An agent whose untrusted input can reach a destructive or financial tool
  no longer publishes.** The draft's declared steps are checked against the
  tool catalogue before the version is saved, and a path from an untrusted
  source to a non-reversible act is refused with the path named.

  Reversible writes still publish. They are answered by the Gate at the
  concrete call, with the real arguments in front of a person — which is a
  decision somebody can actually make. Destructive and financial acts are not
  made safe that way: by the time the approval is asked, the model has already
  chosen the target from text it did not verify.

  The way out is order, not an override. Sources carry forward, so a
  non-reversible tool in a step *before* any untrusted read still publishes —
  the same rule taint follows at runtime. An unclassified tool does not block
  publication, because the Gate already refuses to call one.

### Added

- **The trail shows the labels each step carried.** `planned` names the labels
  on the model input for that planning call; `gate_decided`,
  `approval_requested` and `tool_called` name the labels on the tool input
  being judged or executed. Reading a shared context artifact also records the
  run it came from and its digest.

  Provenance stops being something to reconstruct by reading the whole trail
  and becomes something each step states about itself.

## [0.32.0] — 2026-08-24

### Added

- **One agent can hand another its work without copying it.** An agent may
  publish named artifacts when it finishes, and the event it emits declares
  which of them it exposes. A listening run receives the contract only — name,
  reference, digest, origin and labels — never the bytes. To read one, the
  model has to call `$fuseone.context.read` by name, and that call crosses the
  Gate as an ordinary read and lands in the trail like any other.

  The model cannot ask for a reference. It asks for a name, matched against
  the list the event declared and the ledger recorded when the run opened, so
  reaching content nobody offered is not refused — it cannot be expressed. A
  digest that no longer matches fails closed, and so does content that retention has
  already erased.

  Labels travel with the artifact. Reading something an untrusted source
  produced taints the reading run, so the write it tries next meets the same
  Gate the first run would have.

- **The runtime page opens with what needs attention.** One queue over every
  operational projection the page already held — stuck leases and backing-off
  runs, model provider failures, MCP tool failures, channel delivery failures
  and stdio egress denials — ordered by how often each is happening. Items
  carry a stable code and a count and nothing else: no run id, tool argument,
  conversation text, URL or provider diagnostic. The list pages rather than
  truncating, so "8 of 23" is visible instead of a cut nobody sees.

  Egress denials appear only for a caller who can read the whole installation,
  because that signal belongs to the worker process rather than to any one
  scope.

- **An agent's overview says what would justify trusting it further.** The
  Trust Center reads evidence the platform already has — how its runs ended,
  whether a regression corpus exists, whether anybody owes it a decision, and
  whether it is published and running — and recommends the next step for its
  stage.

  It separates four answers rather than two: missing, unknown, bad and good. A
  run still in flight is not a failure, an absent regression corpus is not a
  bad one, and **a run waiting for a person is not a mark against the agent** —
  a copilot agent waits for people by design, and the Gate stopping a call is
  the platform working. Execution failures are what count against it, and they
  still do even while something else is pending.

### Security

- **Scope labels are a data barrier, not a search filter.** A run may only act
  while every company and area label it carries is inside its own scope, and
  the check runs before taint, policy and budget. It stops an event carrying
  another company's data before the listening run is opened — before the input
  is even stored — and stops it again at the Gate if it arrives another way.

  **A human approval does not release it.** Every other rule in the ladder can
  be answered by a person clicking approve; this one cannot. Data crossing a
  company boundary needs an explicit recorded authorization, not a decision
  taken one tool call at a time by whoever happens to be on the approvals
  screen.

## [0.31.0] — 2026-08-24

### Upgrade notes

- **Migration 0058 creates the durable stdio egress-denial projection.** It adds
  `mcp_egress_denials`, keyed by server, exact configured destination and stable
  denial code. The table stores counts and first/last seen times, not request
  paths, query strings, headers, bodies, tokens or tool arguments.

- **Retention now expires stdio MCP egress-denial rows too.** Rows age out by
  `last_seen`, in batches, and the `content.expired` audit event reports the
  number as `egressRecords`. A denial that keeps recurring remains current
  operational evidence until it stops and ages past the retention window.

### Added

- **The runtime cockpit now shows stdio MCP egress denials.** When a local MCP
  process is routed through the FuseOne egress proxy and tries to leave its
  allow-list, the worker reports the attempt with a stable code. The console
  reads the durable projection under installation scope, so it can answer
  "which contained servers are trying to leave their declared route" without
  opening pod logs.

### Security

- **Denied stdio destinations are bounded before they become evidence.** Exact
  configured destinations are stored with host and port; wildcard children and
  destinations chosen entirely by the MCP process are aggregated by code only.
  That keeps the table limited by operator configuration rather than by
  untrusted process input.

## [0.30.0] — 2026-08-24

### Added

- **MCP connection health now separates discovery from concrete tool calls.**
  The integration panel can show "the server answered discovery" beside "the
  last tools/call failed with `mcp_personal_credential_missing`", which is the
  diagnostic operators used to need pod logs for. The call side records only a
  stable code, timestamp and worker name; tool arguments, URLs, payloads and
  vendor diagnostics stay out of the health table.

### Fixed

- **A tools/call observation no longer invents discovery health.** If a runtime
  call happens before discovery has written a row, or just after a server was
  removed, the platform leaves discovery unknown instead of creating a fresh
  "unreachable" discovery row from a successful call.

## [0.29.0] — 2026-08-24

### Upgrade notes

- **A strong stdio MCP egress statement now requires an explicit operator
  declaration.** `worker.stdioEgress.networkPolicy.enforced=true` tells the
  console that worker pods are covered by CNI-enforced NetworkPolicy, whether
  rendered by this chart or installed separately. Do not enable it on clusters
  where NetworkPolicy is ignored or not covering the worker pods; the platform
  cannot infer that from manifest presence.

### Changed

- **Proxied stdio MCP connections now distinguish proxy-only from proxy plus
  deployment containment.** Without the declaration, the console still says the
  local proxy refuses destinations outside the allow-list but direct sockets
  need deployment NetworkPolicy. With the declaration, it says the operator has
  stated that policy is enforced.

## [0.28.0] — 2026-08-24

### Upgrade notes

- **Runtime failure diagnostics add partial indexes over `run_steps`.** The
  release keeps `/runtime` from folding the audit trail during an incident by
  indexing failed tool-return steps both newest-first and by scope. On
  installations with large partitioned history, expect the migration to spend
  time building those indexes before the new image is ready.

- **Retention now deletes channel operational records too.** `channel_inbox`,
  `channel_deliveries` and `channel_delivery_failures` age out on the same
  window as content. The migration adds an index over `channel_inbox.at` so the
  sweep does not scan the inbox table. Open channel debts are kept until they
  are answered, so a run that waits through the weekend for approval can still
  answer the Slack thread that asked. After a debt is answered, that row follows
  the same retention boundary as old channel input and delivery diagnostics.

### Added

- **The worker's `/metrics` endpoint now answers for MCP and channels too**, not
  only the run pool: tool calls by result and stable code, calls refused before
  leaving the worker, channel sweeps by task and result, and each channel
  failure counted by its own code rather than whichever one sorted first.

  This endpoint is deliberately the volatile half. It reports what *this worker
  process* has seen since it started, it resets on restart, and two replicas
  answer two different numbers. It is the right source for "is this happening
  right now" and the wrong one for "since when" — which is why the console does
  not read it.

  Every label is drawn from a fixed vocabulary and anything outside it becomes
  `other`, so a code that arrives from a third-party MCP server cannot grow the
  label set or the process's memory.

- **The runtime page now shows durable channel delivery failures.** When a
  channel announcement cannot be delivered, the reporter records the stable
  failure code with the run's scope, target conversation, first seen time, last
  seen time and retry count. The operations cockpit reads that durable source
  under the caller's run scope; Prometheus still answers only what the current
  worker process has seen.

- **Simulations now show cost before they open runs.** The start screen counts
  pasted, written and saved situations, estimates spend from the agent's
  historical average when one exists, and shows the maximum exposure enforced
  by the per-run money ceiling. If no money ceiling exists, it says the
  exposure is not capped instead of letting a dry tool layer read as a free
  run.

### Fixed

- **Channel configuration failures no longer count as one affected
  conversation.** When the reporter cannot read the channel map, it does not
  know which conversations would have received the message. The runtime page
  now marks that as scope-wide instead of inventing a single conversation.

## [0.27.0] — 2026-08-23

### Added

- **The cost page now shows planning spend by model and by agent.** It reads
  the forward-only projection introduced in 0.26.0, says when that projection
  began, and flags buckets with calls that had no configured rate so token
  volume with unknown money is not mistaken for cheap work.

### Fixed

- **A Postgres restart during a rollout no longer fails the rollout.** Both the
  API and the worker used to exit the moment the first database ping was
  refused, so a database that took half a minute to come back turned into a
  crash loop with backoff — and an upgrade that timed out waiting for pods that
  were never going to start in time. Both now retry for up to 45 seconds before
  binding, which fits inside the startup-probe budget with room for the
  migration and identity setup that follow. A database that still does not
  answer fails loudly, naming the connection error rather than the timeout.

- **Simulated runs no longer enter the planning-spend projection**, and rows
  that 0.26.0 already projected for them are removed by the migration. A
  rehearsal was being reported as production spend. The sweep still advances
  past simulations, so one sitting near the cursor cannot keep the aggregate
  from reaching later production calls.

## [0.26.0] — 2026-08-23

No new screen. This release corrects a rate that was applied to the wrong
model, and lays the projection that a cost-by-model and cost-by-agent view will
read from.

### Upgrade notes

- **The cost aggregate starts empty and fills forward.** The migration creates
  a projection of what each planning call cost and starts a sweep that runs
  every minute from that moment. It does not read the history: runs recorded
  before the upgrade stay in the ledger and never enter the aggregate. For the
  first days the totals are therefore short — and short is not cheap. Anything
  built on them has to say which period it covers, or it reports "we have not
  looked that far back" as "the installation spent little".

- **Past runs are not repriced.** The fix below applies from this version
  onward. Steps already in the chain keep the figures they were recorded with,
  because the ledger does not amend — a correction is a new step, never an edit
  to an old one. An installation that used per-step models should read its
  historical cost figures with the caveat below in mind.

### Fixed

- **A step that chooses its own model is now billed as that model.** In a
  multi-step agent a stage may name its own model — the usual reason being to
  run one stage on something cheaper. The request went to the model the step
  named, but the rate came from the model the *agent* was configured with, so
  the cost recorded was the wrong model's price for the right model's tokens.
  A cheaper stage was recorded as expensive, a more expensive one as cheap, and
  a stage whose model had no configured rate could still record money.

  The effective model is now resolved once per call and answers all three
  questions — which model to send to, which pair to record, which rate to
  apply. Agents that never override the model per step were never affected.

### Added

- **A planning step names the model it called**, alongside the provider and
  whether the rate applied was configured, missing, or deliberately zero.
  "Configured" is now a claim about the rate that was *applied*, not about one
  that happened to exist somewhere in the price list.

- **What each planning call cost is projected into its own table**, summable by
  provider and model or by agent, over a window. The sweep is idempotent per
  step, so a pass can be repeated without doubling the money, and a step it
  cannot attribute — one recorded before the pair was written — is passed over
  rather than guessed at or allowed to stall the sweep behind it.

  A rollup carries how many of its calls had no configured rate. A bucket of
  those has real tokens and zero money, and folding it into a total silently
  would report unknown as cheap.

## [0.25.0] — 2026-08-23

### Added

- **A run says where its spend came from.** The run page shows the cost, the
  four token counters, and what the prompt was made of — instructions,
  platform text, input, notes, tool schemas, tool arguments and tool results —
  attributed to the tool each came from. Opening an expensive run now answers
  "which part of this was large" without reading the trail by hand.

  Tokens and bytes sit side by side and are never combined. Tokens are what the
  provider reported and the only thing money derives from; bytes are what the
  platform measured while assembling the prompt. Dividing one by the other
  would invent a rate.

- **Zero cost says which kind of zero it is.** A run that called nothing spent
  nothing; a run that called a model with no configured rate also reads zero,
  and so does one whose rate is deliberately zero, or whose call rounded below
  a micro. The planning step now records which, so the screen reports it
  instead of guessing from the figure. Runs recorded before this say the
  provenance is unknown, which is the honest answer.

- **What compaction removed is measured where it happens**, and shown per tool.
  A prompt dominated by one server now says how much was already trimmed there,
  which is what turns "compact something next" into a choice rather than a
  guess.

- **Cache hits are counted as calls avoided, never as tokens saved.** A cached
  result still goes into the prompt whole: the cache saves a request to
  somebody else's system, not model input. The two savings are reported
  separately and there is no combined figure, because one would overstate what
  was spared and point at compacting what the cache already covers.

## [0.24.0] — 2026-08-22

### Upgrade notes

- **New runs now finish only through the platform finish action.** A model that
  returns text without calling a tool or the finish action parks the run for a
  person to inspect instead of ending silently. Older runs that finished by
  returning text keep their recorded reason and continue to render correctly.
  Agents that used to finish by returning plain text may start parking after
  this upgrade; watch the human queue and any configured channel alerts for
  `no_finish_action` during the first runs after rollout.
- **Workers expose Prometheus metrics by default on `/metrics` inside the
  cluster.** The chart creates a worker-only ClusterIP service when
  `worker.metrics.enabled` is true. The metrics carry low-cardinality pool
  movement only; they do not include run ids, agent ids, tools, user text or
  provider diagnostics.

### Added

- The model is now always offered a platform-owned finish action. Calling it
  records `finish_tool` in the run trail, stores the closing answer in the
  content store as before, and keeps the reason for ending explicit instead of
  inferred from the absence of a tool call.
- Workers now export Prometheus text metrics for configured slots, claim
  results, advance phases, stable failure codes and worker-supervisor parking
  reasons. Binding the metrics listener fails worker startup rather than
  silently running without the surface the chart advertised.

## [0.23.0] — 2026-08-22

### Added

- Large channel inputs are compacted before they are sent to the model. The
  original ask stays in the content store and trail; the model receives a JSON
  projection with long fields shortened, plus a separate platform note with
  the stored input size and digest, so a long Slack alert or thread does not
  dominate the first turn.
- Large GitHub pull-request, file, commit and log reads are compacted before
  they are replayed to the model. The full result stays in the content store
  and trail; the model receives the beginning, the end, the stored size and a
  digest, so a large PR diff does not get paid for again on every turn.

## [0.22.0] — 2026-08-22

### Added

- Each model proposal now records the prompt content composition in the trail:
  instructions, platform text, run input, notes, tool arguments and tool
  results, with tool-result bytes attributed by tool. These are measured
  content bytes, not provider tokens or money, so they identify what made a
  turn large without estimating cost.
- Large Grafana Loki and Prometheus query results are compacted before they
  are replayed to the model. The full result stays in the content store and
  trail; the model receives the beginning, the end, the stored size and a
  digest so observability dumps do not dominate every later turn.

## [0.21.0] — 2026-08-22

### Added

- Channels that receive failed-run notifications now hear about the first time
  a new Gate block shape appears in their scope. The alert is deduplicated by
  rule or policy code, tool, effect and verdict, links to the first concrete
  run, and starts from the upgrade time rather than replaying historical
  refusals.
- MCP servers can now carry a per-worker result cache for successful read-only
  tool calls. Cache hits are keyed by tool definition, arguments, scope and
  `OnBehalfOf`, write a fresh content reference into the current run, and are
  shown in the trail with the original run and step instead of pretending the
  server was called again.

### Security

- MCP servers can now carry an optional per-worker token bucket for tool calls.
  When a bucket is empty, the call does not leave the worker and the run retries
  later instead of recording a failed tool call. The limit is not distributed:
  multiple worker replicas each have their own bucket.

## [0.20.0] — 2026-08-21

### Upgrade notes

- **An HTTP tool server whose address resolves to cloud metadata or link-local
  is refused** — `169.254.0.0/16`, `fe80::/10`, `fd00:ec2::/64`. Private
  addresses stay allowed, because an installation reaching its own network is
  the normal case; those ranges are where instance credentials live and no
  tool server belongs. It is checked when the address is saved **and again at
  the moment of dialling, after DNS**, so a name that later resolves there does
  not get through.
- **A proxy in the environment is refused rather than ignored** for those
  calls. With a proxy, the name is resolved by the proxy and this worker can no
  longer prove where the connection went. `NO_PROXY` is honoured, so a server
  excluded there still works — but **an installation that reaches its tool
  servers only through a mandatory proxy cannot connect them**, and now says so
  instead of failing quietly.
- **Announcing new Gate refusals starts from this upgrade**, not from history.
  Its cursor begins now, so syncing does not post months of past refusals.

### Added

- **The first time a Gate refusal of a given shape appears in a scope, the
  channel is told.** A shape is its rule or policy code, tool, effect and
  verdict — so the first one is signal and the hundredth repeat is not. It
  carries a link to the run it happened in, and rehearsals are excluded because
  a rehearsal produces refusals by design.
- **A tool server can carry a rate limit**, in calls per second with a burst.
  Exceeding it does not send the call and does not end the run: it is retryable
  and carries how long to wait. **The limit is per worker**, so a pool of two
  admits twice what one does — the console says so where it is configured.

## [0.19.1] — 2026-08-21

### Fixed

- No customer name appears in the product. Fixtures, manual examples and test
  data use the fictional companies this repository already used for the
  purpose — `acme`, and `globex` where a second one is needed. This repository
  is public and the product is installed by whoever installs it.

## [0.19.0] — 2026-08-21

### Added

- **Six more manual pages, in both languages — twelve per language now.**

  **Your first agent, end to end** is one worked example carried the whole way:
  an agent that answers infrastructure questions in Slack. It only reads, so
  the platform can be seen working without risking anything, and then it shows
  what changes at the step where writing starts. Every other page had explained
  a piece and none had given the order.

  **Policies** was a screen in the navigation with no page at all, using
  vocabulary nobody guesses — deny beats escalate, allow is the only thing that
  loosens, monitor versus enforce — on the screen where a mistake reaches every
  step of every agent.

  **Draft, copilot and autonomous** answers what changes between the stages by
  saying what does not: the Gate evaluates the same in all three, so autonomous
  means no human wait on what was already cleared. It gives questions instead
  of a number, and says that promoting is the moment policy stops having a
  human behind it.

  **Companies, areas and roles**, **Approving an action** and **Reading a run**
  cover where things live and who reaches them, what you are deciding when a
  run stops, and where to look when something already went wrong.

  Every page ends in use cases rather than concepts.

### Fixed

- Manual headings read as headings. `h2` sat one size above its prose and
  shared its weight while `h3` was body text in bold, so the start of a section
  read as an emphasised sentence.

## [0.18.2] — 2026-08-21

### Fixed

- **A screen scrolls once.** The document was scrolling behind the content
  container, so the tallest screen in the console — creating a policy — showed
  two scrollbars and its header scrolled away. The shell now clips to the
  viewport, leaving the content container as the only thing that scrolls.
  Shorter screens never crossed the threshold, which is why this looked like
  one page's defect.

## [0.18.1] — 2026-08-21

### Fixed

- Creating or editing a policy no longer overflows. The actions moved to the
  page header, where every other screen keeps them, and the sticky footer that
  sat inside the scrolling content — pinned to a container that scrolled with
  it — is gone.

## [0.18.0] — 2026-08-21

### Added

- **Simulation is a rehearsal, and reads like one.** Situations come from what
  already happened, from something you write, or from pasted JSON, and the
  report says which situations were tried and how each went.

  A case the Gate refused counts as needing a look **even when the run went on
  to finish**, because that is the finding a rehearsal exists to surface —
  hiding it under "finished" is how a blocked action reaches publishing review
  as a green row. A case where the Gate asked for approval is not flagged: a
  human being consulted is the agent working, and marking it teaches people to
  ignore the mark.

  The screen does not promise a rehearsal is free, because it is not: tools are
  dry, and every planning call is billed by the provider, which is why the same
  ceilings apply. What it promises is that nothing is sent to or changed in an
  external system.

### Fixed

- The policy editor contains its overflow, and creating a policy is named in
  the breadcrumb instead of showing an empty crumb.

## [0.17.1] — 2026-08-20

### Fixed

- **Simulation says an agent cannot be simulated before you spend anything on
  finding out.** A paused or withdrawn agent let the buttons be clicked and
  refused afterwards; the screen now reads the agent's state and disables them
  with the reason shown. A simulation costs real provider calls, so discovering
  this at the click is discovering it after deciding to pay.
- The policy editor holds its width: a grid column that has to shrink is
  `minmax(0,1fr)` rather than `1fr`, condition rows stack on narrow screens,
  and the bottom bar wraps instead of pushing the page sideways.
- Three catalogue keys were rendering as their own names in the policy screen.

## [0.17.0] — 2026-08-20

### Added

- **The manual is something you can look things up in.** Pages declare a
  section and tags, their headings are extracted, and the console gained a
  sidebar by section, search across titles, summaries, sections, tags and
  headings, and an on-page outline. The index carries all of that without the
  page bodies, so opening the menu is still not downloading the book.
- **Four more pages, in both languages**: writing good blocks, Slack and
  channels, MCP servers and credentials, costs and limits. Six per language
  now. They answer the questions this platform's refusals actually raise — why
  a call needs a personal credential, why a run stopped at a ceiling — which
  until now were answered only in commit messages and by whoever was nearby.

## [0.16.0] — 2026-08-20

### Added

- **A run records why it finished.** Today that is `no_tool_call`: the model
  answered with text instead of proposing another tool call, and text ends a
  run. The trail says so rather than leaving somebody to conclude the run
  stopped for no reason. Runs recorded before this carry no reason and keep the
  general wording, which is the right answer for a run that never stated one.

  Finishing is still an omission rather than an act — a run ends when the model
  *does not* call a tool, so "I will continue" and "I am done" arrive
  identically. This release makes that visible; it does not make it impossible.

### Fixed

- **A configured model rate takes effect without restarting the worker.**
  Prices were read once at start-up and the planner was cached per agent
  version, so an installation that configured a rate kept billing at the old
  one — usually zero — until something restarted. Two layers held the stale
  value, and fixing either alone would have looked like the fix had failed.
  A failed refresh keeps the last good rate rather than falling back to zero.
  **Runs already recorded are not repriced**; what was written as zero stays
  zero.
- Line tabs no longer overflow their container.

## [0.15.1] — 2026-08-20

### Fixed

- **A model price accepts decimals again.** The form converted what was typed
  into micros on every keystroke and rendered the result back, so an
  intermediate state that is valid typing and invalid as a number — `0`, on the
  way to `0.5` — was reformatted to empty and the next character landed alone:
  `0.5` became `5`, ten times the intended rate, in the field that decides what
  every run is billed. The field now holds text while it is being edited and
  converts once, on save.

## [0.15.0] — 2026-08-20

### Added

- **The installation declares its currency.** Cost and ceilings are integers in
  millionths of it, and which currency that was lived in a comment — obeyed
  until somebody brought a number in good faith from somewhere else, which is
  how published prices in dollars reached a ceiling written in reais. It is now
  an ISO code, validated, changed with `budget:write` and recorded as an
  administrative event. Existing installations keep `BRL`.

  **Changing it converts nothing.** Converting stored amounts would rewrite an
  audit record with a rate nobody reviewed, at the moment somebody changed a
  dropdown. So changing the currency changes how historical costs, ceilings and
  configured rates are *read* — the numbers stay and what they claim to be
  changes. **Review your ceilings and configured rates before changing it.** The
  screen says so permanently rather than in a dialog, because whoever opens it
  months later to check a rate needs it as much as whoever changes it today.

### Fixed

- The agent overview's tabs read as an underline rather than as buttons that
  lost their fill, fixed in the primitive where the defect was.

## [0.14.0] — 2026-08-20

### Upgrade notes

- **A model with no rate configured for this installation reports zero cost
  again.** Published market prices are shown as a reference and no longer act
  as rates. They are in USD, and cost is millionths of the installation's own
  currency — the same value a budget ceiling is compared against — so a ceiling
  written as R$300 would have been compared against dollars and only fired once
  real spending was several times past what whoever wrote it intended.
  **Configure the rate for each model you use, in your own currency**, and
  until you do, cost reads zero rather than plausible-but-wrong.

### Added

- A refusal by budget ceiling now carries the ceiling, what has been spent
  against it, and the estimate for the call that crossed it, so deciding
  whether to raise the ceiling does not mean reading the ledger by hand.
- The prices screen distinguishes a **market default** from a **rate this
  installation set**, showing the default's source, currency, URL and the date
  it was read. Overriding one opens empty fields rather than the published
  figure, so nobody confirms an amount they did not type. Defaults can be
  reloaded from the release.

### Fixed

- The agent overview's header and tabs hold at the widths people actually use.

## [0.13.0] — 2026-08-20

### Upgrade notes

- **Migration `0049` adds `answer_due` to the channel inbox, defaulting to
  `false`.** That default is the behaviour, not a detail: every ask recorded
  before the upgrade counts as already settled, so upgrading does not pour
  answers for finished runs into Slack threads that went cold — at whatever
  hour the rollout happens, to people who moved on. Only asks opened afterwards
  become answer debts.

### Added

- **A run that started from a Slack ask now answers in that thread.** Before,
  the answer stayed in the trail and Slack got refusals and a generic
  `finished` card in the root channel, so the person who asked had to go looking
  for the reply to their own question. An answer erased by retention or a
  subject request is reported as erased rather than posted blank.

  The `finished` announcement is separate and still fires if the conversation
  asks for it — a conversation that now gets the real answer in-thread probably
  wants `Finished` unticked in **What to announce**.
- **kagent** in the tool catalogue: a customer-operated control plane over HTTP
  MCP. It suggests no classifications, because its tools are the agents
  configured in that cluster — one may read Kubernetes state and the next may
  deploy or delete, so the handshake and the Curator's review are the first
  trustworthy description of what a specific endpoint can do.

### Fixed

- **The agent page opens on Runs**, with the definition and steps behind tabs
  and a compact rail. The tool list truncates with a `+n` that expands, because
  "which tools does this agent reach" is asked before taking an agent out of
  draft and cannot be answered with "six of them".
- **The trail renders what it shows.** Payloads are coloured from the parsed
  value and fall back to the raw text when they are not JSON; a run's closing
  answer renders as the report the model wrote. In that answer a link is shown
  with its destination but is not clickable, and an image is not fetched: the
  answer restates what the agent read from outside, and the trail should not
  offer an auditor a destination a third party chose.

## [0.12.0] — 2026-08-20

### Added

- **The editor warns when a step still does what the instructions forbid.** An
  agent told never to use a tool would use it anyway and read as ignoring its
  author: a step's `stopsWhen` and the tools in its reach are separate sources,
  and prose changes neither. The warning names the step and the tool, on both
  tabs, before publishing.

  It warns rather than refuses. "Never" is prose, and turning prose into policy
  would have the platform infer intent from text. What removes capability is
  taking the tool out of reach. The match is loose on purpose — a tool counts
  as named by its full name or the part after the dot — so it can be silent on
  a real conflict, and no warning is not a clean bill of health.

### Fixed

- **A tool call no longer dies with `client is closing`.** The HTTP transport
  opened the optional server-initiated SSE stream, an idle connection proxies
  close, after which the SDK closed the whole client and unrelated calls
  failed.

  It stays closed as a boundary rather than as tuning. That stream carries
  requests a tool server makes back into the worker — sampling among them,
  which is a model call arriving outside the ceiling and the cost record every
  other model call goes through. Reopening it needs a policy for notifications
  and for any server request that spends, not just code that can receive one.
- **A rediscovered tool server no longer refuses classified tools.** Its
  catalogue was briefly unclassified, so a tool somebody had ruled on was
  refused until the next reconcile. Rulings are reapplied as soon as discovery
  succeeds, through the same path as before — the digest still has to match, so
  a ruling is reconfirmed against the schema that exists rather than restored.
- The credentials panel says that a shared credential covers discovery while a
  tool call needs the personal credential of the principal the run acts for.

## [0.11.0] — 2026-08-20

### Added

- **`protocolMode` on an HTTP tool server.** A server that does not accept the
  current discovery probe can be connected in `legacy` mode, and the recipe for
  a known server supplies that on its own — Outline is marked as such, so an
  installation that already configured it does not have to reconnect. Only the
  discovery probe is answered locally; listing and calling tools go to the
  server unchanged, and classification and the tool surface are untouched.

### Changed

- Properties are edited in a side sheet rather than a modal: a channel
  connection, a conversation, a tool server, a provider, a company, an area, a
  ceiling, a price, a person. Configuring something while still seeing what it
  belongs to is the point. Acts stay modal — starting, stopping, classifying —
  and **erasing content is an `AlertDialog`**, which does not close on a stray
  click or key, because it is the one administrative operation nobody can undo.

### Fixed

- A tool failure keeps its diagnostics.

## [0.10.0] — 2026-08-20

### Added

- **A conversation can pass the thread as context.** When somebody mentions an
  agent inside a Slack thread, the earlier messages can be read and handed to
  the run — bounded to twenty messages and 32KB, with the mention itself
  excluded. A missing Slack history scope becomes `thread.unavailable` in the
  input rather than a retry that cannot succeed, so the agent knows the context
  was unavailable instead of assuming the thread was empty.
- **A channel can do both at once**: answer mentions and watch permitted
  sources in the same place.

### Changed

- Thread context is off unless somebody turns it on, and the console says what
  turning it on means: a mention is a person consenting about their own
  message, while reading the thread includes messages other people wrote, and
  those reach the configured model provider. DP-001 records the same.

## [0.9.0] — 2026-08-19

### Upgrade notes

- **Publishing an agent now requires the company as well as the area.** The
  scope used to be resolved by matching the area name against the caller's
  grants and taking the company from whichever matched first — and area
  identifiers are scoped by a company, so an author editing an agent shown as
  `acme/platform` could publish it into `default/platform`. The authority used
  was not the authority the screen displayed. A request that names no company
  is now refused rather than inferred, so **an automation that publishes
  through the API without a company will start receiving 403** and needs the
  field added. **An agent published before this may carry the wrong scope and
  is worth republishing.**
- **Saving a watched conversation refuses an agent that cannot be started by a
  message.** A conversation configured against an agent whose published version
  does not declare the `Conversation` trigger was accepted and then refused at
  runtime, in Slack, in front of whoever posted. Existing conversations are not
  rewritten; the next save of one validates it.

### Fixed

- The agent picker for a watched conversation offers only agents a message can
  start, instead of every agent in scope.

## [0.8.2] — 2026-08-19

### Fixed

- **A channel with no conversations reads as one waiting to be configured**,
  not as a failed search. The tab counted a connected channel while the list
  under it said no conversation matched the filter — both true, and together
  they told somebody who had just connected Slack to look for a mistake they
  had not made. Searching still filters: look for a conversation that does not
  exist and the channel goes away, because there the empty result is the
  answer.

## [0.8.1] — 2026-08-19

### Fixed

- **A watched conversation can be corrected.** It could be created and not
  changed, so fixing a `runAs` or a permitted source meant creating a second
  conversation for the same place. Editing opens the same form with the
  identifier fixed, because changing that would create another conversation
  rather than correct this one.
- Channels refreshes every ten seconds, so **Seen recently** answers whether
  Slack is delivering at all — which is the question somebody wiring up Socket
  Mode is actually asking, and a column that only moved on a manual reload
  could not answer it. The refresh pauses when the tab is not in focus.
- A channel's conversations render as a compact scrollable table rather than a
  list that grew until the screen stopped being usable.

## [0.8.0] — 2026-08-19

### Upgrade notes

- **Watched Slack conversations must name who the automated run acts for.**
  A watched message is not a person asking in the moment: it is an event that
  matched a configured source. The conversation therefore needs a `runAs`
  principal configured ahead of time. API writes that choose `mode: watch`
  without `runAs` are refused.
- **Delegating watched messages to another principal now requires identity
  administration at the installation.** `runAs` becomes the run's
  `OnBehalfOf`, and personal MCP credentials are sealed by that principal. A
  channel configurator may run watched messages as themselves, but naming
  somebody else requires `identity:write` on the installation scope. The
  chosen principal must exist, be active and hold a grant in the conversation's
  scope.

### Added

- **Slack Socket Mode can receive asks without a public FuseOne callback URL.**
  A worker opens Slack's WebSocket with an app-level token and writes events to
  the same durable inbox as the HTTP callback path before acknowledging them.
  The message text is still marked untrusted, and approval buttons continue to
  require the HTTP interaction path.
- **Conversations can watch selected Slack sources.** A watched conversation
  starts one configured agent only when a Slack user, bot or app id matches the
  configured source list. The Slack text never chooses the agent, and the
  source never grants authority; authority comes from the configured `runAs`
  principal.
- **Recently seen Slack accounts appear while linking people.** Signed Slack
  mentions and interactions record account ids as binding hints. Clicking one
  fills the form, but grants nothing until an administrator chooses a platform
  person and saves.

### Fixed

- Creating an agent now marks every required field and explains what is still
  missing before publishing. The publish button, the footer copy and the field
  markers all read the same requirement list.

## [0.7.0] — 2026-08-19

### Added

- **The interview starts free.** Somebody describes the work in their own
  words, and the console turns that into the seven parts the product's contract
  names (FU-01..FU-07), shown for review before anything is generated. The
  parts are a constant in the console, not something the model chooses, so two
  people describing the same work are asked for the same things and the
  interview costs no extra call.
- **"Suggest answers"** fills those seven fields from the free description. It
  goes through the same authoring assistant, the same daily ceiling and the
  same cost record as the interview itself — one definition of what has been
  spent today rather than a second path that also checks. It fills only the
  seven fields: it does not draft the agent, choose the questions, choose tools,
  or overwrite anything already edited.

### Fixed

- Creating an agent scrolls in one column instead of overflowing. The template
  gallery adapts how many cards it shows to the width it has rather than
  forcing four into whatever is there.

## [0.6.1] — 2026-08-19

### Fixed

- **An installation-wide administrator can pick a company again.** The scope
  switcher built its list from the caller's own grants, so the one grant an
  installation administrator holds — company `*`, no area — appeared as a
  company literally called `*`. Choosing it sent `*` as a filter and the
  screens came back empty, which reads as an installation with no runs rather
  than as a filter that cannot match.

  The same assumption was underneath, in `ListScopes`: an installation grant
  was matched as a company name, so it found the areas of a company that does
  not exist. Fixing only the switcher would have removed the visible symptom
  and left an administrator with no areas at all. Both halves require company
  `*` **and** an empty area, which is what installation-wide has meant since
  0.6.0 — `*` with an area filled in is an ordinary scope and always was.

  A scope already saved in a browser as `*` is normalised to "everything"
  rather than left pointing at a filter that no longer exists.
- Companies and Areas read like People: a toolbar, a table header, expandable
  rows and per-row actions. The Areas form also built its company list from
  grants, so an installation administrator was offered `*` there too instead
  of the companies that exist.

## [0.6.0] — 2026-08-19

### Upgrade notes

- **Administering identity now requires a grant at the installation, not at
  `default`/`platform`.** Creating people, setting grants, setting a local
  password and listing people were all checked against that one company and
  area — an ordinary, grantable pair. Anyone administering it could mint
  installation-wide administrators, which is the escalation the company
  permission was already protected against and this one was not. **If the
  person who administers your installation holds their grant on
  `default`/`platform`, they will not be able to manage people after this
  upgrade.** Give them a grant with company `*` and an empty area first, or
  recover with `agentd bootstrap --reopen "<reason>"`, which is audited.
- **Migration `0046` allows the new role in the database** and canonicalises
  any malformed wildcard row before adding the constraint. A row with company
  `*` and an area filled in was never installation-wide and no longer reads as
  though it might be.

### Added

- **An `administrator` role**, carrying every permission, granted at the
  installation with company `*` and an empty area. Assembling an administrator
  out of four scoped roles worked and was a poor thing to operate. The console
  grants it with a button rather than asking anybody to type `*`, and an
  identity provider can map a group to it.
- Administration is grouped by job in the navigation, and People reads as a
  table: access summarised per scope, the matrix on an expanded row, and where
  each grant came from. **A grant an identity provider asserts is re-derived on
  every sign-in**, so it is marked as such — revoking it here lasts until its
  holder signs in again, and the group is the thing to change.

### Changed

- `/me` reports a permission only where the caller actually holds it at the
  scope the server checks, so the console stops offering screens the backend
  will refuse.

### Fixed

- Existing grants are left alone. Compacting four roles into `administrator`
  would widen what somebody reaches inside a migration nobody reviews.

## [0.5.0] — 2026-08-19

### Upgrade notes

- **A tool server whose credentials are personal by nature will not act on the
  installation's credential.** Where every credential shape a catalogue recipe
  documents carries a user's authority — Google, GitHub, Slack and the like —
  a real tool call now requires the person the run is on behalf of to have
  connected their own. A scheduled run has nobody behind it, so it stops rather
  than acting as a different identity than the same agent would use when a
  person triggered it. Discovery, probing and health checks still use the
  installation's credential: asking a server what it offers is not acting for
  anyone.
- **Abandoning a run now requires `run:cancel` rather than `run:trigger`.** The
  permission existed and enforced nothing; a role built to start runs but not
  end them did not actually withhold anything. Any role that could abandon a
  run before can still abandon it — author and curator hold both — but a role
  assembled by hand from individual permissions may need `run:cancel` added.

### Added

- **Personal MCP credentials.** A person connects their own credential to a
  configured HTTP tool server, and a run uses it for the person it acts on
  behalf of. The credential is sealed against the pair — server and principal
  together — never returned by any listing, and chosen at the moment of the
  call rather than when the server was configured. Local (stdio) servers are
  deliberately excluded: one shared process in the worker has no per-user
  request to carry a credential on.

### Changed

- A model failure that will pass answers `503` with `fuseone:upstream-busy`
  instead of `400`. Saturation and rate limiting are "try again shortly", and
  they used to read as a rejected form — in the status, and in the sentence the
  console showed.

### Fixed

- The console no longer offers controls the caller cannot use: `Administration`
  is shown for the permissions its screens actually require rather than for
  `tool:read`, its tabs are filtered individually, and a stop offers only the
  scopes the person reaches. The server always refused these; the screen was
  promising what the backend would decline.
- A failed session lookup no longer reads as an installation with no identity.
  `401` and `404` mean nobody, and anything else is now an error — a transient
  `503` used to be cached as "open mode" for a minute, removing every
  permission filter in the console.
- Agent cards, run trails, the decision feed and integration toasts go through
  i18n. A card also says `unclassified` rather than `read` for a tool nobody has
  classified, which is what the Gate already believed.

## [0.4.1] — 2026-08-18

### Fixed

- **The interview no longer logs transient model-provider failures as bad
  requests.** Overload, rate limiting and network failures now answer `503`
  with `fuseone:upstream-busy`, so the console says to try again rather than
  saying the provider refused the request. Non-retryable provider refusals,
  such as an invalid key or model, still answer `400` with
  `fuseone:upstream-refused`.

- **Signing in no longer depends on which `serve` replica answers.** The OIDC
  registry is per process, so a provider configured through one replica was
  invisible to the other: sign-in succeeded or failed depending on where the
  request landed, with `no such identity provider` one time and `none of your
  groups map to a role` the next. Every sign-in start and callback now
  reconciles against the durable configuration before deciding anything.

  It reconciles **by revision**, so an unchanged provider costs one read and
  nothing else — no vault open, no rediscovery of the issuer. Doing that work
  per request would have put an outbound call to the customer's identity
  provider behind an endpoint that takes no credential to reach.

  A transient failure of the database, the vault or discovery keeps the live
  provider rather than evicting it, because a login path that goes down with
  the database is a worse trade than the staleness it was fixing. A provider
  disabled or deleted is still evicted — that half is what the reconciliation
  is for.

## [0.4.0] — 2026-08-18

### Upgrade notes

- **A provider failure that cannot succeed now parks the run on the first
  turn.** Model failures are classified, and one classified as not retryable —
  a rejected key, a malformed request, a refusal — stops immediately instead of
  spending five attempts with exponential backoff first. Runs will be seen
  parking sooner than before, and that is the fix rather than a regression: the
  attempts it used to spend were spent on a call the classification already
  knew would fail again. Anything known to be transient — network, 429, 529,
  5xx — still retries as before.
- **An unclassified provider failure also parks rather than retrying.** The
  conservative side during an incident is to stop consuming the queue rather
  than to keep a worker slot busy on something nobody could name.
- **Migration `0045` adds five columns and an index to `runs`.** It runs before
  the new pods, as every migration does. On an installation with a large runs
  table, `add column` is metadata-only in modern PostgreSQL, but the partial
  index is built at that moment.

### Added

- A **Runtime** screen: the worker queue, what each phase is holding, and the
  provider failures of the last day, so provider saturation is visible without
  opening a pod log.
- Model failures carry a stable code — `model_provider_overloaded`,
  `model_rate_limited`, `model_auth_failed`, and others — with the provider,
  status, retryability and request id beside it. The code appears while the run
  is still backing off, not only once it parks.
- The run screen says plainly when a run stopped because of a provider rather
  than because of anything the agent or its policy did.

### Changed

- `runs.last_error` stores the stable code for a classified provider failure
  instead of the provider's raw response body. The body could carry text
  derived from what the agent read, and `runs` is a projection that neither
  retention nor erasure reaches. The provider's message stays in the worker log
  for diagnosis. DP-001 names the exception that remains: an unclassified local
  or integration failure can still put free text there.

## [0.3.4] — 2026-08-18

### Security

- **Each authenticated MCP server gets its own HTTP transport.** Bearer, header
  and OAuth transports all wrapped the process-wide `http.DefaultTransport`, so
  every configured tool server shared one connection pool. Sharing a pool
  across servers holding different credentials means a connection opened for
  one can be handed to a request for another whenever they resolve to the same
  host — the credential still travels on the right request, but the isolation a
  reviewer would assume between two tool servers was not there. Each now clones
  its own.

### Fixed

- The overview's status labels go through i18n. They were English literals on a
  screen the rest of which was translated, which is the failure the string
  check exists to catch and did not, because the check itself allowed them.

## [0.3.3] — 2026-08-18

### Upgrade notes

- **New runs keep the model's final answer in the content store.** Retention
  and erasure can now reach the closing answer the same way they reach run
  input, tool arguments and tool results. Runs recorded before this release may
  still carry an inline final answer in the immutable chain; those historical
  bytes are not rewritten.

### Added

- The console serves the manual from the same `docs/manual` files reviewed in
  the repository.
- A data protection note describes what the platform stores, what can be
  erased, and which provider endpoints a default installation may call.

### Changed

- Simulation reports no longer put a `fuseone:` sentinel inside `outcome` when
  a stored answer is gone. `outcome` is now only model text; `outcomeState`
  carries `unavailable` when the answer was erased or otherwise unreadable.

### Fixed

- The run trail can open a finished run's stored final answer.
- Long tool names, arguments, modals and instruction text stop stretching the
  console layout beyond the viewport.
- Demo seeding writes the content its stub tools return references to, so the
  seeded runs do not look erased.
- The published Docker image includes the embedded manual package, so the image
  build passes after adding `/manual`.

## [0.3.2] — 2026-08-18

### Fixed

- **An agents directory mounted at an absolute path is readable.** The chart
  has always offered `worker.specs.configMap`, mounted it at `/agents` and
  passed that path to the worker — which refused to start, because `io/fs`
  treats a leading slash as invalid rather than absolute. Setting that value
  took the worker down; it now works, which is the first time it ever has.

## [0.3.1] — 2026-08-18

### Upgrade notes

- **Upgrade to this from 0.2.0 or 0.3.0.** Both carry a chart defect that makes
  the worker Deployment invalid unless `worker.specs.configMap` is set: the
  container mounted a `/tmp` volume the chart did not render. An installation
  that never set that value could not deploy either version at all. Nothing to
  do beyond upgrading.
- **`--reuse-values` works again.** Upgrading with it across a version that
  added a chart value failed to render, which is the ordinary path for an
  installation somebody runs rather than an edge case.

### Added

- Branding: an installation carries its own name, logo, compact mark and accent
  colour, on the sidebar and on the sign-in screen. An external image URL says
  what it costs — it does not load without a route to the internet, and it
  tells that host whenever the sign-in screen opens.

### Fixed

- The audit trail shows one page at a time instead of appending every page
  fetched so far.

### Changed

- `make check` renders the chart and refuses a volumeMount that resolves to no
  volume — the defect above passed schema validation, which checks shape rather
  than whether a mount can be satisfied.

## [0.3.0] — 2026-08-18

### Upgrade notes

- **The connection form may ask for different fields than before.** It now
  follows what the recipe declares, so a server documented as taking two
  headers stops offering a bearer field. A credential already stored in the
  older shape is still shown, with its own button to remove it — nothing is
  hidden and nothing is sent that was not configured. No action is needed
  unless you want to move a credential to the shape the server documents.

### Added

- **Try now** on a tool server: the worker reconnects it on request instead of
  waiting for the next sweep, and says what it found.
- Remote servers can authenticate with a single custom header or with Basic
  credentials rendered as one.
- Recipes may name several required HTTP headers, and the console asks for
  each one instead of pretending they are one bearer token. Local DSN recipes
  may name the environment variable that receives the sealed connection string.

## [0.2.0] — 2026-08-17

### Upgrade notes

- **A writable `/tmp` is mounted.** The chart already ran with a read-only root
  filesystem, and a local tool server's sealed configuration file is
  materialised under the system temp directory — so that feature could not
  work in any installation until now. Nothing to do before upgrading; this
  fixes a shape that was already broken.
- **Replicas are spread across nodes where the cluster allows it.** Scheduling
  never refuses over it — `ScheduleAnyway` — so nothing goes Pending, but pods
  may land on different nodes than before.

### Added

- The connection form asks for the credential the server actually takes. A
  recipe that authenticates by OAuth no longer offers a bearer field, and a
  shape the runtime cannot yet send — custom headers, basic auth, a connection
  string — is shown as documented rather than dressed up as a token.
- The chart declares a pod and container security context, so a cluster
  enforcing Pod Security Standards `restricted` will run it. The root
  filesystem is read-only, with a writable `/tmp` for the config file a local
  tool server reads.
- Replicas spread across nodes where the cluster allows it, and schedule anyway
  where it does not.
- Optional Gateway API routing as an alternative to Ingress, and an optional
  egress NetworkPolicy, both off by default.
- Six more servers in the catalogue, all incident response and observability:
  PagerDuty, incident.io, New Relic, Honeycomb, ServiceNow and Elastic Agent
  Builder. Forty-one in total.
- Six more remote-first servers in the catalogue, focused on Cloudflare and
  Vercel: Cloudflare API, Documentation, Observability, Audit Logs and Workers
  Builds, plus Vercel. Forty-seven in total.

## [0.1.0] — 2026-08-17

The first tagged release. Installations before it ran untagged builds pinned by
commit, so the notes below apply to them: this is an upgrade, not a first
install, for anyone already running one.

### Upgrade notes

- **Local (stdio) tool servers stop until somebody accepts them.** A local
  server is a program the platform starts inside the worker, running as the
  worker and reaching whatever it reaches. That is now an explicit decision,
  recorded with the name of whoever made it, and a stored server carries no
  such acceptance. Re-save each local server from the console to continue.
  Remote servers are unaffected.
- **A local server no longer inherits the worker's environment.** It receives
  only what it is configured with, so one that relied on a variable the worker
  happened to hold needs that variable set on the server itself.
- **Tools whose definition changed are refused until re-ruled.** A ruling now
  names the definition it judged. Rulings made before this release carry no
  such record and keep working unchanged; a tool a server has since redefined
  reads as needing review.

### Added

- A catalogue of thirty-five tool servers the platform knows about — identity,
  documentation, how each is usually reached, the credential it expects, and
  suggested classifications where the publisher documents its tool names. A
  recipe fills a form and decides nothing.
- Per-server tool surface: of what a server offers, choose what this
  installation brings in. Outside it a tool is not restricted — it is not
  there, and no agent is told about it.
- OAuth for remote tool servers, including refreshing an expired access token
  and persisting a rotated refresh token before it is used.
- A sealed configuration file for local servers, materialised when the process
  starts and removed when it stops.
- Events between agents may carry a context key and artefact references.
- Slack conversations can start runs: a mention becomes an ask, and a refusal
  is said back in the thread it was asked in.

### Changed

- Tool classification happens on the server's own page, beside the tools it
  offers. The administration area keeps the queue of what is waiting.
- Long inventories are paged.

### Security

- A run started by an event inherits the labels and the delegating person of
  the run that emitted it. Composition no longer launders taint or acquires
  authority nobody granted.
- A correspondent may open a bounded number of runs in a rolling window, taken
  before the run opens rather than counted after.
