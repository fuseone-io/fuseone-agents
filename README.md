# FuseOne Agents

A governed platform for running AI agents inside real business operations:
agents that act across CRM, ERP and legacy systems under explicit rules, with a
traceable record of every decision and hard ceilings on what they can spend.

It installs into the customer's own environment. One binary, one PostgreSQL.

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
cosign verify ghcr.io/fuseone-io/fuseone-agents:0.1.0 \
  --certificate-identity-regexp '^https://github.com/fuseone-io/fuseone-agents/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Installing, operating and proving it to an auditor:
[deploy/helm/fuseone-agents/README.md](deploy/helm/fuseone-agents/README.md).

## The idea in one paragraph

Everything is a projection of an append-only, hash-chained ledger. The audit
trail, the cost accounting, a run's state, and the ability to replay a run are
all reads over the same record — not four systems that have to agree. No effect
reaches the outside world without passing a deterministic Gate, and the Gate's
ruling is written down before the effect happens.

Functional reference: [docs/PRD-001-fuseone-agents.md](docs/PRD-001-fuseone-agents.md).

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
