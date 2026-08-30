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

## [Unreleased]

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
