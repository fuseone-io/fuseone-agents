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
| Triggers — manual, cron, webhook | Delivered. Event is declared and unimplemented |
| Overview, Runs, Run trail, Human queue, Agents, Agent detail, Cost, Audit trail | Delivered |
| Administration — tools, MCP servers, model providers, budgets | Delivered |
| **Policies** | Enforced in code, not authorable, not visible. Next |
| **Agent authoring** | Only by dropping a file in a directory |
| **Generated diagram** | Nothing, and nothing to draw yet |
| Integrations | Delivered, with health beside configuration |
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

## 4. Integrations — delivered

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
| 3 | Does the specification gain a graph | The diagram, and FU-17's wording | Open |
| 4 | Health reading or scheduled syncs | Integrations | **Settled** — health reading, delivered |

One decision left, and it is the one where the answer is worth more than the
work.
