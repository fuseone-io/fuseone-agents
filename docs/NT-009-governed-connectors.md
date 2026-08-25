# NT-009 — Governed connectors

Status: accepted, first catalogue slice

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

## First slice

The first implementation is deliberately a catalogue only:

- it creates no credentials;
- it starts no worker;
- it exposes no tool to an agent;
- it performs no network or secret-store call;
- it lists only planned connector shapes and the contract each future runtime
  must satisfy.

This avoids the worst intermediate state: a screen that looks ready enough for
an operator to trust, while the runtime still lacks the controls the screen
implies.

## Initial connector shapes

- Vault secret storage: write generated keys, certificates and bundles from
  content references; read metadata without returning secret values.
- Approved automation jobs: run registered job templates such as CSR
  generation without giving the model shell access.
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
- Write, destructive, financial and secret effects must be visible before the
  operation can execute, so the Gate can stop or ask.
- Job runners must run approved templates, not arbitrary command strings.
- Generic HTTP is a bridge, not a permanent shape. A common workflow should
  graduate into a named connector with narrower operation semantics.
- The catalogue is not evidence that an installation can reach the external
  system; reachability belongs to a connector instance and its health.

## Performance

The catalogue is static and read-only. It does not query credentials,
discover surfaces or contact external services. A future runtime must preserve
that distinction: health and execution state belong to instances, while this
catalogue remains the low-cardinality product contract.
