import { describe, expect, it } from "vitest";
import { correctionOptions } from "@/features/agents/correction-options";
import type { SimulationCase } from "@/features/agents/simulation-api";

function aCase(over: Partial<SimulationCase>): SimulationCase {
  return { settled: "finished", steps: 3, cost: { micros: 0 }, ...over };
}

describe("correction options", () => {
  it("offers never-call for something that would have happened", () => {
    const options = correctionOptions(
      aCase({
        acted: [
          {
            step: "Responder",
            tool: "crm.refund",
            effect: "financial",
            verdict: "allow",
            reached: true,
          },
        ],
      }),
    );

    const never = options.find((o) => o.expectation.kind === "never_calls");
    expect(never?.expectation.value).toBe("crm.refund");
    // Anchored, so correcting the reply step does not break when the lookup
    // step changes.
    expect(never?.expectation.step).toBe("Responder");
  });

  it("offers should-call for something the gate stopped", () => {
    const options = correctionOptions(
      aCase({
        acted: [
          {
            tool: "crm.lookup",
            effect: "read",
            verdict: "block",
            reached: false,
          },
        ],
      }),
    );

    expect(options.map((o) => o.expectation.kind)).toContain("calls");
    expect(options.map((o) => o.expectation.kind)).not.toContain("never_calls");
  });

  it("never offers the state the case is already in", () => {
    // "It should finish" beside a case that finished is an option that can
    // only ever be a no-op, and offering it invites recording one.
    const options = correctionOptions(aCase({ settled: "finished" }));
    const settles = options.filter((o) => o.expectation.kind === "settles");

    expect(settles).toHaveLength(1);
    expect(settles[0]!.expectation.value).toBe("awaiting_approval");
  });
});

describe("the same call proposed more than once", () => {
  it("is one correction to make, not three", () => {
    // A planner refused three times proposes three times. The author has one
    // thing to say about it, and three identical checkboxes invite recording
    // the same correction three times.
    const options = correctionOptions(
      aCase({
        settled: "parked",
        acted: [
          {
            tool: "crm.lookup",
            effect: "read",
            verdict: "block",
            reached: false,
          },
          {
            tool: "crm.lookup",
            effect: "read",
            verdict: "block",
            reached: false,
          },
          {
            tool: "crm.lookup",
            effect: "read",
            verdict: "block",
            reached: false,
          },
        ],
      }),
    );

    const calls = options.filter((o) => o.expectation.kind === "calls");
    expect(calls).toHaveLength(1);
    expect(new Set(options.map((o) => o.key)).size).toBe(options.length);
  });
});
