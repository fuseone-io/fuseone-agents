---
title: Integrations
description: MCP servers, governed connectors, channels and native platform tools.
---

FuseOne separates integration paths by the kind of control the platform can
honestly provide.

## MCP servers

MCP servers expose tools. Operators choose which tools are visible, and a
Curator classifies the effect of each tool before published agents can rely on
it.

HTTP MCP traffic refuses environment proxies and blocks cloud metadata and
link-local destinations. Stdio MCP servers can be routed through the local
egress proxy with a per-server allow-list, and the console shows whether the
operator has declared that deployment NetworkPolicy enforces the proxy path.

## Governed connectors

Governed connectors are first-party shapes. They are used when the platform
should own the operation contract, secret handling, storage semantics and
approval posture instead of delegating all of that to an MCP server.

The first runtime connector is Vault. The catalogue also names shapes for SQL
reads, object storage, identity actions, DNS, Kubernetes, SMTP and governed
HTTP. A catalogue entry is not the same as a ready runtime adapter; the console
must not imply a connector is operational until the runtime exists.

## Channels

Channels are how people talk to agents and how agents report back. Channel
delivery is operational evidence, not a run step. The runtime cockpit can show
stable channel failure codes without exposing conversation identifiers.

## Native platform tools

Some tools are part of FuseOne itself: memory, context sharing and duplicate
effect recognition. They are governed by the same Gate and label rules as
external integrations.

## Related notes

- [NT-001: integration boundary and execution model](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/NT-001-integration-boundary-and-execution-model.md)
- [NT-009: governed connectors](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/NT-009-governed-connectors.md)
