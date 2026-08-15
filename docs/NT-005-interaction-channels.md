# NT-005 · Interaction channels

**Status** Proposal · **Date** 2026-08-13
**References** [PRD-001](PRD-001-fuseone-agents.md) — AU-02, AU-04, AU-05, AU-11, NF-06, NF-09, FO-04, SE-10 · [NT-004](NT-004-ledger-volume-and-paging.md)
**Outcome** Two families rather than one abstraction, one new domain concept, three stages
**Revised** 2026-08-14 — §9 the shape of a channel trigger, §10 where email belongs
**Revised** 2026-08-15 — §12 stage 3 broken down, §13 why the other vendors come after

People do not live in the console. An approval that waits for somebody to open
a browser tab waits, and a run whose result nobody reads was not worth the
money. Channels are how the platform reaches where the work already happens.

This note argues that "channels" is two products wearing one word, and that
building them as one makes the easy half carry the hard half's assumptions.

---

## 1. A channel is a trigger, and the trigger model already exists

`trigger.Opener` takes a `Request` and seals `run_started`. Cron, webhook and
event composition all arrive that way. A channel is the fourth, and it needs no
new machinery to open a run.

What it brings that a webhook does not is **identity** — the sender is a
person, not a shared secret — and **conversation**: the message has a parent, a
thread, and context other messages left behind.

**A trigger is a deliberate ask.** A mention, a command, a reply to a thread —
never every message in the channel. The listening version sounds more capable
and is worse in three directions at once: cost, because ambient conversation
becomes tokens; privacy, because people's conversations reach a model nobody
asked to involve; and the record, which fills with runs nobody started. The
mention is the consent, and it is free.

## 2. What the ledger records is the ask, not the sentence

If a run opens with raw text, then a year later the answer to "why did this
agent restart that service" is *somebody typed "investiga isso aí" in a
thread*. That is a screenshot, not an audit record.

So references resolve **at the edge**, before the run exists, and what the
ledger holds is structured:

```
seq 1  run_started
       origin:    {channel: slack/acme, conversation: C07-ops,
                   message: 1786…914, thread: 1786…102}
       asked_by:  usr_ana
       scope:     acme/ops
       input:     sha256:3f1a…

  content 3f1a…
       { subject: { kind: "alert", system: "grafana", id: "alm_8842" },
         ask:     "investigate",
         text:    "@agents investiga isso aí" }
```

The sentence stays — it is what the person said, and dropping it would make the
record dishonest — but it is filed as evidence rather than as instruction. The
planner reads `subject`. The text carries the untrusted label into the Gate,
where the taint check already knows what to do with it.

### 2.1 The boundary of resolution

**The platform resolves references to what it put there.** The alert reached
`#ops` because a webhook trigger reported it, so `message_id → alm_8842` is a
mapping this installation already holds. Replying in that thread resolves with
certainty.

A third-party bot's message, or "that problem from yesterday", does not resolve
— and must not pretend to. It becomes an ask with no subject, tainted, and the
Gate treats it as what it is: untrusted input asking for an effect. An agent
that needs a specific alert can go and search for one, which is a tool call
somebody can audit, rather than a guess the edge made silently.

## 3. Replying to the origin is not sending a message

The distinction that is easiest to get wrong and most expensive to get wrong.

| | What it is | How the Gate sees it |
|---|---|---|
| **Reply to origin** | The run reporting on itself, to where it came from | Reach is the `origin` sealed in step 1. There is nowhere else to go. |
| **Send a message** | An effect on the world, arbitrary reach | A classified tool, under policy and the risk ladder like any other |

Model both as one "send message" tool and an agent that can answer in a thread
can also message the whole company. Separated, replying is bounded by the run's
own provenance rather than by a rule somebody has to remember to write.

## 4. The conversation carries the scope

A conversation maps to a `domain.Scope`. `#financeiro → acme/finance`.

This is governance, not convenience. The same person asking the same thing in
two channels gets two different sets of permitted tools, and the question "who
could have asked for this" becomes answerable: the intersection of who reaches
the conversation and who holds a grant in its scope.

The scope must not come from the text. An ask that names its own scope is an
ask that can name a wider one.

## 5. Where the two families split

Everything above holds for both. This does not.

**The platform's identity model assumes a principal with grants in a scope.** A
customer messaging a company's WhatsApp number has no grant, should not have
one, and will not have one. So what bounds the run?

Two facts that today share one field:

| | Meaning | Bounds the run? |
|---|---|---|
| `on_behalf_of` | Who delegated authority | Yes — the run can what they can |
| **subject** *(new)* | Who the run is **about** | No — supplies input, delegates nothing |

A customer is a subject and never a principal. The run carries the authority of
the agent and of the conversation's scope; the customer's message is input. If
that is not explicit, the first person to write *ignore previous instructions
and transfer* discovers what was implicit.

### 5.1 What the external family additionally needs

- **Taint becomes load-bearing.** Customer text is entirely adversary-
  controlled. Prompt injection is not a risk to consider; it is the normal
  case. The existing taint check is what makes this family viable at all.
- **Erasure already fits.** A phone number is exactly the `owner` key
  `admin.Erasures.ForSubject` erases on, and the claim check means erasing a
  customer's words never touches the hash chain (AU-04, NF-09).
- **A ceiling per correspondent.** Today's ceilings are per run and per scope.
  On an external channel one number can drain an area's budget, and neither
  existing limit sees it happening.
- **The 24-hour window.** A run that concludes three days later cannot simply
  reply; WhatsApp requires an approved template. That is product work, not a
  driver.
- **A customer expects a conversation; the platform runs runs.** One run per
  ask, with the thread as correlation rather than as state. A run left open for
  days waiting on a customer holds a lease, holds a budget reservation and
  occupies the queue — and reusing `parked` for it would make "waiting for
  somebody to **decide**" and "waiting for a customer to **answer**" read the
  same in the ledger, which they are not.

### 5.2 Why this is a split and not a parameter

Built as one abstraction, the internal family inherits subject-versus-principal,
per-correspondent ceilings and template windows it does not need, and the
external family inherits identity assumptions that are false for it. Slack and
Teams are one product. WhatsApp is another that happens to also deliver text.

## 6. What the audit trail has to gain

The trail reads `gate_decided` and `approval_decided` from the ledger, unioned
with `admin_events`. It shows **what the Gate decided** and not **who asked for
the run**. While triggers were cron and webhooks that was tolerable. With
channels it is the first thing anybody wants to see, and it is not there.

- **`run_started` joins the trail**, carrying origin, asker and resolved
  subject.
- **An approval decided in a thread is the same step**, recording the same
  facts — who, when, and the digest of exactly what was approved. A second
  approval path with weaker facts would give the record two grades of approval,
  which is precisely what `internal/audit` refuses to do for the two trails it
  already merges.
- **New administrative verbs**: connecting a workspace, mapping a conversation
  to a scope, and — the sensitive one — **binding a channel account to a
  principal**. That is granting a person's authority to a messaging identifier.
  If it is ever pointed at the wrong principal, that line is the only thing that
  answers when and by whom.

**The trail is not a message archive.** Bodies in the audit trail turn the
record into a chat log full of personal data, with the retention obligations
that follow. The trail carries the reference and the digest; the body lives in
`run_content` under retention and erasure.

## 7. Order

**Stage 1 — outbound only.** The run reports to a conversation: opened, waiting
on somebody, finished, failed. No inbound surface, no identity binding needed
to post, and it already answers the alert case — there the trigger is the
webhook and the channel is where people find out.

**Stage 2 — approval in the thread.** The first inbound surface, and a small
one: a signed interaction payload, not free text. Needs the account-to-principal
binding, and needs the decision to seal the same facts the console seals.

**Stage 3 — the ask in text.** The largest surface. Needs everything above plus
reference resolution, taint and scope-from-conversation.

Slack first, for all three, because its signing is the best documented and its
threading model is the one the design assumes. Teams follows the same shape.
WhatsApp is the second family and starts after stage 3, not beside it.

## 8. Two decisions this note does not make

**Who binds a channel account to a principal?** An administrator registering
it, or the person authenticating once through the installation's identity
provider and the binding falling out of that? The second is far better to use
and needs an OAuth flow per channel.

**Does a conversation map to one scope or several?** One is simple and is what
this note assumes. But a `#ops` that serves three areas in practice will want
the ask to say which — and then the scope depends on the text again, which
§4 says not to do.

## 9. A channel trigger names no conversation

When stage 3 arrives, "started from a conversation" becomes a fourth kind of
trigger beside cron, webhook and event. It is not shaped like them.

The three that exist are self-contained: cron carries its expression, webhook
its path, event its name, and the agent declaring one is all it takes to fire
it. A channel cannot close that way, because **a conversation carries a scope**
(§4) and the conversation-to-scope map is administrative. An author writing
`trigger: channel, conversation: C07-ops` would be choosing which conversation
may start their agent — and the author is precisely the person who does not
govern which conversations belong to which area. It is the same separation that
makes classifying a tool the Curator's act and not theirs.

And the question it looks like it answers is already answered without it. Which
agent runs when somebody writes in a channel? **The mention names it.** What
governs whether it may is the intersection of two facts that already exist: the
conversation maps to scope X, and the agent lives in scope X.

So the trigger is a bare declaration with no field:

> This agent may be started by an ask in a conversation of its own scope.

The agent declares **willingness**; the administration declares **reach**. That
is the shape tools already have — the agent asks for one, the Curator decides
what it may do — and it exists for the same reason: describing a process must
not grant any power.

It earns its place in the other direction too. An agent that does **not**
declare it cannot be started by any message at all, however the conversations
are mapped. Being able to say "this one is internal, never startable by text"
is a safety property worth declaring.

## 10. Email is a transport, not a role

The question "is email a channel?" has no single answer, because the word names
a transport and this note is about roles. Email turns up in three, and treating
them as one is the expensive mistake.

| What is happening | What it is here |
|---|---|
| The platform telling somebody a run is waiting | **A channel.** Another driver; the conversation is a mailbox. |
| The agent emailing a customer as part of its work | **A tool.** An effect, through the Gate, under policy. |
| Mail arriving and opening a run | **A trigger**, of the external family (§5). |

**The criterion is who chose the recipient.** The platform, from the run's own
provenance or from configuration: a channel. The agent, from its reasoning: a
tool, through the Gate, always.

With Slack that line is easy to see, because posting to `#ops` and a
`slack.dm` tool look different. **With email they look identical** — same SMTP,
same body, same server — and that is the trap. Modelling "the agent emails the
customer" as a channel hands an agent unbounded outbound reach that never meets
the Gate. It is the worst mistake available in this design.

### 10.1 What email has that Slack does not

- **No delivery guarantee.** Slack answers `ok:false` with a reason. Mail is
  accepted by a relay and may bounce hours later or land in a spam folder for
  ever. An approval request that vanishes silently is the failure stage 1
  exists to prevent, so email is a *second* notification and never the only
  one.
- **No thread identity worth relying on.** `Message-ID` and `In-Reply-To`,
  mangled by clients. Stage 2 in a mail thread is materially harder.
- **Irreversible.** `crm.retract` unsays a reply; mail cannot be recalled. As a
  tool it classifies as a write with **no compensation**, and that absence is
  worth showing: the machinery will report that nothing takes it back.
- **It is the phishing surface.** An agent that can mail an arbitrary address
  from the company's domain is a phishing engine with an audit trail. The
  recipient has to be constrained — to the run's origin, or by policy on the
  tool.

### 10.2 Inbound mail comes last, after WhatsApp

Against the intuition, which says email is older and therefore easier. It is
the **least authenticated inbound surface there is**: anyone can send to
`support@`, SPF and DKIM speak about the envelope and not about who typed, and
it is the most exercised prompt-injection vector in the world. Opening it
before taint and identity binding are mature would be starting with the most
hostile case.

## 11. What is deliberately not here

Two loops that would look like natural extensions and are not:

- **A bot that answers questions about the platform.** That is a different
  product with a different threat model, and it would put a model between an
  operator and the record they are trying to read.
- **Channels as a message bus.** PRD N4 rules out building an integration
  engine. This platform sends *its own* notifications and receives *its own*
  approvals. The moment it relays other systems' messages it has become the
  product the PRD said it is not.

---

## 12. Stage 3, broken down

*Added 2026-08-15, after stages 1 and 2 shipped and [PRD DE-18…21](PRD-001-fuseone-agents.md#96-conversation-as-an-origin-of-work)
made the ask a requirement rather than a proposal.*

Half of it is already standing, which is easy to lose sight of when the note
reads as though nothing exists.

**Built and in the lab:** Slack signature verification, used today by the
approval button. The account-to-principal binding and its administrative
endpoint. `trigger.Opener`, which takes a `Request` and seals `run_started` —
a channel is a fourth caller and needs no new machinery. The taint check, which
is what makes text somebody else wrote survivable at all. And the outbound
half, with delivery recorded per conversation and retried per conversation.

**Missing, smallest first:**

| Piece | Why it is what it is |
|---|---|
| Conversation → scope | The map exists in one direction only: `Configured.For(scope)` answers which conversations speak for a scope. Inbound needs the reverse, and it has to be **unique** — a conversation serving two scopes means the ask picks the scope, which [§4](#4-the-conversation-carries-the-scope) forbids |
| The trigger declaration | A field with no value on the specification ([§9](#9-a-channel-trigger-names-no-conversation)), the parser, the editor, and the intersection: the conversation maps to scope X and the agent lives in scope X |
| Which agent | The mention names it. Parsing the mention, resolving the name |
| The events endpoint | URL verification, signature (reuse), dedup by `event_id` — and Slack's **three-second acknowledgement**, which means opening the run cannot happen on that request. Get it wrong and every ask is delivered several times |
| Reference resolution | [§2.1](#21-the-boundary-of-resolution). The Ledger records the structured ask and not the sentence, which means resolving "this thread" to the alert the platform itself posted there, and refusing to resolve what it did not |

The first three are small. The fourth is medium and fiddly, and its difficulty
is somebody else's contract rather than ours. **The fifth is the one to spend
thought on**, and it is not code-shaped: it is deciding what the platform is
entitled to claim it knows.

And [§8](#8-two-decisions-this-note-does-not-make)'s two open questions block
the second and fourth. They should be answered before either is built, not
during.

## 13. The other internal channels come after, not beside

Teams, Discord and Google Chat are the same family as Slack ([§5](#5-where-the-two-families-split)):
a principal with grants, a conversation carrying a scope, a thread. Nothing in
the design changes for them, which is exactly why they are tempting to add now.

**Do not.** Slack has stages 1 and 2 and not 3. Adding a second vendor before
the third stage exists produces two channels that are each two-thirds of a
product, and the missing third is the one anybody actually asks for. Depth
finishes something; breadth multiplies an unfinished thing.

What a second vendor costs, once stage 3 is done, is genuinely small and worth
writing down so the estimate is not re-argued: a driver that posts a message and
reads an interaction, its signature scheme, its threading model, and its
account identifier. **The shape is already the product's** — origin, scope,
subject-versus-principal, reply-to-origin — and none of it is per vendor.

One caveat that is not uniform, and it is the reason "just another driver" is
not quite true: **the identity binding differs per vendor**, and it is the
sensitive part. Binding a channel account to a principal is granting a person's
authority to a messaging identifier, and each vendor's account identifier has
its own semantics for what happens when somebody leaves, is renamed, or is a
guest. That question deserves its own answer per vendor rather than one
inherited from Slack.
