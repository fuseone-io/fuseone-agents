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
| `-migrate` | A pre-install and pre-upgrade hook. Nothing serves against a schema it does not know. |

## The probes are not the same question

Liveness touches nothing outside the process. One that asked the database would
restart every pod during a database blip, turning a recoverable outage into a
crash loop across the installation.

Readiness does ask, and answers 503 with the reason. A pod that cannot read the
ledger leaves the load balancer's rotation and is not restarted for it.

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
