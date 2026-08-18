# DP-001 — Data protection

For a customer's data protection officer, security reviewer or legal team,
answering the questions asked before a pilot rather than after an incident:
what this installation stores, what leaves it, what can be erased, and what
this platform cannot do for you.

Every statement below names the mechanism it rests on. Where a property is a
consequence of how something is built, it says so; where it depends on a
decision the customer makes, it says that instead. Section 8 lists what this
document does **not** establish.

Written in English because this repository is public and its documents are
reviewed in English. If it is to be handed to a DPO who reads Portuguese, it
needs a translation — that is a deliverable, not a formality, and pretending
otherwise is how a document gets approved by people who did not read it.

---

## 1. The shape that decides everything else

The installation keeps two things, and the difference between them is what
makes erasure possible at all.

**The chain.** `run_steps` records what happened, step by step, each step
sealed against the one before it by a hash. It is append-only and the database
enforces it: there is no `UPDATE` and no `DELETE`. A correction is a new step.

**The content.** `run_content` holds the bytes a step refers to — the text of a
ticket the agent read, the arguments a tool was called with, a model's reply. A
step carries a **reference and a digest**, never the bytes.

This is why an erasure does not break anything. Deleting content leaves the
chain untouched and still verifiable: every hash still checks, because no hash
ever covered the bytes — it covered the reference to them.

An installation that had recorded the content inside the steps would face a
choice between honouring an erasure request and keeping an auditable record.
This one does not have that choice to make.

---

## 2. What is stored

Roughly twenty-five tables. The ones that can hold personal data:

| Where | What it can hold |
|---|---|
| `run_content` | Everything an agent read or wrote: ticket text, email bodies, tool arguments, model replies |
| `run_steps` | References and digests, never the bytes; plus who asked, which agent, which tool, what the Gate decided |
| `principals`, `sessions`, `role_grants` | The people who use the console: identity from the customer's own provider |
| `channel_inbox`, `channel_deliveries` | Messages exchanged on a connected channel, and what was sent back |
| `admin_events`, `audit` records | Who changed what configuration, and when |

`agent_specs`, `policies`, `areas`, `scopes`, `settings` and the trigger tables
hold configuration written by the customer's own staff. They can hold personal
data only if somebody writes it into an instruction.

Everything is in the customer's PostgreSQL, in the customer's environment.
Nothing is stored by the vendor, because there is no vendor-side component.

---

## 3. What leaves the installation

This is the section that matters most, and the honest answer is that data does
leave — deliberately, to places the customer chooses.

**The model provider.** An agent works by sending text to a large language
model. That text is whatever the agent has read. The providers this platform
can be pointed at are configured by the customer, and the built-in list spans
several jurisdictions — `api.openai.com`, `api.anthropic.com`,
`api.mistral.ai`, `api.groq.com`, `api.x.ai`, `api.deepseek.com`,
`api.moonshot.cn`. **The last two are outside the EU and the US**, which is a
fact a DPO needs before choosing, not after.

There is no built-in provider that runs inside the installation. If model
inference must not leave the customer's network, the platform must be pointed
at a compatible endpoint the customer hosts. That is supported by
configuration; it is not the default.

**Remote MCP servers.** A tool server reached over HTTP is a third party the
customer connected, and tool arguments go to it. A tool server run locally over
stdio does not leave the machine — and does not inherit the process
environment: the child receives an allowlist (`PATH`, `HOME`, `TMPDIR`, `LANG`,
`LC_ALL`, `TZ`) rather than the worker's secrets.

**Channels.** A connected channel — Slack, for instance — carries messages to
that provider under the customer's own workspace agreement.

Nothing else opens an outbound connection. There is no telemetry to the vendor,
no usage reporting, no phone-home. An installation with no model provider and
no remote tool server configured makes no outbound requests at all.

---

## 4. Erasing a person's data

Two mechanisms, deliberately distinct.

**On request.** `Erase(owner, reason)` clears the content of the runs an
operator identified. The bytes become null; `erased_at` and `erased_reason`
remain as a tombstone. Something that later resolves that reference gets a
distinct error meaning *this was deliberately erased* — never confused with a
reference that was always wrong.

The erasure is itself recorded, as an admin event carrying the number of runs,
the number of objects and the reason. That is on purpose: **an erasure nobody
can account for is indistinguishable from data loss**, and a DPO asked to prove
a request was honoured needs a record that it was.

**Finding the runs is the operator's work.** The platform erases what it is
given. It does not offer "everything about this person" as a query, because it
does not index by data subject — a run is identified by agent, scope and time,
not by whose data it touched.

**What survives an erasure.** The chain: that a run happened, which agent, which
tool, what the Gate decided, and the digest of content that no longer exists.
This is the deliberate trade. If a regulator requires that no trace of the
processing survive, this platform does not meet that requirement, and no
configuration changes it.

---

## 5. Retention

Content ages out on a window the customer sets. A sweep runs daily and erases
everything older, with the reason `retention`, recording `content.expired` when
it erased anything.

The window is read on every sweep rather than cached, so shortening retention
takes effect on the next run.

There is a floor. A window below 24 hours is refused, and it is checked twice —
when the setting is written and again when the sweep reads it back. The second
check is what stops a corrupted row from becoming an installation-wide delete.

**The chain is not swept.** Retention governs content, not the record that
processing happened.

---

## 6. Secrets

Credentials — provider keys, tool server tokens, channel secrets — are sealed
with AES-256-GCM under a master key supplied as an environment variable and
never stored in the database.

Sealing is bound to context: the key identifier and the credential's context
are authenticated alongside the ciphertext, so a sealed value lifted from one
row and pasted into another fails to open rather than opening as something
else.

Sealed credentials are never returned by any endpoint that lists them. Revealing
one is a separate, deliberate act.

**The master key is not in the backup.** A database backup restored without it
yields an installation that cannot open any credential it holds.

---

## 7. What the platform does not do

Stated plainly, because each of these gets assumed:

- **It does not narrow a credential.** A token issued with more reach than an
  agent needs, reaches that far. Scope belongs where the credential is issued —
  in GitHub, in SAP, in the customer's own systems.
- **It does not classify personal data.** It does not know which field is a
  name. The taint mechanism marks content by **origin**, not by sensitivity.
- **It does not index by data subject.** See section 4.
- **It does not log tool arguments.** The tool execution path emits no log
  lines at all, so there is no second copy of the content in a log aggregator
  outside the retention window.
- **It does not anonymise or pseudonymise.** What an agent reads is stored as
  it was read, until erased.

---

## 8. What this document has not established

- **No independent audit.** Everything here was verified by reading this
  repository. No third party has assessed it.
- **No DPIA, and no legal-basis analysis.** Whether a given agent's processing
  is lawful depends on what the customer configures it to read, and this
  document does not assess it.
- **Erasure has not been rehearsed at volume.** The mechanism is tested; a
  subject request across a large installation has not been carried out.
- **No certification is claimed.** Not ISO 27001, not SOC 2, not any other.
- **The provider list may drift.** Section 3 names what the built-in list
  carries today. An installation's configuration is what actually governs where
  data goes, and it is the customer's to read.
