import { describe, expect, it } from "vitest";
import {
  citableAsEvidence,
  hasContent,
} from "@/features/runs/step-content-ref";
import type { Step } from "@/lib/api/client";

function step(kind: string, payload: Record<string, unknown>): Step {
  return { kind, payload } as unknown as Step;
}

describe("what a step offers", () => {
  it("opens anything that references stored content", () => {
    expect(
      hasContent(step("tool_returned", { result_ref: "run://x/2/ab" })),
    ).toBe(true);
    expect(hasContent(step("planned", {}))).toBe(false);
  });

  // Narrower than hasContent, and deliberately so: the server resolves a
  // citation against what a run published, so a tool result holds bytes and
  // still cannot be evidence. Offering a button that leads to a refusal is the
  // failure people report; offering one that never appears is the one they do
  // not.
  it("only lets a memory cite what a finished run published", () => {
    expect(
      citableAsEvidence(step("run_finished", { outcome_ref: "run://x/9/ab" })),
    ).toBe(true);
    expect(
      citableAsEvidence(
        step("run_finished", { artifacts: [{ name: "report" }] }),
      ),
    ).toBe(true);
    expect(citableAsEvidence(step("run_finished", {}))).toBe(false);
    expect(
      citableAsEvidence(step("tool_returned", { result_ref: "run://x/2/ab" })),
    ).toBe(false);
  });
});
