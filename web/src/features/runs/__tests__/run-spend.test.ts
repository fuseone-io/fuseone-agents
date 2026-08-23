import { describe, expect, it } from "vitest";
import { runSpend } from "@/features/runs/run-spend";
import type { Step } from "@/lib/api/client";

function planned(prompt: Record<string, unknown>): Step {
  return {
    seq: 1, kind: "planned", at: "2026-08-23T00:00:00Z", hash: "h",
    payload: { prompt },
  } as Step;
}

describe("what a run spent", () => {
  it("says a rate is missing rather than letting zero read as free", () => {
    // The number is the same either way. Without the reason, an operator
    // cannot tell a cheap run from a model nobody priced — which is exactly
    // the confusion configured rates were made honest to avoid.
    const spend = runSpend({ micros: 0, inputTokens: 4000 }, []);

    expect(spend.priced).toBe(false);
    expect(spend.reason).toBe("no_rate");
  });

  it("does not call a run free when it truly spent nothing", () => {
    // No tokens either: nothing happened, and that is not a pricing problem.
    const spend = runSpend({ micros: 0 }, []);

    expect(spend.reason).toBe("nothing_spent");
  });

  it("sums prompt composition across turns without inventing tokens", () => {
    const spend = runSpend({ micros: 900, inputTokens: 100 }, [
      planned({ unit: "content_bytes", instructions: 10, tool_results: 90 }),
      planned({ unit: "content_bytes", instructions: 10, tool_results: 400 }),
    ]);

    expect(spend.bytes).toEqual({ instructions: 20, tool_results: 490 });
    // Bytes are measurement and tokens are billing. A component that added
    // them, or derived one from the other, would be inventing a rate.
    expect(spend).not.toHaveProperty("bytesPerToken");
  });

  it("ignores a composition that does not declare content bytes", () => {
    // The unit is written into the payload so a future unit cannot be summed
    // into this one by accident.
    const spend = runSpend({ micros: 900 }, [planned({ unit: "tokens", instructions: 50 })]);

    expect(spend.bytes).toEqual({});
  });
});
