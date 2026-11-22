import type { CorrectedCase } from "@/features/agents/correction-options";
import type { Run, Step } from "@/lib/api/client";

type Act = NonNullable<CorrectedCase["acted"]>[number];

/**
 * A rehearsal read as a case to correct (NT-018 phase 5b), folded as a simulation report
 * folds one: each decision the Gate made is an act at the step planned before it, and the
 * call after it says the act reached the tool layer. A decision naming no tool is nothing
 * an author could correct.
 */
export function rehearsalCase(run: Run, steps: Step[]): CorrectedCase {
  const acted: Act[] = [];
  let node = "";
  let last: Act | undefined;
  for (const step of steps) {
    const payload = (step.payload ?? {}) as { node?: string; tool?: string };
    if (step.kind === "planned") node = payload.node ?? "";
    if (step.kind === "gate_decided") {
      last = payload.tool ? { step: node || undefined, tool: payload.tool, reached: false } : undefined;
      if (last) acted.push(last);
    }
    if (step.kind === "tool_called" && last) last.reached = true;
  }
  return {
    runId: run.runId,
    settled: run.phase === "finished" ? "finished" : undefined,
    acted,
  };
}
