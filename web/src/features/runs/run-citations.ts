import type { Step } from "@/lib/api/client";

/**
 * What a step lets a memory point at, and what such a memory would carry.
 *
 * Reading the trail the console already holds, so the person deciding to teach
 * a fact sees what the platform will record before they decide. The server
 * proves all of it again from the ledger and its answer is the one that counts
 * — nothing here is trusted on the way in, because none of it is sent: the
 * request names a run and an artifact, and the digest and labels come back from
 * the resolver.
 */

/** The name the platform gives a run's closing answer. */
export const FINAL_ANSWER = "final_answer";

export interface Citation {
  seq: number;
  artifact: string;
  digest: string;
}

/**
 * The citations a step offers, closing answer first.
 *
 * A reference and a digest are written together about the same bytes, so an
 * artifact carrying only one of them is refused by the server before anything
 * else looks at it. Offering it would be offering a button whose one outcome is
 * a refusal, so it is left out here too.
 *
 * The server resolves a third form this leaves out on purpose: the arguments of
 * a `$fuseone.memory.suggest` tool call, cited as the artifact
 * `memory_suggestion` (citationIn, internal/memory/citation.go). That form
 * exists so the platform can resolve provenance for proposals the agent already
 * made, and every one of them is already in the review queue. Offering "remember
 * this" on it would open a second way to reach the same fact — one that writes
 * the assertion and leaves the proposal pending against it. Somebody who wants
 * that proposal accepts it; somebody who wants a different fact teaches it from
 * the run that showed it.
 *
 * So this is narrower than the server, and the narrowing is the decision. A
 * later form the server learns to resolve is a different question — that one
 * belongs here unless there is a reason like this one to keep it out.
 */
export function citationsOf(step: Step): Citation[] {
  if (step.kind !== "run_finished") return [];
  const payload = (step.payload ?? {}) as Record<string, unknown>;
  const out: Citation[] = [];
  if (payload.outcome_ref && typeof payload.outcome_digest === "string") {
    out.push({
      seq: step.seq,
      artifact: FINAL_ANSWER,
      digest: payload.outcome_digest,
    });
  }
  const artifacts = Array.isArray(payload.artifacts) ? payload.artifacts : [];
  for (const entry of artifacts) {
    const a = (entry ?? {}) as Record<string, unknown>;
    if (typeof a.name !== "string" || typeof a.digest !== "string") continue;
    if (!a.ref || !a.digest) continue;
    out.push({ seq: step.seq, artifact: a.name, digest: a.digest });
  }
  return out;
}

/**
 * Whether this screen offers to teach a memory from this step.
 *
 * Defined by what there is to cite rather than beside it. Written as its own
 * rule it drifted immediately: it read "the run published artifacts" while
 * citationsOf refuses an artifact the ledger only half recorded, so a button
 * appeared over a panel with nothing in it. One rule, asked two ways.
 */
export function citableAsEvidence(step: Step): boolean {
  return citationsOf(step).length > 0;
}

/**
 * The labels a memory cited at this step would carry, or null when the trail
 * loaded so far cannot answer.
 *
 * A fold up to the cited step, which is the server's rule: what a step produced
 * is not what the run knew, and a clean answer inside a poisoned run is still a
 * fact the poison reached.
 *
 * Null rather than an empty set, and checked here rather than remembered by the
 * caller. The trail arrives a page at a time from the run's first step, so a set
 * loaded halfway is a prefix — and a fold over a prefix that stops short of the
 * cited step returns fewer labels, not none. That is the dangerous direction:
 * the screen would understate the taint of the thing somebody is about to teach,
 * and nothing on it would say so. Both conditions below mean "not enough trail",
 * never "no labels".
 */
export function labelsUpTo(steps: Step[], seq: number): string[] | null {
  if (steps[0]?.seq !== 1) return null;
  if (!steps.some((s) => s.seq === seq)) return null;
  const labels = new Set<string>();
  for (const s of steps) {
    if (s.seq > seq) continue;
    for (const label of s.labels ?? []) labels.add(label);
  }
  return [...labels].sort();
}
