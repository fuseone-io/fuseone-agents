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
 * A mirror of the server's rule, and mirrors drift. The one that matters is the
 * server's: it refuses a citation it cannot resolve, so the worst this can do is
 * offer a button that leads to a refusal. Offering one that never appears is the
 * failure that hides.
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
 * Whether a memory may cite this step.
 *
 * The closing answer of a finished run, or an artifact that run published.
 * Narrower than hasContent on purpose: the platform resolves a citation by
 * looking at what the run finished with, so a step that merely holds bytes is
 * not something a person can point a memory at yet.
 */
export function citableAsEvidence(step: Step): boolean {
  if (step.kind !== "run_finished") return false;
  const payload = (step.payload ?? {}) as Record<string, unknown>;
  const artifacts = payload.artifacts;
  return Boolean(
    payload.outcome_ref || (Array.isArray(artifacts) && artifacts.length > 0),
  );
}
