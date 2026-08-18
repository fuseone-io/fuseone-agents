# FuseOne Agents

FuseOne Agents is a control plane for AI agents that work inside real business
operations: reading systems, proposing actions, asking for approval when the
risk calls for it, and leaving a traceable record of what happened.

It is built for agents that touch CRM, ERP, support queues, observability
systems, internal APIs and legacy tools. The product is not a connector suite
and not a generic chat shell: MCP servers are tools, channels are how people
talk to agents, and the Gate is the boundary every external effect crosses.

It installs into the customer's own environment. One binary, one PostgreSQL,
one Helm chart.

## What it does

- Runs authored agents from manual starts, webhooks, events and channels.
- Connects MCP tool servers, discovers their tools, lets an operator choose
  the surface area, and requires a Curator to classify what each tool can do.
- Evaluates every tool call through a deterministic Gate before anything
  reaches the outside world.
- Carries untrusted labels from inputs, logs, tool results and agent-to-agent
  events so a later write cannot quietly launder risky context.
- Records runs in an append-only, hash-chained ledger for replay, audit,
  budget accounting and incident review.
- Stores large or sensitive run content behind references, so retention and
  erasure can remove what a run carried without rewriting the audit chain.
- Ships with a console for authoring, approvals, run inspection, MCP
  governance, audit trail, data retention, branding and the in-product manual.

## Why it is different

Most automation platforms ask whether an integration can call an API. FuseOne
asks a different question first: who decided this agent may do this thing, with
this input, in this scope, at this cost?

Everything important is a projection of the same ledger. The audit trail, a
run's current state, cost accounting, replay and simulation are reads over one
record, not parallel systems that have to stay in sync. The Gate's ruling is
written before the effect happens, and a grant can release an action that only
needed approval; it cannot override a check that blocked it.

Functional reference: [docs/PRD-001-fuseone-agents.md](docs/PRD-001-fuseone-agents.md).

## Installing

```sh
helm install agents oci://ghcr.io/fuseone-io/charts/fuseone-agents \
  --namespace fuseone --create-namespace \
  --set secret.existingSecret=fuseone-agents \
  --set baseUrl=https://agents.example.com
```

Images are built in the open for `linux/amd64` and `linux/arm64`, and signed
with no key at all — the signature's identity is the workflow that built it.
This runs inside your network where you cannot watch it build, so check it
before it does:

```sh
cosign verify ghcr.io/fuseone-io/fuseone-agents:0.3.3 \
  --certificate-identity-regexp '^https://github.com/fuseone-io/fuseone-agents/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Installing, operating and proving it to an auditor:
[deploy/helm/fuseone-agents/README.md](deploy/helm/fuseone-agents/README.md).

## Releases

A release is a `v`-prefixed tag. `make release V=x.y.z` refuses a dirty tree,
runs the full local gate, creates the tag, and lets CI publish the versioned
image, `latest`, the OCI Helm chart, and a GitHub Release whose notes come from
the matching `CHANGELOG.md` section.

## Running it locally

```sh
make dev      # database, api, worker, console, and stand-ins for the model
              # provider and an MCP server — everything, one command
make stop     # end it, from any terminal
make reset    # throw the development database away and start over
```

`make dev` prints where the console is and the setup token to claim the
installation with. Nothing external is needed: no API key, no network.

```sh
make check       # build, vet, generated-code drift, tests, race
make check-pg    # the same suites against a real PostgreSQL
make smoke       # what the shipping binary serves, and what it refuses to
```

## Layout

```
cmd/agentd/     the product: serve | worker | start | bootstrap | keygen | migrate
cmd/devstack/   local stand-ins for a model provider and an MCP server
internal/
  domain/       core types. No I/O, stdlib only
  ledger/       the append-only hash-chained ledger and its projections
  gate/         the seven checks and four verdicts
  engine/       the loop: fold the ledger, decide the next action
  worker/       leases runs and advances them
  admin/        what operators change, and the record that they did
  auth/         OIDC, sessions, delegation
  httpapi/      HTTP + the OpenAPI contract's implementation
api/            openapi.yaml — the contract both sides are generated from
web/            the console: React 19 + Vite + shadcn/ui
docs/           the PRD and the notes that argue each design decision
```

## The reasoning, written down

Every consequential decision here has a note arguing it, including what it
gives up. They are worth more than the code comments for anyone deciding
whether this design fits their problem.

| | |
|---|---|
| [PRD-001](docs/PRD-001-fuseone-agents.md) | What the product is, requirement by requirement |
| [DP-001](docs/DP-001-data-protection.md) | What is stored, what leaves the installation, and what can be erased |
| [OP-001](docs/OP-001-running-an-installation.md) | Installing it, the decisions an operator owns, and what to expect when something is wrong |
| [NT-001](docs/NT-001-integration-boundary-and-execution-model.md) | Where MCP ends and integration begins |
| [NT-003](docs/NT-003-conversational-authoring.md) | Authoring an agent by conversation |
| [NT-004](docs/NT-004-ledger-volume-and-paging.md) | What the ledger costs at volume, measured, and why it is partitioned on the run's opening time |
| [NT-005](docs/NT-005-interaction-channels.md) | Channels, and why Slack and WhatsApp are two products |
| [NT-006](docs/NT-006-evaluating-agents.md) | Evaluating agents, and why not to adopt a harness |
| [NT-007](docs/NT-007-drawing-a-process.md) | A canvas that authors the stages, without the specification becoming a picture |
| [NT-008](docs/NT-008-a-catalogue-by-shape.md) | The tool servers to ship, chosen by shape and never by vendor |

Engineering rules are in [CLAUDE.md](CLAUDE.md) for the Go core and
[web/CLAUDE.md](web/CLAUDE.md) for the console. Everything written down is in
English, including commits and these documents: the repository is public, and a
document half its readers cannot read is one that does not get reviewed.
