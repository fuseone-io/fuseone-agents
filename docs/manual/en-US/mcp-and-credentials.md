---
title: MCPs and credentials
summary: How to connect servers, choose a surface, classify tools and decide who supplies the credential.
section: integrations
tags: mcp, credential, surface, classification, oauth, stdio, local
order: 5
---

## The catalogue is a recipe, not a connector

A recipe says how a server is usually reached, which credentials it expects
and which tools its documentation publishes. It is not endorsement and it does
not apply trust by itself.

Connecting does not classify. Classifying does not bring a tool into the
surface. A credential does not narrow reach. They are three different
decisions.

## Transport

| Transport | When to use it | Care |
|---|---|---|
| Remote HTTP | Preferred in production | The platform sends only the configured credential |
| Local stdio | Useful for development or servers with no remote option | Runs as the worker, inside the worker container |

A stdio server does not inherit the worker environment. It receives only the
platform allowlist and the variables configured for it. If it needs a file,
the platform seals the content, materialises it in a 0700 directory with a
0600 file, and removes it when the process stops.

## Surface

The surface says which tools from this server exist for this installation.
Outside the surface a tool is not forbidden: it does not exist for agents.

A new tool on a server with a chosen surface starts outside the surface. That
is the safe behaviour: novelty does not enter reach by accident.

## Classification

Each tool needs a Curator decision:

- effect: read, write, destructive or financial;
- whether the result is untrusted;
- which tool compensates it, when one exists.

The decision carries the digest of the definition that was judged. If the
server changes a tool's schema or description, the old decision becomes stale
and the Gate refuses until a fresh review.

## Installation credential

Use an installation credential when the server acts as a service account:
discovery, health checks, probes and tools that do not represent a particular
person.

That credential is shared by every run that uses the server in that mode. If
the token reaches the whole account, the platform cannot narrow it; surface
and classification only control what agents may try.

## Personal credential

Use a personal credential when the server expects user authority: Google
Workspace, Slack OAuth, personal GitHub, Atlassian and similar cases.

The credential is sealed by server and principal. A run can use it only when
it carries `OnBehalfOf` for that principal. Scheduled runs have no person; if
the recipe says the credential is personal, they stop instead of falling back
to the installation credential.

## Authentication shapes

The catalogue declares the shape, but the runtime sends only what was
configured.

| Shape | What to configure |
|---|---|
| Bearer | Simple token |
| Header | One or more exact headers required by the server |
| OAuth | Access token, refresh token, token URL, client id and secret |
| Basic | Ready-made header, when the server expects Basic |
| DSN | Connection string |
| Env | Variables for stdio |
| Config file | Sealed document materialised in the worker |

Do not fill two shapes at the same time. The screen refuses that so a person
does not configure one thing while the runtime sends another.

## Quick diagnosis

- Server is "answering" but a tool fails with personal credential: configure
  the credential in Credentials for the user that started the run.
- Tool is classified but the Gate refuses: check stale digest or whether the
  tool is outside the surface.
- HTTP MCP fails on protocol: set the server protocol mode to legacy when the
  recipe or server requires it.
- stdio does not start after an upgrade: explicitly accept local execution.
