# Changelog

What changed between versions, written for the person who has to decide whether
to upgrade an installation that is currently working.

Versions are [semantic](https://semver.org). A release is a `v`-prefixed tag,
and pushing one builds the image and chart that carry that version — nothing is
released by merging.

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
