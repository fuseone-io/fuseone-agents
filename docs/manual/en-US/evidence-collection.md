---
title: Evidence collection
summary: Collect events from a declared source and analyze them in aggregate, with the coverage of every number stated.
section: integrations
tags: evidence, security, incident response, connector, coverage
order: 25
---

# Evidence collection

An investigation asks telemetry how much left, to where, and whether something
happened at all. Answering takes more events than a model can read, and a number
without its coverage is a wrong answer that looks right: a source that hit a
ceiling, refused the query or never records the event all look like a small count.

This connector collects from a source you declare and answers in aggregate. The
model never sees an event, and never writes a query.

## The three tools

Each instance gives an agent three tools, all reads:

- **`check`** validates the credential and runs the source's positive control. Run
  it first.
- **`collect`** runs a named query over a window and returns a manifest and an
  evidence reference. It needs approval if your policy says so.
- **`analyze`** counts, sums and groups what this run collected. It also takes a
  narrower window and a match on a field.

A collection too long for one call answers `partial` with its evidence reference.
Call `collect` again with that reference in `resume` to continue.

## Reading the answer

Every answer states its window in UTC, its scope, its counts and its coverage,
before anything else.

| Coverage | Meaning |
|---|---|
| `complete` | Every event was read |
| `partial` | The call ran out of budget. Resume it |
| `floor` | A ceiling was reached, or the source stopped advancing. The number is a minimum |

`analyze` then says only what that supports:

| Outcome | Meaning |
|---|---|
| `observed` | Matching events exist. This proves what the source recorded, not intent |
| `not_observed` | Nothing matched, every event was read, the control passed and no event was unmeasured. It does not mean it did not happen |
| `not_observable` | Nothing matched, and something above is missing. Nobody can tell |

A sum reports how many events it could not count and how many were zero, because
a field some event types never carry is not a zero.

## What it refuses

- **A value outside its pattern**, and any value holding a quote, a backslash or a
  control character, whatever the pattern says.
- **A query with a filter missing.** Parameters are never optional, and a filter
  that finds nothing never falls back to a broader one.
- **Evidence from another run.** Evidence is sealed, and belongs to the run that
  collected it.
- **A window on `analyze` outside what was collected.**

A source refusing a query or denying the credential is a failure, never zero events.

## Declaring a source

Authoring goes through the administration API: `PUT /admin/integrations/connectors/instances/{name}` with
`"connector": "evidence"`, its scope, a read-only token in `token`, and the
declaration in `evidence`. The console shows a summary. Read-only is the
credential's guarantee, so give it a token with no write scope.

The first example has fixed relative periods, offset paging and a refusal inside a
200 response. It marks events the gateway could not decrypt as unmeasured.

```json
{
  "baseUrl": "https://tenant.example.com",
  "authHeader": "Netskope-Api-Token",
  "control": "all_pages",
  "rows": "result",
  "refusal": { "field": "ok", "value": "0" },
  "gap": { "field": "bypass_traffic", "value": "yes" },
  "maxEvents": 200000,
  "paging": { "kind": "offset", "limitParam": "limit", "pageSize": 5000, "offsetParam": "offset" },
  "window": { "kind": "period", "periodParam": "timeperiod", "periods": [3600, 86400, 604800, 2592000, 7776000] },
  "queries": [
    {
      "id": "all_pages",
      "description": "Every page event, unfiltered. The positive control.",
      "path": "/api/v2/events/data/page",
      "fields": ["_id"],
      "idField": "_id"
    },
    {
      "id": "pages_by_user",
      "description": "One user's page events, with bytes.",
      "path": "/api/v2/events/data/page",
      "filters": [{ "name": "query", "value": "user eq '{user}'" }],
      "parameters": [{ "name": "user", "pattern": "[A-Za-z0-9._+-]+@[A-Za-z0-9.-]+" }],
      "fields": ["_id", "timestamp", "user", "hostname", "domain", "app", "appcategory", "client_bytes", "server_bytes", "bypass_traffic"],
      "idField": "_id",
      "timeField": "timestamp"
    },
    {
      "id": "uploads_by_user",
      "description": "One user's uploads, with file names and no bytes.",
      "path": "/api/v2/events/data/application",
      "filters": [{ "name": "query", "value": "user eq '{user}' and activity ~ 'Upload'" }],
      "parameters": [{ "name": "user", "pattern": "[A-Za-z0-9._+-]+@[A-Za-z0-9.-]+" }],
      "fields": ["_id", "timestamp", "user", "app", "activity", "object", "object_type", "file_size", "bypass_traffic"],
      "idField": "_id",
      "timeField": "timestamp"
    }
  ]
}
```

The second has absolute ranges and cursor paging. `find_agent` is a lookup with no
window; `analyze` with `groupBy: ["id"]` gives the agent id for the next query.

```json
{
  "baseUrl": "https://console.example.com",
  "authHeader": "Authorization",
  "authPrefix": "ApiToken ",
  "control": "all_activities",
  "rows": "data",
  "paging": { "kind": "cursor", "limitParam": "limit", "pageSize": 1000, "cursorParam": "cursor", "nextCursor": "pagination.nextCursor" },
  "window": { "kind": "range", "startParam": "createdAt__gte", "endParam": "createdAt__lte", "format": "iso" },
  "queries": [
    {
      "id": "all_activities",
      "description": "Every console activity, unfiltered. The positive control.",
      "path": "/web/api/v2.1/activities",
      "fields": ["id"],
      "idField": "id"
    },
    {
      "id": "find_agent",
      "description": "Finds a device by its exact name, to get its agent id.",
      "path": "/web/api/v2.1/agents",
      "filters": [{ "name": "computerName", "value": "{host}" }],
      "parameters": [{ "name": "host", "pattern": "[A-Za-z0-9._-]{1,64}" }],
      "fields": ["id", "computerName", "osName", "agentVersion", "lastActiveDate", "networkStatus", "siteName"],
      "idField": "id",
      "windowless": true
    },
    {
      "id": "activities_by_agent",
      "description": "One agent's console activities, including logons.",
      "path": "/web/api/v2.1/activities",
      "filters": [{ "name": "agentIds", "value": "{agent}" }],
      "parameters": [{ "name": "agent", "pattern": "[0-9]{1,32}" }],
      "fields": ["id", "createdAt", "activityType", "primaryDescription", "secondaryDescription"],
      "idField": "id",
      "timeField": "createdAt"
    }
  ]
}
```

These are starting points, and the names are examples. Run `check` before trusting
one: a source can answer a period it does not accept with an empty success, and a
failed control is how you find out.

Do not declare an export endpoint that keeps a position on the server. Its GET moves
the stream, so another reader loses events.

## Limits

One collection holds up to 200,000 events by default and 1,000,000 at most. Events
stay in the content store under the run's retention. Only `GET` sources are
supported, and a query that runs as an asynchronous job is not yet.
