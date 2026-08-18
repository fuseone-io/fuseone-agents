# OP-001 — Running an installation

**Written** 2026-08-18, against 0.3.3.

For whoever installs this in a customer's environment and keeps it running.
The PRD says what the platform is for and the NTs argue about how it works;
this says what an operator has to do, decide and expect.

Everything here has been exercised. Where something has not, it says so.

---

## 1. What this platform refuses by default, and why that is not breakage

Most of what surprises a new operator is deliberate. This platform treats
**absence of a decision as a refusal**, everywhere, because the alternative is a
default that grants something nobody chose.

Concretely, on a fresh install and after several upgrades:

- **A discovered tool does nothing.** Connecting a tool server imports its
  tools and every one arrives unclassified. The Gate refuses an unclassified
  tool, so an agent that names one stops. Somebody has to say what each tool
  does to the world.
- **A local (stdio) server does not start until somebody accepts it.** A local
  server is a program this platform runs inside the worker, as the worker,
  reaching what the worker reaches. That is a decision with a name attached to
  it, and a stored server carries no acceptance until an administrator gives
  one.
- **A run will not finish without somewhere to put its answer.** From 0.3.3 the
  worker writes a run's closing answer to the content store, behind a
  reference, so retention and erasure reach it like everything else. It refuses
  to finish rather than falling back to writing the text into the chain, where
  no erasure request could ever reach it. An installation deployed by this
  chart always has that store; a run failing on it means the worker was wired
  by hand and the wiring is incomplete.
- **A published agent is paused and in draft.** Draft cannot open a real run.
  It can be simulated, which is how it earns its way out.
- **A tool whose definition changed waits for a fresh ruling.** A ruling names
  the definition it judged; when a server redefines a tool under the same name,
  the old ruling stops applying.

None of these is a fault to work around. An operator who reads them as breakage
disables the thing that makes the platform worth installing.

## 2. Installing

The chart is in `deploy/helm/fuseone-agents`. Two secrets have no default and
never will: `DATABASE_URL` and `FUSEONE_MASTER_KEY`. The master key is **32
bytes, base64** — `agentd keygen` prints one.

```
helm upgrade --install fuseone-agents deploy/helm/fuseone-agents \
  -n <namespace> --create-namespace \
  --set image.tag=0.3.3 \
  --set secret.existingSecret=<secret> \
  --set ingress.enabled=true --set ingress.host=<host> \
  --set baseUrl=https://<host>
```

Migrations run as a job before the pods that need them, and take an advisory
lock, so a second one waits rather than racing.

**The first person to open the console claims the installation** with a setup
token the API prints once at start-up:

```
kubectl -n <namespace> logs -l app.kubernetes.io/component=serve --tail=100 | grep setup
```

After that the token stops working. If everyone loses access, `agentd bootstrap
--reopen "<reason>"` issues a new one and **records the reopening in the
administrative trail with that reason** — it is a recovery path, not a back
door, and it leaves a mark.

## 3. The decisions an operator owns

These cannot be defaulted, which is why they are work rather than
configuration.

**Classifying tools.** What a tool does to the world — read, write,
destructive, financial — plus whether its results are untrusted, and what
undoes it. The suggestion a recipe ships is a proposal with reasoning; it never
applies itself. Untrusted is the one people skip and should not: it is what
makes a later write stop for a person.

**Choosing each server's surface.** Of what a server offers, which tools this
installation brings in. Outside the surface a tool is not restricted — it is
not there. This is also the only thing that bounds a credential that is wider
than it should be: a classification bounds what a tool does, never where.

**Accepting local execution**, per local server, with a name recorded.

**Budgets and ceilings**, per scope. A run has one, a scope has one, and a
channel correspondent has one.

## 4. Credentials

Everything is sealed with the master key. A credential is never returned by a
listing — only whether one is stored.

- **Remote servers** take a bearer, OAuth, a single custom header, or Basic
  rendered as one. A rotated OAuth refresh token is persisted before it is
  used.
- **Local servers** take environment variables and a configuration file, both
  sealed. The worker materialises the file when the process starts, at `0600`
  inside a `0700` directory, and removes it when the process stops. It does
  **not** inherit the worker's environment: a local server sees only what it
  was configured with.
- **Scope the credential where it is issued.** A token that reaches an entire
  account gives every tool on that server that reach. This platform cannot
  narrow it and does not pretend to.

Removing a credential is its own gesture, on the server's page. An empty field
means "leave what is stored", so there has to be a way to say the other thing.

## 5. Upgrading

Read `CHANGELOG.md` first, and its **Upgrade notes** section before anything
else — it is where a release says what it stops.

```
helm upgrade fuseone-agents deploy/helm/fuseone-agents \
  -n <namespace> --reuse-values --set image.tag=<version>
```

Known, and learned the hard way:

- **0.2.0 and 0.3.0 cannot deploy** unless `worker.specs.configMap` is set. Go
  to 0.3.1 or later.
- **Before 0.3.3, a run's closing answer was written into the immutable chain**
  and no erasure request could reach it. Upgrading stops that for new runs;
  runs already recorded keep their answer inline for ever, because the chain is
  not rewritten. An installation with a data subject request covering
  historical runs should read DP-001 before answering it.
- **Before 0.3.1, `--reuse-values` failed** when the target version had added a
  chart value.
- **Before 0.3.2, `worker.specs.configMap` never worked** at all.

Rulings made before a version that changed how they are recorded keep working.
That is deliberate: an upgrade that invalidated every existing decision would
stop every agent on the installation.

## 6. What to expect when something is wrong

**An agent stops at its first tool call.** Almost always an unclassified tool,
or one outside the server's surface. The administration area keeps a queue of
what is waiting; the console says which.

**A tool server shows as not answering.** Its credential, its address, or the
server itself. **Try now** asks a worker to reconnect through the same path it
uses in production and report what it found — a failed attempt leaves the
existing session alone.

**A run is parked.** It is waiting for a person. The reason says why, and the
reason is worth reading: "this writes" and "this writes in a run that read
somebody else's text" are different sentences and different risks.

**The queue stops draining.** A worker has no HTTP surface to probe, so a
wedged one shows as work not moving rather than as a port not answering. Look
at the worker's log and the run's last step.

## 7. What survives, and what does not

The ledger is the record: append-only, hash-chained, and every projection is a
fold of it. `agentd verify` checks a signed export.

**Back up the database.** Everything else — the catalogue, the classifications,
the channels, the branding — lives in settings rows beside it. Credentials are
sealed with the master key, so **a backup without that key restores an
installation that cannot talk to anything.** Keep them separately and keep both.

Content a run read or wrote is held by reference with a digest. Erasing it
never touches the chain, which is what makes deletion and audit coexist.

## 8. What this document has not exercised

Written honestly, so nobody reads confidence into silence:

- Restoring from a backup has not been rehearsed here.
- A multi-worker installation has run, but not under load worth reporting.
- OAuth has been exercised with a stored grant, not through a browser consent
  flow — that flow does not exist yet.
- The channel path has been exercised end to end in tests, and in the lab only
  as far as configuration.
