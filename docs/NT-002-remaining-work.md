# NT-002 · What is left, and in what order

**Status** Proposal · **Date** 2026-08-11
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
| Triggers — manual, cron, webhook | Delivered. Event is declared and unimplemented |
| Overview, Runs, Run trail, Human queue, Agents, Agent detail, Cost, Audit trail | Delivered |
| Administration — tools, MCP servers, model providers, budgets | Delivered |
| **Policies** | Enforced in code, not authorable, not visible |
| **Agent authoring** | Only by dropping a file in a directory |
| **Generated diagram** | Nothing, and nothing to draw yet |
| **Integrations screen** | Forms exist; the handoff's screen does not |
| i18n | Deferred to last, by decision |

---

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

### The decision that has to come first

**What is a rule?**

| Option | For | Against |
|---|---|---|
| **Decision table** — conditions over tool, effect, labels and scope → verdict | Readable by a non-programmer; renders almost directly into the handoff's table; auditable row by row; the hash covers a set of rows that can be stored and re-read | Deliberately limited. Some rules will not fit and will have to be refused or built in |
| **Expression language** (CEL, Rego) | Expresses anything | Hands the customer exactly the "think like a programmer" the PRD rejects as its reason for not being a flow builder (N5). A policy nobody in the business can read is a policy nobody in the business owns |

**Recommendation: the decision table.** It fits the screen, it is auditable
line by line, and it makes the policy hash mean something — a versioned set of
rows that can be fetched and replayed.

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

**Nothing.** The Agents screen has an "Import" and a "New agent" button; no
screen behind either. This is worth stating plainly, because it was on the
list as though a design existed.

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

### The decision that has to come first

**How much of the Studio is in scope now?** Three honest steps:

1. A form over the fields that already exist — name, model, tools, ceilings,
   triggers — publishing a new version. Small, useful, and not what the PRD
   describes.
2. The interview producing the same fields plus instructions, with a narrative
   read-back before publishing. Closer to the PRD, and needs a model call.
3. The full Studio with simulation and regression cases. Its own project.

### Size

Step 1 is comparable to the administration forms already built. Step 2 is
comparable to a screen plus a new model-backed endpoint. Step 3 is not
estimable yet.

---

## 3. The generated diagram

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

### The decision that has to come first

**Does the specification gain a graph?** If yes, that changes authoring,
FU-13's per-step corrections, and the engine. If no, the diagram is a
projection of something else and FU-17 should be rewritten to say so.

This is the only front where the decision is worth more than the work.

---

## 4. Integrations

### What exists

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

### Size

Smallest of the four if it takes the health reading. The cost half of §8 is
already delivered — spend KPIs, the fourteen-day chart, per-scope spend
against caps.

---

## Recommended order

1. **Integrations** — smallest, closes a screen the handoff already designs,
   and the health reading is genuinely missing operationally: today nothing
   says whether an MCP server is answering.
2. **Policies** — largest, and the one blocking a PRD promise rather than a
   screen. Needs the rule-model decision before any code.
3. **Agent authoring, step 1** — a form publishing a new version. Makes the
   platform usable without filesystem access, which is what currently stops
   anybody but an engineer from creating an agent.
4. **The diagram** — after the graph question is answered, and possibly never
   in the handoff's form.

Then i18n, by prior decision.

### Why not policies first

It is the most valuable and the least ready. Starting it before the rule model
is settled means building storage and evaluation twice. Integrations buys a
finished screen and an operational gap closed while that decision is made.

---

## Open decisions, collected

| # | Decision | Blocks |
|---|---|---|
| 1 | Decision table or expression language for rules | Policies, entirely |
| 2 | How much of the Studio is in scope | Agent authoring |
| 3 | Does the specification gain a graph | The diagram, and FU-17's wording |
| 4 | Health reading or scheduled syncs | Integrations, partly |

Nothing here is blocked on code. All four are product decisions.
