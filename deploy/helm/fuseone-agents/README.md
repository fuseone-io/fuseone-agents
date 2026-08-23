# Installing FuseOne Agents

One chart, one binary, one PostgreSQL. No orchestration cluster, no external
queue, no time-series database (PRD DE-01).

```sh
helm install agents oci://ghcr.io/fuseone-io/charts/fuseone-agents \
  --version 0.1.0 \
  --namespace fuseone --create-namespace \
  --set secret.existingSecret=fuseone-agents \
  --set baseUrl=https://agents.exemplo.com
```

From a clone, `deploy/helm/fuseone-agents` stands in for the `oci://` reference.

## What you are installing

Every image is built by a workflow in the open, published to
`ghcr.io/fuseone-io/fuseone-agents` for `linux/amd64` and `linux/arm64`, and
signed with no key at all — the signature's identity is the workflow that built
it. This software is installed inside your network, where you cannot watch it
build, so verify it before it runs:

```sh
cosign verify ghcr.io/fuseone-io/fuseone-agents:0.1.0 \
  --certificate-identity-regexp '^https://github.com/fuseone-io/fuseone-agents/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Each image also carries its build provenance and a bill of what is in it:

```sh
docker buildx imagetools inspect ghcr.io/fuseone-io/fuseone-agents:0.1.0 \
  --format '{{ json .SBOM }}'
```

## The two things with no default

**The database.** The chart does not bring one. Bundling PostgreSQL in a chart
is how an installation ends up with its production database in a StatefulSet
nobody backs up — and backup and restore here are PostgreSQL operations plus
object storage, with no proprietary procedure (DE-17). Point it at the database
your organisation already knows how to run.

**The master key.** It seals every stored credential — model provider keys,
tool server tokens. Losing it means re-entering all of them; leaking it means
they are all readable. It is never a default and never a value on a container.

```sh
kubectl -n fuseone create secret generic fuseone-agents \
  --from-literal=DATABASE_URL='postgres://agents:...@postgres/agents?sslmode=require' \
  --from-literal=FUSEONE_MASTER_KEY="$(head -c 32 /dev/urandom | base64)"
```

`secret.values.*` exists for a first look. It puts both in your values file,
and therefore in whatever holds your values file.

## What it installs

| | |
|---|---|
| `-serve` | The API and the console, in one process. Stateless; scale freely. |
| `-worker` | The pool that drains the run queue. Several are safe: the queue is claim-based with leases, and a run has one writer at a time. |
| `-worker` service | Optional Prometheus metrics for worker movement. It exposes counters and gauges, not run content or diagnostics. |
| `-migrate` | A pre-install and pre-upgrade hook. Nothing serves against a schema it does not know. |

## The probes are not the same question

Liveness touches nothing outside the process. One that asked the database would
restart every pod during a database blip, turning a recoverable outage into a
crash loop across the installation.

Readiness does ask, and answers 503 with the reason. A pod that cannot read the
ledger leaves the load balancer's rotation and is not restarted for it.

Workers expose `/metrics` when `worker.metrics.enabled` is true. That listener
is observational, not a health check: a stuck worker is a queue that stops
draining, not a port that stops answering.

## Where agents come from

Publishing an agent is an interface action, not a deploy (DE-07). An
installation authors through the console — the interview, the editor, the
template catalogue — and the registry in PostgreSQL is what a worker resolves
from. Nothing has to be redeployed to publish an agent, and no volume is
needed to run one.

`worker.specs.configMap` mounts a directory of `*.agent.md` files. It is a way
to *seed* an installation from a repository, not the way to run one: what it
holds is published at start-up and then lives in the registry like everything
else.

## First sign-in

The API prints a setup token once at start-up. The first person to use it
claims the installation, and it stops working.

```sh
kubectl -n fuseone logs -l app.kubernetes.io/component=serve --tail=100 | grep setup
```

After that, sign-in goes through the identity provider configured in the
administration area — the platform keeps no password store (DE-04).

## Upgrading

`helm upgrade` runs the migration hook first, then rolls the workers one at a
time and the API normally. Two workers of different versions can share the
queue safely: a run is pinned to the agent version it started with, and the
older one stays the only correct explanation of the runs that used it (DE-09).

## How much it will hold

Measured, not estimated: a million steps seeded into PostgreSQL 18.

| | |
|---|---|
| One step, indexes included | **863 bytes** |
| A busy installation at 200k steps a day | ~55 GB of ledger a year |
| Partitions, one per month | created a year ahead, automatically |

The ledger only grows. Nothing updates it and nothing deletes from it, by
design and by trigger — corrections are new steps, and erasure reaches the
content a step references and never the step (AU-04, NF-09). Plan storage for
the record you are keeping on purpose.

`run_steps` is partitioned by the month its run opened, so a year nobody reads
can be detached and moved to slower storage as a unit. A worker keeps twelve
months ahead of the clock; if it is ever stopped for longer than that, steps
land in the default partition and are recorded correctly — they simply cannot
be archived as a month. The log says so when it happens.

## What to watch

| Signal | Means |
|---|---|
| `readyz` answering 503 | This pod cannot read the ledger. The reason is in its log, not in the reply — the endpoint answers without a credential, so it says what is unavailable and not which database. |
| `healthz` answering anything but 200 | The process itself is wrong. This is the only one worth restarting a pod for. |
| "steps are in the ledger's default partition" | The partition job has been stopped for over a year, or could not create a month. Nothing is lost; archiving that month is. |
| Budget alerts in the console | An agent, area or company approaching a ceiling — before it stops work mid-afternoon, which is the alternative (FO-05). |
| Runs parked and not decided | Somebody is waiting on a person. Point a channel at the scope and they will be told (NT-005). |

## Backing it up

There is no proprietary procedure, and that is deliberate (DE-17). Two things
hold state:

**PostgreSQL.** Back it up the way your organisation already backs up
PostgreSQL — `pg_dump`, a physical base backup with WAL archiving, a managed
snapshot. The ledger, the projections, the registry, the configuration and the
administrative trail are all in it.

**The master key**, which is *not* in the database and must be backed up
separately. Restoring a database without it gives you every run, every decision
and every trail — and no readable credential. That is a recoverable state (an
operator re-enters the provider keys) but a surprising one to discover during
a restore.

Restoring is restoring PostgreSQL. Nothing in the platform has to be told it
happened: state is a fold of the ledger, so a restored database produces the
same answers the old one did.

Content under retention is the one thing that changes shape: a restore brings
back bytes an erasure had already removed. Run the retention sweep after a
restore, or a subject's erasure request is quietly undone by it.

## Proving it to an auditor

The point of the record is that somebody outside can check it. Two commands do
that, and neither needs this platform to be trusted.

**Re-check a run's chain**, from the console or the API: every step is hashed
onto the one before it, so an altered step breaks the chain at the step that
was altered and names it.

**Verify an exported bundle offline.** The console exports a run — or a
period — signed with the installation's key. The bundle carries the public half
with it, so checking it needs no network and no access here at all:

```sh
agentd verify audit-2026-08.json
# OK — 1,284 steps, chain intact, signature valid
#   company     acme
#   signed by   a3f1-9c2e-…
#
# Compare the fingerprint against the key the installation publishes.
# Nothing in the file can tell you it is theirs.
```

That last line is the part that matters, and the command prints it because it
is easy to skip. A bundle carrying its own key proves only that it is
internally consistent — anybody can sign anything with a key they made up and
put that key in the file. What ties it to *this* installation is comparing the
fingerprint against the one the installation publishes at
`GET /api/v1/audit/signing-key`, which is short enough to read out loud over a
phone. That comparison is the whole trust anchor; without it the signature is
arithmetic.

What the bundle proves is that the record was not altered after the fact. It
does not prove the agent behaved well — that is what a reviewed simulation and
the evaluation corpus are for (FU-10, and [NT-006](../../../docs/NT-006-evaluating-agents.md)).
