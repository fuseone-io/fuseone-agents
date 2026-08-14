# NT-007 · Drawing a process

**Status** Proposal · **Date** 2026-08-14
**References** [PRD-001](PRD-001-fuseone-agents.md) — N5, §3.2, FU-08, FU-13, FU-17, FU-18, SE-07 · [NT-003](NT-003-conversational-authoring.md) · [NT-006](NT-006-evaluating-agents.md)
**Outcome** A canvas that authors the stages, without the specification ever becoming a picture

The design handoff describes a visual builder: a component rail, a canvas, an
inspector. The PRD refuses one — as the *primary* interface. §3.2 settles the
apparent contradiction: prose remains the way in, and drawing is offered beside
it for the author who would rather draw.

This note is about the part that is not a matter of taste. A builder can be
added without changing what the platform is, or it can quietly replace the
artefact everything else in this product depends on. The difference is three
decisions, and they are all made here.

---

## 1. What is drawn, and what is stored

An agent has two halves and they are not the same kind of thing.

| | Instructions | Steps |
|---|---|---|
| Who reads it | The model, and an auditor | The Gate |
| Shape | Prose, in the author's words | A sequence, each with `reaches` |
| Authored by | A person, always | A person, or a proposal a person accepts |
| Approved as | Itself (FU-08) | Part of the definition |

The canvas draws and edits **the steps**. It does not author the instructions.
Generating those from fields would produce, by machine, the one part of a
definition that exists to be read by people — and FU-08 asks for approval of
the prose, not of the boxes.

The reverse direction is already built and is the model for this one: the
assistant reads the instructions and proposes the stages, and what the author
leaves on screen is what gets published. Drawing is the same contract in the
other direction — the canvas proposes a paragraph, the author edits it, and it
is theirs when they publish.

## 2. The layout is derived, never stored

Nothing about the picture is persisted: no `position`, no node identifier, no
edge handle. The layout is recomputed on every read from the step sequence.

This is not tidiness. A stored position is a second artefact that can disagree
with the text, and it disagrees silently — a version whose steps changed and
whose saved coordinates did not draws a diagram of an agent that never existed.
The diagram appears on approval screens and in audit records (FU-17), so the
requirement is exact: **the same version draws the same picture, at any time,
on any machine.** Deriving it every time is what makes that true by
construction rather than by discipline.

The handoff's serpentine grid is a deterministic function of the sequence — an
ELK layered pass, wrapped into at most four columns, alternating direction —
so it satisfies this as written, provided the column count depends on the
canvas size and nothing else. A layout that depended on when it ran, or on
which nodes were dragged last, would not.

### 2.1 What that costs

Dragging a node cannot mean what it means in a diagramming tool. There is
nowhere to keep "the author put this box here", and inventing one is the
decision this note refuses.

So the canvas is not free-form: moving a card reorders the sequence, and the
grid re-derives. That is a smaller gesture than a whiteboard offers and it is
the honest one — the sequence is the fact, and the position is a rendering of
it.

## 3. A proposal is never a grant

Whatever an author draws, a step may only narrow the capability pack. A tool
named on the canvas that the agent does not hold is dropped, exactly as one
the assistant proposes is (§3.2).

This matters more on a canvas than in a form, because a rail of draggable
components looks like a catalogue of things you may have. It is a catalogue of
things that exist; what this agent may reach is the pack, and the pack is
granted elsewhere, by somebody else, for a reason.

## 4. Two shapes, and what they are allowed to mean

The handoff draws two: a step card, and a condition as a pill — "so a decision
reads as a connector, not a card".

The card maps to `spec.Step` exactly. The pill does not map to anything: the
specification has no branch. `stops_when` is an exception in the author's own
words and **nothing judges it** — NT-003 left who decides a step is over open
on purpose, and that is still open.

So the pill renders `stops_when` and offers no way to author a condition that
the platform would then have to evaluate. Drawing a branch the runtime cannot
take would be the worst kind of builder: one that lets somebody express a
process the platform will not follow, on a screen that looks like it will.

## 5. What this does not settle

**The Gate still does not obey the steps.** `EnvelopeAt` has no caller: today
the whole pack applies from the first turn to the last. Everything above is
about authoring a declaration; making it a permission is a separate piece of
work in the engine, and it needs a run to know which step it is at.

Until that lands, a canvas draws something true about the definition and
nothing about what the run will be allowed to do, and the screen should not
imply otherwise.

**Composition between agents stays out.** The events an agent emits are a
different graph — which agent triggers which — and it already has a screen. A
canvas that mixed the two would put "what this agent does" and "what happens
afterwards" in one picture, and they are answerable by different people.
