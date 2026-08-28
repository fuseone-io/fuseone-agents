import type { Step } from "@/lib/api/client";

/**
 * Which steps reference stored content, and which of those a memory may cite.
 *
 * Its own module because two screens ask different questions of the same rule.
 * The trail asks whether there is anything to open; the memory flow asks
 * whether this step can be evidence, and the answers are not the same — a tool
 * result has content and cannot be cited, because the server resolves citations
 * against what a run published, not against everything it stored.
 *
 * Neither is a mirror of the server's rule. hasContent is looser than any
 * server rule by design, and citableAsEvidence is deliberately tighter — see
 * the note on it. Where a difference is accidental rather than chosen, the
 * server's rule is the one that matters: it refuses a citation it cannot
 * resolve, so the worst a loose reading does is offer a button that leads to a
 * refusal. A tight one hides a button nobody knows to ask for.
 */
export function hasContent(step: Step): boolean {
  const payload = (step.payload ?? {}) as Record<string, unknown>;
  return Boolean(
    payload.args_ref ||
    payload.result_ref ||
    payload.input_ref ||
    payload.outcome_ref,
  );
}

/**
 * Whether this screen offers to teach a memory from this step.
 *
 * The closing answer of a finished run, or an artifact that run published.
 *
 * The server resolves a third form, and this leaves it out on purpose: the
 * arguments of a `$fuseone.memory.suggest` tool call, cited as the artifact
 * `memory_suggestion` (citationIn, internal/memory/citation.go). That form
 * exists so the platform can resolve provenance for proposals the agent already
 * made, and every one of them is already in the review queue. Offering "remember
 * this" on it would open a second way to reach the same fact — one that skips
 * the queue and leaves the proposal pending against a memory that now exists.
 * Somebody who wants that proposal accepts it; somebody who wants a different
 * fact teaches it from the run that showed it.
 *
 * So this is narrower than the server, and the narrowing is the decision. A
 * later form the server learns to resolve is a different question — that one
 * belongs here unless there is a reason like this one to keep it out.
 */
export function citableAsEvidence(step: Step): boolean {
  if (step.kind !== "run_finished") return false;
  const payload = (step.payload ?? {}) as Record<string, unknown>;
  const artifacts = payload.artifacts;
  return Boolean(
    payload.outcome_ref || (Array.isArray(artifacts) && artifacts.length > 0),
  );
}
