import { describe, expect, it } from "vitest";
import { hasContent } from "@/features/runs/step-content-ref";
import type { Step } from "@/lib/api/client";

function step(kind: string, payload: Record<string, unknown>): Step {
  return { kind, payload } as unknown as Step;
}

describe("what a step offers to open", () => {
  it("opens anything that references stored content", () => {
    expect(
      hasContent(step("tool_returned", { result_ref: "run://x/2/ab" })),
    ).toBe(true);
    expect(hasContent(step("planned", {}))).toBe(false);
  });
});
