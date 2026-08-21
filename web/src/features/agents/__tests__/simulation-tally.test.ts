import { describe, expect, it } from "vitest";
import {
  caseNeedsLook,
  countCases,
  tally,
} from "@/features/agents/simulation-tally";
import type { SimulationCase } from "@/features/agents/simulation-api";

function aCase(over: Partial<SimulationCase>): SimulationCase {
  return { settled: "finished", steps: 4, cost: { micros: 100 }, ...over };
}

describe("countCases", () => {
  it("does not count the newline every export ends with", () => {
    // Counting it would promise the author one more case than they get.
    expect(countCases('{"a":1}\n{"a":2}\n')).toBe(2);
  });
});

describe("tally", () => {
  it("counts a blocked case apart from where it ended", () => {
    // A run refused by the Gate and carried on to a finish is still one
    // somebody has to look at, and folding it into "finished" hides exactly
    // what the simulation was run to find.
    const counted = tally({
      id: "sim-1",
      running: false,
      cases: [
        aCase({
          acted: [
            {
              tool: "crm.refund",
              effect: "financial",
              verdict: "block",
              reached: false,
            },
          ],
        }),
        aCase({}),
      ],
    });

    expect(counted.finished).toBe(2);
    expect(counted.stopped).toBe(1);
  });

  it("counts a case that has not settled as still running", () => {
    const counted = tally({
      id: "sim-1",
      running: true,
      cases: [
        aCase({ settled: "unsettled", steps: 1 }),
        aCase({ settled: "parked" }),
      ],
    });

    expect(counted.running).toBe(1);
    expect(counted.parked).toBe(1);
  });

  it("adds up what the whole set cost", () => {
    const counted = tally({
      id: "sim-1",
      running: false,
      cases: [
        aCase({ cost: { micros: 1500 } }),
        aCase({ cost: { micros: 2500 } }),
      ],
    });

    expect(counted.micros).toBe(4000);
  });
});

describe("caseNeedsLook", () => {
  it("keeps a finished run with a blocked proposal out of the green bucket", () => {
    // The report is read before publishing. A run that finished only after a
    // Gate refusal is exactly the rehearsal finding somebody needs to review.
    expect(
      caseNeedsLook(
        aCase({
          acted: [
            {
              tool: "crm.refund",
              effect: "financial",
              verdict: "block",
              reached: false,
            },
          ],
        }),
      ),
    ).toBe(true);
  });
});
