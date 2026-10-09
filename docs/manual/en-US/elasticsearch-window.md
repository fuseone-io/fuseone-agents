---
title: Elasticsearch window
summary: Give an agent historical traffic questions as named queries over one fixed index, with a fixed projection and no DSL.
section: integrations
tags: elasticsearch, search, analytics, connector, security
order: 21
---

# Elasticsearch window

A label-indexed log store answers "give me this pod's recent logs" well and
"which addresses hit this path over seven days" badly — the second question
means scanning the stream whole. A search engine that indexed every field
at write time answers it in milliseconds. This connector gives an agent
that power, governed.

## Named queries, never DSL

The model never writes a query. It calls one of two named shapes with
values, and the platform builds the body:

- **`top_ips`** — the busiest client addresses on a path over a window,
  with per-day and per-status breakdowns. Arguments: the path, whether it
  is a prefix or the exact value, the window in hours, how many addresses.
- **`resource_history`** — the recent requests to one exact path,
  projected to timestamp, address, status, method and latency.

An unknown argument — an index, a query object — is refused outright. The
index pattern and the field mapping live in the instance's configuration;
the one thing a model can never do is choose what to read or widen what
comes back.

## The projection is the boundary

Results carry only the projected fields. Whatever else the ingestion
pipeline stored in a document — payloads, headers, keys — never reaches a
run, and everything that does come back is labeled untrusted: log content
is data the Gate's taint check must keep seeing.

## Configuring an instance

Integrations → Connectors → New Elasticsearch window. The endpoint may be
plain http for an in-cluster search; the username is configuration and the
password is sealed on save, answered only as stored/missing. The index
pattern is fixed — e.g. `logs-*` — and the field mapping defaults to a
common gateway access-log shape (`uri`, `remote-address`, `status`,
`@timestamp`); write your documents' names if they differ. The window
ceiling (default 7 days, at most 30) caps every query, because an
unbounded window is an unbounded bill on somebody else's cluster.

The connector speaks the 6.x/7.x query dialect; an 8.x cluster is untested.
