import { describe, expect, it } from "vitest";
import {
  citableAsEvidence,
  citationsOf,
  labelsUpTo,
} from "@/features/runs/run-citations";
import type { Step } from "@/lib/api/client";

function finished(seq: number, payload: Record<string, unknown>): Step {
  return { seq, kind: "run_finished", payload } as unknown as Step;
}

function step(seq: number, labels?: string[]): Step {
  return { seq, kind: "tool_called", labels } as unknown as Step;
}

const ANSWER = {
  outcome_ref: "run://x/9/ab",
  outcome_digest: "ab".repeat(32),
};

describe("what a step offers a memory to cite", () => {
  it("names the closing answer and every artifact the run published", () => {
    const cites = citationsOf(
      finished(9, {
        ...ANSWER,
        artifacts: [
          { name: "report", ref: "run://x/9/cd", digest: "cd".repeat(32) },
        ],
      }),
    );
    expect(cites).toEqual([
      { seq: 9, artifact: "final_answer", digest: "ab".repeat(32) },
      { seq: 9, artifact: "report", digest: "cd".repeat(32) },
    ]);
  });

  // The ledger records a reference and a digest together about the same bytes,
  // and the server refuses a citation missing either (proved, citation.go).
  // Offering it here would be offering a button whose only outcome is a refusal.
  it("leaves out an artifact the ledger only half recorded", () => {
    const half = [
      { name: "report" },
      { name: "report", ref: "run://x/9/cd" },
      { name: "report", digest: "cd".repeat(32) },
    ];
    for (const artifact of half) {
      expect(citationsOf(finished(9, { artifacts: [artifact] }))).toEqual([]);
    }
  });

  // The same rule for the closing answer, which is the citation the console
  // offers by default and so the one a gap would reach first.
  it("leaves out a closing answer the ledger only half recorded", () => {
    expect(citationsOf(finished(9, { outcome_ref: "run://x/9/ab" }))).toEqual(
      [],
    );
    expect(
      citationsOf(finished(9, { outcome_digest: "ab".repeat(32) })),
    ).toEqual([]);
  });
});

describe("whether the screen offers to teach from a step", () => {
  it("offers what a finished run published, and nothing a tool merely stored", () => {
    expect(citableAsEvidence(finished(9, ANSWER))).toBe(true);
    expect(
      citableAsEvidence({
        seq: 2,
        kind: "tool_returned",
        payload: { result_ref: "run://x/2/ab" },
      } as unknown as Step),
    ).toBe(false);
  });

  // The same rule as citationsOf, asked as a yes or no. Written separately it
  // drifted at once — it read "the run published artifacts" and said yes to one
  // the ledger only half recorded, which is a button over an empty panel.
  it("does not offer a step whose artifacts cannot be cited", () => {
    expect(citableAsEvidence(finished(9, { artifacts: [{ name: "r" }] }))).toBe(
      false,
    );
  });

  // The server resolves this one — a memory_suggestion citation points at these
  // arguments — and the screen still does not offer it. That proposal is in the
  // review queue: accepting it is how it becomes memory. A second door here
  // would write the fact while leaving the proposal pending against it. Anyone
  // widening this rule to match the server has to delete this test first, which
  // is the point of it being here.
  it("does not offer to teach from a proposal already in the queue", () => {
    expect(
      citableAsEvidence({
        seq: 4,
        kind: "tool_called",
        payload: {
          tool: "$fuseone.memory.suggest",
          args_ref: "run://x/4/cd",
          args_digest: "cd".repeat(8),
        },
      } as unknown as Step),
    ).toBe(false);
  });
});

describe("the labels a memory taught here would carry", () => {
  // The server folds the run up to the cited step, not the step alone: a clean
  // answer inside a poisoned run is still a fact the poison reached
  // (labelsUpTo, evidence_resolver.go).
  it("folds every step up to the one cited", () => {
    const steps = [
      step(1, ["channel:email"]),
      step(2, ["untrusted"]),
      step(3, ["channel:email"]),
    ];
    expect(labelsUpTo(steps, 3)).toEqual(["channel:email", "untrusted"]);
  });

  it("stops at the cited step and ignores what came after", () => {
    const steps = [step(1), step(2, ["untrusted"])];
    expect(labelsUpTo(steps, 1)).toEqual([]);
  });

  // The trail arrives a page at a time, so a partial answer here would be a
  // short one — and short means a taint the memory will carry is missing from
  // the screen where somebody decides to teach it. Absent, not empty.
  it("has no answer until the trail reaches the cited step", () => {
    expect(labelsUpTo([step(1, ["untrusted"])], 9)).toBeNull();
  });

  it("has no answer when the trail does not start at the run's first step", () => {
    expect(labelsUpTo([step(2, ["untrusted"]), step(3)], 3)).toBeNull();
  });
});
