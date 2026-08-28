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

/**
 * The name the platform gives a run's closing answer, and one no published
 * artifact may take.
 *
 * A citation naming it is resolved from the finished step's outcome_ref before
 * the server looks at what the run published (citationIn), so an artifact under
 * this name is bytes no citation can reach. The engine now refuses to publish
 * one, but runs that finished earlier still carry them and the ledger does not
 * change — and listing one here would show two entries under a single name,
 * with the memory taught from the second citing the first.
 */
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

  const answer = cite(step.seq, FINAL_ANSWER, payload.outcome_ref, payload.outcome_digest);
  if (answer) out.push(answer);

  const artifacts = Array.isArray(payload.artifacts) ? payload.artifacts : [];
  for (const entry of artifacts) {
    const a = (entry ?? {}) as Record<string, unknown>;
    const named = recorded(a.name);
    if (named === undefined || named === FINAL_ANSWER) continue;
    const artifact = cite(step.seq, named, a.ref, a.digest);
    if (artifact) out.push(artifact);
  }
  return out;
}

/**
 * One citation, or nothing when the ledger did not finish writing it.
 *
 * Both halves through the same test and compared against undefined rather than
 * for truth. Written as `if (ref && digest)` the emptiness rule would live at
 * the call site instead of in `recorded`, which is how a rule ends up at two
 * strengths — and a sabotage that removed it from `recorded` went unnoticed,
 * because the call site was quietly enforcing it a second time.
 */
function cite(
  seq: number, artifact: string, ref: unknown, digest: unknown,
): Citation | undefined {
  const held = recorded(digest);
  if (recorded(ref) === undefined || held === undefined) return undefined;
  return { seq, artifact, digest: held };
}

/**
 * The value a ledger field holds, or undefined when it holds nothing.
 *
 * One rule for every part of a citation, because written as separate checks
 * they came out at three different strengths: a reference that only had to be
 * truthy, a digest that only had to be a string and so could be empty, and a
 * name that was never checked for content at all. Each of those produced a
 * citation the server refuses — the safe direction of the asymmetry, and still
 * a button offered over a record the ledger did not finish writing.
 */
function recorded(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
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
