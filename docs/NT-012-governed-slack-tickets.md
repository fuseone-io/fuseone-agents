# NT-012 - Governed Slack tickets for Gravitee subscription approval

## Status

Implemented. The two Gravitee operations are runtime only through the governed
native connector path. Ticket admission, immutable revisions, informed
approval, one execution claim, orphan reconciliation and deterministic thread
completion are covered independently and by the Slack-to-ticket end-to-end
path described below.

## Problem

A support conversation receives requests to approve pending Gravitee API-key
subscriptions. Today a person reads the thread, identifies the subscription,
checks who requested it, asks an authorised colleague, accepts the subscription
in Gravitee and reports the outcome.

FuseOne should perform that process without turning Slack text into authority,
without giving the model a general HTTP client and without persisting or
exposing the API key. The requester retrieves the key from Gravitee after the
subscription is accepted.

The dominant risk is not Slack or Gate. It is claiming success after an
ambiguous external effect, or accepting a subscription for the wrong person,
remote scope or snapshot.

## Product boundary

The first release supports API subscriptions through the Gravitee Management
API v2 shape published for Gravitee 4.7.6. It does not support API Product
subscriptions. A later version may add another explicitly versioned adapter;
it must not reinterpret a new response as the shape described here.

The public contract inspected for this decision is:

- `GET /environments/{envId}/apis/{apiId}/subscriptions/{subscriptionId}`
- `POST /environments/{envId}/apis/{apiId}/subscriptions/{subscriptionId}/_accept`
- [Gravitee Management API v2 OpenAPI, tag 4.7.6](https://raw.githubusercontent.com/gravitee-io/gravitee-api-management/refs/tags/4.7.6/gravitee-apim-rest-api/gravitee-apim-rest-api-management-v2/gravitee-apim-rest-api-management-v2-rest/src/main/resources/openapi/openapi-apis.yaml)

The accept request permits `reason`, `startingAt`, `endingAt` and
`customApiKey`. FuseOne never sends `customApiKey`. The declared response is a
`Subscription`; API-key values live in a separate endpoint that this connector
never calls. A fixed decoder still rejects or discards every field outside the
safe projection because a deployed version may return more than its published
schema.

The published contract exposes no ETag, conditional update or idempotency-key
header. The target installation must still be measured for read-after-write
consistency before runtime activation. Until a stronger contract is proven, an
ambiguous accept is reconciled and never submitted a second time automatically.

## Identity and authority

Four identities remain distinct throughout the ticket:

| Identity | Meaning |
|---|---|
| `RequestedBy` | The linked person who wrote the root Slack message |
| `AddressedBy` | The configured bot or app that named approval recipients |
| `RunAs` | The configured platform principal under which watched runs execute |
| `DecidedBy` | The linked person who approved or refused the exact Gate request |

`Call.OnBehalfOf` is `RunAs` for a watched message. It is not the requester and
must never be used as one.

The first release accepts only a root message written by a Slack user whose
account is linked to a FuseOne person in the conversation scope. Bot-authored
ticket roots require a separate, authenticated requester claim and are outside
this release. Admission and addressing are separate configuration:

- `openFrom: linked_users` admits linked human roots in this scoped room.
- `addressFrom` names the exact bot or app allowed to replace the recipient
  list with structured mentions.

The initial remote ownership rule is deliberately narrow. `RequestedBy` must
match the expanded Gravitee application's primary-owner email. A group owner,
a missing email or a different owner fails closed. Supporting application
members requires an explicit identity mapping and is a later capability. The
target-installation lab must prove that the expansion supplies this field.

A mention routes a card; it grants no authority. Recipients are intersected
with people who hold `approval:act` in the run scope, and the decision endpoint
checks that permission again.

## Remote scope

A scoped connector instance binds FuseOne authority to remote resources:

```yaml
organization: org-id
environment: env-id
allowedReferences:
  - type: API
    id: checkout-api
minTTL: 24h
maxTTL: 90d
allowNoExpiry: false
```

The first runtime requires exactly one API reference per connector instance.
The model supplies only `subscriptionId` and the requested expiration; the
environment and API path are therefore derived entirely from the instance,
never from model-authored path components. Supporting several APIs means
configuring several scoped instances. This avoids probing every allowed API to
discover which one owns a subscription and keeps a negative inspection to one
remote request. Inspection and acceptance both verify ownership and the remote
binding.

The Gravitee credential should hold only subscription read and update access
for the declared environment and APIs. FuseOne checks remain necessary even
when the remote role is narrower; neither is a substitute for the other.

## Ticket state

A Slack thread is one ticket and may produce several short runs while fields are
collected.

- `TicketKey` is a length-prefixed encoding of connection, conversation and
  root message.
- A monotonically increasing revision names each request shape.
- `TicketRef{key, revision}` is platform-owned context, never a model argument.
- Ticket content and canonical drafts live in erasable content storage. The
  ticket projection holds references, digests, identities and state only.
- The minimum state machine is `collecting -> awaiting_approval -> executing`
  and then `completed`, `rejected` or `cancelled`. An ambiguous write that
  exceeds its automatic check window enters non-terminal `needs_attention`:
  the active execution claim remains held while observation continues.

Only the original requester changes request fields. Only `AddressedBy` changes
the recipient list. FuseOne messages, approver prose and every other source are
ignored and are not placed in model context. Slack `event_id` deduplicates each
transition.

Before execution is claimed, a new request revision invalidates the pending
approval and closes its cards. Claiming `(TicketKey, revision, runID,
approvalAtSeq)` is one compare-and-set operation. After the claim, that revision
may finish; a later revision waits and cannot start a second external write.
After repeated attempts to open that saved revision, the inbox records a
separate reply debt and tells the requester once that it is waiting. Delivering
that notice does not settle the revision event; it remains pending and opens
after the active execution releases the ticket.
Completing an older revision records its outcome but cannot overwrite the
current revision's state.

The invariant is one active approval card, not one message ever posted. Closing
an obsolete card and posting the current one is legitimate.

## Approval snapshot

An approver must see authoritative data, not only a subscription identifier.
Inspection stores a safe canonical snapshot by reference. Its contract is:

```text
TicketRef
RemoteSnapshotRef
RemoteSnapshotDigest
```

The approval request carries those values beside `ArgsRef`, `ArgsDigest` and
`ContractDigest`. The safe snapshot contains the subscription id, status,
application id and name, primary owner, API id and name, plan id and name,
requested expiration and the remote timestamps needed for comparison. It never
contains an API key, consumer message or unrestricted metadata.

The card renders this snapshot directly. Before accepting, the connector reads
the subscription again and compares the fields the approver saw. A mismatch
invalidates the approval without sending the write. If the target API offers no
conditional update, the note and the test report the remaining remote TOCTOU
rather than claiming it is absent.

The first release deliberately authorizes only the application's primary owner.
Membership or authorship of the remote subscription does not substitute for
that identity. Mailbox local-parts compare exactly; only the domain is folded.

## Connector surface

The named connector exposes only:

- `gravitee.inspect_subscription`, a governed read requiring trusted ticket
  context;
- `gravitee.accept_subscription`, a governed write requiring the same ticket
  revision and snapshot.

Reconciliation is an internal capability of the accept runtime, not a model
tool. Both operations use a fixed code-owned projection. Instance configuration
cannot add output fields.

The transport requires verified HTTPS, disables redirects, never logs request
or response bodies, limits response bytes and time, and is never cached. It
reads configuration and its Vault credential in one snapshot. Errors expose a
stable code, status and request id where available, never the body.

## Ambiguous outcomes and orphan recovery

The engine records `tool_called` before invocation. A process can therefore die
after Gravitee accepts but before `tool_returned` is sealed. Ordinary orphan
handling correctly refuses to repeat that write, but cannot confirm it.

Before the POST, the connector stores a safe execution attempt keyed by the
tool idempotency key, ticket revision and snapshot digest. A worker-owned sweep
finds orphaned Gravitee attempts and calls an internal reconcile capability. It
never asks the model and never repeats the POST.

Reconciliation has these outcomes:

| Observation | Outcome |
|---|---|
| `ACCEPTED` and the approved fields match | Seal the confirmed result |
| `REJECTED` or `CLOSED` | Seal a terminal non-success result |
| `PENDING` before any write attempt | The first accept may proceed |
| `PENDING` after an ambiguous attempt | Poll within the bounded window |
| Unknown, inconsistent or still pending at the deadline | Notify that manual verification is required, retain the execution claim and continue GET-only observation |

The attempt record contains no response body, API key or connector credential.
Finishing recovery is conditional on the same ticket revision so an old result
cannot complete a newer ticket.

When the ordinary `tool_returned` step is absent, confirmed recovery appends an
`effect_reconciled` audit step with the safe result reference and digest. That
step records what became known without changing the run phase, a later pending
approval or a terminal outcome. The external-attempt journal is settled only
after either the ordinary return or this immutable correction exists.

Manual observation is stopped by the existing **Abandon run** action. The
reconciler reads that person's immutable decision, seals a terminal failed
result and retires the journal without another remote request. Automatic expiry
would release an uncertain effect without a person accepting that consequence,
so elapsed time alone never abandons it.

## Message admission and performance

The content matcher is compiled when configuration is saved and runs only on
root messages. Replies find an existing ticket through an indexed lookup by
connection, conversation and root. The implementation limits pattern count,
pattern bytes, extracted context, Slack reads, open tickets and per-ticket
concurrency.

Non-matching messages create no run and no durable refusal. Aggregate metrics
may count them without retaining their content. FuseOne messages, reactions and
message edits are ignored. A correction is a new reply, and the thread and
manual say so.

Two different replies are merged under the ticket lock, so neither loses fields
from the other. Re-delivering the same Slack event is a no-op. Repeating an
identical recipient list does not close and recreate cards.

## Delivery sequence

1. A linked person writes a matching root request.
2. The ticket stores its requester and first revision.
3. Short runs collect `subscriptionId`, expiration and reason by reference.
4. The configured addressing source names recipients.
5. `inspect_subscription` produces the safe authoritative snapshot.
6. The Gate records the exact arguments, contract and snapshot.
7. Authorised recipients receive the same card in the room and by DM.
8. One decision wins through the existing approval precondition.
9. The ticket claims that revision and the connector repeats the preflight.
10. The connector accepts once, or an orphan reconciler later confirms it.
11. A deterministic thread message reports the subscription, expiration and
    retrieval and revocation guidance. It never reports the key or remote
    display names.

## Delivery slices

Each slice is green and reviewable on its own. The behavioural test is written
and observed failing before implementation.

1. Record this contract and validate the target Gravitee installation.
2. Add trusted ticket context to run state and tool calls.
3. Add ticket identity, revision, event deduplication and compare-and-set state.
4. Add the planned Gravitee catalogue and validated instance configuration.
5. Add fixed inspection, snapshot storage and informed approval rendering.
6. Add acceptance, safe attempt recording and orphan reconciliation.
7. Add indexed root matching, reply routing and dynamic approval addressing.
8. Add console configuration, agent authoring and deterministic completion.
9. Run the connector and whole-ticket end-to-end tests, then change maturity to
   runtime in the final commit. Complete.

## Required proofs

- A cross-scope subscription is not inspectable and produces no approval.
- A changed remote snapshot is refused before the POST.
- A Slack event replay does not increment a revision.
- Concurrent replies preserve both updates and leave one active card.
- A stranger cannot alter revision, context or recipients.
- A process killed after remote acceptance is reconciled without a second POST.
- An older result cannot complete a newer revision.
- A requested expiration outside policy is refused before approval.
- API-key and credential canaries are absent from body, headers, errors, model
  input, ledger, content artifacts, Slack and logs in raw, JSON-escaped,
  percent-encoded, base64 and normalised forms.
- The final activation test enters through `Layer.Invoke` using the real tool
  id; the whole-ticket test starts at a Slack delivery and ends at the edited
  thread.

## Deferred deliberately

- Bot-authored ticket roots and their authenticated requester claim.
- API Product subscriptions and other Management API versions.
- Reaction-based approval.
- A general governed HTTP connector.
- Manager hierarchy instead of scoped approvers.
- Cancellation of an external effect after execution has been claimed.

## Related

- [NT-005](NT-005-interaction-channels.md) - conversation scope and authority
- [NT-009](NT-009-governed-connectors.md) - connector execution boundaries
- [NT-011](NT-011-durable-agent-execution-and-workflow-engines.md) - orphaned effects and recovery
- [DP-001](DP-001-data-protection.md) - content placement and erasure
