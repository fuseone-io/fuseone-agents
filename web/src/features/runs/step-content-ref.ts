import type { Step } from "@/lib/api/client";

/**
 * Which steps reference stored content.
 *
 * A presentation question and nothing more: is there anything behind this row
 * to open. Deliberately looser than any rule the server enforces — a tool
 * result has content and can never be evidence — and the authoring question
 * lives apart from it, in run-citations.ts, because the two answers being
 * different is the point rather than an accident.
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
