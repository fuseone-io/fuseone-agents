import type { Expectation } from "@/features/agents/regressions-api";
import type { SimulationCase } from "@/features/agents/simulation-api";

/**
 * What an author might want to say about a case, read from what it did.
 *
 * A blank form asking for an expectation is a form only somebody who already
 * knows the vocabulary can fill. The case just showed what happened, so the
 * corrections worth offering are the ones about that: this call should never
 * happen, this one should have, you should have been asked.
 *
 * Each option carries the step it came from (FU-13), so a correction about the
 * reply step does not start failing when the lookup step changes.
 */
export interface CorrectionOption {
  /** Stable across renders, so a selection survives a refetch. */
  key: string;
  expectation: Expectation;
  /** Catalogue key for the sentence shown, with `tool` interpolated. */
  label: string;
  tool?: string;
}

export function correctionOptions(entry: SimulationCase): CorrectionOption[] {
  // Keyed, because an expectation is about a tool at a step: a case that
  // proposed the same call three times is one correction to make, and
  // offering it three times invites recording it three times.
  const seen = new Map<string, CorrectionOption>();
  const offer = (option: CorrectionOption) => {
    if (!seen.has(option.key)) seen.set(option.key, option);
  };

  for (const act of entry.acted ?? []) {
    const at = act.step ? `${act.step}:` : "";
    if (act.reached) {
      // It would have happened. The correction worth the whole mechanism.
      offer({
        key: `never:${at}${act.tool}`,
        tool: act.tool,
        label: "correction.neverCalls",
        expectation: { kind: "never_calls", step: act.step, value: act.tool },
      });
      offer({
        key: `asks:${at}${act.tool}`,
        tool: act.tool,
        label: "correction.asksFirst",
        expectation: { kind: "asks", step: act.step, value: act.tool },
      });
    } else {
      // It was proposed and stopped. Sometimes that is the bug.
      offer({
        key: `calls:${at}${act.tool}`,
        tool: act.tool,
        label: "correction.shouldCall",
        expectation: { kind: "calls", step: act.step, value: act.tool },
      });
    }
  }

  // Where it ended is worth correcting on its own, and it is the only thing
  // left to say about a case that proposed nothing at all.
  for (const settled of ["finished", "awaiting_approval"] as const) {
    if (entry.settled === settled) continue;
    offer({
      key: `settles:${settled}`,
      label:
        settled === "finished"
          ? "correction.shouldFinish"
          : "correction.shouldAsk",
      expectation: { kind: "settles", value: settled },
    });
  }

  return [...seen.values()];
}
