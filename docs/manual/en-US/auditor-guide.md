---
title: Auditor guide
summary: How to verify a run, inspect what was stored, understand erasure, and export evidence without trusting a screenshot.
section: reference
tags: audit, ledger, verification, export, content store, erasure, evidence
order: 15
---

## The ledger is the source of truth

Every run is a chain of steps. Each step points to the previous one, so a
changed step breaks verification. The console view, exports and verification
all read the same chain.

An audit should start with the run id, not with a screenshot. Open the run,
verify the trail, then inspect the steps that matter.

## What lives in a step

A step records facts the platform must keep:

- what opened the run;
- which version was pinned;
- who the run acted on behalf of;
- the Gate verdict and rule;
- cost, token and prompt-size measurements;
- references and digests for bulky content.

The step should not carry large or personal payloads inline. Tool arguments,
tool results, run input, final answers and shared context artifacts live in the
content store. The ledger records a reference and a digest.

That distinction matters: the ledger is long-lived evidence; the content store
is where retention and erasure apply.

## Verify before interpreting

Use **Verify the trail** on the run. Verification recomputes the chain and
reports the first step that does not match.

If verification fails, stop interpreting the run as evidence until the
integrity problem is resolved. A broken chain means the record no longer says
what happened.

## Opening sealed content

Opening arguments or results is a deliberate action because those bytes may
contain personal data or third-party content. When content still exists, the
screen shows the bytes and the digest ties them to the step. When content was
erased, the screen says erased rather than empty.

Empty, missing and erased are different facts:

| State | Reading |
|---|---|
| Empty | The run produced no content for that field |
| Missing | The referenced object cannot be found |
| Erased | Retention or a data-subject request removed the bytes |

Do not treat an erased answer as "the agent said nothing". It said something;
the bytes are no longer retained.

## Export evidence

Export the run when a review needs to leave the console. The export is useful
because it carries step data, references, digests and the verification result.

Do not replace an export with copied text from a tool result. Copied text loses
the digest, the Gate decision that allowed it, and the labels it carried.

## Erasure and retention

Erasure reaches content, not the immutable ledger. The ledger keeps the fact
that content existed, with its reference and digest, so the platform can still
explain the run without retaining the personal data itself.

Operational channel rows have their own retention. Rows that still owe a
reply are preserved until the platform has either answered or no longer has a
deliverable run to answer from.

## What to check for common questions

### "Who authorised this?"

Read `on behalf of` on the run and the approval step if one exists. A run
opened by Slack watched messages uses the configured `runAs` principal, not
the author of the Slack message.

### "Why did this write not happen?"

Read the Gate step. A data barrier or destructive action blocks outright. An
approval-needed verdict waits for a person. Approval releases only the exact
call and arguments that were approved.

### "Did this use another agent's context?"

Look for the `$fuseone.context.read` tool result. The trail names the artifact,
source run and digest. The content is read through a platform tool, not copied
into the listener's opening prompt.

### "Can I compare this to production logs?"

Yes. Tool calls record references and timing, but not secret headers or
credentials. Compare by run id, step number, tool name and external system
timestamps.

For day-to-day diagnosis, start with [Reading a run](reading-a-run.md). For
data-flow boundaries, read [Data labels and barriers](data-barriers.md).
