# NT-002 · What is left, and in what order

**Status** Proposal · **Date** 2026-08-11 · **Revised** 2026-08-11, after the
handoff added `Agente.dc.html` and `Politica.dc.html`
**References** [PRD-001](PRD-001-fuseone-agents.md) · design handoff (`design_handoff_fuseone_console`)
**Outcome** An order for the four remaining fronts, and the decisions each needs first

The console can now observe everything the platform does and start a run three
ways. What is left is the half that lets somebody *change* what the platform
does: policies, agent authoring, the diagram, and integrations.

This note says what exists for each, what the handoff and the PRD ask for,
what is missing, and which decision has to be made before the work can start.
Sizes are given against work already delivered, because that is the only
estimate anybody here can check.

---

## Where the product actually is

| Area | State |
|---|---|
| Ledger, Gate, engine, worker | Delivered |
| Triggers — manual, cron, webhook, event | Delivered |
| Overview, Runs, Run trail, Human queue, Agents, Agent detail, Cost, Audit trail | Delivered |
| Administration — tools, MCP servers, model providers, budgets | Delivered |
| **Policies** | Enforced in code, not authorable, not visible. Next |
| **Agent authoring** | Only by dropping a file in a directory |
| Generated diagram | Delivered — the run drawn from the ledger, beside the list |
| Integrations | Delivered, and now a screen of its own |
| i18n | Deferred to last, by decision |

---

## Budget alerts have no outbound channel

FO-05 asks for progressive alerts per area at 50%, 80% and 100% of the monthly
budget. The platform notices the crossings, records them once per threshold per
period, writes them to the administrative trail and shows them on the cost
screen. What it does not do is *send* anything: this installation has no mail,
no chat, no outbound webhook.

That is the honest half. A person who opens the console sees it; a person who
does not, does not. Wiring a channel is a small piece on top of what exists —
`budget.Watcher.Sweep` returns exactly the crossings it announced — and it
needs a decision about where an installation wants to be told, which is a
question for a customer rather than for this repository.

## Stops: "per pack" became "per scope"

FO-06 asks for a global switch per agent, **per pack** and per installation.
This implementation has no named packs — an author lists tools directly in the
specification, and `gate.Pack` is derived from that list rather than referring
to a curated object somebody registered. There is a `pack:write` permission and
no pack to write.

The middle level is therefore the scope. It is the grouping the rest of the
platform is built on, it is what an operator says out loud during an incident
("stop everything in billing"), and it composes with the hierarchy: stopping a
company stops its areas.

If named packs arrive, a pack-level stop is a fourth `StopLevel` and a fourth
case in `Stop.Covers`. Nothing else has to move.

The console offers the two wide levels; one agent is the pause that already
exists on the agent's own screen. A scope stop can also be thrown through the
API by anything that holds `run:read` in that scope.


## 1. Policies

### What exists

The Gate's fourth check is an effect ladder written in Go:

```
read → allow · write → require approval · destructive, financial → block
```

Its own comment says it is the safe default rather than the design. It works,
it is versioned as `builtin/v1`, and every step carries that hash.

### What is missing

The handoff's screen (§5) lists a policy table: code, name, scope, effect
pill, owner, hits, and an enforcement toggle. None of that has anywhere to
live — there is one function, not a set of rows.

Three consequences, in increasing order of seriousness:

1. **Hits per policy are uncountable.** Every policy decision records
   `rule: "policy"`, never *which* policy. The ledger cannot tell two apart.
2. **Nothing can be turned off.** The toggle implies an act of governance:
   an administrative event, and a new policy version.
3. **The policy hash points at nothing retrievable.** The PRD promises
   reconstructing any decision *under the policy in force at the time*, and
   gives the Auditor the job of re-evaluating old decisions against a new
   policy. With policy in code, `builtin/v1` cannot be fetched after a deploy.
   The seal names something the platform does not keep.

The third is why this is not a screen. It is a promise in the PRD that the
current design cannot keep.

### The decision, now made

The handoff's policy screen settles it: **a decision table**. Rows of
`quando / e` over field, operator and value, a resource glob, action chips, and
a reach — org, teams or named agents. Not an expression language.

It adds two things this note did not propose, both of which change the build:

- **Monitor before enforce.** A `Monitorar` / `Impor` control, offered as a
  real button on creation (`Criar em modo monitorar`) rather than a checkbox.
  A policy in monitor mode is evaluated and recorded and changes no verdict.
  That has to exist in the Gate, not only on the screen: it means a rule can
  produce a decision the run does not obey, which the trail must be able to
  express without an auditor reading it as a bug.
- **Simulation against the last 500 runs**, with the count of would-be
  denials, before saving. "A policy is never saved blind." This needs replay:
  evaluating a draft rule against recorded decisions — which is the same
  machinery the PRD's Auditor needs to re-evaluate old decisions under a new
  policy, and the reason the policy hash has to name something retrievable.

One more requirement falls out of the screen: **the compiled restatement**.
The builder is never the only representation; the operator always reads the
sentence the engine will evaluate. So the rule model needs a rendering to
prose that is generated from the same structure the Gate evaluates, or the two
will drift and the screen will lie.

### Size

Largest of the four. Comparable to triggers end to end, probably more:
a rule model, versioned storage, evaluation inside the Gate without loosening
the seven-check order, inheritance down the scope hierarchy (the PRD requires
policy to inherit downwards and never widen), then the screen.

---

## 2. Agent authoring

### What exists

An agent is a Markdown file with front matter, loaded from a directory by the
worker and published as an immutable version. There is no way to create or
change one from the console.

### What the handoff has

`Agente.dc.html`, added after this note was first written. One screen, two
modes. Four form sections — identity, model and instructions, tools,
governance — a sticky action bar, and a side rail that differs by mode: a
pre-flight checklist when creating, an unsaved-diff card when editing.

Two of its decisions resolve the tension below, and resolve it well:

- **A new agent is created paused.** Authoring never starts something.
- **The primary button names the version**: `Salvar e publicar v1.5.0`, never
  `Salvar`. Editing is authoring the next version, and the button says so
  rather than leaving somebody to discover it.

### What the PRD asks for

Something much larger than a form. The Studio (FU-01 – FU-06) is an
**interview**: when does this start, what do you need to know, what are the
steps, what usually goes wrong, what would you not decide alone, how do you
know you are done. The author approves a **narrative read-back** — "never
YAML, JSON or a graph" — and corrections are anchored to a step and kept as
regression cases (FU-12, FU-13).

### The tension to resolve

Publishing creates a version and never edits one, and that is load-bearing:
runs are pinned to versions, and older versions are the only correct
explanation of the runs that used them. So "editing an agent" can only mean
*authoring the next version*, with the current one as the starting point.
The console has to make that obvious or people will expect a save button and
get a new version they did not ask for.

### The decision, now made

The handoff picks the form, not the interview. That is a smaller scope than
the PRD's Studio (FU-01 – FU-06) and a coherent one: the interview can produce
the same fields later and land on this screen for review.

What the screen adds beyond today's specification:

- **A per-tool approval rule** — `Nunca` / `Dado não confiável` / `Sempre` —
  which is policy attached to a tool inside an agent, a thing the model does
  not have.
- **Attached policies**, with inherited ones locked and marked
  `herdada da organização`.
- **A tool blocked by policy** rendered visible but unavailable, with the
  policy code in its subtitle.

All three reference policies. Which reorders this note.

### Size

Comparable to the administration forms already built, plus the diff view and
the pre-flight checklist. Small next to policies — and it depends on them.

---

## 3. The generated diagram — delivered

> Built as decided below: the run drawn from the ledger, as a second view
> of the trail rather than a separate screen. Eight node kinds, no branch.
> Laid out as a serpentine rather than by elkjs — a run is a chain, and a
> computed layout is more deterministic than a solver whose output can shift
> between versions. When the ledger gains a branch, elk is the answer.

### The conflict

The handoff's §7 is a **visual builder**: a left rail of draggable components,
an "Add step" dropdown, Draft/Staging/Production environments. The PRD makes a
drag-and-drop builder a non-goal (N5) and asks instead for a **read-only,
generated** diagram with deterministic layout (FU-17), whose render model is
never persisted (FU-18).

### The harder problem underneath

**There is no graph.** An agent's specification has instructions, a tool pack,
ceilings and triggers. `PlannedPayload` carries a `Node` field that is always
empty. The loop belongs to the interpreter, not to a structure anybody
authored — so FU-17's "the agent's graph" presupposes something the
specification does not have.

Two things could be drawn today:

- **The run's actual path**, derived from the ledger. Real data, immediately
  useful, and it is a picture of one execution rather than of the agent.
- **The agent's declared topology** — trigger → model → available tools →
  Gate → effects. Derivable from the specification, and closest to FU-17.

The handoff's visual language survives either choice: node shapes, the
serpentine grid, edge rules, condition pills.

### The decision, answered by the handoff's own data

The prototype settles it, and not in the direction its chrome suggests.

`FLOW_NODES` in `ui_kits/console/Console.jsx` is nine nodes, and every one of
them carries a **latency** and a **health**:

```
Webhook received      12ms   Policy gate POL-114    8ms
Analyze profile       1.4s   CRM · read account   180ms
Human approval      4h SLA   Apply new limit      220ms
Seal audit trail      14ms
```

A specification does not have 12ms. That is an execution — the design draws a
run and wraps it in an authoring shell. Which means the diagram is buildable
today, from the ledger, with no change to the specification at all.

Eight node kinds, seven of which are step kinds we already record:

| Node | Comes from |
|---|---|
| trigger | `run_started`, with its trigger |
| policy | `gate_decided`, with the policy code |
| agent | `planned` |
| tool | `tool_called` / `tool_returned` |
| human | `approval_requested` / `approval_decided` |
| action | `tool_called` whose effect is not a read |
| seal | the hash chain, and `run_finished` |
| **branch** | **nothing — the loop belongs to the interpreter** |

So the specification does not gain a graph, FU-17's "the agent's graph" is
really the run's, and the honest screen is the run trail drawn as a diagram
rather than as a list.

What stays out is the chrome around it: the flow dropdown, Draft/Staging/
Production, "Test run", and a searchable rail of draggable components. That is
the builder N5 rules out, and it is separable from the picture.

---

## 4. Integrations — delivered, and now at the handoff's level

> The top-level screen exists: its own navigation entry, title and header
> action. What is still missing from the handoff is the syncs table.

### The mismatch worth naming

The prototype's cards are **business systems**: Salesforce (CRM), SAP (ERP),
Postgres · risk, Slack, S3 · documents — each with read/write scopes, a row
count, and a last sync.

Ours are **MCP servers**: `crm`, `kb`. In this architecture Salesforce sits
*behind* an MCP server, and the platform never talks to it directly (N4: tools
arrive over MCP, heavy integration stays the FuseOne platform's problem). So
the handoff's cards describe a layer this product deliberately does not own.

Two ways to close that, and they are not the same product:

- **Name the server after what it fronts.** A server called `salesforce`
  offering `salesforce.*` tools reads exactly like the handoff's card, and the
  platform keeps knowing nothing about Salesforce. Costs nothing, buys most of
  the look.
- **Model the system behind the server.** The platform learns that `crm` fronts
  Salesforce, with credentials and sync state of its own. That is the
  integration engine N4 rules out.

The first is the one consistent with the PRD, and it is mostly a naming
convention plus a screen that shows what a server offers.

### Still missing: its own screen

The handoff gives Integrations a top-level view with its own title, a
`Connect system` primary action and a `Recent syncs` table. Ours is a tab
inside Administração. Promoting it is small; the syncs table is the part with
no data behind it, and the honest substitute is the health reading already
recorded — when a server was last reached, by which worker, and how many tools
it offered.

### What was delivered

### What existed

MCP servers and model providers are configurable from Administration, with
forms, credential storage in the vault, and an administrative trail. The
mechanism is complete.

### What the handoff asks for (§8)

Connected-system **cards** — icon tile, name and kind, state pill, description,
scope badges, last sync — plus a **recent syncs** table.

### What is missing

Mostly presentation, with one real gap: **there is no sync**. Nothing polls a
connected system or records when it last answered. "Last sync" and "recent
syncs" describe a liveness concept the platform does not have.

Two ways to make it true rather than decorative:

- **Health rather than sync**: when the server was last reached, whether its
  tool list changed, how many tools it offers. The worker already connects to
  every MCP server at start-up — recording that is small.
- **Actual scheduled syncs**: a periodic tool-catalogue refresh with its own
  record. Larger, and it needs the same single-owner discipline as cron.

### Still open after remote transport: the published list never shrinks

A server that is removed or switched off is disconnected within the reconcile
interval, and its tools leave that worker's catalogue — an agent can no longer
call them, which is the half that matters. What does not happen is the
administration area forgetting them: the tool list is published with upsert
semantics, so publishing a smaller set changes nothing, and the console keeps
listing tools nothing offers.

Replacing wholesale is not the fix. Two workers connected to different servers
each publish what they see, and a replace would have them delete each other's
tools on every pass — a worse failure than a stale row, and an intermittent
one.

What it needs is either ownership on a published entry (which worker saw it,
so a set can be replaced within its own scope) or the console reading liveness
from `integration_health`, which already records when each server was last
reached, rather than trusting the list to be current. The second is smaller and
is probably right: the list answers "what has this installation ever offered",
and health answers "what answers now", and those are different questions that
one table is currently being asked to answer at once.

### Queued: remote tool servers

**Delivered.** A server is now either a process this installation runs or an
address it calls, the token for a remote one is sealed in the vault, and a
reconciler keeps the connected set matching the configured one on a timer, so
nothing waits for a restart. The development stack can serve MCP over HTTP with
a bearer token, so the remote path is exercised locally rather than only in
production.

What follows is the original entry, kept because the reasoning is still the
reasoning.

Only one transport was wired. `cmd/agentd` connected every MCP server with
`mcp.CommandTransport` over a locally executed command, so the form asked for a
command and arguments because that was the only thing the platform could do.
The SDK already in `go.mod` ships `StreamableClientTransport` and
`SSEClientTransport`; neither was used. Remote MCP was never decided against —
it was not built.

Three consequences, in the order they bit:

- **An installation cannot reach a hosted MCP server at all.** The honest
  answer to "connect this to Google Sheets" is currently "write an MCP server",
  which is a development project rather than a configuration.
- **A command with arguments is remote code execution by configuration.**
  Whoever may register a server runs an arbitrary binary inside the worker's
  container. In a product installed in the customer's own environment that is a
  property worth being deliberate about, and it argues for the remote transport
  being the primary path rather than an addition to it.
- **A server registered from the console does nothing until the worker
  restarts.** `servers.connect` runs once at start-up; the tool rulings
  refresh on a timer, the servers themselves do not. Nothing on the screen says
  so, so the observed behaviour is a server that is configured and offers no
  tools.

The work: a transport on the record (`stdio | http`), a URL, the credential
through the vault that already holds model provider keys, a branch at the
connection point, and the form. Plus reconnecting without a restart, and the
screen saying why a server the console does not own cannot be edited here —
which today is a Portuguese literal in the component rather than a catalogue
entry.

### Connectors: reopened, argued, and settled for now

**Decision: a tool is an MCP server. Connectors are revisited only if the need
turns up in practice.**

The conversation was worth having, and it invalidated the reason N4 gives
rather than its conclusion.

N4 justifies refusing an integration engine with "heavy integration remains the
FuseOne platform's domain". That is only a boundary if the FuseOne platform is
on the other side of it, and it is not: this product is installed on its own,
and an installation may have no FuseOne anywhere near it. Standalone, "remains
the domain of" delegates nothing — it describes work that does not happen,
while the customer hears "not our problem" from the only thing they installed.

So the line was redrawn on grounds that stand without FuseOne: **action per
call belongs here; moving data in volume does not.** An agent is the
orchestrator — mapping a field, choosing a route, transforming a record is what
it does by reasoning, and that is the premise of the product. What an agent is
bad at is moving half a million rows every night: that is a pipe, and a pipe
driven by a language model is expensive, slow and wrong.

Most of what an EIP does either already exists here or is the agent itself:
retry and backoff are the worker's, scheduling is the cron trigger's,
orchestration is the loop, mapping is the reasoning. What is left is volume,
and volume does not become an agent by decree.

**Why not a generic HTTP tool**, which is the obvious way to cover an API with
no MCP server: the Gate decides per tool, and effect classification lives on
the tool. One tool that can issue any request makes `DELETE /customers/123`
and `GET /balance` the same entry in the pack, with the same classification. A
policy saying "nothing destructive without approval" is a sentence about the
catalogue, and a generic tool empties it. The trail would record "called
http.request", which tells an auditor nothing in two years.

**If the need does turn up**, the shape to reach for is a *declared* HTTP tool
— name, method, URL, argument schema, credential and effect, configured rather
than coded — served by a built-in MCP server. That keeps every property the
Gate depends on and is not an integration engine: no field mapping, no
transformation, no per-vendor semantics. It is a way of minting a tool without
writing one.

**What the closest adjacent product does.** Tessera (tesseraagent.ai) presents
named systems rather than a protocol: Salesforce, SAP, and an "API interna" of
twelve REST routes, each with a state — connected, syncing, queued. MCP is not
mentioned anywhere public.

Two things are worth taking from it, and one thing is worth not taking.

- **The internal REST API is a first-class tile.** A customer's own service,
  with no MCP server and nobody to write one, is treated as a normal case
  rather than an edge. That is the same gap identified above, and it is
  evidence the gap is real rather than theoretical.
- **The states are sync states.** "Syncing" and "queued" against an ERP
  describe data movement, not a tool being called. That is the other side of
  the line drawn above — they appear to be an agent platform *and* an
  integration engine, which is a larger product than this one has chosen to be.
- **What not to take: the screen is not the architecture.** A tile reading
  "Salesforce · connected" looks identical whether it fronts a bespoke
  connector or an MCP server named after what it fronts — which is exactly the
  cheap option §4 above already recommends. Marketing pages do not distinguish
  the two, and this one does not.

So it is evidence for the declared HTTP tool being the first thing to reach for
when the need arrives, and not evidence for a per-system catalogue.

**N4 needs rewriting rather than amending**, because its stated reason is void.
The conclusion survives on the new reasoning above.

### Size

Smallest of the four if it takes the health reading. The cost half of §8 is
already delivered — spend KPIs, the fourteen-day chart, per-scope spend
against caps.

---

## Recommended order

1. ~~**Integrations**~~ — delivered. The screen reports what the platform
   observed rather than only what was configured, and a server that will not
   answer no longer stops the worker from starting.
2. **Policies** — now unblocked, and it comes before agent authoring because
   the agent screen references policies in three places: the per-tool approval
   rule, the attached-policies list, and tools rendered unavailable with a
   policy code. Building the agent screen first means building those three
   inert and returning to them.
3. **Agent create / edit** — the form, publishing a new version, created
   paused.
4. **The diagram** — still waiting on the graph question, which the new
   handoff does not touch.

Then i18n, by prior decision.

### Order within policies

The screen implies more than a table of rules, and the parts are separable:

1. The rule model, versioned storage, and a `policy_hash` that names something
   retrievable — which is what makes the PRD's replay promise keepable.
2. Evaluation inside the Gate, at the existing fourth position, without
   loosening the seven-check order.
3. Monitor mode: evaluated, recorded, obeyed by nothing.
4. The compiled restatement, generated from the same structure the Gate reads.
5. The screen.
6. Simulation against recorded runs — last, and the piece that also serves the
   Auditor's re-evaluation.

---

## Open decisions, collected

| # | Decision | Blocks | State |
|---|---|---|---|
| 1 | Decision table or expression language for rules | Policies | **Settled** — decision table, by the handoff |
| 2 | How much of the Studio is in scope | Agent authoring | **Settled** — the form, by the handoff |
| 3 | Does the specification gain a graph | The diagram, and FU-17's wording | **Answered** — no. The handoff's own node data is a run |
| 4 | Health reading or scheduled syncs | Integrations | **Settled** — health reading, delivered |

None left open. The diagram turns out to be a projection of the ledger, which
is data that already exists.
