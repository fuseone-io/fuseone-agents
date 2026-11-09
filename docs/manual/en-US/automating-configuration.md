---
title: Automating configuration
summary: How a pipeline reads one resource, tells whether it drifted and writes it without overwriting somebody else, and what the API promises not to break.
section: reference
tags: api, automation, terraform, pipeline, etag, if-match, 412, compatibility, breaking change
order: 26
---

## Who this is for

Whoever keeps an installation's configuration in a repository and applies it
from a pipeline, a script or a Terraform run. The console uses the same API,
under the same rules, so a person and a pipeline editing the same resource each
see the other's change instead of erasing it.

A pipeline acts through a service account (see
[Companies, areas and roles](scopes-and-roles.md)). Every path below is under
`/api/v1`, and the contract is `api/openapi.yaml` in the repository.

## What can be managed this way

| Resource | Path | Needs |
|---|---|---|
| Policy | `/policies/{code}` | `policy:write` |
| Tool server | `/admin/integrations/mcp-servers/{name}` | `provider:write` |
| Model provider | `/admin/integrations/providers/{name}` | `provider:write` |
| Channel | `/admin/channels/{name}` | `provider:write` |
| Connector instance | `/admin/integrations/connectors/instances/{name}` | `provider:write` |

Each one takes `GET`, `PUT` and `DELETE` by name. A connector instance is
named within a scope as well: `GET` and `DELETE` take `scopeKind` in the query,
plus `company` and `area` as that scope needs, and `PUT` carries the same in its
body. Without them the request is a 400. The permission is checked
where administration is held, so the role has to be granted there; today Admin
and Curator carry both. Reading takes the same permission as writing, except
for a policy, which `policy:read` anywhere is enough to read: a rule that stops
you should be one you can read.

## Reading back what was written

`GET` answers what `PUT` stored. A secret never comes back: the answer says
only whether one is stored (`hasSecret`, `hasKey`, `hasToken` and the like).
A tool server, a model provider and a channel also give a `secretFingerprint`,
keyed, that changes when a secret does and says nothing else about it. A
connector instance gives none; the only sign that its token changed is its
`ETag`, which names the whole resource. The answer also carries what the server
keeps for itself, such as `updatedAt` and `updatedBy`, which a write does not
send.

So a client compares the non-secret fields its `PUT` sends with the same fields
in what `GET` answers, and a difference is real drift. Whether a secret it
holds is the one stored is a question for the fingerprint where there is one,
and otherwise for the `ETag` it kept from its last write.

**Writing what is already stored changes nothing.** No new `updatedAt`, no
trail line, no new policy snapshot, and the same `ETag` comes back. A pipeline
can apply its whole configuration on every run and leave no trace where nothing
changed.

## Writing without overwriting somebody else

Every `GET` above answers an `ETag`, and every `PUT` answers the one it left.
Send it back as `If-Match` on the next `PUT` or `DELETE`:

```http
GET /api/v1/admin/integrations/providers/gateway
→ 200, ETag: "v1-3f9c…"

PUT /api/v1/admin/integrations/providers/gateway
If-Match: "v1-3f9c…"
→ 204, ETag: "v1-77ab…"   written
→ 412                     somebody changed it first; nothing was written
```

- **A 412 wrote nothing.** Its problem type is `fuseone:precondition-failed`,
  and its `ETag` header is what the resource is now. It has no `ETag` when the
  resource no longer exists. Read again and decide with what is there; never
  retry the same body under the new tag without looking.
- **`If-Match: *`** means "only if it exists". Any `If-Match` on a resource
  that does not exist is a 412, `*` included.
- **A weak tag (`W/"…"`) matches the revision it weakens.** A proxy that
  compresses responses marks the `ETag` it forwards as weak, and a client hands
  that tag back; the revision names a configuration, not bytes, so it is still
  the same one.
- **A header that does not parse is a 400**, never ignored: ignoring it would
  turn off the protection the client asked for.
- **Without `If-Match`, the last write wins**, as it always has.

The `ETag` names the resource's content, not when it was written. It moves when
the configuration or a stored secret changes. A channel's also moves when a
conversation or an identity is attached to it, and a tool server's when
somebody pastes an OAuth grant, but not when a worker rotates one. Changing the
installation's master key moves the `ETag` of every resource that holds a
secret. Treat the tag as opaque: compare it, never parse it.

## What the API promises not to break

The API is a public contract. The console, pipelines and the integrations a
customer writes are all clients of the same paths.

**Not a breaking change**, and announced under **Added** or **Changed**:

- a new path, a new operation, or a new optional field in a request;
- a new field in a response. A client must ignore fields it does not know;
- a new problem type for a refusal of something new, such as a value of a new
  field or a request to a new path. It neither turns a request that used to
  succeed into a refusal nor replaces the type of a refusal that already
  exists; either of those is a breaking change, below.

**A breaking change:**

- removing or renaming a path, an operation or a field, or changing a field's
  type or what it means;
- making an optional request field required, or refusing a request that used
  to succeed. The platform does this on purpose when a governance rule starts
  to apply, and it is still a breaking change, announced the same way;
- changing the status or the problem type of a refusal a client acts on;
- changing what a request without `If-Match` does;
- changing how an `ETag` is made. Every tag starts with its format, `v1-`, so
  such a change retires every old tag visibly: a client holding one gets a 412
  and reads again, and never a match that means something else.

**How a breaking change is announced.** In the **Upgrade notes** of the release
that ships it, which come first in the [changelog](https://github.com/fuseone-io/fuseone-agents/blob/main/CHANGELOG.md),
saying what stops working and what to do instead. Never in a patch release,
unless it closes a security hole, and then the notes say so.

**Problem types never change once shipped.** Branch on `type`, never on
`title` or `detail`: the title is an English fallback, and the detail carries
particulars such as an identifier or a permission.

**During a rollout, two versions answer.** A pod still on the previous version
can serve a request while the new one rolls out. The upgrade notes say when a
client should wait for every replica before relying on something new, as they
did for `If-Match` itself in 0.97.0.
