# Changelog

What changed between versions, written for the person who has to decide whether
to upgrade an installation that is currently working.

Versions are [semantic](https://semver.org). A release is a `v`-prefixed tag,
and pushing one builds the image and chart that carry that version — nothing is
released by merging.

## What each section is for

**Upgrade notes** comes first when it exists, and it is the only section that
is not optional. This platform refuses by default: a permissive value standing
in for "nobody has said" is a bug here, so several changes deliberately *stop*
things that used to work until somebody decides. An operator who meets that
without warning reads governance as breakage and works around it, which is the
one outcome worth writing a changelog to prevent.

**Added**, **Changed**, **Fixed**, **Security** describe what a reader would
notice. A refactor nobody can observe does not belong in any of them.

Entries say what happens, not what was implemented. "Local tool servers stop
until somebody accepts running them" is useful; "added AcceptsLocalExecution
field" is a commit message.

---

## [Unreleased]

### Added

- **Try now** on a tool server: the worker reconnects it on request instead of
  waiting for the next sweep, and says what it found.
- Remote servers can authenticate with a single custom header or with Basic
  credentials rendered as one.

## [0.2.0] — 2026-08-17

### Upgrade notes

- **A writable `/tmp` is mounted.** The chart already ran with a read-only root
  filesystem, and a local tool server's sealed configuration file is
  materialised under the system temp directory — so that feature could not
  work in any installation until now. Nothing to do before upgrading; this
  fixes a shape that was already broken.
- **Replicas are spread across nodes where the cluster allows it.** Scheduling
  never refuses over it — `ScheduleAnyway` — so nothing goes Pending, but pods
  may land on different nodes than before.

### Added

- The connection form asks for the credential the server actually takes. A
  recipe that authenticates by OAuth no longer offers a bearer field, and a
  shape the runtime cannot yet send — custom headers, basic auth, a connection
  string — is shown as documented rather than dressed up as a token.
- The chart declares a pod and container security context, so a cluster
  enforcing Pod Security Standards `restricted` will run it. The root
  filesystem is read-only, with a writable `/tmp` for the config file a local
  tool server reads.
- Replicas spread across nodes where the cluster allows it, and schedule anyway
  where it does not.
- Optional Gateway API routing as an alternative to Ingress, and an optional
  egress NetworkPolicy, both off by default.
- Six more servers in the catalogue, all incident response and observability:
  PagerDuty, incident.io, New Relic, Honeycomb, ServiceNow and Elastic Agent
  Builder. Forty-one in total.
- Six more remote-first servers in the catalogue, focused on Cloudflare and
  Vercel: Cloudflare API, Documentation, Observability, Audit Logs and Workers
  Builds, plus Vercel. Forty-seven in total.

## [0.1.0] — 2026-08-17

The first tagged release. Installations before it ran untagged builds pinned by
commit, so the notes below apply to them: this is an upgrade, not a first
install, for anyone already running one.

### Upgrade notes

- **Local (stdio) tool servers stop until somebody accepts them.** A local
  server is a program the platform starts inside the worker, running as the
  worker and reaching whatever it reaches. That is now an explicit decision,
  recorded with the name of whoever made it, and a stored server carries no
  such acceptance. Re-save each local server from the console to continue.
  Remote servers are unaffected.
- **A local server no longer inherits the worker's environment.** It receives
  only what it is configured with, so one that relied on a variable the worker
  happened to hold needs that variable set on the server itself.
- **Tools whose definition changed are refused until re-ruled.** A ruling now
  names the definition it judged. Rulings made before this release carry no
  such record and keep working unchanged; a tool a server has since redefined
  reads as needing review.

### Added

- A catalogue of thirty-five tool servers the platform knows about — identity,
  documentation, how each is usually reached, the credential it expects, and
  suggested classifications where the publisher documents its tool names. A
  recipe fills a form and decides nothing.
- Per-server tool surface: of what a server offers, choose what this
  installation brings in. Outside it a tool is not restricted — it is not
  there, and no agent is told about it.
- OAuth for remote tool servers, including refreshing an expired access token
  and persisting a rotated refresh token before it is used.
- A sealed configuration file for local servers, materialised when the process
  starts and removed when it stops.
- Events between agents may carry a context key and artefact references.
- Slack conversations can start runs: a mention becomes an ask, and a refusal
  is said back in the thread it was asked in.

### Changed

- Tool classification happens on the server's own page, beside the tools it
  offers. The administration area keeps the queue of what is waiting.
- Long inventories are paged.

### Security

- A run started by an event inherits the labels and the delegating person of
  the run that emitted it. Composition no longer launders taint or acquires
  authority nobody granted.
- A correspondent may open a bounded number of runs in a rolling window, taken
  before the run opens rather than counted after.
