# NT-009 — Governed connectors

Status: accepted; Vault and governed SQL reads have runtime adapters

## Decision

FuseOne will support first-party governed connectors for workflows that should
not be modeled as arbitrary MCP tools or channels. A governed connector is a
small set of native operations with declared effects, approval posture, secret
handling and runtime limits. The model asks for an operation; the platform
validates the operation, stores or reads content by reference, and records the
effect before anything reaches the external system.

This is different from an MCP server recipe. A recipe says what somebody else
publishes and helps an operator connect it. A governed connector is a platform
contract: FuseOne owns the operation shape and can enforce the guarantees it
renders.

## Runtime model

The catalogue describes connector shapes. Runtime configuration creates
connector instances. An executable tool is the pair of one instance and one
operation, rendered as:

```text
<connector>.<instance>.<operation>
```

For example, the catalogue operation `vault.write_secret` becomes
`vault.prod.write_secret` for the `prod` Vault instance. The instance name is
part of the tool id so two Vaults cannot collapse into one capability.

A connector runtime is a FuseOne-owned tool layer, not a side channel:

- the model sees only schemas for tools in the run's pack;
- the Gate sees the catalogue effect before the external system is reached;
- arguments and results are claim-checked through the content store;
- configuration changes are settings plus an admin event;
- credentials are sealed settings and are revealed only by the worker during
  execution.

An instance can be installation, company or area scoped. The runtime may execute
only when the instance scope contains the run scope. A tool id named from a
different area may still appear in a stale agent definition, but the call fails
closed before the connector reaches the external system.

## First slice

The first implementation was deliberately a catalogue only:

- it creates no credentials;
- it starts no worker;
- it exposes no tool to an agent;
- it performs no network or secret-store call;
- it lists only planned connector shapes and the contract each future runtime
  must satisfy.

This avoids the worst intermediate state: a screen that looks ready enough for
an operator to trust, while the runtime still lacks the controls the screen
implies.

The first runtime slice starts with Vault because it exercises the hardest
constraint: secret material may move, but it must not become ordinary model
text. It supports generic secret writes from named content references, metadata
reads and lease revocation. Certificate generation itself belongs to an
approved job connector; Vault stores the generated material after it is already
in the content store.

The SQL runtime is the next completed slice. An enabled SQL instance registers
one or more read-only templates and binds to a Vault database secrets-engine
role. A model-visible call contains only a template id and typed parameters.
After the Gate and any required approval, the worker resolves the binding,
asks Vault for one short-lived credential, opens one TLS-verified PostgreSQL
connection, describes the statement against the database, runs it in a
read-only transaction and streams a bounded result into the content store.
The effective execution budget is enforced twice: by the worker deadline and
by PostgreSQL `statement_timeout` plus `idle_in_transaction_session_timeout`.
The connection is closed before the lease is revoked; TTL remains the
backstop after ambiguous worker loss.

Rows use the database codecs rather than whatever Go type pgx happens to
choose. UUIDs and network identifiers are canonical text, exact `numeric`
values are decimal strings, and `bytea` is base64 in JSON. This keeps keys
recognisable and numbers lossless when a result is audited or used as input to
a later call.

Approval binds both halves of the act: the digest of model-authored parameters
and a versioned digest of the server-owned target, Vault binding and endpoint,
selected query, parameter declarations and limits. The latter is recomputed
after an approval and checked again against current settings before opening
the database. Changing a template under the same id or repointing the named
Vault therefore asks again instead of borrowing the earlier decision.

SQL does not reuse the MCP result cache or a dynamic credential. Repeating a
read is another execution: it crosses the Gate again and, when policy asks,
needs another human decision before another credential is issued. Safe
provenance records the SQL instance, Vault instance, role, lease duration and
issuance/revocation outcomes. Username, password, Vault token, DSN and lease id
are never part of the tool schema, result, ledger or metrics.

SQL instance authoring currently belongs to the administration API. The
console exposes the safe configured summary and no editor until it can round
trip the complete target, binding and template contract; a Vault-shaped form
must never rewrite an SQL instance.

## Rollout order

1. Vault secret storage.
2. Approved automation jobs, including certificate/CSR generation templates.
3. Governed SQL read templates. (runtime)
4. Object storage for governed artifacts.
5. Identity actions.
6. DNS and Kubernetes operational connectors.
7. Outbound email.
8. Governed HTTP as the bridge for workflows that have not earned a named
   connector yet.

## Initial connector shapes

- Vault secret storage: write generated keys, certificates and bundles from
  content references; read metadata without returning secret values.
- Approved automation jobs: run registered job templates such as CSR
  generation without giving the model shell access.
- Governed SQL read access: run registered read-only templates against
  approved schemas with declared columns, filters and row limits.
- Object storage: move governed artifacts through content references inside
  approved buckets and prefixes.
- Identity actions: read principals and perform narrow account actions with
  stable subject identifiers.
- Kubernetes operations: read diagnostics and perform narrow operational
  writes under namespace and verb policy.
- DNS management: read, upsert and delete records inside approved zones.
- Outbound email: send approved templates without becoming a conversational
  channel.
- Governed HTTP: call declared internal endpoints while a dedicated connector
  does not exist yet.

## Security invariants

- Secret values should move as content references by default. Returning
  plaintext is a separate runtime decision and must not be implied by the
  catalogue.
- Write, destructive and financial effects must be visible before the operation
  can execute, so the Gate can stop or ask.
- Connector reads are untrusted sources by default. A first-party connector
  governs shape, scope and storage, not the truth of data returned by an
  external system. A static flow from a connector read into a non-reversible
  effect must block publication unless a later connector explicitly proves a
  narrower trust boundary.
- Secret movement is not a separate effect. It is declared through
  `secretHandling`, because the Gate's effect ladder and the data-handling
  contract answer different questions.
- Job runners must run approved templates, not arbitrary command strings.
- SQL connectors must run approved read-only templates, not arbitrary query
  strings supplied by the model.
- Object payloads should move as content references unless a runtime explicitly
  declares and gates plaintext.
- Identity actions must resolve display names to stable provider identifiers
  before any write or destructive operation reaches the provider.
- Generic HTTP is a bridge, not a permanent shape. A common workflow should
  graduate into a named connector with narrower operation semantics.
- The catalogue is not evidence that an installation can reach the external
  system; reachability belongs to a connector instance and its health.

## Performance

The catalogue is static and read-only. It does not query credentials,
discover surfaces or contact external services. A future runtime must preserve
that distinction: health and execution state belong to instances, while this
catalogue remains the low-cardinality product contract.
