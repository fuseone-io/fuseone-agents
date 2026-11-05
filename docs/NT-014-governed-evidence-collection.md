# NT-014 — Governed evidence collection

Status: proposed · 2026-11-05 · extends [NT-009](NT-009-governed-connectors.md)

## What this is

An incident-response team asks questions of telemetry: what did this machine do,
did this user reach that service, how much left, and to where. Answering takes
hundreds of thousands of events from an EDR, a secure web gateway or an audit
log, and then a sum.

Two things make this hard for an agent. The events cannot go to a model: one
payload is kept up to 1 MiB and a transcript carries far less. And **a number
without its coverage is a wrong answer that looks right**. A source that hit a
result ceiling, refused the query or never records the event all look like a
small count, or like zero.

This note specifies one connector, `evidence`, that collects from a declared
source and answers in aggregate, with the coverage of every number stated.

## Why a connector and not an MCP server

[NT-008](NT-008-a-catalogue-by-shape.md) and
[N8](PRD-001-fuseone-agents.md#non-goals) refuse a catalogue with one connector
per vendor, and this is not one: the connector holds no vendor's name, and each
source is an instance declared in configuration. The criterion is shape, and the
shape is real. These APIs differ in query language and agree on the rest: a
token in a header, a named filter, a window, pages of JSON, and a ceiling.

[NT-009](NT-009-governed-connectors.md) says a governed connector is for what
the platform must guarantee itself. Three properties here cannot be promised
across an MCP boundary:

- **Coverage is computed by the code that paged.** A server can claim anything.
- **Events live in the content store**, under retention and erasure that already
  exist, rather than in a second store nobody remembers to purge.
- **Evidence is fenced to the run that collected it**, which requires the run's
  identity on the call.

A vendor's own MCP server remains the right answer for what it does well. It
sits beside this connector and is classified by the Curator like any other.

## Operations

All three read, return untrusted content and never answer from a cache.

| Tool | Model supplies | Returns |
|---|---|---|
| `evidence.<instance>.check` | nothing | Credential state and the positive control |
| `evidence.<instance>.collect` | query id, parameters, window, optional resume | A manifest and a reference to the events |
| `evidence.<instance>.analyze` | evidence reference, window, declared fields to group and sum | Aggregates with the coverage inherited |

`check` needs no approval. `collect` and `analyze` touch personal data, so policy
decides. An approval is bound to a digest of the query's and the control's declarations
through `ApprovalBinding`, as SQL does: changing either under the same id asks
again.

## The declaration

An instance declares its base URL over https, the header that carries its sealed
token, a positive control, and what is common to the source:

- where the events are in a response;
- paging, either `offset` or `cursor`, with a page size and a ceiling on events;
- its window, either a list of relative periods or an absolute range;
- what a refusal looks like in a 200 response;
- which field and value mark an event the source could not measure.

And its named queries. A query declares:

- a path and a template for its query string, requested with GET;
- parameters, all required, each with a pattern;
- the fields kept from each event, which must include the unmeasured marker, and
  which of them identify and timestamp it;
- whether it is a lookup with no window.

**A value outside its pattern is refused, never escaped.** Escaping needs the
query language of each vendor, and a mistake there lets a model rewrite the
filter. Refusing needs a regular expression. A value containing a quote simply
does not reach the source.

**A method says nothing about the effect.** One vendor's read is a POST, and
another's GET moves a stream position on the server, so that a second reader
loses events. Declarations cannot name an effect; operations are `read`, and the
operator's token is what makes that true. A stateful export endpoint does not
belong in a declaration.

## Coverage

Every `collect` and `analyze` result states its absolute window in UTC, its scope,
its counts and one of:

| Coverage | Meaning |
|---|---|
| `complete` | Paging ended on a short page or the end of a cursor |
| `partial` | The call's budget ran out; the result carries a `resume` |
| `floor` | The ceiling was reached, or paging stopped advancing. The number is a minimum |

`analyze` then says only what that supports:

| Outcome | When |
|---|---|
| `observed` | Matching events exist. It proves what the source recorded, not intent |
| `not_observed` | No match, coverage `complete`, the positive control passed, and no event in the set is unmeasured |
| `not_observable` | No match, and any of those is missing |

`not_observed` never reads "did not happen". Events marked as unmeasured are
counted and reported, because a sum that silently includes them understates, and
they rule out `not_observed`: the event that would have matched may be exactly
the one the source could not see.

A relative window moves between resumes, so a collection can hold an event twice.
`analyze` counts each id once and reports the duplicates, which makes it the
authority on counts; the number on a `collect` is what was stored. A window on
`analyze` cannot reach outside what was collected, and a sum reports the events it
could not count, because a field some event types never carry is not a zero.

The positive control is not left to anyone's memory: a collection runs it over
the same window before its first page and records the answer in the evidence. A
parameter is never optional, because a query with a filter missing is a different
query, and it gets its own name.

The evidence state is sealed with a key derived from the instance's token. A
reference alone would let content from another tool in the same run pose as a
complete collection.

## Failures that must not become numbers

Each of these has been seen in practice, and each has a test:

1. A 200 response whose body refuses the query is a failure, never zero events.
2. 401 and 403 are `connector_upstream_refused_auth`, never zero events.
3. An offset that stops advancing is `floor`, never `complete`.
4. There is no fallback chain. A filter that returns nothing stays empty, because
   trying a broader one attributes other people's traffic to the subject.
5. A relative period the source does not accept is a configuration error at
   validation, never an empty result.
6. Hostnames compare by suffix on a label boundary, never by substring: a
   substring match counts `attacker.example.com.evil.test` as `example.com`.

## Execution

There is no background goroutine, sweeper or new table. `collect` runs inside the
call, under a budget of pages and time, and answers `partial` when it runs out.
The next call crosses the Gate and the ledger like any other, which keeps one
writer per run and needs nothing to be reconciled after a crash.

Events are stored in blocks under the 1 MiB payload limit. The reference already
contains the digest, so many blocks fit under one sequence number. The manifest
lists them, and a block that would be truncated is a defect the tests refuse.

`analyze` and `resume` refuse a reference that does not belong to the calling
run. Sharing evidence across runs is a decision about cases and retention, and it
is not made here.

## Not in this note

- **Asynchronous queries with window splitting.** Some sources run a query as a
  job and cap each one, so the window is halved on saturation. It waits for an
  API worth building against: at least one vendor's legacy API for this is being
  retired, and the replacement should be read from the vendor's documentation.
- **Process lineage analysis.** Reconstructing a process tree from events is
  generic; the noise lists around it are not. It follows once `analyze` has proved
  its contract.
- **Directory and configuration sources.** Per-entity fan-out and delegated
  tokens fit the shape; the domain rules that classify what they return do not,
  and those need their own decision.
- **A console editor.** Authoring goes through the administration API, as SQL's
  does, until a form can round-trip the whole declaration.
