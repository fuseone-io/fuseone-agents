# NT-007 — Standing approvals, and the taint waiver we rejected

Status: accepted · 2026-10-08

## The problem

An agent whose entire input is untrusted content — a log-sweeping security
responder is the canonical case — cannot act autonomously. `checkTaint`
(SE-06) sends every write whose arguments derive from untrusted data to a
human, and for this class of agent that is every meaningful write. At three
in the morning the approval card reaches a channel where nobody is awake,
and the action the agent exists to take does not happen.

The pressure to "just let policy override it" is real and was asked for
directly. This note records why we said no, and what we built instead.

## Rejected: a policy- or catalog-declared taint waiver

Two designs were considered that would let `checkTaint` pass for specific
tools: an authored policy allow that also waives taint, and a catalog-level
declaration ("this operation is designed for untrusted arguments behind
structural guards") with a policy as a second key.

Both were rejected against the written security model:

- SE-06 names **exactly one** exception: *"The Gate rejects a high-effect
  action whose arguments derive from untrusted data, **except with explicit
  human approval**."* No policy exception appears anywhere in section 10.4.
- The rationale box after SE-07 calls taint the check the others depend on:
  without it, *"malicious content read at step 2 becomes the premise of the
  action executed at step 6, and the other six checks are circumvented by
  text that came from outside."*
- AU-17: *"If only one line of this subsection survives review, it is this
  one."*
- The autonomy manual commits that even the autonomous stage does not waive
  it: *"taint still stops a write, a ceiling still holds."*

The concrete blast radius settled it. Policies are rows written through the
console in thirty seconds, and the first policy an operator wrote for this
very feature carried `tool: *`. Had a policy allow been able to cancel
taint, that one line of configuration would have reopened prompt injection
for every tool its agent touches. A defense whose off-switch is a data row
is not the defense SE-06 describes.

## Built instead: the human approval, made durable

The Gate already has exactly one downgrade path, and it is SE-06's
exception: a human's approval releases a final RequireApproval, never a
Block, applied after all checks. A **standing approval** is that approval
given ahead of time:

> One person pre-approves **one exact tool** (never a pattern) for **one
> agent** in **one scope**, with a **daily ceiling** (at most 100) and an
> **expiry** (at most 90 days — renewing is a re-decision), and writes the
> **reason** down.

Mechanically the Gate is untouched. The park still happens; the engine then
asks the standing store to *claim* a covering grant — coverage and spending
one of today's uses are a single transaction, so two runs cannot spend the
same slot — and appends the same `approval_decided` step a click produces,
with the grant owner's name and the mandate's id in the note. The fold, the
projection, the model's transcript and the card closer all read an
ordinarily approved run. Expiry, revocation, a spent ceiling, a store
error, or no store at all leave the park standing for a human: the grant
path fails closed in every direction.

Granting needs `approval:grant` (Admin only) — approving a class of future
actions is strictly more than clicking one card, which stays with the
Approver. Creation and revocation land in `admin_events`; every use does
too, and the count of today's uses *is* the ceiling's ledger.

## What this deliberately does not solve

A standing approval releases the park; it does not make the tool safe. That
remains the tool's own duty — the Cloudflare block-list connector carries
its guards in code (literal addresses only, protected and reserved ranges
refused unconditionally, its own daily ceiling, expiring blocks) and that
is what makes a standing grant for it a defensible decision. Granting one
for a tool without structural guards is possible and is the grantor's
responsibility; the reason field exists so they write that judgement down.
