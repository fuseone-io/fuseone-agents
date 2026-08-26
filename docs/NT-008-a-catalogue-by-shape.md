# NT-008 · A catalogue by shape

**Status** Proposal · **Date** 2026-08-15
**References** [PRD-001](PRD-001-fuseone-agents.md) — N4, N8, N9, FO-12, AU-14…17, SE-11, DE-11…14, DE-22…24 · [NT-005](NT-005-interaction-channels.md)
**Outcome** Four servers to ship, two to refuse, one to split — and the criterion that decides the next one

The platform reaches every system through MCP and ships none of its own, which
leaves an installation with a governed runtime and nothing to run. The obvious
answer is a catalogue, and the obvious catalogue is a list of vendors:
Prometheus, Datadog, Sentry, one per logo. [N8](PRD-001-fuseone-agents.md#non-goals)
refuses that, and this note is what goes in its place.

**The criterion is shape.** A shape serves every vendor that has it and costs
one server; a vendor serves one and costs a server every quarter, because its
API moves. Where a shape genuinely exists — HTTP does, SQL nearly does — one
server covers a category. Where it does not, an abstraction is the intersection
of three products, too weak to use, or it leaks and becomes three adapters
wearing one name. That is the vendor catalogue again with a layer of indirection
on top, which is worse than the vendor catalogue.

---

## 1. What ships

### 1.1 `fuseone.content` — retrieval over what is already held

**First, because the other three depend on it economically.** A tool result
today reaches the model whole and is re-sent on every turn that follows
([FO-12](PRD-001-fuseone-agents.md#84-built-in-efficiency)). Every server below
returns data, so each one makes that worse until this exists.

`search` · `slice` · `near` · `metadata`

It is the one server on this list nobody else will ever publish, because it is
not an integration: it reads `run_content`, which is the platform's own.

**Redaction is not one of its tools.** A `redacted` call is redaction the agent
chooses, and redaction that is optional is redaction that does not happen. What
decides is the label on the content, not the politeness of the caller — the same
tool returns a personal-data payload masked because of what it is.

**What it returns carries the label of what it returned.** A slice of an
untrusted log is untrusted after slicing. Without that the retrieval server is a
laundry for taint, and the check that makes external text survivable
([§10.4](PRD-001-fuseone-agents.md#104-data-labels)) stops meaning anything the
moment an agent reads through this instead of directly.

### 1.2 `declared.http` — an endpoint is a tool

Never a generic `http.request`. **Each declared endpoint *is* a tool** — name,
method, URL, argument schema, credential, timeout, rate limit, effect, labels —
so effect and credential attach to exactly the thing the Gate rules on, rather
than to a shell that is READ and WRITE at once depending on its arguments.

```
billing.invoice.lookup   GET  …/invoices/{id}      read
ticket.create            POST …/tickets            write
deploy.status            GET  …/deploys/{id}       read
```

The property worth having falls out of it: **adding a system is a row, not a
release.**

**A declaration is versioned.** It is configuration, and an endpoint redeclared
with a different effect must not silently reclassify what a published pack
already meant. A pack pins the declaration it was published against, the way a
run pins its version.

### 1.3 `sql.readonly` — named queries, not a prompt that writes SQL

`customer.find_by_email(email)` · `orders.recent_for_customer(account, since)` ·
`sla.current(service)`

Each with an input schema and an output limit. The Gate then rules on a call
with typed arguments instead of on a string, which is what makes both the
classification and the arguments check mean something.

**Read-only is the connection, never an inspection of the query.** A parser
deciding whether some SQL writes is a parser somebody gets past, and the credential
is the only place the answer is not a matter of opinion.

> **Open, and it bears on a case we want.** An agent that validates SQL somebody
> wrote in a channel needs to look at *arbitrary* SQL — that is the job, and
> named queries cannot express it. Either a separate `explain` that accepts free
> text and **cannot execute**, or that agent judges the text without touching a
> database. Worth settling before the server is built, because it changes its
> shape.

### 1.4 `object.content` — documents and exports

`list` · `metadata` · `slice` · `parse_table` · `extract_text`

Registered stores only, always through a handle with a limit, always carrying a
data label — the same discipline as §1.1, because a 40 MB PDF read whole is the
same defect as a log read whole.

> `extract_text` over a PDF is the highest injection density this platform will
> ever handle. A PDF carries text nobody sees on opening it, and an agent
> reading one is reading whatever the sender chose to hide there. It is untrusted
> by construction, never by classification.

---

## 2. What does not ship, and why

### 2.1 `observability.query` — the shape is not there

The proposal is `metric.query_range`, `log.search`, `trace.find` over registered
datasources, with adapters for Prometheus, Loki, Datadog, CloudWatch. It is the
most attractive item on the list and the one where the criterion fails.

HTTP is genuinely one shape. SQL is nearly one. **Metric query is not one
shape**: PromQL, Datadog's language and CloudWatch Metric Insights differ in
their label model, their window semantics and their aggregation — the parts a
query *is*. An abstraction over the three is their intersection, which cannot
express the query anybody actually runs, or it leaks and becomes three adapters
behind one name.

**What serves the case today is §1.2.** A declared endpoint per query actually
run — `errors.count_by_signature(window)` — which is the same discipline as the
named queries above, and is exactly the "ask for counts, not lines" an agent
reading three hours of logs needs in order to afford it at all.

If a real abstraction emerges after five installations have declared the same
four endpoints, promote it then. It will be a better abstraction for having been
derived rather than imagined.

### 2.2 `source.code` — the vendor's own MCP is the answer

GitHub and GitLab publish MCP servers, and [DE-11](PRD-001-fuseone-agents.md#94-tool-catalogue)
says how a tool arrives. Writing a client for a system that ships one is
building what the criterion exists to avoid; **what this platform adds is
classification and the Gate, not a client.** CI logs are §1.2 and §1.4.

The read/write split the proposal makes is right and survives the cut — it is
just made by the Curator classifying the vendor's tools, which is where that
decision already lives.

---

## 3. What splits: `case.memory`

The proposal is `remembered.find`, `remembered.assert`, `dedupe.check`. One of
the three is a tool. The other two are the platform, and shipping them as tools
undoes requirements written a day earlier.

| Proposed | Verdict | Why |
|---|---|---|
| `remembered.find` | **A tool.** Ships | Reading what the agent knows is a read, and the agent choosing when to look is correct |
| `remembered.assert` | **Not a tool** | As a tool, memory becomes active because the model said so, written by a model call and stored wherever the server keeps it — a mutable store beside the Ledger, which [AU-14](PRD-001-fuseone-agents.md#76-what-an-agent-remembers-between-runs) forbids. What is remembered is derived from reviewed or repeatedly observed evidence, not from the model asserting it |
| `dedupe.check` | **The Gate** | As a tool, the agent has to remember to check. [SE-11](PRD-001-fuseone-agents.md#76-what-an-agent-remembers-between-runs) makes the idempotency check span the agent, which is a guarantee of the platform — and a call the model may forget is not a guarantee, it is a duplicate waiting for a bad day |

The distinction generalises, and it is the same one [NT-005 §10](NT-005-interaction-channels.md)
makes about channels: **what the agent chooses is a tool; what the platform
owes is not.** An agent choosing to recall is reasoning. An agent being unable
to do the same thing twice is a property somebody bought.

The later `$fuseone.memory.suggest` tool keeps that boundary by being a write
effect that writes review material, not active memory. A pending suggestion is
invisible to `memory.find`; promotion happens only by human review or by a
versioned auto-confirm policy that counts distinct runs. The tool is a way for
an opted-in agent to point at a candidate assertion, not a way for it to write
remembered truth. Because it persists state, tainted context reaches it through
the normal Gate path as a tainted write.

---

## 4. Where they run

A server reached over `http` is an address. A server run as `stdio` is **code
executed inside the worker's container**, and it inherits the worker's network
position and credentials — a larger grant than any tool in the catalogue, made
by packaging rather than by decision ([DE-24](PRD-001-fuseone-agents.md#94-tool-catalogue)).

So anything shipped to run there declares what it reaches and runs with its own
credential. `fuseone.content` is the one that reads the platform's own store and
is the one to be most careful with: it is also the one whose blast radius is
every payload the installation ever kept.

---

## 5. Order

1. `fuseone.content`
2. `declared.http`
3. `sql.readonly`
4. **Not `observability.query`** — the reading half of memory, and the Gate's
   idempotency across the agent

Three and four are the pair that makes an operational agent operational. Without
them it opens the same issue every three hours over a beautiful catalogue: it
cannot afford to read, and it does not remember having read.

`object.content` slots in wherever a case needs it; nothing depends on it.

## 6. What this note does not decide

- **Whether `sql.readonly` gets an `explain` that accepts free text.** §1.3.
- **How a declaration is versioned against a pack.** §1.2 states that it must
  be; the mechanism is the pack's, not this catalogue's.
- **What `remembered.find` returns.** The shape of an assertion is
  [§7.6](PRD-001-fuseone-agents.md#76-what-an-agent-remembers-between-runs)'s
  question, and it should be answered there before a tool reads it.
