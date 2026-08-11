# FuseOne Agents

A governed platform for running AI agents inside real business operations:
agents that act across CRM, ERP and legacy systems under explicit rules, with a
traceable record of every decision and hard ceilings on what they can spend.

It installs into the customer's own environment. One binary, one PostgreSQL.

## The idea in one paragraph

Everything is a projection of an append-only, hash-chained ledger. The audit
trail, the cost accounting, a run's state, and the ability to replay a run are
all reads over the same record — not four systems that have to agree. No effect
reaches the outside world without passing a deterministic Gate, and the Gate's
ruling is written down before the effect happens.

Functional reference: [docs/PRD-001-fuseone-agents.html](docs/PRD-001-fuseone-agents.html).

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
docs/           PRD and technical notes (pt-BR)
```

Engineering rules are in [CLAUDE.md](CLAUDE.md) for the Go core and
[web/CLAUDE.md](web/CLAUDE.md) for the console.
